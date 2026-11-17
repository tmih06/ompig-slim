#!/usr/bin/env python3
"""Drive /goal, Escape, and /goal resume in the real TUI against a loopback model.

Records every model request so the scenario can assert what the runtime did:
which requests carried a goal continuation, and whether a resume started work.
"""
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time

calls = []
lock = threading.Lock()
gate = threading.Event()
quota_error = "--quota" in sys.argv


class Model(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def do_POST(self):
        body = self.rfile.read(int(self.headers["Content-Length"]))
        with lock:
            calls.append(json.loads(body))
            n = len(calls)
        if quota_error:
            wire = json.dumps({"error": {"message": "Quota exhausted", "type": "insufficient_quota"}}).encode()
            self.send_response(402)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(wire)))
            self.end_headers()
            self.wfile.write(wire)
            return
        # Keep the third turn running until Escape has cancelled it.
        if n == 3:
            gate.wait(30)
        text = f"REPLY_{n}"
        chunks = [
            {"id": "x", "object": "chat.completion.chunk",
             "choices": [{"index": 0, "delta": {"role": "assistant", "content": text}, "finish_reason": None}]},
            {"id": "x", "object": "chat.completion.chunk",
             "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n"
        try:
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(wire.encode())))
            self.end_headers()
            self.wfile.write(wire.encode())
        except OSError:
            pass


def snapshot():
    with lock:
        return list(calls)


def messages(call):
    return call.get("messages", [])


def has_poke(call):
    """True when the request itself is a goal continuation, not merely a later
    request whose transcript still contains an earlier poke."""
    msgs = messages(call)
    return bool(msgs) and "Goal poke" in json.dumps(msgs[-1])


def objective_prompt(call):
    """True when the request's last message is the objective as a plain prompt."""
    msgs = messages(call)
    if not msgs:
        return False
    last = msgs[-1]
    if last.get("role") != "user":
        return False
    content = last.get("content")
    text = content[0].get("text") if isinstance(content, list) and content else content
    return text == "finish the fixture"


def main(binary):
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Model)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    port = server.server_address[1]
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-goal-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (agent / "models.json").write_text(json.dumps({"providers": {"fx": {
                "api": "openai-completions",
                "baseUrl": f"http://127.0.0.1:{port}/v1",
                "apiKey": "fixture-only",
                "models": [{"id": "fixture", "name": "fixture", "contextWindow": 128000,
                            "maxTokens": 1024}]}}}))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True}))
            env = dict(os.environ, PIG_HOME=directory, PI_HOME=directory, TERM="xterm-256color",
                       PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)
            for key in list(env):
                if key.startswith("OMPIG_BROKER_"):
                    del env[key]
            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
            process = subprocess.Popen([str(binary), "--model", "fixture"], stdin=slave, stdout=slave,
                                       stderr=slave, cwd=root, env=env, start_new_session=True)
            os.close(slave)
            captured = bytearray()

            def pump(seconds):
                end = time.monotonic() + seconds
                while time.monotonic() < end:
                    if select.select([master], [], [], 0.05)[0]:
                        try:
                            captured.extend(os.read(master, 262144))
                        except OSError:
                            return

            def wait_requests(count, seconds):
                end = time.monotonic() + seconds
                while time.monotonic() < end and len(snapshot()) < count:
                    pump(0.05)
                return len(snapshot())

            try:
                pump(8)
                # 1. Start a goal; let it continue autonomously for a few turns.
                os.write(master, "/goal finish the fixture\r".encode())
                if quota_error:
                    wait_requests(1, 15)
                    pump(4)
                    assert len(snapshot()) == 1, \
                        f"quota error triggered {len(snapshot())} provider requests, want 1"
                    os.write(master, b"/goal status\r")
                    pump(1)
                    assert b"paused" in captured, "quota error did not pause the goal"
                    os.write(master, b"/goal resume\r")
                    wait_requests(2, 15)
                    pump(4)
                    assert len(snapshot()) == 2, \
                        f"resume after quota error triggered {len(snapshot())} total requests, want 2"
                    print("PASS: quota error paused the goal without poking; explicit resume retried once")
                    return
                wait_requests(3, 15)
                first = snapshot()
                assert len(first) == 3, \
                    f"expected the held third goal turn, got {len(first)} requests"
                assert all(has_poke(c) for c in first[1:]), "an active goal did not continue"

                # 2. Escape while a continuation is running. The editor gives the
                # first Escape to an open completion popup, so a user interrupts
                # with a second press; sending both is what a person does.
                os.write(master, b"\x1b")
                pump(0.5)
                os.write(master, b"\x1b")
                pump(3)
                settle = len(snapshot())
                deadline = time.monotonic() + 10
                while time.monotonic() < deadline:
                    pump(0.2)
                    if len(snapshot()) == settle:
                        break
                    settle = len(snapshot())
                after_escape = len(snapshot())
                pump(4)
                assert len(snapshot()) == after_escape, \
                    f"Escape left a continuation running: {len(snapshot()) - after_escape} more turns"

                gate.set()

                # 3. An ordinary question must not resume the paused goal.
                os.write(master, "what is the status?\r".encode())
                wait_requests(after_escape + 1, 15)
                pump(3)
                question_calls = snapshot()[after_escape:]
                assert question_calls, "the question ran no turn"
                assert not any(has_poke(c) for c in question_calls), \
                    "a paused goal continued after an ordinary question"

                # 4. Explicit resume must restart the objective.
                before_resume = len(snapshot())
                os.write(master, "/goal resume\r".encode())
                wait_requests(before_resume + 1, 15)
                pump(2)
                resumed = snapshot()[before_resume:]
                assert resumed, "explicit resume started no turn"
                assert any(objective_prompt(c) for c in resumed), \
                    "resume did not restart the objective"

                print(f"PASS: {len(first)} goal turns continued autonomously, "
                      f"Escape stopped continuation, the question ran {len(question_calls)} "
                      f"turn(s) without a poke, resume restarted the objective")
            finally:
                gate.set()
                if process.poll() is None:
                    process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                os.close(master)
    finally:
        server.shutdown()


if __name__ == "__main__":
    main(Path(sys.argv[1]).resolve())
