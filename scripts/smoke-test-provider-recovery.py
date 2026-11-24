#!/usr/bin/env python3
"""Verify provider recovery and cancellation with an isolated loopback model."""
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import sqlite3
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time


class Fixture(http.server.ThreadingHTTPServer):
    def __init__(self):
        super().__init__(("127.0.0.1", 0), Provider)
        self.calls = []
        self.case = "quota"
        self.delay = 2
        self.api = "openai-completions"
        self.account_mode = False
        self.accounts = []
        self.lock = threading.Lock()

    def reset(self, case, delay=2):
        with self.lock:
            self.calls = []
            self.accounts = []
            self.case = case
            self.delay = delay

    def snapshot(self):
        with self.lock:
            return list(self.calls)


class Provider(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        with self.server.lock:
            self.server.calls.append((time.monotonic(), request))
            count = len(self.server.calls)
            case = self.server.case
            delay = self.server.delay
            account_mode = self.server.account_mode
            account = self.headers.get("Authorization", "").removeprefix("Bearer ")
            self.server.accounts.append(account)
            if account_mode:
                delay = 2 if account == "fixture-a" else 4
        status = {"quota": 429, "server": 503,
                  "auth": 401, "forbidden": 403, "billing": 402,
                  "invalid": 400, "unknown-quota": 429, "bounded": 503}[case]
        if count <= (2 if account_mode else 1) or case in ("auth", "forbidden", "billing", "invalid", "unknown-quota", "bounded"):
            code = {"quota": "usage_limit_reached",
                    "server": "server_error", "auth": "invalid_api_key",
                    "forbidden": "permission_denied", "billing": "insufficient_quota",
                    "invalid": "invalid_request_error", "unknown-quota": "insufficient_quota",
                    "bounded": "server_error"}[case]
            wire = json.dumps({"error": {"code": code, "type": code, "message": code}}).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            if case == "quota":
                self.send_header("Retry-After", str(delay))
        else:
            chunks = [
                {"id": "fixture", "object": "chat.completion.chunk",
                 "choices": [{"index": 0, "delta": {"role": "assistant", "content": "RECOVERY_OK"},
                              "finish_reason": None}]},
                {"id": "fixture", "object": "chat.completion.chunk",
                 "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
                 "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
            ]
            if self.server.api == "anthropic-messages":
                chunks = [
                    {"type": "message_start", "message": {"id": "fixture", "type": "message",
                     "role": "assistant", "content": [], "model": "fixture",
                     "usage": {"input_tokens": 1, "output_tokens": 0}}},
                    {"type": "content_block_start", "index": 0,
                     "content_block": {"type": "text", "text": ""}},
                    {"type": "content_block_delta", "index": 0,
                     "delta": {"type": "text_delta", "text": "RECOVERY_OK"}},
                    {"type": "content_block_stop", "index": 0},
                    {"type": "message_delta", "delta": {"stop_reason": "end_turn"},
                     "usage": {"output_tokens": 1}},
                    {"type": "message_stop"},
                ]
            elif self.server.api == "google-generative-ai":
                chunks = [{"candidates": [{"content": {"role": "model", "parts": [{"text": "RECOVERY_OK"}]},
                                           "finishReason": "STOP", "index": 0}],
                           "usageMetadata": {"promptTokenCount": 1, "candidatesTokenCount": 1}}]
            wire = "".join(("event: " + chunk["type"] + "\n" if self.server.api == "anthropic-messages" else "")
                           + "data: " + json.dumps(chunk) + "\n\n" for chunk in chunks)
            if self.server.api == "openai-completions":
                wire += "data: [DONE]\n\n"
            wire = wire.encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire)))
        self.end_headers()
        try:
            self.wfile.write(wire)
        except (BrokenPipeError, ConnectionResetError):
            pass


