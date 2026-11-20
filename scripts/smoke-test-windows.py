#!/usr/bin/env python3
"""Exercise a native Windows release executable without credentials or WSL."""

from __future__ import annotations

import argparse
import ctypes
from ctypes import wintypes
import dataclasses
import http.server
import json
import ntpath
import os
from pathlib import Path
import queue
import re
import shutil
import subprocess
import tempfile
import threading
import time
import unittest


MODEL_PROVIDER = "windows-smoke"
MODEL_ID = "fixture"
TOOL_ALLOWLIST = "read,write,bash,codemode"
STARTUP_TIMEOUT = 60
NATIVE_BASH_RUNTIMES = ("mingw", "msys", "clangarm64", "clang64", "ucrt64")


@dataclasses.dataclass(frozen=True)
class Scenario:
    marker: str
    answer: str
    tool: str | None = None
    arguments: dict | None = None
    proof: Path | None = None
    content: str | None = None
    proof_kind: str = "text"
    prior_answer: str | None = None


def request_user_text(body):
    for message in reversed(body.get("messages", [])):
        if message.get("role") != "user":
            continue
        content = message.get("content", "")
        if isinstance(content, str):
            return content
        if isinstance(content, list):
            return "\n".join(part.get("text", "") for part in content
                             if isinstance(part, dict))
    return ""


def strings_in(value):
    if isinstance(value, str):
        yield value
    elif isinstance(value, dict):
        for child in value.values():
            yield from strings_in(child)
    elif isinstance(value, list):
        for child in value:
            yield from strings_in(child)


def tools_after_last_user(body):
    messages = body.get("messages", [])
    last_user = max((i for i, message in enumerate(messages)
                     if message.get("role") == "user"), default=-1)
    names = []
    calls = {}
    for message in messages[last_user + 1:]:
        for call in message.get("tool_calls", []):
            calls[call.get("id")] = call.get("function", {}).get("name", "")
        if message.get("role") == "tool":
            names.append(message.get("name") or calls.get(message.get("tool_call_id"), ""))
    return names


def environment_block(environment):
    entries = sorted(environment.items(), key=lambda item: item[0].casefold())
    return "\0".join(f"{key}={value}" for key, value in entries) + "\0\0"


_TERMINAL_QUERIES = (b"\x1b[c", b"\x1b[0c", b"\x1b[6n", b"\x1b[?6n")
_TERMINAL_QUERY_RE = re.compile(rb"\x1b\[(?:0?c|6n|\?6n)")


def terminal_replies(data, tail=b""):
    combined = tail + data
    matches = list(_TERMINAL_QUERY_RE.finditer(combined))
    replies = bytearray()
    for match in matches:
        query = match.group(0)
        replies.extend(b"\x1b[1;1R" if query.endswith(b"n") else b"\x1b[?1;2c")
    floor = max((match.end() for match in matches), default=0)
    next_tail = b""
    for start in range(max(floor, len(combined) - 5), len(combined)):
        suffix = combined[start:]
        if suffix and any(query.startswith(suffix) for query in _TERMINAL_QUERIES):
            next_tail = suffix
            break
    return bytes(replies), next_tail


def verify_bash_proof(proof, expected_cwd, expected_bash_root):
    fields = dict(line.split("=", 1) for line in proof.splitlines() if "=" in line)
    system = fields.get("uname", "").casefold()
    if not any(runtime in system for runtime in NATIVE_BASH_RUNTIMES):
        raise AssertionError(f"Git for Windows Bash did not report a Windows runtime: {system!r}")
    if fields.get("wsl_distro") or fields.get("wsl_interop"):
        raise AssertionError("Bash inherited WSL environment markers")
    actual_cwd = fields.get("windows_cwd", "")
    if not actual_cwd or os.path.normcase(os.path.normpath(actual_cwd)) != os.path.normcase(os.path.normpath(str(expected_cwd))):
        raise AssertionError(f"Bash Windows cwd differs: {actual_cwd!r}")
    if "\u03a9" not in actual_cwd or " " not in actual_cwd:
        raise AssertionError("Bash did not preserve the Unicode and space path")
    raw_bash_path = fields.get("bash_path", "")
    bash_path = ntpath.normcase(ntpath.normpath(raw_bash_path))
    bash_root = ntpath.normcase(ntpath.normpath(str(expected_bash_root)))
    try:
        inside_git = bool(raw_bash_path) and ntpath.commonpath((bash_root, bash_path)) == bash_root
    except ValueError:
        inside_git = False
    if not inside_git:
        raise AssertionError(f"Bash process path is outside Git for Windows: {bash_path!r}")
    comspec = fields.get("comspec", "").strip().strip('"').replace("/", "\\").casefold()
    if not comspec.endswith("\\cmd.exe") or "\\windows\\" not in comspec:
        raise AssertionError(f"Bash did not launch native Windows cmd.exe: {comspec!r}")


