#!/usr/bin/env python3
"""Verify the /session stats report renders the chart sections in a real PTY."""
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


def run(binary, server):
    with tempfile.TemporaryDirectory(prefix="ompig-session-stats-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        (agent / "models.json").write_text(json.dumps({"providers": {"stats-proof": {
            "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
            "apiKey": "fixture", "models": [{"id": "stats-proof", "cost": {
                "input": 2, "output": 4, "cacheRead": 1, "cacheWrite": 0}}]}}}))
        (agent / "settings.json").write_text(json.dumps({
            "quietStartup": True, "enableInstallTelemetry": False}))
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
            os.write(master, b"/session\r")
            wait_for(b"Tokens by model")
            os.write(master, b"/quit\r")
            wait_for()
            assert process.wait(timeout=5) == 0, "quit failed"
            plain = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", bytes(captured)).decode(errors="replace")
            report = plain[plain.index("Session Stats"):]
            for want in ("Session Stats", "Messages", "Tokens",
                         "Tokens by model", "stats-proof/stats-proof", "%",
                         "Total", "Timeline", "Started:", "Activity:",
                         "Cache Warming", "Cost"):
                assert want in report, f"missing {want!r}:\n{report}"
            bars = re.findall(r"\u2588+", report)
            assert bars, f"no model bars:\n{report}"
            spark = re.findall(r"[\u2581-\u2588]{8,}", report)
            assert spark, f"no timeline sparkline:\n{report}"
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    server = http.server.HTTPServer(("127.0.0.1", 0), ModelServer)
    import threading
    threading.Thread(target=server.serve_forever, daemon=True).start()
    run(args.binary.resolve(), server)
    print("PASS session-stats")


if __name__ == "__main__":
    main()
