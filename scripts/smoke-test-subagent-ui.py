#!/usr/bin/env python3
"""Verify the hidden-until-Down subagent tree, footer count and live thinking through a PTY."""
import codecs
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time


class Screen:
    """Minimal terminal: enough CSI handling to reconstruct the visible frame."""

    def __init__(self, rows, cols):
        self.rows, self.cols = rows, cols
        self.grid = [[" "] * cols for _ in range(rows)]
        self.r = self.c = 0
        self.wrap_pending = False
        self.saved = (0, 0)
        self.decoder = codecs.getincrementaldecoder("utf-8")("replace")
        self.pending = ""

    def newline(self):
        self.wrap_pending = False
        if self.r == self.rows - 1:
            self.grid.pop(0)
            self.grid.append([" "] * self.cols)
        else:
            self.r += 1

    def put(self, ch):
        if self.wrap_pending:
            self.newline()
            self.c = 0
        if self.c < self.cols:
            self.grid[self.r][self.c] = ch
        if self.c == self.cols - 1:
            self.wrap_pending = True
        else:
            self.c += 1

    def erase_line(self, mode):
        if mode == 0:
            self.grid[self.r][self.c:] = [" "] * (self.cols - self.c)
        elif mode == 1:
            self.grid[self.r][: self.c + 1] = [" "] * (self.c + 1)
        else:
            self.grid[self.r] = [" "] * self.cols

    def csi(self, params, final):
        parts = params.split(";") if params else [""]
        args = []
        for p in parts:
            try:
                args.append(int(p) if p else 0)
            except ValueError:
                args.append(0)
        n = args[0] if args else 0
        if final in "Hf":
            row = (args[0] if args[0] else 1) - 1
            col = (args[1] if len(args) > 1 and args[1] else 1) - 1
            self.r = min(max(0, row), self.rows - 1)
            self.c = min(max(0, col), self.cols - 1)
        elif final == "A":
            self.r = max(0, self.r - max(1, n))
        elif final == "B":
            self.r = min(self.rows - 1, self.r + max(1, n))
        elif final == "C":
            self.c = min(self.cols - 1, self.c + max(1, n))
        elif final == "D":
            self.c = max(0, self.c - max(1, n))
        elif final == "E":
            self.r = min(self.rows - 1, self.r + max(1, n))
            self.c = 0
        elif final == "F":
            self.r = max(0, self.r - max(1, n))
            self.c = 0
        elif final == "G":
            self.c = min(self.cols - 1, max(0, (n or 1) - 1))
        elif final == "d":
            self.r = min(self.rows - 1, max(0, (n or 1) - 1))
        elif final == "J":
            if n == 2 or n == 3:
                self.grid = [[" "] * self.cols for _ in range(self.rows)]
            elif n == 0:
                self.erase_line(0)
                for row in range(self.r + 1, self.rows):
                    self.grid[row] = [" "] * self.cols
            elif n == 1:
                for row in range(self.r):
                    self.grid[row] = [" "] * self.cols
                self.erase_line(1)
        elif final == "K":
            self.erase_line(n)
        elif final == "L":
            for _ in range(max(1, n)):
                self.grid.insert(self.r, [" "] * self.cols)
                del self.grid[self.rows]
        elif final == "M":
            for _ in range(max(1, n)):
                del self.grid[self.r]
                self.grid.append([" "] * self.cols)
        elif final == "s":
            self.saved = (self.r, self.c)
        elif final == "u":
            self.r, self.c = self.saved
        if final != "m":
            self.wrap_pending = False

    def feed(self, data):
        text = self.pending + self.decoder.decode(data)
        i = 0
        while i < len(text):
            ch = text[i]
            if ch == "\x1b":
                if i + 1 >= len(text):
                    break
                if text[i + 1] == "[":
                    j = i + 2
                    while j < len(text) and not ("@" <= text[j] <= "~"):
                        j += 1
                    if j >= len(text):
                        break
                    params = text[i + 2 : j]
                    if not params.startswith("?") and not params.startswith(">") and not params.startswith("!"):
                        self.csi(params, text[j])
                    i = j + 1
                    continue
                if i + 1 < len(text) and text[i + 1] == "]":
                    j = i + 2
                    while j < len(text):
                        if text[j] == "\x07" or (text[j] == "\x1b" and j + 1 < len(text) and text[j + 1] == "\\"):
                            break
                        j += 1
                    if j >= len(text):
                        break
                    i = j + (2 if j + 1 < len(text) and text[j] == "\x1b" else 1)
                    continue
                if i + 1 < len(text) and text[i + 1] in "PX^_":
                    j = i + 2
                    while j < len(text):
                        if text[j] == "\x1b" and j + 1 < len(text) and text[j + 1] == "\\":
                            break
                        j += 1
                    if j + 1 >= len(text):
                        break
                    i = j + 2
                    continue
                if text[i + 1] in "()#*+":
                    if i + 2 >= len(text):
                        break
                    i += 3
                    continue
                if i + 1 < len(text) and text[i + 1] == "O":
                    i += 3
                    continue
                if i + 1 < len(text) and text[i + 1] == "c":
                    self.grid = [[" "] * self.cols for _ in range(self.rows)]
                    self.r = self.c = 0
                    i += 2
                    continue
                i += 2
                continue
            if ch == "\n" or ch == "\x0b" or ch == "\x0c":
                self.newline()
            elif ch == "\r":
                self.c = 0
                self.wrap_pending = False
            elif ch == "\x08":
                self.c = max(0, self.c - 1)
                self.wrap_pending = False
            elif ch >= " ":
                self.put(ch)
            i += 1
        self.pending = text[i:]

    def lines(self):
        return ["".join(row).rstrip() for row in self.grid]

    def text(self):
        return "\n".join(self.lines())

    def row_of(self, needle):
        for index, line in enumerate(self.lines()):
            if needle in line:
                return index
        return -1