class ModelFixture:
    def __init__(self, workspace, scenarios, bash_root):
        self.workspace = workspace
        self.bash_root = bash_root
        self.scenarios = {scenario.marker: scenario for scenario in scenarios}
        self.counts = {scenario.marker: 0 for scenario in scenarios}
        self.events = {scenario.marker: threading.Event() for scenario in scenarios}
        self.failures = []
        self.lock = threading.Lock()
        self.call_number = 0

    def handle(self, body):
        user_text = request_user_text(body)
        scenario = next((item for marker, item in self.scenarios.items()
                         if marker in user_text), None)
        if scenario is None:
            raise AssertionError("model request did not include a smoke scenario")
        with self.lock:
            self.counts[scenario.marker] += 1
            self.call_number += 1
            call_number = self.call_number
            self.events[scenario.marker].set()
        if scenario.prior_answer and scenario.prior_answer not in "\n".join(strings_in(body.get("messages", []))):
            raise AssertionError("resumed session did not include the earlier assistant answer")
        tool_results = tools_after_last_user(body)
        offered = {item.get("function", {}).get("name")
                   for item in body.get("tools", []) if isinstance(item, dict)}
        if scenario.tool and not tool_results:
            if scenario.tool not in offered:
                raise AssertionError(f"required tool {scenario.tool!r} was not offered")
            if scenario.tool == "bash" and "BASH_WINDOWS_SMOKE" not in str(scenario.arguments):
                raise AssertionError("Bash fixture command was not the native Windows probe")
            delta = {"role": "assistant", "tool_calls": [{
                "index": 0, "id": f"windows-smoke-{call_number}", "type": "function",
                "function": {"name": scenario.tool,
                             "arguments": json.dumps(scenario.arguments, ensure_ascii=False)},
            }]}
            return delta, "tool_calls"
        if scenario.tool:
            if scenario.tool not in tool_results:
                raise AssertionError(f"expected {scenario.tool!r} tool result in the request")
            try:
                self.check_proof(scenario)
            except AssertionError as exc:
                results = [message.get("content") for message in body.get("messages", [])
                           if message.get("role") == "tool"]
                raise AssertionError(f"{exc}; last tool result={str(results[-1:])[-2000:]}") from exc
        return {"role": "assistant", "content": scenario.answer}, "stop"

    def check_proof(self, scenario):
        if scenario.proof is None:
            return
        try:
            actual = scenario.proof.read_text(encoding="utf-8")
        except OSError as exc:
            raise AssertionError(f"tool did not create {scenario.proof.name}") from exc
        if scenario.proof_kind == "bash":
            verify_bash_proof(actual, self.workspace, self.bash_root)
        elif actual != scenario.content:
            raise AssertionError(f"tool contents differ for {scenario.proof.name}")

    def assert_clean(self):
        if self.failures:
            raise AssertionError("loopback model fixture failed: " + "; ".join(self.failures))
        for scenario in self.scenarios.values():
            expected = 2 if scenario.tool else 1
            actual = self.counts[scenario.marker]
            if actual != expected:
                raise AssertionError(f"{scenario.marker}: received {actual} model requests, expected {expected}")


class FixtureHTTPServer(http.server.ThreadingHTTPServer):
    daemon_threads = True
    block_on_close = False
    allow_reuse_address = True


class FixtureHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def do_POST(self):
        try:
            if not self.path.endswith("/chat/completions"):
                raise AssertionError("unexpected loopback endpoint")
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            delta, finish = self.server.fixture.handle(body)
            chunks = [
                {"id": "windows-smoke", "object": "chat.completion.chunk", "created": 0,
                 "model": MODEL_ID, "choices": [{"index": 0, "delta": delta,
                                                     "finish_reason": None}]},
                {"id": "windows-smoke", "choices": [{"index": 0, "delta": {},
                                                         "finish_reason": finish}],
                 "usage": {"prompt_tokens": 12, "completion_tokens": 3}},
            ]
            payload = ("".join("data: " + json.dumps(chunk, ensure_ascii=False) + "\n\n"
                               for chunk in chunks) + "data: [DONE]\n\n").encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(payload)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(payload)
            self.close_connection = True
        except Exception as exc:
            failure = f"{type(exc).__name__}: {exc}"
            self.server.fixture.failures.append(failure)
            payload = failure.encode("utf-8")
            try:
                self.send_response(500)
                self.send_header("Content-Length", str(len(payload)))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(payload)
            except OSError:
                pass
            self.close_connection = True


class FixtureServer:
    def __init__(self, fixture):
        self.httpd = FixtureHTTPServer(("127.0.0.1", 0), FixtureHandler)
        self.httpd.fixture = fixture
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)

    @property
    def port(self):
        return self.httpd.server_address[1]

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *_):
        self.httpd.shutdown()
        self.httpd.server_close()
        self.thread.join(timeout=5)
        if self.thread.is_alive():
            raise RuntimeError("loopback fixture server did not stop")


def isolated_environment(root, profile, git_bash):
    system_root = os.environ.get("SystemRoot") or os.environ.get("SYSTEMROOT")
    if not system_root:
        raise RuntimeError("native Windows SystemRoot is unavailable")
    inherited_path = os.environ.get("PATH", "")
    paths = [str(git_bash.parent)]
    for parent in git_bash.parents:
        if parent.name.casefold() == "git":
            paths.extend((str(parent / "bin"), str(parent / "usr" / "bin")))
            break
    paths.extend(inherited_path.split(os.pathsep))
    unique_paths = []
    seen = set()
    for value in paths:
        normalized = os.path.normcase(os.path.normpath(value))
        if value and normalized not in seen:
            unique_paths.append(value)
            seen.add(normalized)
    temp = root / "temporary files"
    local = root / "local data"
    appdata = root / "roaming data"
    for path in (temp, local, appdata):
        path.mkdir(parents=True, exist_ok=True)
    return {
        "PATH": os.pathsep.join(unique_paths),
        "SystemRoot": system_root,
        "WINDIR": system_root,
        "COMSPEC": os.environ.get("COMSPEC", str(Path(system_root) / "System32" / "cmd.exe")),
        "PATHEXT": os.environ.get("PATHEXT", ".COM;.EXE;.BAT;.CMD"),
        "HOME": str(root),
        "USERPROFILE": str(root),
        "APPDATA": str(appdata),
        "LOCALAPPDATA": str(local),
        "TEMP": str(temp),
        "TMP": str(temp),
        "PI_HOME": str(root / "profile home"),
        "PIG_HOME": str(root / "pig home"),
        "XDG_CONFIG_HOME": str(root / "config"),
        "XDG_CACHE_HOME": str(root / "cache"),
        "PI_CODING_AGENT_DIR": str(profile),
        "PIG_CODING_AGENT_DIR": str(profile),
        "OMPIG_SLIM_CODING_AGENT_DIR": str(profile),
        "OMP_CODING_AGENT_DIR": str(profile),
        "PI_OFFLINE": "1",
        "PI_SKIP_VERSION_CHECK": "1",
        "PI_TELEMETRY": "0",
        "NO_COLOR": "1",
        "TERM": "xterm-256color",
        "TERM_PROGRAM": "ompig-windows-smoke",
        "LANG": "C.UTF-8",
    }


