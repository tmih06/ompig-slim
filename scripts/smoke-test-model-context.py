#!/usr/bin/env python3
"""Check model identity and cache-stable history through the real RPC client."""
import http.server
import json
import os
from pathlib import Path
import queue
import subprocess
import sys
import tempfile
import threading


def main(binary):
    calls = []

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            calls.append(body)
            chunks = [
                {"id": "fixture", "choices": [{"index": 0, "delta": {"role": "assistant", "content": "Fixture response"}, "finish_reason": None}]},
                {"id": "fixture", "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}]},
            ]
            wire = ("".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n").encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            self.wfile.write(wire)

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-model-context-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (agent / "models.json").write_text(json.dumps({"providers": {
                provider: {"api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                           "apiKey": "fixture-key", "models": [
                               {"id": model, "name": model, "contextWindow": 128000, "maxTokens": 1024}
                           ]}
                for provider, model in (("first", "alpha"), ("second", "beta"))
            }}))
            env = dict(os.environ, PIG_HOME=directory, PI_HOME=directory, PI_OFFLINE="1",
                       PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)
            for key in list(env):
                if key.startswith("OMPIG_BROKER_"):
                    del env[key]
            process = subprocess.Popen([str(binary), "--mode", "rpc", "--model", "first/alpha"],
                                       cwd=root, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                       stderr=subprocess.PIPE, text=True)
            events = queue.Queue()

            def read_events():
                for line in process.stdout:
                    try:
                        events.put(json.loads(line))
                    except json.JSONDecodeError:
                        pass

            threading.Thread(target=read_events, daemon=True).start()

            def command(kind, **fields):
                process.stdin.write(json.dumps({"id": kind, "type": kind, **fields}) + "\n")
                process.stdin.flush()

            def wait_for(predicate):
                while True:
                    event = events.get(timeout=30)
                    if event.get("type") == "extension_error":
                        raise AssertionError(event)
                    if predicate(event):
                        return event

            def prompt(text):
                command("prompt", message=text)
                wait_for(lambda event: event.get("type") == "agent_end")

            try:
                prompt("First turn")
                prompt("Second turn")
                command("set_model", provider="second", modelId="beta")
                response = wait_for(lambda event: event.get("type") == "response" and event.get("command") == "set_model")
                assert response.get("success"), response
                prompt("After switch")
                prompt("Same model again")
                command("set_model", provider="first", modelId="alpha")
                response = wait_for(lambda event: event.get("type") == "response" and event.get("command") == "set_model")
                assert response.get("success"), response
                prompt("Switch back")
                assert len(calls) == 5, calls
                messages = [call["messages"] for call in calls]
                first_identity = "Selected provider: first\nSelected model: alpha"
                second_identity = "Selected provider: second\nSelected model: beta"
                assert first_identity in json.dumps(messages[0]).replace("\\n", "\n"), messages[0]
                assert messages[1][:len(messages[0])] == messages[0], messages[:2]
                assert messages[2][:len(messages[1])] == messages[1], messages[1:3]
                assert second_identity in json.dumps(messages[2]).replace("\\n", "\n"), messages[2]
                assert messages[3][:len(messages[2])] == messages[2], messages[2:4]
                assert json.dumps(messages[3]).count("Selected provider: second") == 1, messages[3]
                assert messages[4][:len(messages[3])] == messages[3], messages[3:]
                assert json.dumps(messages[4]).count("Selected provider: first") == 2, messages[4]
                print("PASS: model identity follows RPC switches without rewriting the cached message prefix")
            finally:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main(Path(sys.argv[1]).resolve())