def profile(root, server, api="openai-completions"):
    agent = root / "agent"
    agent.mkdir()
    (agent / "models.json").write_text(json.dumps({"providers": {"recovery-fixture": {
        "api": api, "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
        "apiKey": "fixture-only", "models": [{"id": "fixture", "contextWindow": 128000,
                                             "maxTokens": 1024}]}}}))
    (agent / "settings.json").write_text(json.dumps({
        "quietStartup": True, "retry": {"maxRetries": 2, "baseDelayMs": 100,
                                        "maxAgentDelayMs": 200, "maxDelayMs": 300,
                                        "waitForUsageReset": True}}))
    env = dict(os.environ, PIG_HOME=str(root), PI_HOME=str(root), TERM="xterm-256color",
               PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
    for name in tuple(env):
        if name.startswith("OMPIG_BROKER_"):
            del env[name]
    for name in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
        env[name] = str(agent)
    return env


def headless(binary, root, env, server):
    for case in ("quota", "auth", "forbidden", "billing", "invalid", "unknown-quota", "bounded"):
        server.reset(case)
        result = subprocess.run([str(binary), "-ne", "-p", "--model", "recovery-fixture/fixture",
                                 "--session-dir", str(root / case), "Continue the fixture task."],
                                env=env, cwd=root, capture_output=True, text=True, timeout=20)
        calls = server.snapshot()
        expected = 2 if case == "quota" else 3 if case == "bounded" else 1
        assert len(calls) == expected, f"{case}: {len(calls)} requests, want {expected}; {result.stderr}"
        if case == "quota":
            assert calls[1][0] - calls[0][0] >= server.delay, "request preceded quota reset"
            assert result.returncode == 0 and "RECOVERY_OK" in result.stdout, result
            key = "contents" if server.api == "google-generative-ai" else "messages"
            assert calls[0][1][key] == calls[1][1][key], "retry changed the task context"
        print(f"PASS: {case} made {expected} provider requests")


def transport_budget(binary, root, env, server):
    settings_path = root / "agent" / "settings.json"
    settings = json.loads(settings_path.read_text())
    settings["retry"]["provider"] = {"maxRetries": 3, "maxRetryDelayMs": 50}
    settings_path.write_text(json.dumps(settings))
    for case in ("unknown-quota", "quota"):
        server.reset(case)
        result = subprocess.run([str(binary), "-ne", "-p", "--model", "recovery-fixture/fixture",
                                 "Continue the transport fixture task."], env=env, cwd=root,
                                capture_output=True, text=True, timeout=15)
        calls = server.snapshot()
        expected = 2 if case == "quota" else 1
        assert len(calls) == expected, f"transport {case}: {len(calls)} requests, want {expected}; {result.stderr}"
        if case == "quota":
            assert calls[1][0] - calls[0][0] >= server.delay, "transport retried before quota reset"
            assert result.returncode == 0 and "RECOVERY_OK" in result.stdout, result
    del settings["retry"]["provider"]
    settings_path.write_text(json.dumps(settings))
    print("PASS: transport retries did not poll permanent quota or shorten reset waits")


def account_pool(binary, root, env, server):
    server.reset("quota")
    server.account_mode = True
    agent = root / "agent"
    models = json.loads((agent / "models.json").read_text())
    del models["providers"]["recovery-fixture"]["apiKey"]
    (agent / "models.json").write_text(json.dumps(models))

    def run(*args):
        return subprocess.run([str(binary), *args], cwd=root, env=env,
                              capture_output=True, text=True, timeout=15)

    result = run("auth", "accounts", "--json")
    assert result.returncode == 0, result
    with sqlite3.connect(agent / "agent.db") as db:
        for account in ("a", "b"):
            db.execute("""INSERT INTO auth_credentials
                (provider, credential_type, data, identity_key) VALUES (?, ?, ?, ?)""",
                       ("recovery-fixture", "api_key", json.dumps({"key": "fixture-" + account}), account))
    result = run("auth", "use", "recovery-fixture", "a")
    assert result.returncode == 0, result
    result = run("-ne", "-p", "--model", "recovery-fixture/fixture", "Continue the pool task.")
    calls = server.snapshot()
    assert result.returncode == 0 and "RECOVERY_OK" in result.stdout, result
    assert server.accounts == ["fixture-a", "fixture-b", "fixture-a"], server.accounts
    assert calls[2][0] >= calls[0][0] + 2, "pool retried before the first account reset"
    assert calls[2][0] < calls[1][0] + 4, "pool waited for the later account reset"
    print("PASS: exhausted account pool resumed at the nearest eligible quota reset")
    server.account_mode = False


def interactive(binary, root, env, server):
    server.reset("quota", 4)
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    process = subprocess.Popen([str(binary), "-ne", "--model", "recovery-fixture/fixture"],
                               env=env, cwd=root, stdin=slave, stdout=slave, stderr=slave,
                               start_new_session=True)
    os.close(slave)
    captured = bytearray()

    def pump(seconds):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if select.select([master], [], [], 0.05)[0]:
                try:
                    captured.extend(os.read(master, 262144))
                except OSError:
                    break

    def visible():
        return re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", captured.decode(errors="replace"))

    def until(predicate, seconds, message):
        deadline = time.monotonic() + seconds
        while not predicate() and time.monotonic() < deadline:
            pump(0.05)
        assert predicate(), message + "\n" + visible()[-5000:]

    try:
        pump(4)
        os.write(master, b"Continue the fixture task.\r")
        until(lambda: len(server.snapshot()) == 1, 10, "no initial request")
        pump(1)
        waiting = visible()
        assert re.search(r"── [^\r\n]*[Ww]aiting", waiting), "missing prompt-border waiting status\n" + waiting[-5000:]
        assert "quota" in waiting.lower() or "usage" in waiting.lower(), "missing wait reason"
        assert len(server.snapshot()) == 1, "provider was polled before reset"
        until(lambda: len(server.snapshot()) == 2, 10, "quota did not resume")
        until(lambda: "RECOVERY_OK" in visible(), 5, "resumed answer not displayed")
        calls = server.snapshot()
        assert calls[1][0] - calls[0][0] >= 4, "TUI retried before reset"
        print("PASS: TUI displayed quota wait and resumed after reset")
        pump(1)
        server.reset("quota", 30)
        captured.clear()
        os.write(master, b"Continue again.\r")
        until(lambda: len(server.snapshot()) == 1, 10, "no cancellation fixture request")
        until(lambda: "waiting" in visible().lower(), 5, "no cancellable wait status")
        os.write(master, b"\x1b")
        pump(0.5)
        os.write(master, b"\x1b")
        pump(1)
        server.reset("server")
        captured.clear()
        os.write(master, b"A new task after cancellation.\r")
        until(lambda: len(server.snapshot()) >= 1, 5, "Escape did not release the waiting session")
        pump(0.5)
        assert process.poll() is None, "TUI exited during cancellation"
        print("PASS: Escape cancelled the quota wait and accepted a new task")
    finally:
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        os.close(master)


def main(binary):
    server = Fixture()
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-provider-recovery-") as directory:
            for api in ("openai-completions", "anthropic-messages", "google-generative-ai"):
                root = Path(directory) / api
                root.mkdir()
                server.api = api
                env = profile(root, server, api)
                headless(binary, root, env, server)
                transport_budget(binary, root, env, server)
                print(f"PASS: {api} recovery policy")
                if api == "openai-completions":
                    interactive(binary, root, env, server)
                    pool_root = root / "pool"
                    pool_root.mkdir()
                    pool_env = profile(pool_root, server)
                    account_pool(binary, pool_root, pool_env, server)
    finally:
        server.shutdown()
        server.server_close()
        worker.join()


if __name__ == "__main__":
    main(Path(sys.argv[1]).resolve())
