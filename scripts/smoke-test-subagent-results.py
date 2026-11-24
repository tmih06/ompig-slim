#!/usr/bin/env python3
"""Verify background result delivery, idle compaction, steering, and transcript folding."""
import fcntl
import http.server
import importlib.util
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
from xml.sax.saxutils import escape

from benchmark import isolated_env

SPEC = importlib.util.spec_from_file_location(
    "subagent_ui", Path(__file__).with_name("smoke-test-subagent-ui.py"))
UI = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(UI)
FULL_RESULT = "\n".join(f"RESULT_ROW_{index:02d} " + "Unicode λ <result> & exact text. " * 7
                        for index in range(12)) + "\nRESULT_TAIL_VISIBLE"
SUMMARY = "RESULT_IDLE_CHECKPOINT. The child result request remains pending."


def message_text(message):
    content = message.get("content") or ""
    if isinstance(content, str):
        return content
    return "".join(block.get("text", "") for block in content if block.get("type") == "text")


class Fixture(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    release = threading.Event()
    summarized = threading.Event()
    requests = []
    failure = ""
    phase = 0
    warmed = False
    agent_id = ""
    notifications = 0

    def log_message(self, *_):
        pass

    def tool(self, name, args):
        return {"role": "assistant", "tool_calls": [{"index": 0,
            "id": f"result-call-{type(self).phase}", "type": "function",
            "function": {"name": name, "arguments": json.dumps(args)}}]}, "tool_calls"

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        cls = type(self)
        messages = body["messages"]
        child = any("<active_agent name=" in str(message.get("content", ""))
                    for message in messages if message["role"] in ("system", "developer"))
        last_user = next((str(message.get("content", "")) for message in reversed(messages)
                          if message["role"] == "user"), "")
        cls.requests.append(body)
        if child:
            cls.release.wait(40)
            delta, reason = {"role": "assistant", "content": FULL_RESULT}, "stop"
        elif "conversation to summarize" in last_user:
            delta, reason = {"role": "assistant", "content": SUMMARY}, "stop"
            cls.summarized.set()
        elif not cls.warmed and "WARM_RESULT_HISTORY" in json.dumps(messages):
            cls.warmed = True
            delta, reason = {"role": "assistant", "content": "WARM_HISTORY_READY " + "older fixture text " * 300}, "stop"
        elif cls.phase == 0:
            cls.phase = 1
            delta, reason = self.tool("Agent", {"prompt": "Hold until released, then return the fixture result.",
                "description": "Background result proof", "role": "@task", "tool": ["read"],
                "name": "result-worker"})
        elif cls.phase == 1:
            tool_result = str(next(message for message in reversed(messages) if message["role"] == "tool")["content"])
            match = re.search(r"agent_[a-z0-9]{6,}", tool_result)
            cls.agent_id = match[0] if match else "missing"
            cls.phase = 2
            delta, reason = self.tool("get_subagent_result", {"agent_id": cls.agent_id, "wait": True})
        elif cls.phase == 2:
            tool_result = str(next(message for message in reversed(messages) if message["role"] == "tool")["content"])
            if "running" not in tool_result or "pending" not in tool_result.lower():
                cls.failure = "first read did not register a pending result: " + tool_result
            cls.phase = 3
            delta, reason = self.tool("get_subagent_result", {"agent_id": "@result-worker", "wait": False})
        elif cls.phase == 3:
            tool_result = str(next(message for message in reversed(messages) if message["role"] == "tool")["content"])
            if "already" not in tool_result.lower() or "pending" not in tool_result.lower():
                cls.failure = "duplicate read did not report an existing request: " + tool_result
            cls.phase = 4
            delta, reason = {"role": "assistant", "content": "ROOT_IDLE_READY"}, "stop"
        elif cls.phase == 4 and not cls.release.is_set() and "STEER_WHILE_RESULT_PENDING" in json.dumps(messages):
            delta, reason = {"role": "assistant", "content": "STEER_ACCEPTED_WHILE_CHILD_WORKS"}, "stop"
        elif cls.phase == 4:
            payload = "\n".join(message_text(message) for message in messages)
            if escape(FULL_RESULT) not in payload:
                cls.failure = "automatic completion did not carry the exact full result"
            cls.notifications += 1
            cls.phase = 5
            delta, reason = self.tool("get_subagent_result", {"agent_id": cls.agent_id, "wait": True})
        else:
            tool_result = message_text(next(message for message in reversed(messages) if message["role"] == "tool"))
            if tool_result != FULL_RESULT:
                cls.failure = "terminal retrieval changed the exact result"
            delta, reason = {"role": "assistant", "content": "RESULT_READ_DONE"}, "stop"
        chunks = [
            {"id": "result-proof", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "result-proof", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}],
             "usage": {"prompt_tokens": 4000, "completion_tokens": 400}},
        ]
        wire = ("".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n").encode()
        try:
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(wire)))
            self.end_headers()
            self.wfile.write(wire)
        except (BrokenPipeError, ConnectionResetError):
            pass