def write_profile(profile, server):
    profile.mkdir(parents=True)
    model = {
        "providers": {
            MODEL_PROVIDER: {
                "api": "openai-completions",
                "baseUrl": f"http://127.0.0.1:{server.port}/v1",
                "apiKey": "loopback-fixture-only",
                "models": [{"id": MODEL_ID, "name": "Windows loopback fixture",
                            "contextWindow": 128000, "maxTokens": 1024}],
            },
        },
    }
    (profile / "models.json").write_text(json.dumps(model), encoding="utf-8")
    (profile / "settings.json").write_text(json.dumps({
        "quietStartup": True,
        "enableInstallTelemetry": False,
        "extensionsAutoUpgrade": False,
        "tuiMode": "regular",
    }), encoding="utf-8")


def cli_base(binary, session_dir, tools=TOOL_ALLOWLIST):
    return [str(binary), "--provider", MODEL_PROVIDER, "--model", MODEL_ID,
            "--thinking", "off", "--tools", tools, "--no-extensions", "--offline",
            "--no-context-files", "--no-skills", "--no-prompt-templates", "--no-themes",
            "--session-dir", str(session_dir)]


def run_command(command, cwd, environment, timeout, name):
    try:
        result = subprocess.run(command, cwd=cwd, env=environment, stdin=subprocess.DEVNULL,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout,
                                encoding="utf-8", errors="replace")
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"{name} timed out after {timeout:g}s") from exc
    if result.returncode:
        raise RuntimeError(f"{name} exited {result.returncode}\nstdout:\n{result.stdout[-5000:]}"
                           f"\nstderr:\n{result.stderr[-5000:]}")
    return result


def run_rpc(binary, cwd, environment, session_dir, timeout):
    command = cli_base(binary, session_dir, "read,write,bash")
    command += ["--mode", "rpc", "--no-session"]
    try:
        process = subprocess.Popen(command, cwd=cwd, env=environment, stdin=subprocess.PIPE,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   text=True, encoding="utf-8", errors="replace", bufsize=1)
    except OSError as exc:
        raise RuntimeError(f"cannot start RPC executable: {exc}") from exc
    events = queue.Queue()
    stderr = bytearray()

    def read_events():
        try:
            for line in process.stdout:
                try:
                    events.put(json.loads(line))
                except json.JSONDecodeError as exc:
                    events.put(RuntimeError(f"RPC stdout was not JSONL: {exc}"))
        finally:
            events.put(None)

    def read_errors():
        while True:
            chunk = process.stderr.read(4096)
            if not chunk:
                return
            stderr.extend(chunk.encode("utf-8", errors="replace"))
            if len(stderr) > 65536:
                del stderr[:-65536]

    stdout_thread = threading.Thread(target=read_events, daemon=True)
    stderr_thread = threading.Thread(target=read_errors, daemon=True)
    stdout_thread.start()
    stderr_thread.start()

    def send(command_value):
        process.stdin.write(json.dumps(command_value, ensure_ascii=False) + "\n")
        process.stdin.flush()

    def until(predicate, label, observed=None):
        deadline = time.monotonic() + timeout
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError(f"RPC timed out waiting for {label}")
            try:
                event = events.get(timeout=min(remaining, 0.2))
            except queue.Empty:
                if process.poll() is not None:
                    raise RuntimeError(f"RPC process exited before {label}: {process.returncode}")
                continue
            if event is None:
                raise RuntimeError(f"RPC stdout closed before {label}")
            if isinstance(event, Exception):
                raise event
            if event.get("success") is False or event.get("isError"):
                raise RuntimeError(f"RPC command/tool failed: {event}")
            if observed is not None:
                observed.append(event)
            if predicate(event):
                return event

    try:
        send({"type": "get_state", "id": "windows-ready"})
        state = until(lambda event: event.get("type") == "response"
                      and event.get("id") == "windows-ready", "get_state response")
        if state.get("command") != "get_state" or not state.get("success"):
            raise AssertionError(f"RPC get_state failed: {state}")
        send({"type": "prompt", "id": "windows-prompt",
              "message": "WINDOWS_RPC_PROBE Exercise the RPC stdio tool turn."})
        transcript = []
        started = False

        def run_settled(event):
            nonlocal started
            if event.get("type") == "agent_start":
                started = True
            return started and event.get("type") == "agent_settled"

        until(run_settled, "agent_start followed by agent_settled", transcript)
        if "WINDOWS_RPC_OK" not in "\n".join(strings_in(transcript)):
            raise AssertionError("RPC agent_end did not contain the fixture response")
        process.stdin.close()
        try:
            process.wait(timeout=timeout)
        except subprocess.TimeoutExpired as exc:
            raise RuntimeError("RPC process did not exit after stdin EOF") from exc
        if process.returncode:
            raise RuntimeError(f"RPC exited {process.returncode}: {stderr.decode(errors='replace')}")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait(timeout=5)
        if process.stdin and not process.stdin.closed:
            process.stdin.close()
        stdout_thread.join(timeout=5)
        stderr_thread.join(timeout=5)
        process.stdout.close()
        process.stderr.close()


class COORD(ctypes.Structure):
    _fields_ = [("X", ctypes.c_int16), ("Y", ctypes.c_int16)]


