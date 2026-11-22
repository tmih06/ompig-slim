#!/usr/bin/env python3
"""Verify /session and /hotkeys open their own boards in a real PTY.

The text must leave the transcript alone: it appears only in the board. A
regular chat switches to the alternate screen for it and back on close; a
fullscreen chat stays where it is."""
import argparse
import fcntl
import http.server
import json
import os
import pty
import re
import select
import struct
import subprocess
import tempfile
import termios
import threading
import time
from pathlib import Path


class ModelServer(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        self.rfile.read(int(self.headers["Content-Length"]))
        chunks = [
            {"id": "stats-proof", "choices": [{"index": 0, "delta": {
                "role": "assistant", "content": "STATS_PROOF_DONE."},
                "finish_reason": None}]},
            {"id": "stats-proof", "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
             "usage": {"prompt_tokens": 42000, "completion_tokens": 1500,
                       "prompt_tokens_details": {"cached_tokens": 39000}}},
        ]
        wire = ("".join("data: " + json.dumps(c) + "\n\n" for c in chunks)
                + "data: [DONE]\n\n").encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire)))
        self.end_headers()
        self.wfile.write(wire)


def run(binary, server, mode):
    with tempfile.TemporaryDirectory(prefix="ompig-session-stats-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        (agent / "models.json").write_text(json.dumps({"providers": {"stats-proof": {
            "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
            "apiKey": "fixture", "models": [{"id": "stats-proof", "cost": {
                "input": 2, "output": 4, "cacheRead": 1, "cacheWrite": 0}}]}}}))
        (agent / "settings.json").write_text(json.dumps({
            "quietStartup": True, "enableInstallTelemetry": False, "tuiMode": mode}))
        env = dict(os.environ, HOME=directory, PIG_HOME=directory, PI_HOME=directory,
                   PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                   PIG_STARTUP_TRACE="1",
                   TERM="xterm-256color", TERM_PROGRAM="stats-proof")
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        for key in ("OMPIG_BROKER_URL", "OMPIG_BROKER_TOKEN", "OMPIG_BROKER_TOKEN_FILE"):
            env.pop(key, None)
        args = [str(binary), "--no-extensions", "--provider", "stats-proof", "--model", "stats-proof",
                "--no-context-files", "--no-skills", "--offline"]
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        process = subprocess.Popen(args, stdin=slave, stdout=slave, stderr=slave,
                                   cwd=root, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()

        def wait_for(token=None, timeout=25, count=1):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if select.select([master], [], [], .05)[0]:
                    try:
                        data = os.read(master, 262144)
                    except OSError:
                        break
                    if not data:
                        break
                    captured.extend(data)
                if token is not None and captured.count(token) >= count:
                    return
            if token is not None:
                raise AssertionError(f"missing {token!r} x{count}; exit={process.poll()}; tail={captured[-4000:]!r}")

        try:
            wait_for(b"interactive-ready")
            # Two turns so the timeline has a spread of message timestamps.
            os.write(master, b"Give me session stats.\r")
            wait_for(b"STATS_PROOF_DONE")
            os.write(master, b"Again.\r")
            wait_for(b"STATS_PROOF_DONE", count=2)
            alt_on, alt_off = b"\x1b[?1049h", b"\x1b[?1049l"

            def open_board(command, token):
                """Run a slash command that opens a board; return where its output starts."""
                start = len(captured)
                os.write(master, command + b"\r")
                wait_for(token, count=bytes(captured).count(token) + 1)
                wait_for(b"close")
                opened = bytes(captured[start:])
                if mode == "regular":
                    assert alt_on in opened, f"{command!r} did not switch to the alternate screen"
                    assert opened.index(alt_on) < opened.index(token), \
                        f"{command!r} drew its page before the alternate screen"
                else:
                    assert alt_on not in opened, f"fullscreen chat switched screens for {command!r}"
                return start

            def close_board():
                offs = bytes(captured).count(alt_off)
                os.write(master, b"\x1b")
                if mode == "regular":
                    wait_for(alt_off, count=offs + 1, timeout=10)
                else:
                    wait_for(timeout=1)  # no screen switch to wait for; the next command proves it closed
                return len(captured)

            before = open_board(b"/session", b"Tokens by model")
            wait_for(b"refresh")
            os.write(master, b"r")  # refresh keeps the board open
            wait_for(timeout=1)
            assert alt_off not in bytes(captured[before:]), "refresh closed the board"
            session_closed = close_board()

            # /hotkeys opens the same board; it has nothing to refresh.
            hotkeys = open_board(b"/hotkeys", b"Keyboard Shortcuts")
            wait_for(b"Send message")
            assert b"refresh" not in bytes(captured[hotkeys:]), "hotkeys advertises a refresh it cannot do"
            closed_at = close_board()

            os.write(master, b"/quit\r")
            wait_for()
            assert process.wait(timeout=5) == 0, "quit failed"
            assert b"Tokens by model" not in bytes(captured[session_closed:]), \
                "the report leaked into the chat after the view closed"
            assert b"Keyboard Shortcuts" not in bytes(captured[closed_at:]), \
                "the shortcuts leaked into the chat after the board closed"
            plain = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", bytes(captured[before:session_closed])).decode(errors="replace")
            report = plain[plain.index("Session stats"):]
            for want in ("Session stats", "Messages", "Input tokens", "Started",
                         "Tokens by model", "stats-proof/stats-proof", "%",
                         "Total", "Cache Warming", "Cost"):
                assert want in report, f"missing {want!r}:\n{report}"
            track = re.search(r"stats-proof/stats-proof\s+([·█▌▐]+)\s", report)
            assert track, f"no time trace row:\n{report}"
            assert track.group(1).strip("·"), f"the model's track has no busy cell:\n{report}"
            # The view widens the 40-cell transcript track to the 120-column terminal.
            assert len(track.group(1)) > 40, f"trace was not widened: {len(track.group(1))} cells"
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    server = http.server.HTTPServer(("127.0.0.1", 0), ModelServer)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    for mode in ("regular", "fullscreen"):
        run(args.binary.resolve(), server, mode)
    print("PASS session-stats")


if __name__ == "__main__":
    main()
