#!/usr/bin/env python3
"""Verify compaction recovery through the real binary and loopback providers."""
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

    class Model(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            model = body["model"]
            calls.append(model)
            if model in ("primary", "second"):
                self.send_response(429)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b'{"error":{"message":"billing quota exceeded","type":"insufficient_quota"}}')
                return
            chunks = [
                {"id": "fixture", "object": "chat.completion.chunk", "model": model,
                 "choices": [{"index": 0, "delta": {"role": "assistant", "content": "Summary recovered."}, "finish_reason": None}]},
                {"id": "fixture", "object": "chat.completion.chunk", "model": model,
                 "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}]},
            ]
            wire = "".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n"
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            self.wfile.write(wire.encode())

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Model)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix="omp-compact-fallback-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (agent / "models.json").write_text(json.dumps({"providers": {
                "fixture": {"api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                            "apiKey": "fixture-key", "models": [
                                {"id": model, "name": model, "contextWindow": 128000, "maxTokens": 1024}
                                for model in ("chat", "primary", "second", "third")
                            ]}
            }}))
            (agent / "settings.json").write_text(json.dumps({
                "modelRoles": {"default": "fixture/chat", "compact": "fixture/primary"},
                "compaction": {"keepRecentTokens": 100, "reserveTokens": 1024},
                "retry": {"maxRetries": 0, "fallbackChains": {"compact": ["fixture/second:off", "fixture/third:off"]}},
            }))
            env = dict(os.environ, PIG_HOME=directory, PI_HOME=directory,
                       PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)
            process = subprocess.Popen([str(binary), "-ne", "--mode", "rpc", "--model", "fixture/chat"],
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
                    if predicate(event):
                        return event

            try:
                for index in range(2):
                    command("prompt", message=f"Remember fixture turn {index}. " + "context " * 100)
                    wait_for(lambda event: event.get("type") == "agent_end")
                command("get_state")
                before = wait_for(lambda event: event.get("type") == "response" and event.get("command") == "get_state")
                command("compact")
                result = wait_for(lambda event: event.get("type") == "response" and event.get("command") == "compact")
                assert result.get("success"), f"compaction failed: {result}; requests={calls}"
                assert calls == ["chat", "chat", "primary", "second", "third"], calls
                command("get_state")
                state = wait_for(lambda event: event.get("type") == "response" and event.get("command") == "get_state")
                assert state["data"]["model"]["id"] == "chat", state
                for key in ("model", "modelRole", "thinkingLevel"):
                    assert state["data"].get(key) == before["data"].get(key), (key, before, state)
                print("PASS: RPC compaction recovered primary -> second -> third without changing the chat model")
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


if __name__ == "__main__":
    main(Path(sys.argv[1]).resolve())