class ConPTY:
    def __init__(self, binary, arguments, cwd, environment, cols=100, rows=36):
        if os.name != "nt":
            raise RuntimeError("ConPTY is available only on native Windows")
        self.kernel = ctypes.WinDLL("kernel32", use_last_error=True)
        self.input_write = None
        self.output_read = None
        self.pseudo_console = None
        self.process = None
        self.thread = None
        self.attributes = None
        self.captured = bytearray()
        self.query_tail = b""
        self._declare_apis()
        self._start(binary, arguments, cwd, environment, cols, rows)

    def _declare_apis(self):
        kernel = self.kernel
        for name in ("CreatePseudoConsole", "ResizePseudoConsole", "ClosePseudoConsole"):
            if not hasattr(kernel, name):
                raise RuntimeError(f"native Windows runner does not provide required ConPTY API {name}")
        kernel.CreatePipe.argtypes = [ctypes.POINTER(wintypes.HANDLE),
                                      ctypes.POINTER(wintypes.HANDLE),
                                      ctypes.c_void_p, wintypes.DWORD]
        kernel.CreatePipe.restype = wintypes.BOOL
        kernel.SetHandleInformation.argtypes = [wintypes.HANDLE, wintypes.DWORD, wintypes.DWORD]
        kernel.SetHandleInformation.restype = wintypes.BOOL
        kernel.CreatePseudoConsole.argtypes = [COORD, wintypes.HANDLE, wintypes.HANDLE,
                                               wintypes.DWORD, ctypes.POINTER(wintypes.HANDLE)]
        kernel.CreatePseudoConsole.restype = ctypes.c_int32
        kernel.ResizePseudoConsole.argtypes = [wintypes.HANDLE, COORD]
        kernel.ResizePseudoConsole.restype = ctypes.c_int32
        kernel.ClosePseudoConsole.argtypes = [wintypes.HANDLE]
        kernel.ClosePseudoConsole.restype = None
        kernel.InitializeProcThreadAttributeList.argtypes = [ctypes.c_void_p, wintypes.DWORD,
                                                              wintypes.DWORD,
                                                              ctypes.POINTER(ctypes.c_size_t)]
        kernel.InitializeProcThreadAttributeList.restype = wintypes.BOOL
        kernel.UpdateProcThreadAttribute.argtypes = [ctypes.c_void_p, wintypes.DWORD,
                                                      ctypes.c_size_t, ctypes.c_void_p,
                                                      ctypes.c_size_t, ctypes.c_void_p,
                                                      ctypes.POINTER(ctypes.c_size_t)]
        kernel.UpdateProcThreadAttribute.restype = wintypes.BOOL
        kernel.DeleteProcThreadAttributeList.argtypes = [ctypes.c_void_p]
        kernel.DeleteProcThreadAttributeList.restype = None
        kernel.CreateProcessW.argtypes = [wintypes.LPCWSTR, wintypes.LPWSTR,
                                          ctypes.c_void_p, ctypes.c_void_p, wintypes.BOOL,
                                          wintypes.DWORD, ctypes.c_void_p, wintypes.LPCWSTR,
                                          ctypes.c_void_p, ctypes.c_void_p]
        kernel.CreateProcessW.restype = wintypes.BOOL
        kernel.CloseHandle.argtypes = [wintypes.HANDLE]
        kernel.CloseHandle.restype = wintypes.BOOL
        kernel.PeekNamedPipe.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                                         ctypes.c_void_p, ctypes.POINTER(wintypes.DWORD),
                                         ctypes.c_void_p]
        kernel.PeekNamedPipe.restype = wintypes.BOOL
        kernel.ReadFile.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                                    ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]
        kernel.ReadFile.restype = wintypes.BOOL
        kernel.WriteFile.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                                     ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]
        kernel.WriteFile.restype = wintypes.BOOL
        kernel.WaitForSingleObject.argtypes = [wintypes.HANDLE, wintypes.DWORD]
        kernel.WaitForSingleObject.restype = wintypes.DWORD
        kernel.GetExitCodeProcess.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)]
        kernel.GetExitCodeProcess.restype = wintypes.BOOL
        kernel.TerminateProcess.argtypes = [wintypes.HANDLE, wintypes.UINT]
        kernel.TerminateProcess.restype = wintypes.BOOL

    @staticmethod
    def _check(ok, operation):
        if not ok:
            raise ctypes.WinError(ctypes.get_last_error(), operation)

    @staticmethod
    def _hresult(result, operation):
        if result < 0:
            raise OSError(f"{operation} failed with HRESULT 0x{result & 0xffffffff:08x}")

    def _start(self, binary, arguments, cwd, environment, cols, rows):
        kernel = self.kernel
        attribute_initialized = False

        class SECURITY_ATTRIBUTES(ctypes.Structure):
            _fields_ = [("nLength", wintypes.DWORD), ("lpSecurityDescriptor", ctypes.c_void_p),
                        ("bInheritHandle", wintypes.BOOL)]

        class STARTUPINFOW(ctypes.Structure):
            _fields_ = [("cb", wintypes.DWORD), ("lpReserved", wintypes.LPWSTR),
                        ("lpDesktop", wintypes.LPWSTR), ("lpTitle", wintypes.LPWSTR),
                        ("dwX", wintypes.DWORD), ("dwY", wintypes.DWORD),
                        ("dwXSize", wintypes.DWORD), ("dwYSize", wintypes.DWORD),
                        ("dwXCountChars", wintypes.DWORD), ("dwYCountChars", wintypes.DWORD),
                        ("dwFillAttribute", wintypes.DWORD), ("dwFlags", wintypes.DWORD),
                        ("wShowWindow", wintypes.WORD), ("cbReserved2", wintypes.WORD),
                        ("lpReserved2", ctypes.c_void_p), ("hStdInput", wintypes.HANDLE),
                        ("hStdOutput", wintypes.HANDLE), ("hStdError", wintypes.HANDLE)]

        class STARTUPINFOEXW(ctypes.Structure):
            _fields_ = [("StartupInfo", STARTUPINFOW), ("lpAttributeList", ctypes.c_void_p)]

        class PROCESS_INFORMATION(ctypes.Structure):
            _fields_ = [("hProcess", wintypes.HANDLE), ("hThread", wintypes.HANDLE),
                        ("dwProcessId", wintypes.DWORD), ("dwThreadId", wintypes.DWORD)]

        security = SECURITY_ATTRIBUTES(ctypes.sizeof(SECURITY_ATTRIBUTES), None, True)
        input_read = wintypes.HANDLE()
        input_write = wintypes.HANDLE()
        output_read = wintypes.HANDLE()
        output_write = wintypes.HANDLE()
        try:
            self._check(kernel.CreatePipe(ctypes.byref(input_read), ctypes.byref(input_write),
                                          ctypes.byref(security), 0), "CreatePipe(input)")
            self._check(kernel.CreatePipe(ctypes.byref(output_read), ctypes.byref(output_write),
                                          ctypes.byref(security), 0), "CreatePipe(output)")
            self._check(kernel.SetHandleInformation(input_write, 1, 0), "SetHandleInformation(input)")
            self._check(kernel.SetHandleInformation(output_read, 1, 0), "SetHandleInformation(output)")
            self.pseudo_console = wintypes.HANDLE()
            self._hresult(kernel.CreatePseudoConsole(COORD(cols, rows), input_read,
                                                     output_write, 0,
                                                     ctypes.byref(self.pseudo_console)),
                          "CreatePseudoConsole")
            attribute_size = ctypes.c_size_t()
            kernel.InitializeProcThreadAttributeList(None, 1, 0, ctypes.byref(attribute_size))
            if not attribute_size.value:
                raise ctypes.WinError(ctypes.get_last_error(), "InitializeProcThreadAttributeList(size)")
            self.attributes = ctypes.create_string_buffer(attribute_size.value)
            self._check(kernel.InitializeProcThreadAttributeList(self.attributes, 1, 0,
                                                                  ctypes.byref(attribute_size)),
                        "InitializeProcThreadAttributeList")
            attribute_initialized = True
            self._check(kernel.UpdateProcThreadAttribute(
                self.attributes, 0, 0x00020016, self.pseudo_console,
                ctypes.sizeof(self.pseudo_console), None, None), "UpdateProcThreadAttribute(ConPTY)")
            startup = STARTUPINFOEXW()
            startup.StartupInfo.cb = ctypes.sizeof(STARTUPINFOEXW)
            # Null standard handles keep redirected parent streams from bypassing ConPTY.
            startup.StartupInfo.dwFlags = 0x00000100
            startup.lpAttributeList = ctypes.cast(self.attributes, ctypes.c_void_p)
            process_info = PROCESS_INFORMATION()
            command_line = ctypes.create_unicode_buffer(subprocess.list2cmdline(
                [str(binary), *arguments]))
            env_string = environment_block(environment)
            env_block = ctypes.create_unicode_buffer(env_string)
            flags = 0x00080000 | 0x00000400
            self._check(kernel.CreateProcessW(
                str(binary), command_line, None, None, False, flags,
                ctypes.cast(env_block, ctypes.c_void_p), str(cwd),
                ctypes.byref(startup), ctypes.byref(process_info)), "CreateProcessW(ConPTY)")
            self.process = process_info.hProcess
            self.thread = process_info.hThread
            kernel.CloseHandle(input_read)
            input_read = wintypes.HANDLE()
            kernel.CloseHandle(output_write)
            output_write = wintypes.HANDLE()
            self._check(kernel.CloseHandle(self.thread), "CloseHandle(thread)")
            self.thread = None
            self.input_write = input_write
            input_write = wintypes.HANDLE()
            self.output_read = output_read
            output_read = wintypes.HANDLE()
        except Exception:
            self.close()
            raise
        finally:
            if attribute_initialized and self.attributes:
                kernel.DeleteProcThreadAttributeList(self.attributes)
                self.attributes = None
            for handle in (input_read, input_write, output_read, output_write):
                if handle:
                    kernel.CloseHandle(handle)

    def _write(self, data):
        buffer = ctypes.create_string_buffer(data)
        written = wintypes.DWORD()
        self._check(self.kernel.WriteFile(self.input_write, buffer, len(data),
                                          ctypes.byref(written), None), "WriteFile(ConPTY)")
        if written.value != len(data):
            raise OSError("ConPTY accepted only part of the input")

    def _pump(self):
        available = wintypes.DWORD()
        if not self.kernel.PeekNamedPipe(self.output_read, None, 0, None,
                                         ctypes.byref(available), None):
            return False
        while available.value:
            amount = min(available.value, 65536)
            buffer = ctypes.create_string_buffer(amount)
            read = wintypes.DWORD()
            if not self.kernel.ReadFile(self.output_read, buffer, amount, ctypes.byref(read), None):
                return False
            if not read.value:
                break
            chunk = buffer.raw[:read.value]
            if len(self.captured) + len(chunk) > 8 * 1024 * 1024:
                raise RuntimeError("ConPTY output exceeded the 8 MiB smoke limit")
            self.captured.extend(chunk)
            replies, self.query_tail = terminal_replies(chunk, self.query_tail)
            if replies:
                self._write(replies)
            available = wintypes.DWORD()
            if not self.kernel.PeekNamedPipe(self.output_read, None, 0, None,
                                             ctypes.byref(available), None):
                break
        return True

    def wait_for(self, predicate, label, timeout):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self._pump()
            if predicate(self.captured):
                return
            state = self.kernel.WaitForSingleObject(self.process, 0)
            if state == 0:
                self._pump()
                if not predicate(self.captured):
                    code = wintypes.DWORD()
                    self._check(self.kernel.GetExitCodeProcess(self.process, ctypes.byref(code)),
                                "GetExitCodeProcess(ConPTY)")
                    raise RuntimeError(f"TUI exited before {label}; exit=0x{code.value:08x}; "
                                       f"tail={bytes(self.captured[-3000:])!r}")
                return
            if state == 0xFFFFFFFF:
                raise ctypes.WinError(ctypes.get_last_error(), "WaitForSingleObject(ConPTY)")
            time.sleep(0.01)
        raise TimeoutError(f"TUI timed out waiting for {label}; tail={bytes(self.captured[-3000:])!r}")

    def resize(self, cols, rows):
        self._hresult(self.kernel.ResizePseudoConsole(self.pseudo_console, COORD(cols, rows)),
                      "ResizePseudoConsole")

    def wait_for_exit(self, timeout):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self._pump()
            state = self.kernel.WaitForSingleObject(self.process, 10)
            if state == 0:
                self._pump()
                code = wintypes.DWORD()
                self._check(self.kernel.GetExitCodeProcess(self.process, ctypes.byref(code)),
                            "GetExitCodeProcess")
                return code.value
            if state == 0xFFFFFFFF:
                raise ctypes.WinError(ctypes.get_last_error(), "WaitForSingleObject(ConPTY)")
        raise TimeoutError("TUI did not exit after /quit")

    def close(self):
        if self.process:
            state = self.kernel.WaitForSingleObject(self.process, 0)
            if state == 0xFFFFFFFF:
                raise ctypes.WinError(ctypes.get_last_error(), "WaitForSingleObject(cleanup)")
            if state != 0:
                if not self.kernel.TerminateProcess(self.process, 1):
                    state = self.kernel.WaitForSingleObject(self.process, 0)
                    if state != 0:
                        raise ctypes.WinError(ctypes.get_last_error(), "TerminateProcess(ConPTY child)")
                if self.kernel.WaitForSingleObject(self.process, 5000) != 0:
                    raise TimeoutError("ConPTY child did not exit after termination")
            self.kernel.CloseHandle(self.process)
            self.process = None
        if self.thread:
            self.kernel.CloseHandle(self.thread)
            self.thread = None
        for name in ("input_write", "output_read"):
            handle = getattr(self, name)
            if handle:
                self.kernel.CloseHandle(handle)
                setattr(self, name, None)
        if self.pseudo_console:
            self.kernel.ClosePseudoConsole(self.pseudo_console)
            self.pseudo_console = None