class ModelServer(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    advance = [threading.Event() for _ in range(3)]
    finished = threading.Event()
    spawned = False

    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        child = any("<active_agent name=" in str(m.get("content", ""))
                    for m in body["messages"] if m["role"] in ("system", "developer"))
        if child and body["model"] == "ui-primary":
            self.send_response(404)
            self.send_header("Content-Type", "application/json")
            self.send_header("Connection", "close")
            self.end_headers()
            self.close_connection = True
            self.wfile.write(json.dumps({"error": {"message": "Model unavailable", "type": "not_found"}}).encode())
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()
        self.close_connection = True
        try:
            if child:
                self.chunk({"role": "assistant", "reasoning_content": "Earlier context. " * 30 + "THOUGHT_WIDGET_FIRST"})
                for index, text in enumerate((" THOUGHT_HUB_SECOND", " THOUGHT_VIEWER_THIRD", " THOUGHT_WIDGET_FOURTH")):
                    if not type(self).advance[index].wait(30):
                        return
                    self.chunk({"reasoning_content": text})
                if not type(self).finished.wait(30):
                    return
                self.chunk({"content": "CHILD_UI_DONE"})
                self.chunk({}, "stop")
            elif not type(self).spawned:
                type(self).spawned = True
                args = {"prompt": "Think through the fixture.", "description": "Inspect live thought stream",
                        "role": "@slow", "tool": ["read"], "name": "named-ui-worker"}
                self.chunk({"role": "assistant", "tool_calls": [{"index": 0, "id": "ui-agent",
                    "type": "function", "function": {"name": "Agent", "arguments": json.dumps(args)}}]})
                self.chunk({}, "tool_calls")
            else:
                self.chunk({"role": "assistant", "content": "PARENT_UI_READY. Should I continue?"})
                self.chunk({}, "stop")
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

    def chunk(self, delta, reason=None):
        data = {"id": "subagent-ui", "choices": [{"index": 0, "delta": delta, "finish_reason": reason}]}
        self.wfile.write(("data: " + json.dumps(data) + "\n\n").encode())
        self.wfile.flush()


def plain(data):
    return re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", data)


def run(binary, mode):
    ModelServer.advance = [threading.Event() for _ in range(3)]
    ModelServer.finished = threading.Event()
    ModelServer.spawned = False
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-subagent-ui-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (root / ".pi").mkdir()
            (root / ".pi" / "subagents.json").write_text(json.dumps({"enabled": True}))
            (agent / "models.json").write_text(json.dumps({"providers": {"ui-fixture": {
                "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "local-fixture", "models": [
                    {"id": "ui-primary", "reasoning": True}, {"id": "ui-fixture", "reasoning": True}]}}}))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True, "tuiMode": mode,
                "enableInstallTelemetry": False, "extensionsAutoUpgrade": False,
                "modelRoles": {"default": "ui-fixture/ui-fixture", "slow": "ui-fixture/ui-primary"},
                "retry": {"maxRetries": 0, "modelFallback": True, "fallbackOn": "any",
                          "fallbackChains": {"slow": ["ui-fixture/ui-fixture"]}},
                "modelTags": {"slow": {"color": "error"}}}))
            env = {k: v for k, v in os.environ.items() if not k.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN"))}
            env.update(HOME=directory, PIG_HOME=directory, PI_HOME=directory, PI_OFFLINE="1",
                       PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0", PIG_STARTUP_TRACE="1", TERM="xterm-256color")
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)
            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 48, 160, 0, 0))
            process = subprocess.Popen([str(binary), "--no-extensions", "--model", "@slow", "--offline",
                "--no-context-files", "--no-skills"], cwd=root, env=env,
                stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
            os.close(slave)
            captured = bytearray()
            screen = Screen(48, 160)

            def wait(token):
                observed = bytearray()
                deadline = time.monotonic() + 25
                while time.monotonic() < deadline:
                    if select.select([master], [], [], .05)[0]:
                        try:
                            data = os.read(master, 262144)
                        except OSError:
                            break
                        observed.extend(data)
                        captured.extend(data)
                        screen.feed(data)
                    if token in plain(bytes(observed)):
                        settle = time.monotonic() + .3
                        while time.monotonic() < settle and select.select([master], [], [], .05)[0]:
                            data = os.read(master, 262144)
                            observed.extend(data)
                            captured.extend(data)
                            screen.feed(data)
                        return bytes(observed)
                    if process.poll() is not None:
                        break
                raise AssertionError(f"{mode}: missing {token!r}; exit={process.poll()}\n{bytes(captured[-8000:])!r}")

            def idle(window=0.5):
                observed = bytearray()
                deadline = time.monotonic() + window
                while time.monotonic() < deadline:
                    if select.select([master], [], [], .05)[0]:
                        data = os.read(master, 262144)
                        observed.extend(data)
                        captured.extend(data)
                        screen.feed(data)
                return bytes(observed)

            def wait_gone(pattern, window):
                deadline = time.monotonic() + window
                while time.monotonic() < deadline:
                    if select.select([master], [], [], .05)[0]:
                        data = os.read(master, 262144)
                        captured.extend(data)
                        screen.feed(data)
                    if not re.search(pattern, screen.text()):
                        return
                raise AssertionError(f"{mode}: {pattern!r} still on screen")

            try:
                startup = wait(b"interactive-ready")
                os.write(master, b"Exercise the live subagent UI.\r")
                footer = wait(b"1 subagent \xc2\xb7 1 working")
                assert b"subagent" in plain(footer), "footer lost the agent summary"
                assert "THOUGHT_" not in screen.text(), "tree drew thinking before Down"
                assert screen.row_of("● Agents") == -1, "tree drew rows before Down"
                os.write(master, b"\x1b[B")
                opened = wait(b"THOUGHT_WIDGET_FIRST")
                assert b" Agents" in plain(opened), "Down did not draw the tree"
                assert re.search("[\u251c\u2514]\u2500.*@slow named-ui-worker  Inspect live thought stream.*turns".encode(), plain(opened)), "tree row must show role, name, task, and usage"
                frame = screen.text()
                assert 0 <= screen.row_of("1 subagent · 1 working") < screen.row_of("● Agents"), \
                    f"tree is not drawn below the status line:\n{frame}"
                assert "\u203a" in frame, "selected row lost its cursor"
                role_pattern = rb"(\x1b\[[0-9;]*m)@slow"
                parent_role = re.findall(role_pattern, startup + opened)
                assert parent_role, "main role color was not rendered"
                os.write(master, b"\r")
                wait(b"enter:send")
                assert screen.row_of("● Agents") == -1, "the tree stayed open behind the inspector"
                os.write(master, b"\x1b")
                time.sleep(0.4)
                wait(b"named-ui-worker")
                assert screen.row_of("● Agents") == -1, "the tree silently reopened after the inspector"
                ModelServer.advance[0].set()
                os.write(master, b"\x1ba")
                hub = wait(b"Agent Hub")
                assert b"named-ui-worker" in plain(hub), "hub lost the subagent name"
                assert b"ui-fixture/ui-fixture" in plain(hub), "hub must show the active fallback model, not ui-primary"
                hub_role = re.findall(role_pattern, hub)
                assert parent_role[-1] in hub_role, "hub role color differs from the main role catalog color"
                assert "THOUGHT_HUB_SECOND" in screen.text(), "hub inspector lost the streamed thinking"
                os.write(master, b"\r")
                wait(b"enter:send")
                ModelServer.advance[1].set()
                wait(b"THOUGHT_VIEWER_THIRD")
                os.write(master, b"\x1b")
                time.sleep(0.4)
                wait(b"named-ui-worker")
                os.write(master, b"x\x1b[By")
                wait(b"xy")
                assert screen.row_of("● Agents") == -1, "tree opened over a nonempty prompt"
                os.write(master, b"\x7f\x7f")
                os.write(master, b"first prompt\r")
                wait(b"PARENT_UI_READY")
                os.write(master, b"second\r")
                wait(b"PARENT_UI_READY")
                os.write(master, b"\x1b[A\x1b[A")
                wait(b"first prompt")
                assert screen.row_of("● Agents") == -1, "history Up opened the tree"
                os.write(master, b"\x1b[B")
                wait(b"second")
                assert screen.row_of("● Agents") == -1, "history Down opened the tree"
                os.write(master, b"\x1b[B")
                idle(0.6)
                assert screen.row_of("● Agents") == -1, "restoring the empty draft opened the tree"
                os.write(master, b"\x1b[B")
                wait(b" Agents")
                ModelServer.advance[2].set()
                wait(b"THOUGHT_WIDGET_FOURTH")
                assert not ModelServer.finished.is_set(), "proof ran after child completion"
                os.write(master, b"\x1b")
                idle(0.4)
                ModelServer.finished.set()
                wait(b"1 subagent \xc2\xb7 0 working")
                # A lone Esc resolves after the terminal escape timeout; the
                # completion frame can land first, so wait out the redraw.
                wait_gone(r"● Agents", 5)
                wait_gone(r"\d+ subagents? ·", 35)
                os.write(master, b"\x1b[B")
                idle(0.6)
                assert screen.row_of("● Agents") == -1, "Down reopened an empty tree"
                print(f"{mode}: hidden tree, live footer count, Down selection + Enter inspector, "
                      f"history restore, live thinking while open, 30s cleanup: OK")
            finally:
                ModelServer.finished.set()
                for event in ModelServer.advance:
                    event.set()
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
                os.close(master)
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def verify_screen_chunks():
    payload = "\x1b[2J\x1b[2;1H› ✓\x1b]0;title\x07tree".encode()
    for split in range(len(payload) + 1):
        screen = Screen(3, 20)
        screen.feed(payload[:split])
        screen.feed(payload[split:])
        assert screen.lines() == ["", "› ✓tree", ""], "screen lost a split terminal sequence"
    screen = Screen(3, 20)
    for byte in payload:
        screen.feed(bytes([byte]))
    assert screen.lines() == ["", "› ✓tree", ""], "screen lost fragmented UTF-8"


if __name__ == "__main__":
    verify_screen_chunks()
    binary = Path(sys.argv[1]).resolve()
    for mode in ("fullscreen", "regular"):
        run(binary, mode)
