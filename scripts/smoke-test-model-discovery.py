#!/usr/bin/env python3
"""Verify live Codex model discovery with isolated profiles and a loopback provider."""
import base64
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import sqlite3
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time


def run(binary, provider):
    state = {"model": "future-discovery-first", "mode": "ok", "account": "first", "rosters": {}}
    requests = []
    post_accounts = []
    tokens = {}
    for account in ("first", "second"):
        claims = {"https://api.openai.com/auth": {
            "chatgpt_plan_type": "plus", "chatgpt_account_id": account,
        }}
        payload = base64.urlsafe_b64encode(json.dumps(claims).encode()).decode().rstrip("=")
        tokens[account] = f"e30.{payload}.discovery-fixture-{account}"

    class Provider(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_GET(self):
            if self.path.split("?", 1)[0] != "/codex/models" or "client_version=" not in self.path:
                self.send_error(404)
                return
            account = self.headers.get("ChatGPT-Account-Id")
            if (account not in tokens or self.headers.get("originator") != "codex_cli_rs"
                    or self.headers.get("Authorization") != f"Bearer {tokens[account]}"):
                self.send_error(401)
                return
            requests.append((account, self.path))
            if state["mode"] == "unavailable":
                self.send_error(503)
                return
            model = state["rosters"].get(account, state["model"])
            models = [{
                "slug": model, "display_name": model,
                "description": "Loopback discovery fixture", "visibility": "list",
                "supported_in_api": False, "context_window": 256000,
                "input_modalities": ["text", "image"],
                "supported_reasoning_levels": [{"effort": "low"}, {"effort": "high"}],
                "default_reasoning_level": "high",
            }]
            if state["mode"] == "empty":
                models = []
            body = b'{"models":' if state["mode"] == "malformed" else json.dumps({"models": models}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            self.rfile.read(int(self.headers["Content-Length"]))
            account = state["account"]
            post_accounts.append(self.headers.get("ChatGPT-Account-Id"))
            if (self.path != "/codex/responses" or self.headers.get("ChatGPT-Account-Id") != account
                    or self.headers.get("Authorization") != f"Bearer {tokens[account]}"):
                self.send_error(401)
                return
            events = [
                {"type": "response.created", "response": {"id": "fixture", "status": "in_progress"}},
                {"type": "response.output_item.added", "output_index": 0,
                 "item": {"type": "message", "id": "fixture-message", "role": "assistant", "content": []}},
                {"type": "response.content_part.added", "output_index": 0, "content_index": 0,
                 "part": {"type": "output_text", "text": ""}},
                {"type": "response.output_text.delta", "output_index": 0, "content_index": 0,
                 "delta": "Live discovery fixture passed."},
                {"type": "response.completed", "response": {"id": "fixture", "status": "completed", "output": [],
                 "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}},
            ]
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            self.wfile.write("".join("data: " + json.dumps(event) + "\n\n" for event in events).encode())

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix="omp-model-discovery-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            env = {
                "PATH": os.environ["PATH"], "HOME": directory,
                "PIG_HOME": directory, "PI_HOME": directory,
                "PI_SKIP_VERSION_CHECK": "1", "PI_TELEMETRY": "0",
                "HTTP_PROXY": "http://127.0.0.1:1", "HTTPS_PROXY": "http://127.0.0.1:1",
                "NO_PROXY": "127.0.0.1,localhost", "TERM": "xterm-256color",
            }
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)
            (agent / "models.json").write_text(json.dumps({"providers": {provider: {
                "api": "openai-codex-responses",
                "baseUrl": f"http://127.0.0.1:{server.server_port}",
                "models": [{"id": "explicit-only", "name": "Explicit fixture", "contextWindow": 64000, "maxTokens": 4096}],
            }}}))
            (agent / "settings.json").write_text(json.dumps({
                "quietStartup": True, "retry": {"maxRetries": 0},
            }))

            def command(*args):
                result = subprocess.run([str(binary), *args], cwd=root, env=env,
                                        capture_output=True, text=True, timeout=45)
                assert result.returncode == 0, f"{provider}: {args}: {result.stderr}"
                for token in tokens.values():
                    assert token not in result.stdout + result.stderr, "credential appeared in command output"
                return result.stdout

            def model_ids(output):
                rows = (line.split() for line in output.splitlines())
                return {row[1] for row in rows if len(row) > 1 and row[0] == provider}

            command("auth", "accounts", "--json")

            def add_account(account):
                with sqlite3.connect(agent / "agent.db") as db:
                    db.execute("INSERT INTO auth_credentials (provider, credential_type, data, identity_key) VALUES (?, ?, ?, ?)",
                               (provider, "oauth", json.dumps({"access": tokens[account], "accountId": account,
                                "expires": int((time.time() + 3600) * 1000)}), account))

            add_account("first")
            listed = command("--list-models", "future-discovery")
            assert state["model"] in model_ids(listed), f"{provider}: online listing missed new model: {listed}"
            assert "256K" in listed, f"{provider}: provider context limit was lost: {listed}"
            assert requests, f"{provider}: listing did not contact the provider"

            previous = state["model"]
            state["model"] = "future-discovery-second"
            listed = command("--list-models", "future-discovery")
            assert state["model"] in model_ids(listed) and previous not in model_ids(listed), "a successful roster did not replace old membership"
            for failure in ("malformed", "unavailable"):
                state["mode"] = failure
                assert state["model"] in model_ids(command("--list-models", "future-discovery")), "discovery failure erased the last good roster"
            state["mode"] = "ok"
            before = len(requests)
            assert state["model"] in model_ids(command("--offline", "--list-models", "future-discovery"))
            assert len(requests) == before, "offline listing contacted the provider"
            answer = command("--offline", "-ne", "-p", "--model", f"{provider}/{state['model']}", "Say hello.")
            assert "Live discovery fixture passed." in answer, "discovered model could not run after an offline restart"
            assert "explicit-only" in model_ids(command("--offline", "--list-models", "explicit-only")), "discovery removed an explicit custom model"

            add_account("second")
            command("auth", "use", provider, "first")
            other_model = "future-discovery-second-account"
            state["rosters"]["second"] = other_model
            listed = command("--list-models", "future-discovery")
            assert state["model"] in model_ids(listed) and other_model in model_ids(listed), "the model list did not include both stored accounts"
            state["account"] = "second"
            before_posts = len(post_accounts)
            answer = command("--offline", "-ne", "-p", "--model", f"{provider}/{other_model}", "Say hello.")
            assert "Live discovery fixture passed." in answer
            assert post_accounts[before_posts:] == ["second"], "a new model was probed with an account that does not advertise it"

            command("auth", "logout", provider, "--account", "first")
            state["rosters"].clear()
            assert state["model"] not in model_ids(command("--offline", "--list-models", "future-discovery")), "another account restored a private roster"
            state["model"] = "future-discovery-other-account"
            assert state["model"] in model_ids(command("--list-models", "future-discovery"))
            assert requests[-1][0] == "second", "discovery used the wrong account"
            state["mode"] = "empty"
            assert state["model"] not in model_ids(command("--list-models", "future-discovery")), "an empty successful roster restored stale membership"
            assert "explicit-only" in model_ids(command("--list-models", "explicit-only")), "an empty native roster removed an explicit custom model"
            state["mode"] = "ok"

            if provider == "codex-json":
                state["model"] = "future-discovery-interactive"
                interactive(binary, root, env, provider, state["model"])

            for snapshot in agent.glob("models*.json"):
                for token in tokens.values():
                    assert token not in snapshot.read_text(), f"credential appeared in {snapshot.name}"
            print(f"PASS [{provider}]: automatic discovery, roster replacement, failed-refresh retention, offline restart and inference, custom models, pooled-account routing, account isolation, empty roster")
    finally:
        server.shutdown()
        server.server_close()


def interactive(binary, root, env, provider, model):
    env = dict(env, PIG_TEST_FAUX="1", PIG_STARTUP_TRACE="1")
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 140, 0, 0))
    process = subprocess.Popen([str(binary), "-ne", "--model", "test-faux/model"],
                               cwd=root, env=env, stdin=slave, stdout=slave,
                               stderr=slave, start_new_session=True)
    os.close(slave)
    captured = bytearray()

    def wait_for(predicate, label):
        end = time.monotonic() + 25
        while time.monotonic() < end:
            if select.select([master], [], [], 0.05)[0]:
                try:
                    captured.extend(os.read(master, 65536))
                except OSError:
                    break
            visible = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", bytes(captured))
            if predicate(visible):
                return
            if process.poll() is not None:
                break
        raise AssertionError(f"{label} did not settle; exit={process.poll()}; terminal={bytes(captured[-4000:])!r}")

    try:
        wait_for(lambda output: b"interactive-ready" in output, "interactive startup")
        os.write(master, b"\x1bp")
        wait_for(lambda output: b"Switch Model" in output, "model picker")
        os.write(master, model.encode())
        wait_for(lambda output: model.encode() in output,
                 "discovered model in picker search")
        os.write(master, b"\x1b")
        wait_for(lambda output: b"interactive-ready" in output, "picker close")
        print(f"PASS [{provider}]: interactive background discovery and Alt+P model search")
    finally:
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        os.close(master)


if __name__ == "__main__":
    binary = Path(sys.argv[1] if len(sys.argv) > 1 else "bin/ompig-slim").resolve()
    for provider in ("openai-codex", "codex-json"):
        run(binary, provider)