def run_tui(binary, workspace, environment, sessions, model_requested, timeout):
    prompt = "WINDOWS_TUI_PROBE"
    arguments = cli_base(binary, sessions, "write,read,bash")[1:] + ["--no-session"]
    conpty = ConPTY(binary, arguments, workspace, {**environment, "PIG_STARTUP_TRACE": "1"})
    try:
        conpty.wait_for(lambda data: b"interactive-ready" in data,
                        "interactive-ready startup marker", STARTUP_TIMEOUT)
        conpty._write((prompt + "\r").encode("utf-8"))
        if not model_requested.wait(timeout):
            raise TimeoutError("TUI typed prompt did not reach the loopback model")
        marker = b"WINDOWS_TUI_RESIZE_STATUS_"
        conpty.wait_for(lambda data: marker in data, "loopback model response", timeout)
        time.sleep(0.15)
        before_resize = len(conpty.captured)
        conpty.resize(72, 28)
        conpty.wait_for(lambda data: marker in data[before_resize:]
                        and re.search(rb"\x1b\[[0-9;?]*[Hf]", data[before_resize:]) is not None,
                        "resized TUI repaint and response status", timeout)
        conpty._write(b"/quit\r")
        code = conpty.wait_for_exit(timeout)
        if code != 0:
            raise RuntimeError(f"TUI /quit exited {code}; tail={bytes(conpty.captured[-3000:])!r}")
        print("PASS: native ConPTY startup, submitted prompt, resize repaint, and /quit")
    finally:
        conpty.close()


