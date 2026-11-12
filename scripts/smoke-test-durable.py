#!/usr/bin/env python3
"""Exercise the shipped durable CLI with a loopback model and a killed process.

All configuration, database files and workspaces are temporary. No external
credentials, inference services, or Kubernetes cluster are required.
"""

import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def run(binary):
    """Kill during a model call, resume, and prove completed admission deduplication."""
    reached = threading.Event()
    release = threading.Event()
    calls = []

    class Handler(BaseHTTPRequestHandler):
        """Implement only the OpenAI streaming endpoint used by the fixture model."""

        def log_message(self, *_):
            """Keep successful smoke output deterministic and free of HTTP logs."""

        def do_POST(self):
            """Hold the first request at the crash point and answer its recovery."""
            body = self.rfile.read(int(self.headers["Content-Length"]))
            calls.append(json.loads(body))
            if len(calls) == 1:
                reached.set()
                release.wait(30)
                return
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            chunk = {"choices": [{"index": 0, "delta": {
                "role": "assistant", "content": "Recovered workdesk"
            }, "finish_reason": None}]}
            self.wfile.write(("data: " + json.dumps(chunk) + "\n\n" +
                              'data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\n' +
                              "data: [DONE]\n\n").encode())

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-durable-") as directory:
            root = Path(directory)
            profile, workspace, state = root / "profile", root / "repo", root / "state"
            profile.mkdir()
            workspace.mkdir()
            (workspace / "preserved.txt").write_text("uncommitted work\n")
            (profile / "models.json").write_text(json.dumps({"providers": {"desk-fixture": {
                "api": "openai-completions",
                "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "fixture-key",
                "models": [{"id": "fixture", "name": "fixture", "contextWindow": 128000, "maxTokens": 1024}],
            }}}))
            env = {**os.environ, "PIG_HOME": str(root / "home"),
                   "OMPIG_SLIM_CODING_AGENT_DIR": str(profile), "PI_OFFLINE": "1"}
            # Do not inherit a live broker connection or bootstrap secret.
            for key in list(env):
                if key.startswith("OMPIG_BROKER_"):
                    del env[key]
            command = [binary, "durable", "run", "--state", str(state), "--cwd", str(workspace),
                       "--model", "desk-fixture/fixture", "--request-id", "smoke-1", "Finish the work"]
            with subprocess.Popen(command, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE) as process:
                try:
                    assert reached.wait(20), "workdesk never reached model request"
                    process.send_signal(signal.SIGKILL)
                    process.communicate(timeout=10)
                finally:
                    release.set()
                    if process.poll() is None:
                        process.kill()
                        process.communicate(timeout=10)
            for args in ([binary, "durable", "resume", "--state", str(state)], command,
                         [binary, "durable", "status", "--state", str(state)]):
                result = subprocess.run(args, env=env, capture_output=True, text=True, timeout=25)
                assert result.returncode == 0, result.stderr
                receipt = json.loads(result.stdout)
                assert receipt["status"] == "done", receipt
                assert receipt["answer"] == "Recovered workdesk", receipt
            assert len(calls) == 2, f"duplicate work: {len(calls)} requests"
            assert (workspace / "preserved.txt").read_text() == "uncommitted work\n"
            version = subprocess.check_output([binary, "--version"], env=env, text=True, timeout=10).strip()
            assert version and version[0].isdigit(), version
            print("durable CLI: SIGKILL recovery, deduplication, status and daily CLI passed")
    finally:
        release.set()
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    run(str(Path(sys.argv[1]).resolve()))
