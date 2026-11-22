#!/usr/bin/env python3
"""Check that /model-author rewrites the agent's real git commits, through the RPC client."""
import http.server
import json
import os
from pathlib import Path
import queue
import subprocess
import sys
import tempfile
import threading
import time

CLAUDE = "Claude Opus 5.5 <noreply@anthropic.com>"
TRAILER = "Co-Authored-By: " + CLAUDE


def main(binary):
    commit = "echo {n} >> f && git add f && "
    script = [
        commit.format(n=1) + 'git commit -q -m "one"',
        commit.format(n=2) + 'git commit -q -m "two"',
        commit.format(n=3) + "git commit -q -F - <<'EOF'\nthree \"quoted\"\n\ngit commit -m inner\nEOF",
        "git commit -q --amend --no-edit",
        commit.format(n=5) + "git commit -q --author='Someone <someone@example.com>' -m five",
        commit.format(n=6) + 'git commit -q -m "six"',
        commit.format(n=7) + 'git commit -q -m "seven"',
    ]
    calls = []

    def sse(*chunks):
        return ("".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n").encode()

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            calls.append(body)
            if body["messages"][-1]["role"] == "tool":
                delta, finish = {"role": "assistant", "content": "Done"}, "stop"
            else:
                command = script[sum(1 for c in calls if c["messages"][-1]["role"] != "tool") - 1]
                delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": "call_%d" % len(calls), "type": "function",
                                                              "function": {"name": "bash", "arguments": json.dumps({"command": command})}}]}
                finish = "tool_calls"
            wire = sse({"id": "f", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
                       {"id": "f", "choices": [{"index": 0, "delta": {}, "finish_reason": finish}]})
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            self.wfile.write(wire)

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-model-author-") as directory:
            root = Path(directory)
            agent = root / "agent"
            work = root / "work"
            agent.mkdir()
            work.mkdir()
            for step in (["init", "-q"], ["config", "user.name", "Human"], ["config", "user.email", "human@example.com"]):
                subprocess.run(["git", *step], cwd=work, check=True)
            (agent / "models.json").write_text(json.dumps({"providers": {
                provider: {"api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                           "apiKey": "fixture-key", "models": [
                               {"id": model, "name": name, "contextWindow": 128000, "maxTokens": 1024}
                           ]}
                for provider, model, name in (("first", "claude-opus-5-5", "Claude Opus 5.5"), ("second", "deepseek-v4", "DeepSeek V4"))
            }}))
            env = dict(os.environ, PIG_HOME=directory, PI_HOME=directory, PI_OFFLINE="1",
                       PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)
            for key in list(env):
                if key.startswith("OMPIG_BROKER_"):
                    del env[key]
            process = subprocess.Popen([str(binary), "--mode", "rpc", "--model", "first/claude-opus-5-5"],
                                       cwd=work, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
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

            def toggle(argument):
                command("prompt", message="/model-author " + argument)
                time.sleep(1.5)

            try:
                prompt("commit one")
                toggle("on")
                prompt("commit two")
                prompt("commit three")
                prompt("amend")
                prompt("commit five")
                toggle("off")
                prompt("commit six")
                toggle("on")
                command("set_model", provider="second", modelId="deepseek-v4")
                response = wait_for(lambda event: event.get("type") == "response" and event.get("command") == "set_model")
                assert response.get("success"), response
                prompt("commit seven")
                assert len(script) == sum(1 for c in calls if c["messages"][-1]["role"] != "tool"), "every scripted command ran"
                log = subprocess.run(["git", "log", "--reverse", "--format=%an <%ae>%x1f%B%x1e"], cwd=work,
                                     check=True, capture_output=True, text=True).stdout
                commits = [entry.strip("\n").split("\x1f") for entry in log.split("\x1e") if entry.strip()]
                assert len(commits) == 6, commits
                human = "Human <human@example.com>"
                (a1, m1), (a2, m2), (a3, m3), (a5, m5), (a6, m6), (a7, m7) = commits
                assert a1 == human and TRAILER not in m1, commits[0]
                assert a2 == CLAUDE and m2.count(TRAILER) == 1, commits[1]
                assert a3 == CLAUDE and m3.count(TRAILER) == 1 and 'three "quoted"' in m3 and "git commit -m inner" in m3, commits[2]
                assert a5 == "Someone <someone@example.com>" and m5.count(TRAILER) == 1, commits[3]
                assert a6 == human and TRAILER not in m6, commits[4]
                assert a7 == human and TRAILER not in m7, commits[5]
                print("PASS: /model-author rewrites real git commits by model ID and stays off otherwise")
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