def git_install_root(git_bash):
    parents = git_bash.parents
    if len(parents) >= 3 and parents[0].name.casefold() == "bin" and parents[1].name.casefold() == "usr":
        return parents[2]
    if len(parents) >= 2 and parents[0].name.casefold() == "bin":
        return parents[1]
    return git_bash.parent


def run_smoke(binary, git_bash, timeout):
    if os.name != "nt":
        raise RuntimeError("run this smoke test on a native Windows runner; WSL and POSIX PTYs are not accepted")
    binary = binary.resolve(strict=True)
    if not binary.is_file() or binary.suffix.casefold() != ".exe":
        raise RuntimeError("binary must be an existing Windows .exe release artifact")
    git_bash = git_bash.resolve(strict=True)
    if not git_bash.is_file() or git_bash.name.casefold() != "bash.exe":
        raise RuntimeError("--git-bash must name Git for Windows bash.exe")
    bash_root = git_install_root(git_bash)
    if timeout <= 0:
        raise ValueError("--timeout must be positive")

    with tempfile.TemporaryDirectory(prefix="ompig Windows smoke \u03a9 ") as temporary:
        root = Path(temporary).resolve()
        profile = root / "profile with spaces \u03a9"
        workspace = root / "workspace with spaces \u03a9"
        sessions = root / "sessions with spaces \u03a9"
        state = root / "durable state \u03a9"
        for path in (workspace, sessions):
            path.mkdir(parents=True)
        fixture = ModelFixture(workspace, [
            Scenario("WINDOWS_PRINT_PROBE", "WINDOWS_PRINT_OK", "write",
                     {"path": "print-proof.txt", "content": "native print write"},
                     workspace / "print-proof.txt", "native print write"),
            Scenario("WINDOWS_SESSION_RESUME_PROBE", "WINDOWS_SESSION_RESUME_OK",
                     prior_answer="WINDOWS_PRINT_OK"),
            Scenario("WINDOWS_JSON_PROBE", "WINDOWS_JSON_OK", "codemode",
                     {"code": 'await tools.write({path:"json-proof.txt",content:"native codemode write"}); return "WINDOWS_CODEMODE_OK";'},
                     workspace / "json-proof.txt", "native codemode write"),
            Scenario("WINDOWS_RPC_PROBE", "WINDOWS_RPC_OK", "write",
                     {"path": "rpc-proof.txt", "content": "native rpc write"},
                     workspace / "rpc-proof.txt", "native rpc write"),
            Scenario("WINDOWS_DURABLE_PROBE", "WINDOWS_DURABLE_OK", "write",
                     {"path": "durable-proof.txt", "content": "native durable write"},
                     workspace / "durable-proof.txt", "native durable write"),
            Scenario("WINDOWS_BASH_PROBE", "WINDOWS_BASH_OK", "bash",
                     {"command": '{ printf "BASH_WINDOWS_SMOKE\\nuname=%s\\nwindows_cwd=%s\\nbash_path=%s\\nwsl_distro=%s\\nwsl_interop=%s\\ncomspec=%s\\n" "$(uname -s)" "$(cygpath -aw "$PWD")" "$(cygpath -aw "$BASH")" "${WSL_DISTRO_NAME:-}" "${WSL_INTEROP:-}" "$(MSYS2_ARG_CONV_EXCL=* cmd.exe /d /c \'echo %ComSpec%\')"; } > bash-proof.txt'},
                     workspace / "bash-proof.txt", proof_kind="bash"),
            Scenario("WINDOWS_TUI_PROBE", "WINDOWS_TUI_RESIZE_STATUS_" + ("paint" * 36), "write",
                     {"path": "tui-proof.txt", "content": "native TUI write"},
                     workspace / "tui-proof.txt", "native TUI write"),
        ], bash_root)
        with FixtureServer(fixture) as server:
            write_profile(profile, server)
            environment = isolated_environment(root, profile, git_bash)
            bash_result = subprocess.run(
                [str(git_bash), "-c", "uname -s"], cwd=workspace, env=environment,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout,
                encoding="utf-8", errors="replace")
            if bash_result.returncode or not any(runtime in bash_result.stdout.casefold()
                                                 for runtime in NATIVE_BASH_RUNTIMES):
                raise RuntimeError("--git-bash is not a working Git for Windows Bash executable")
            version = run_command([str(binary), "--version"], workspace, environment,
                                  timeout, "--version")
            if not (version.stdout + version.stderr).strip():
                raise AssertionError("--version produced no output")
            help_result = run_command([str(binary), "--help"], workspace, environment,
                                      timeout, "--help")
            help_text = (help_result.stdout + help_result.stderr).casefold()
            if "usage" not in help_text or "--mode" not in help_text:
                raise AssertionError("--help did not show CLI usage and mode options")
            print("PASS: native .exe --version and --help")

            base = cli_base(binary, sessions)
            printed = run_command(base + ["--print", "WINDOWS_PRINT_PROBE"], workspace,
                                  environment, timeout, "print tool turn")
            if "WINDOWS_PRINT_OK" not in printed.stdout:
                raise AssertionError(f"print response marker missing: {printed.stdout[-2000:]}")
            session_files = list(sessions.rglob("*.jsonl"))
            if len(session_files) != 1:
                raise AssertionError(f"print session persistence produced {len(session_files)} JSONL files")
            resumed = run_command(base + ["--print", "--session", str(session_files[0]),
                                          "WINDOWS_SESSION_RESUME_PROBE"], workspace,
                                  environment, timeout, "print session resume")
            if "WINDOWS_SESSION_RESUME_OK" not in resumed.stdout:
                raise AssertionError("resumed session did not produce its fixture response")
            print("PASS: print tool side effect and session persistence/resume")

            json_result = run_command(base + ["--mode", "json", "--no-session", "WINDOWS_JSON_PROBE"],
                                      workspace, environment, timeout, "JSON model/tool turn")
            try:
                events = [json.loads(line) for line in json_result.stdout.splitlines() if line.strip()]
            except json.JSONDecodeError as exc:
                raise AssertionError(f"JSON mode emitted a non-JSONL line: {exc}") from exc
            if not events or "WINDOWS_JSON_OK" not in "\n".join(strings_in(events)):
                raise AssertionError("JSON mode did not emit the final fixture response")
            if not (workspace / "json-proof.txt").is_file():
                raise AssertionError("codemode did not make its nested native write")
            print("PASS: JSONL model turn and native codemode nested write")

            run_rpc(binary, workspace, environment, sessions, timeout)
            if (workspace / "rpc-proof.txt").read_text(encoding="utf-8") != "native rpc write":
                raise AssertionError("RPC write tool side effect differs")
            print("PASS: RPC stdio request/response and tool write")

            durable_command = [str(binary), "durable", "run", "--state", str(state),
                               "--cwd", str(workspace), "--model", f"{MODEL_PROVIDER}/{MODEL_ID}",
                               "--request-id", "windows-smoke-1", "--max-turns", "4",
                               "WINDOWS_DURABLE_PROBE Exercise a native durable write."]
            durable = run_command(durable_command, workspace, environment, timeout, "durable run")
            receipt = json.loads(durable.stdout)
            if receipt.get("status") != "done" or "WINDOWS_DURABLE_OK" not in receipt.get("answer", ""):
                raise AssertionError(f"durable run did not complete: {receipt}")
            if (workspace / "durable-proof.txt").read_text(encoding="utf-8") != "native durable write":
                raise AssertionError("durable write side effect differs")
            for action in ("status", "resume"):
                result = run_command([str(binary), "durable", action, "--state", str(state)],
                                     workspace, environment, timeout, f"durable {action}")
                status = json.loads(result.stdout)
                if status.get("status") != "done":
                    raise AssertionError(f"durable {action} did not retain completion: {status}")
            print("PASS: durable run, status, resume, and native file write")

            bash_turn = run_command(base + ["--print", "WINDOWS_BASH_PROBE"],
                                    workspace, environment, timeout, "Git for Windows Bash tool turn")
            if "WINDOWS_BASH_OK" not in bash_turn.stdout:
                raise AssertionError("Bash tool turn did not produce its fixture response")
            print("PASS: Bash tool used Git for Windows, native Windows paths, and no WSL")

            run_tui(binary, workspace, environment, sessions,
                    fixture.events["WINDOWS_TUI_PROBE"], timeout)
            fixture.assert_clean()
            print("PASS: loopback fixture served every request; no external inference or credentials")


