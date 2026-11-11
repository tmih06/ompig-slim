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
    """Replay one scripted turn per request: a valid plan, then a rejected call.

    The second user turn emits a payload validation has to reject, which is the
    shape this check exists for: the Tasks panel must show one short reason
    instead of the harness's whole message with the arguments echoed in it.
    """
    stage = 0

    # One entry per assistant turn: the tool call to emit, or the text ending the
    # turn. Both texts end with a question, the sanctioned stop that keeps the
    # completion reminder from continuing the run.
    script = [
        {"call": {"name": "todo", "arguments": json.dumps({"op": "init", "list": [
            {"phase": "Verify", "items": ["Widget task alpha", "Widget task beta"] +
                [f"Scroll item {index:02d}" for index in range(24)] + ["Widget scroll tail"]}]})}},
        {"text": "WIDGET_DONE. Should I continue?"},
        {"call": {"name": "todo", "arguments": json.dumps(
            {"op": "todo", "list": [{"phase": "Verify", "items": ["Widget task gamma"]}]})}},
        {"text": "WIDGET_ERROR_DONE. Should I continue?"},
        {"call": {"name": "todo", "arguments": json.dumps(
            {"op": "start", "phase": "Verify", "task": "Scroll item 18"})}},
        {"text": "WIDGET_WORK_DONE. Should I continue?"},
    ]

    def log_message(self, *_):
        """Keep the fixture quiet; assertions report any failures."""

    def do_POST(self):
        """Use the production tool schema and OpenAI-compatible SSE transport."""
        self.rfile.read(int(self.headers["Content-Length"]))
        turn = type(self).script[min(type(self).stage, len(type(self).script) - 1)]
        if "call" in turn:
            delta = {"role": "assistant", "tool_calls": [{"index": 0,
                "id": f"widget-{type(self).stage}", "type": "function", "function": turn["call"]}]}
            reason = "tool_calls"
        else:
            delta = {"role": "assistant", "content": turn["text"]}
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
            """Wait for an observable frame instead of guessing startup delays.

            Returns what was read while waiting, so an assertion can be scoped to
            one repaint instead of the whole session's output.
            """
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
                    # A diff frame arrives in several writes. Finish consuming
                    # it before the next key, or stale rows can make a later
                    # reopen assertion pass without that row being repainted.
                    while select.select([master], [], [], .1)[0]:
                        data = os.read(master, 262144)
                        captured.extend(data)
                        observed.extend(data)
                    return bytes(observed)
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

            # A call validation rejects must reach the panel as one short reason.
            # The harness message is a header, the failure bullet and a
            # pretty-printed copy of the model's arguments; the panel is a capped
            # overlay, so it must show the bullet alone.
            os.write(master, b"Send the malformed todo call.\r")
            wait(b"WIDGET_ERROR_DONE")
            rejected = bytes(captured)
            assert b"op: expected one of" in rejected, \
                f"{mode}: the panel did not show the rejected member\n{bytes(captured[-4000:])!r}"
            assert b"Received arguments" not in rejected and b"Validation failed" not in rejected, \
                f"{mode}: the whole validation message reached the terminal\n{bytes(captured[-4000:])!r}"

            # The list the call could not change is still the panel's content.
            # Wait for the last row the reopened panel paints, not the first: a
            # terminal diff can arrive one row per write, so waiting for the top
            # row would sample a frame that has not reached the bottom yet.
            os.write(master, b"\x1bw")
            wait("▸ Widgets".encode())
            os.write(master, b"\x1bw")
            reopened = wait(b"Widget task beta")
            for token in (b"op: expected one of", b"Widget task alpha", b"Widget task beta"):
                assert token in reopened, f"{mode}: reopen lost {token!r}\n{reopened!r}"
            # Reopening must reveal active work, even after a manual scroll to
            # the first page. This drives the actual registered renderer and
            # owner-loop focus callback rather than matching status colors.
            work_start = len(captured)
            os.write(master, b"Start work near the end of the plan.\r")
            wait(b"WIDGET_WORK_DONE")
            assert "☐ Scroll item 18".encode() in captured[work_start:], \
                f"{mode}: advancing work did not reveal the active task"
            os.write(master, b"\x1b[5;3~" * 12)
            wait(b"Widget task alpha")
            os.write(master, b"\x1bw")
            wait("▸ Widgets".encode())
            os.write(master, b"\x1bw")
            # Include the checkbox: the transcript receipt also names this
            # task and can be repainted when the overlay disappears.
            reopened = wait("☐ Scroll item 18".encode())
            assert "☐ Scroll item 18".encode() in reopened, f"{mode}: reopened above the active work"
            print(f"PASS: {mode} prompt-anchored widget, compact todo receipt, native checkbox renderer, "
                  "first/last-page scrolling, border-only collapse/reopen, one-row rejected-call reason, "
                  "reopen at active work")
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