def run(binary, mode):
    Fixture.release, Fixture.summarized = threading.Event(), threading.Event()
    Fixture.phase, Fixture.failure, Fixture.requests, Fixture.notifications = 0, "", [], 0
    Fixture.warmed = False
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-result-proof-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (root / ".pi").mkdir()
            (root / ".pi/subagents.json").write_text(json.dumps({"enabled": True}))
            (agent / "models.json").write_text(json.dumps({"providers": {"result-proof": {
                "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "fixture", "models": [{"id": "result-proof", "contextWindow": 128000,
                                                   "maxTokens": 4096}]}}}))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True, "tuiMode": mode,
                "enableInstallTelemetry": False, "extensionsAutoUpgrade": False,
                "modelRoles": {"default": "result-proof/result-proof"},
                "compaction": {"idleEnabled": True, "idleDelaySeconds": 2,
                               "keepRecentTokens": 100, "reserveTokens": 1024},
                "cacheWarming": "off", "retry": {"enabled": False}}))
            env = isolated_env(root, agent)
            env.update(PIG_STARTUP_TRACE="1", TERM="xterm-256color")
            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 48, 160, 0, 0))
            process = subprocess.Popen([str(binary), "--no-extensions", "--offline", "--model", "@default",
                "--no-context-files", "--no-skills"], cwd=root, env=env, stdin=slave,
                stdout=slave, stderr=slave, start_new_session=True)
            os.close(slave)
            screen = UI.Screen(48, 160)
            captured = bytearray()

            def pump(seconds=.2):
                deadline = time.monotonic() + seconds
                while time.monotonic() < deadline:
                    if select.select([master], [], [], .05)[0]:
                        try:
                            data = os.read(master, 262144)
                        except OSError:
                            break
                        captured.extend(data)
                        screen.feed(data)

            def wait_for(predicate, label, timeout=15):
                deadline = time.monotonic() + timeout
                while time.monotonic() < deadline:
                    pump()
                    if predicate():
                        return
                    if process.poll() is not None:
                        break
                raise AssertionError(f"{mode}: {label}; failure={Fixture.failure!r}; "
                                     f"phase={Fixture.phase}; screen:\n{screen.text()}\n"
                                     f"latest={latest_request()}\n"
                                     f"tail={bytes(captured[-1500:])!r}")

            def latest_request():
                return [(message["role"], str(message.get("content", ""))[:200])
                        for message in Fixture.requests[0]["messages"][-3:]] if Fixture.requests else []

            def entries():
                return [json.loads(line) for path in (agent / "sessions").glob("*/*.jsonl")
                        for line in path.read_text().splitlines()]

            try:
                wait_for(lambda: b"interactive-ready" in captured, "startup")
                os.write(master, b"WARM_RESULT_HISTORY\r")
                wait_for(lambda: "WARM_HISTORY_READY" in screen.text(), "history preparation")
                os.write(master, b"Start the background result proof.\r")
                wait_for(lambda: "ROOT_IDLE_READY" in screen.text(), "root did not settle while child was blocked")
                assert not Fixture.release.is_set(), "child finished before nonblocking proof"
                assert not Fixture.failure, Fixture.failure
                wait_for(lambda: any(entry.get("type") == "compaction" for entry in entries()),
                         "idle compaction did not commit while result was pending")
                assert Fixture.summarized.is_set(), "no provider summary request"
                os.write(master, b"STEER_WHILE_RESULT_PENDING\r")
                wait_for(lambda: "STEER_ACCEPTED_WHILE_CHILD_WORKS" in screen.text(), "root steering")
                assert not Fixture.release.is_set(), "steering proof ran after completion"
                Fixture.release.set()
                wait_for(lambda: "RESULT_READ_DONE" in screen.text(), "automatic full delivery")
                assert not Fixture.failure, Fixture.failure
                assert Fixture.notifications == 1, "duplicate automatic full delivery"
                assert "RESULT_TAIL_VISIBLE" not in screen.text(), "long result did not fold"
                assert "ctrl+o" in screen.text().lower(), "missing expansion hint"
                os.write(master, b"\x0f")
                wait_for(lambda: "RESULT_TAIL_VISIBLE" in screen.text(), "Ctrl+O did not expand exact result")
                os.write(master, b"\x0f")
                wait_for(lambda: "RESULT_TAIL_VISIBLE" not in screen.text(), "Ctrl+O did not collapse result")
                notifications = [entry for entry in entries() if entry.get("type") == "custom_message"
                                 and entry.get("customType") == "subagents:notification"]
                assert len(notifications) == 1, f"persisted duplicate notifications: {len(notifications)}"
                assert escape(FULL_RESULT) in notifications[0]["content"], "saved delivery truncated result"
                print(f"PASS {mode}. Pending wait, duplicate detection, idle compaction, steering, "
                      "one exact automatic result, and Ctrl+O folding verified.")
            finally:
                Fixture.release.set()
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
                os.close(master)
    finally:
        Fixture.release.set()
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


if __name__ == "__main__":
    binary = Path(sys.argv[1]).resolve()
    for mode in ("fullscreen", "regular"):
        run(binary, mode)