def discover_git_bash(explicit):
    candidates = [explicit] if explicit else []
    if not explicit:
        roots = [os.environ.get("ProgramFiles"), os.environ.get("ProgramFiles(x86)"),
                 os.environ.get("LOCALAPPDATA")]
        for root in filter(None, roots):
            candidates.extend((Path(root) / "Git" / "bin" / "bash.exe",
                               Path(root) / "Programs" / "Git" / "bin" / "bash.exe"))
        located = shutil.which("bash.exe") or shutil.which("bash")
        if located:
            candidates.append(Path(located))
    for candidate in candidates:
        path = Path(candidate).expanduser()
        if path.is_file() and path.name.casefold() == "bash.exe":
            return path
    raise RuntimeError("Git for Windows bash.exe was not found; pass --git-bash PATH")


class PortableLogicTests(unittest.TestCase):
    def test_environment_block_round_trips_unicode_and_spaces(self):
        value = environment_block({"PATH": r"C:\Program Files\Git", "HOME": "C:\\smoke \u03a9"})
        self.assertTrue(value.endswith("\0\0"))
        self.assertIn("HOME=C:\\smoke \u03a9", value)
        self.assertIn("PATH=C:\\Program Files\\Git", value)

    def test_terminal_queries_reply_even_when_split_across_reads(self):
        reply, tail = terminal_replies(b"text\x1b[", b"")
        self.assertEqual(reply, b"")
        reply, tail = terminal_replies(b"6n", tail)
        self.assertEqual(reply, b"\x1b[1;1R")
        self.assertEqual(tail, b"")
        reply, tail = terminal_replies(b"\x1b[c", tail)
        self.assertEqual(reply, b"\x1b[?1;2c")

    def test_rpc_tool_results_are_scoped_to_latest_user_turn(self):
        body = {"messages": [{"role": "user", "content": "old"},
                             {"role": "tool", "name": "write"},
                             {"role": "user", "content": "new"}]}
        self.assertEqual(tools_after_last_user(body), [])
        body["messages"].extend([
            {"role": "assistant", "tool_calls": [
                {"id": "call-1", "function": {"name": "write"}}]},
            {"role": "tool", "tool_call_id": "call-1", "content": "written"},
        ])
        self.assertEqual(tools_after_last_user(body), ["write"])

    def test_bash_proof_requires_git_windows_paths_and_native_cmd(self):
        cwd = Path("C:\\Temp\\ompig smoke \u03a9")
        proof = ("uname=MINGW64_NT-10.0\n"
                 f"windows_cwd={cwd}\n"
                 "bash_path=C:\\Program Files\\Git\\usr\\bin\\bash.exe\n"
                 "wsl_distro=\nwsl_interop=\n"
                 "comspec=C:\\Windows\\System32\\cmd.exe\n")
        git_root = Path("C:\\Program Files\\Git")
        verify_bash_proof(proof, cwd, git_root)
        verify_bash_proof(proof.replace("MINGW64", "CLANGARM64"), cwd, git_root)
        with self.assertRaises(AssertionError):
            verify_bash_proof(proof.replace("MINGW64", "Linux"), cwd, git_root)
        with self.assertRaises(AssertionError):
            verify_bash_proof(proof.replace("wsl_interop=", "wsl_interop=/run/wsl"), cwd, git_root)
        with self.assertRaises(AssertionError):
            verify_bash_proof(proof.replace("C:\\Program Files\\Git\\usr\\bin\\bash.exe",
                                            "C:\\Windows\\System32\\bash.exe"), cwd, git_root)

    def test_fixture_failure_reports_the_proof_error_and_tool_result(self):
        import http.client
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            scenario = Scenario("PROBE", "OK", "bash", proof=root / "missing.txt")
            fixture = ModelFixture(root, [scenario], root)
            with FixtureServer(fixture) as server:
                connection = http.client.HTTPConnection("127.0.0.1", server.port, timeout=5)
                self.addCleanup(connection.close)
                body = {"messages": [{"role": "user", "content": "PROBE"},
                                     {"role": "tool", "name": "bash", "content": "spawn failed"}]}
                connection.request("POST", "/chat/completions", json.dumps(body))
                response = connection.getresponse()
                self.assertEqual(response.status, 500)
                diagnostic = response.read().decode("utf-8")
                self.assertIn("tool did not create missing.txt", diagnostic)
                self.assertIn("spawn failed", diagnostic)
                self.assertEqual(len(fixture.failures), 1)

    def test_git_bash_root_supports_bin_and_usr_bin_layouts(self):
        self.assertEqual(git_install_root(Path("/opt/Git/bin/bash.exe")), Path("/opt/Git"))
        self.assertEqual(git_install_root(Path("/opt/Git/usr/bin/bash.exe")), Path("/opt/Git"))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", nargs="?", type=Path,
                        help="path to the Windows release executable (.exe)")
    parser.add_argument("--git-bash", type=Path, help="Git for Windows bash.exe, if not on PATH")
    parser.add_argument("--timeout", type=float, default=90,
                        help="timeout in seconds for each CLI operation (default: 90)")
    parser.add_argument("--self-test", action="store_true",
                        help="run portable harness logic tests without Windows or an executable")
    args = parser.parse_args()
    if args.self_test:
        result = unittest.TextTestRunner(verbosity=2).run(
            unittest.defaultTestLoader.loadTestsFromTestCase(PortableLogicTests))
        return 0 if result.wasSuccessful() else 1
    if args.binary is None:
        parser.error("binary is required unless --self-test is used")
    if os.name != "nt":
        raise SystemExit("ERROR: requires native Windows and ConPTY; WSL/POSIX runs are not accepted")
    try:
        run_smoke(args.binary, discover_git_bash(args.git_bash), args.timeout)
    except Exception as exc:
        raise SystemExit(f"FAIL: {exc}") from exc
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
