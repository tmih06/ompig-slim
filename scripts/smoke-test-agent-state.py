#!/usr/bin/env python3
"""Prove the agent-state document and the Herdr adapter in the real binary.

Two things are asserted against a built executable:

1. The binary publishes <PIG_HOME>/state/agent/status.json and moves it through
   idle -> working -> idle for a real model turn.
2. The installable Herdr extension in extensions/herdr, loaded with `-e`, turns
   those transitions into `herdr pane report-agent` calls, which a fake Herdr
   executable records.

The extension is built by PiG on first load, so this needs a Go toolchain and the
staged extension SDK, exactly like any other installed extension.
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


class Model(http.server.BaseHTTPRequestHandler):
    """Answer one chat completion, slowly enough that `working` is observable."""

    protocol_version = "HTTP/1.1"
    delay = 1.5

    def log_message(self, *_):
        """Keep the fixture quiet; failures are asserted, not logged."""

    def do_POST(self):
        """Return a single assistant message over the OpenAI SSE transport."""
        self.rfile.read(int(self.headers["Content-Length"]))
        time.sleep(type(self).delay)
        chunks = [
            {"id": "state", "choices": [{"index": 0, "delta": {"role": "assistant", "content": "STATE_DONE"},
                                         "finish_reason": None}]},
            {"id": "state", "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())


def run(binary):
    """Drive one interactive turn and inspect the published state and Herdr calls."""
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Model)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    port = server.server_address[1]
    extension = Path(__file__).resolve().parent.parent / "extensions" / "herdr"
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-agent-state-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (agent / "models.json").write_text(json.dumps({"providers": {"state-fixture": {
                "api": "openai-completions",
                "baseUrl": f"http://127.0.0.1:{port}/v1",
                "apiKey": "local-fixture-only",
                "models": [{"id": "state-fixture"}]}}}))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True}))
            herdr_log = root / "herdr.log"
            fake_herdr = root / "herdr"
            fake_herdr.write_text(
                "#!/bin/sh\n"
                'printf "%s\\n" "$*" >> "$HERDR_LOG"\n'
                'echo \'{"id":"cli:request","result":{"ok":{}}}\'\n')
            fake_herdr.chmod(0o755)
            agent_root = root / "state" / "agent"

            env = dict(os.environ,
                       PIG_HOME=directory, PI_HOME=directory, TERM="xterm-256color",
                       PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                       HERDR_ENV="1", HERDR_PANE_ID="w1:p1",
                       HERDR_BIN_PATH=str(fake_herdr), HERDR_LOG=str(herdr_log))
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)

            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
            process = subprocess.Popen([str(binary), "--model", "state-fixture", "-e", str(extension)],
                                       stdin=slave, stdout=slave, stderr=slave,
                                       cwd=root, env=env, start_new_session=True)
            os.close(slave)
            captured = bytearray()
            states = []

            def published():
                """Return the one live agent document, or None.

                The document is one directory per process, so this enumerates
                rather than assuming a path: that is what an external observer
                does, and it is what the test is meant to prove.
                """
                if not agent_root.is_dir():
                    return None
                documents = [entry / "status.json" for entry in agent_root.iterdir()
                             if entry.is_dir() and (entry / "status.json").exists()]
                if len(documents) != 1:
                    return None
                try:
                    return json.loads(documents[0].read_text())
                except ValueError:
                    return None

            def pump(seconds):
                """Read output and sample the published state until the deadline."""
                pump_until(lambda: False, seconds)

            def pump_until(condition, seconds):
                """Read output and sample state until condition holds or time is up."""
                end = time.monotonic() + seconds
                while time.monotonic() < end:
                    if select.select([master], [], [], 0.02)[0]:
                        try:
                            captured.extend(os.read(master, 262144))
                        except OSError:
                            return False
                    document = published()
                    if document and document.get("state"):
                        if not states or states[-1] != document["state"]:
                            states.append(document["state"])
                    if condition():
                        return True
                return condition()

            try:
                # The first load builds the extension against the staged SDK.
                pump(90)
                assert states == ["idle"], f"startup state was {states!r}: {bytes(captured[-2000:])!r}"
                document = published()
                assert document, "no agent state document was published"
                for field in ("session_id", "session_file", "cwd", "pid", "updated_at"):
                    assert document.get(field), f"state document lacks {field}: {document!r}"
                assert document["mode"] == "tui", f"mode was {document.get('mode')!r}"
                # The document is refreshed while the session lives, so a reader
                # applying the staleness rule can always prove it is current. An
                # idle agent between turns is exactly the case that needs it:
                # without a refresh its timestamp ages with no state change to
                # explain it, and a consumer treats the agent as unknown.
                stamped = document["updated_at"]
                refreshed = pump_until(lambda: (published() or {}).get("updated_at") != stamped, 60)
                assert refreshed, f"the document was never refreshed: {published()!r}"

                os.write(master, b"hello\r")
                pump(20)
                assert states == ["idle", "working", "idle"], f"turn states were {states!r}"

                assert herdr_log.exists(), "the Herdr extension reported nothing"
                reports = [line for line in herdr_log.read_text().splitlines() if line.strip()]
            finally:
                if process.poll() is None:
                    process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                os.close(master)

            assert reports, "the Herdr extension reported nothing"
            reported = [line.split("--state ", 1)[1].split()[0] for line in reports if "--state " in line]
            for state in ("idle", "working"):
                assert state in reported, f"{state} never reached Herdr: {reported!r}"
            for line in reports:
                assert line.startswith("pane report-agent w1:p1"), line
                assert "--source custom:ompig-slim" in line, line
                assert "--agent ompig-slim" in line, line
            assert any("-- ompig-slim --session " in line for line in reports), \
                f"no report carried a resume command: {reports!r}"

            print(f"PASS: state document {states}, Herdr reports {reported}, "
                  f"resume command forwarded")
    finally:
        server.shutdown()


def main():
    """Resolve a caller-selected binary; the fixture never touches a real profile."""
    if len(sys.argv) != 2:
        print("usage: smoke-test-agent-state.py <binary>", file=sys.stderr)
        return 2
    run(Path(sys.argv[1]).resolve())
    return 0


if __name__ == "__main__":
    sys.exit(main())
