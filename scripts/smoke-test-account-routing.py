#!/usr/bin/env python3
"""Verify provider-wide session distribution and durable fallback with the real binary."""
import base64
import http.server
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import threading
import time


def main(binary, provider):
    calls = []
    exhausted = set()
    tokens = {}
    for key in ("a", "b"):
        claims = {"https://api.openai.com/auth": {
            "chatgpt_plan_type": "plus", "chatgpt_account_id": key,
        }}
        payload = base64.urlsafe_b64encode(json.dumps(claims).encode()).decode().rstrip("=")
        tokens[key] = f"e30.{payload}.fixture-{key}"

    class Provider(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            self.rfile.read(int(self.headers["Content-Length"]))
            key = next((key for key, token in tokens.items()
                        if self.headers.get("Authorization") == f"Bearer {token}"), None)
            calls.append(key)
            if key is None:
                self.send_error(401)
                return
            if key in exhausted:
                self.send_response(429)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b'{"error":{"message":"usage_limit_reached"}}')
                return
            events = [
                {"type": "response.created", "response": {"id": "fixture", "status": "in_progress"}},
                {"type": "response.output_item.added", "output_index": 0,
                 "item": {"type": "message", "id": "fixture-message", "role": "assistant", "content": []}},
                {"type": "response.content_part.added", "output_index": 0, "content_index": 0,
                 "part": {"type": "output_text", "text": ""}},
                {"type": "response.output_text.delta", "output_index": 0, "content_index": 0,
                 "delta": "Account routing fixture passed."},
                {"type": "response.completed", "response": {"id": "fixture", "status": "completed", "output": [],
                 "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}},
            ]
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            if provider != "codex-json":
                events = [
                    {"id": "fixture", "object": "chat.completion.chunk", "model": "session-fixture",
                     "choices": [{"index": 0, "delta": {"role": "assistant", "content": "Account routing fixture passed."}, "finish_reason": None}]},
                    {"id": "fixture", "object": "chat.completion.chunk", "model": "session-fixture",
                     "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}]},
                ]
            wire = "".join("data: " + json.dumps(event) + "\n\n" for event in events)
            if provider != "codex-json":
                wire += "data: [DONE]\n\n"
            self.wfile.write(wire.encode())

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix="omp-account-routing-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (agent / "models.json").write_text(json.dumps({"providers": {provider: {
                "api": "openai-codex-responses" if provider == "codex-json" else "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}",
                "models": [{"id": "session-fixture", "name": "Session fixture",
                            "contextWindow": 128000, "maxTokens": 1024}],
            }}}))
            (agent / "settings.json").write_text(json.dumps({
                "modelRoles": {"default": f"{provider}/session-fixture"},
                "retry": {"maxRetries": 0},
            }))
            env = dict(os.environ, PIG_HOME=directory, PI_HOME=directory,
                       PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
            for name in tuple(env):
                if name.startswith("OMPIG_BROKER_") or name in ("OPENAI_API_KEY", "CODEX_API_KEY", "MISTRAL_API_KEY"):
                    env.pop(name)
            for name in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[name] = str(agent)

            def run(*args):
                return subprocess.run([str(binary), *args], cwd=root, env=env,
                                      capture_output=True, text=True, timeout=45, check=True)

            run("auth", "accounts", "--json")
            db_path = agent / "agent.db"
            with sqlite3.connect(db_path) as db:
                for key in ("a", "b"):
                    credential = {"access": tokens[key], "accountId": key,
                                  "expires": int((time.time() + 3600) * 1000)}
                    kind = "oauth"
                    if provider == "fixture-accounts":
                        kind, credential = "api_key", {"key": tokens[key]}
                    db.execute("""INSERT INTO auth_credentials
                        (provider, credential_type, data, identity_key)
                        VALUES (?, ?, ?, ?)""", (provider, kind, json.dumps(credential), key))
            run("auth", "use", provider, "b")

            def prompt(session, resume=False):
                args = ["-ne", "-p", "--model", f"{provider}/session-fixture",
                        "--session-dir", str(root / session)]
                if resume:
                    args.append("--continue")
                result = run(*args, "Say hello.")
                assert "Account routing fixture passed." in result.stdout, result

            prompt("one")
            prompt("two")
            prompt("one", resume=True)
            prompt("three")
            assert calls == ["b", "a", "b", "b"], calls
            exhausted.add("b")
            prompt("one", resume=True)
            assert calls[-2:] == ["b", "a"], calls
            exhausted.clear()
            with sqlite3.connect(db_path) as db:
                db.execute("DELETE FROM auth_credential_blocks")
            prompt("one", resume=True)
            assert calls[-1] == "a", calls
            assert len(calls) == 7, calls
            print(f"PASS: {provider} sessions alternate accounts, resume keeps assignments, and quota fallback stays sticky after restart and block reset")
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    for provider in ("codex-json", "mistral", "fixture-accounts"):
        main(Path(sys.argv[1]).resolve(), provider)
