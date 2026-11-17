#!/usr/bin/env python3
"""Verify /quit output after real terminal teardown with isolated sessions."""
import argparse
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import shlex
import struct
import subprocess
import tempfile
import termios
import threading
import time


class ModelServer(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        self.rfile.read(int(self.headers["Content-Length"]))
        chunks = [
            {"id": "quit-proof", "choices": [{"index": 0, "delta": {
                "role": "assistant", "content": "QUIT_PROOF_DONE. Should I continue?"},
                "finish_reason": None}]},
            {"id": "quit-proof", "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
             "usage": {"prompt_tokens": 1000, "completion_tokens": 500,
                       "prompt_tokens_details": {"cached_tokens": 200}}},
        ]
        wire = ("".join("data: " + json.dumps(c) + "\n\n" for c in chunks)
                + "data: [DONE]\n\n").encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire)))
        self.end_headers()
        self.wfile.write(wire)


def run(binary, server, mode, persisted, populated, custom=False):
    with tempfile.TemporaryDirectory(prefix="ompig-quit-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        (agent / "models.json").write_text(json.dumps({"providers": {"quit-proof": {
            "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
            "apiKey": "fixture", "models": [{"id": "quit-proof", "cost": {
                "input": 2, "output": 4, "cacheRead": 1, "cacheWrite": 0}}]}}}))
        (agent / "settings.json").write_text(json.dumps({"quietStartup": True,
            "tuiMode": mode, "enableInstallTelemetry": False}))
        env = dict(os.environ, HOME=directory, PIG_HOME=directory, PI_HOME=directory,
                   PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                   PIG_STARTUP_TRACE="1", TERM="xterm-256color", TERM_PROGRAM="quit-proof")
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        for key in ("OMPIG_BROKER_URL", "OMPIG_BROKER_TOKEN", "OMPIG_BROKER_TOKEN_FILE"):
            env.pop(key, None)
        args = [str(binary), "--no-extensions", "--provider", "quit-proof", "--model", "quit-proof",
                "--no-context-files", "--no-skills", "--offline"]
        if not persisted:
            args.append("--no-session")
        if custom:
            args += ["--session-dir", str(root / "custom sessions")]
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        process = subprocess.Popen(args, stdin=slave, stdout=slave, stderr=slave,
                                   cwd=root, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()

        def wait_for(token=None):
            deadline = time.monotonic() + 25
            while time.monotonic() < deadline:
                if select.select([master], [], [], .05)[0]:
                    try:
                        data = os.read(master, 262144)
                    except OSError:
                        break
                    if not data:
                        break
                    captured.extend(data)
                if token is not None and token in captured:
                    return
            if token is not None:
                raise AssertionError(f"missing {token!r}; exit={process.poll()}; tail={captured[-4000:]!r}")

        try:
            wait_for(b"interactive-ready")
            if populated:
                os.write(master, b"Verify the quit summary.\r")
                wait_for(b"QUIT_PROOF_DONE")
            os.write(master, b"/quit\r")
            wait_for()
            assert process.wait(timeout=5) == 0, f"{mode}: quit failed"
            plain = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", bytes(captured)).decode(errors="replace")
            assert plain.count("Session summary") == 1, plain[-4000:]
            summary = plain[plain.index("Session summary"):]
            # The two lines immediately above the summary, so the startup header and
            # the terminal title cannot satisfy the logo assertion. Apple Terminal and
            # a terminal without block glyphs print the one-line wordmark instead.
            before = re.sub(rb"\x1b\][^\x07]*\x07", b"", bytes(captured[:captured.index(b"Session summary")]))
            before = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", before).decode(errors="replace")
            lines = [line for line in before.splitlines() if line.strip()]
            assert len(lines) >= 2, f"no quit logo: {before[-400:]!r}"
            top, bottom = lines[-2], lines[-1]
            wordmark = "OMPi -slim" in bottom
            assert "-slim" in top or wordmark, f"quit logo top line missing: {top!r}"
            assert wordmark or (" in " in bottom and "\u2588\u2584\u2588" in bottom), \
                f"quit logo bottom line missing: {bottom!r}"
            summary_at = captured.index(b"Session summary")
            if mode == "fullscreen":
                assert 0 <= captured.rfind(b"\x1b[?1049l") < summary_at, \
                    "summary printed inside alternate screen"
            if populated:
                for expected in ("1 user, 1 assistant, 0 tool calls", "↑ 1.0k", "cached 200", "↓ 500", "$0.004"):
                    assert expected in summary, f"missing {expected!r}: {summary}"
            else:
                assert "0 user, 0 assistant, 0 tool calls" in summary, summary
                assert "↑ 0" in summary and "↓ 0" in summary, summary
                assert "Cost" not in summary, summary
            if persisted:
                match = re.search(r"Resume\s+(ompig-slim .*--session \S+)", summary)
                assert match, summary
                command = shlex.split(match.group(1))
                if custom:
                    assert command[command.index("--session-dir") + 1] == str(root / "custom sessions"), summary
            else:
                assert "Resume" not in summary, summary
            print(f"PASS: /quit {mode}, persisted={persisted}, populated={populated}, custom={custom}")
            print(summary.strip())
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
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        for scenario in (("regular", True, True, False), ("fullscreen", True, True, True),
                         ("regular", False, False, False), ("regular", False, True, False)):
            run(args.binary.resolve(), server, *scenario)
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main()
