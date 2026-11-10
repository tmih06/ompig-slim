#!/usr/bin/env python3
"""Exercise native todo widgets through a real PTY and a loopback model."""
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


class ModelServer(http.server.BaseHTTPRequestHandler):
    """Return one todo mutation, then a legitimate handoff (no settle nudge)."""
    stage = 0

    def log_message(self, *_):
        """Keep the fixture quiet; assertions report any failures."""

    def do_POST(self):
        """Use the production tool schema and OpenAI-compatible SSE transport."""
        self.rfile.read(int(self.headers["Content-Length"]))
        if type(self).stage == 0:
            items = ["Widget task alpha", "Widget task beta"]
            items += [f"Scroll item {index:02d}" for index in range(24)]
            items.append("Widget scroll tail")
            args = {"op": "init", "list": [{"phase": "Verify", "items": items}]}
            delta = {"role": "assistant", "tool_calls": [{"index": 0,
                "id": "widget-proof", "type": "function", "function": {
                    "name": "todo", "arguments": json.dumps(args)}}]}
            reason = "tool_calls"
        else:
            delta = {"role": "assistant", "content": "WIDGET_DONE. Should I continue?"}
            reason = "stop"
        type(self).stage += 1
        chunks = [
            {"id": "proof", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "proof", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())


def run(binary, server, mode):
    """Isolate profiles and inspect actual widget output and keyboard disclosure."""
    with tempfile.TemporaryDirectory(prefix="ompig-widget-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        (agent / "models.json").write_text(json.dumps({"providers": {"widget-proof": {
            "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
            "apiKey": "fixture", "models": [{"id": "widget-proof"}]}}}))
        (agent / "settings.json").write_text(json.dumps({"quietStartup": True,
            "tuiMode": mode, "enableInstallTelemetry": False}))
        env = dict(os.environ, HOME=directory, PIG_HOME=directory, PI_HOME=directory,
                   PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                   PIG_STARTUP_TRACE="1", TERM="xterm-256color")
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        ModelServer.stage = 0
        process = subprocess.Popen([str(binary), "--no-extensions", "--provider", "widget-proof",
            "--model", "widget-proof", "--no-context-files", "--no-skills", "--offline"],
            stdin=slave, stdout=slave, stderr=slave, cwd=root, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()

        def wait(token):
            """Wait for an observable frame instead of guessing startup delays."""
            observed = bytearray()
            deadline = time.monotonic() + 25
            while time.monotonic() < deadline:
                if select.select([master], [], [], .05)[0]:
                    try:
                        data = os.read(master, 262144)
                    except OSError:
                        break
                    captured.extend(data)
                    observed.extend(data)
                if token in observed:
                    return
                if process.poll() is not None:
                    break
            raise AssertionError(f"{mode}: missing {token!r}; exit={process.poll()}\n{bytes(captured[-4000:])!r}")

        try:
            wait(b"interactive-ready")
            os.write(master, b"Exercise the task widget.\r")
            wait(b"WIDGET_DONE")
            for token in ("Tasks", "Widget task alpha", "Widget task beta", "☐"):
                assert token.encode() in captured, f"{mode}: missing {token}\n{bytes(captured[-6000:])!r}"
            plain = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", bytes(captured))
            assert b"Todo: init" in plain and b"0/27 done" in plain, "compact todo receipt missing"
            assert b"Validation failed" not in captured, "invalid tool fixture"
            assert b"Remaining items" not in captured, "generic tool fallback bypassed todo renderer"
            assert b"ctrl+o: Collapse" not in captured, "transcript tool frame leaked into widget"
            os.write(master, b"\x1b[6;3~" * 12)  # Alt+PageDown to the last task.
            wait(b"Widget scroll tail")
            os.write(master, b"\x1b[5;3~" * 12)  # Alt+PageUp restores the first page.
            wait(b"Widget task alpha")
            os.write(master, b"\x1bw")
            wait("▸ Widgets".encode())
            os.write(master, b"\x1bw")
            wait("▾ Widgets".encode())
            print(f"PASS: {mode} prompt-anchored widget, compact todo receipt, native checkbox renderer, first/last-page scrolling, border-only collapse/reopen")
        finally:
            if process.poll() is None:
                process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            os.close(master)


def main():
    """Run both TUI modes against the caller's built binary, without paid APIs."""
    if len(sys.argv) != 2:
        raise SystemExit("Usage: smoke-test-info-widgets.py <ompig-slim-binary>")
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        for mode in ("fullscreen", "regular"):
            run(Path(sys.argv[1]).resolve(), server, mode)
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main()
