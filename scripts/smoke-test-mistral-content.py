#!/usr/bin/env python3
"""Verify Mistral multipart streaming in the built CLI without real credentials."""
import http.server
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading


class ModelServer(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        self.rfile.read(int(self.headers["Content-Length"]))
        chunks = [
            {"choices": [{"delta": {"content": [{"type": "thinking", "thinking": [
                {"type": "text", "text": "Fixture reasoning."}]}]}}]},
            {"choices": [{"delta": {"content": [{"type": "text", "text": "MISTRAL_"}]}}]},
            {"choices": [{"delta": {"content": [{"type": "text", "text": "CONTENT_OK é"}]},
                          "finish_reason": "stop"}]},
        ]
        wire = ("".join("data: " + json.dumps(c) + "\n\n" for c in chunks)
                + "data: [DONE]\n\n").encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire)))
        self.end_headers()
        self.wfile.write(wire)


def main():
    binary = Path(sys.argv[1]).resolve()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-mistral-content-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (agent / "models.json").write_text(json.dumps({"providers": {"mistral": {
                "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "fixture", "models": [{"id": "content-proof", "reasoning": True}]}}}))
            env = dict(os.environ, HOME=directory, PIG_HOME=directory, PI_HOME=directory,
                       PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)
            result = subprocess.run(
                [str(binary), "-p", "Say hello.", "--provider", "mistral", "--model", "content-proof",
                 "--no-extensions", "--no-context-files", "--no-skills", "--offline"],
                cwd=root, env=env, capture_output=True, text=True, timeout=60)
            assert result.returncode == 0, (result.stdout, result.stderr)
            assert "MISTRAL_CONTENT_OK é" in result.stdout, result.stdout
            assert "[object Object]" not in result.stdout, result.stdout
            messages = []
            for session in (agent / "sessions").rglob("*.jsonl"):
                for line in session.read_text().splitlines():
                    message = json.loads(line).get("message", {})
                    if message.get("role") == "assistant":
                        messages.append(message)
            assert messages, "no persisted assistant response"
            content = messages[-1]["content"]
            assert "".join(b.get("text", "") for b in content) == "MISTRAL_CONTENT_OK é", content
            assert "".join(b.get("thinking", "") for b in content) == "Fixture reasoning.", content
            print("PASS: Mistral multipart text streams to stdout and persists separately from thinking")
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    main()
