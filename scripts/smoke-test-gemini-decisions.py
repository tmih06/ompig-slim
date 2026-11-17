#!/usr/bin/env python3
"""Exercise Gemini Decisions through codemode and the built binary."""
import http.server
import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile
import threading


class DecisionServer(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    requests = 0
    failure = ""

    def log_message(self, *_):
        pass

    def do_POST(self):
        cls = type(self)
        cls.requests += 1
        try:
            body = self.rfile.read(int(self.headers["Content-Length"]))
            payload = json.loads(body)
            if self.path != "/v1beta/decisions":
                raise AssertionError("unexpected Decisions path: " + self.path)
            if self.headers.get("x-goog-api-key") != "decision-fixture-only" or self.headers.get("Authorization"):
                raise AssertionError("Decisions request used unexpected authentication")
            if b"decision-fixture-only" in body:
                raise AssertionError("API key appeared in the Decisions body")
            if payload.get("model") != "gemini-decision-1":
                raise AssertionError("unexpected model: " + repr(payload.get("model")))
            if payload.get("input") != '{"answer":"4","expected":"4"}':
                raise AssertionError("unexpected state input: " + repr(payload.get("input")))
            question = payload.get("questions", {}).get("smoke", {})
            if (
                question.get("type") != "noul"
                or question.get("instructions") != "Does the answer match?"
                or question.get("criteria") != {"true_description": "Matches", "false_description": "Does not match"}
            ):
                raise AssertionError("unexpected question: " + repr(question))
            result = {"answers": {"smoke": {"type": "noul", "noul": 0.97}}, "usage": {"total_input_tokens": 4, "total_output_tokens": 0}}
            wire = json.dumps(result).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(wire)))
            self.end_headers()
            self.wfile.write(wire)
        except Exception as error:
            cls.failure = str(error)
            wire = json.dumps({"error": {"message": cls.failure}}).encode()
            self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(wire)))
            self.end_headers()
            self.wfile.write(wire)


class ModelServer(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    stage = 0
    failure = ""

    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        if not self.path.endswith("/chat/completions"):
            self.send_error(404)
            return
        cls = type(self)
        offered = {item["function"]["name"] for item in body.get("tools", [])}
        result = next((m.get("content", "") for m in reversed(body["messages"]) if m["role"] == "tool"), "")
        result_text = result if isinstance(result, str) else json.dumps(result)
        if cls.stage == 0 and offered != {"codemode"}:
            cls.failure = "codemode was not the only declared tool: " + repr(offered)
        if cls.stage == 1 and (not re.search(r"(?:^|\n)16(?:\n|$)", result_text) or "classifier result" not in result_text):
            cls.failure = "codemode did not return arithmetic and classifier results: " + result_text
        if cls.stage == 2:
            delta = {"role": "assistant", "content": "GEMINI_CODEMODE_OK" if not cls.failure else "GEMINI_CODEMODE_FAILED"}
            reason = "stop"
        elif cls.failure:
            delta = {"role": "assistant", "content": "GEMINI_CODEMODE_FAILED"}
            reason = "stop"
        else:
            if cls.stage == 0:
                code = '''const classifiers = await models.getModelsOfType('classifier', 'google');
const judge = await models.getModelOfType('classifier', 'google', 'gemini-decision-1');
if (!judge) throw new Error(`Gemini Decision 1 is missing from the classifier registry: ${JSON.stringify(classifiers)}`);
if (judge.provider !== 'google' || judge.id !== 'gemini-decision-1') throw new Error(`invalid classifier model object: ${JSON.stringify(judge)}`);
const judged = await models.classify(judge, {state: {answer: '4', expected: '4'}, questions: {smoke: {type: 'bool', instructions: 'Does the answer match?', criteria: {true: 'Matches', false: 'Does not match'}}}});
if (judged.stopReason !== 'stop' || judged.answers.smoke.probability !== 0.97) throw new Error('classifier result was not usable');
store('classifier result', judged.answers.smoke.probability);
const sum = [3, 5, 8].reduce((a, b) => a + b, 0); store('sum', sum); return `classifier result ${load('classifier result')}\\n${sum}`;'''
            else:
                code = '''const sum = load('sum'); if (sum !== 16) throw new Error('lost session state');
if (load('classifier result') !== 0.97) throw new Error('classifier result was not retained');
return `classifier result ${load('classifier result')}\\n${sum * 2}`;'''
            delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": f"call_{cls.stage}", "type": "function", "function": {"name": "codemode", "arguments": json.dumps({"code": code})}}]}
            reason = "tool_calls"
        cls.stage += 1
        chunks = [
            {"id": "gemini-smoke", "object": "chat.completion.chunk", "created": 0, "model": "codemode-smoke", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "gemini-smoke", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n"
        encoded = wire.encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)
        self.wfile.flush()


def main():
    if len(sys.argv) != 2:
        raise SystemExit("Usage: python3 scripts/smoke-test-gemini-decisions.py <ompig-slim-binary>")
    binary = pathlib.Path(sys.argv[1]).resolve()
    with tempfile.TemporaryDirectory(prefix="ompig-gemini-decisions-smoke-") as directory:
        root = pathlib.Path(directory)
        agent = root / "agent"
        agent.mkdir()
        decision_server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), DecisionServer)
        model_server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
        threads = [threading.Thread(target=server.serve_forever, daemon=True) for server in (decision_server, model_server)]
        for thread in threads:
            thread.start()
        try:
            models = {
                "providers": {
                    "codemode-smoke": {
                        "api": "openai-completions",
                        "baseUrl": f"http://127.0.0.1:{model_server.server_port}/v1",
                        "apiKey": "local-fixture-only",
                        "models": [{"id": "codemode-smoke"}],
                    },
                    "google": {
                        "baseUrl": f"http://127.0.0.1:{decision_server.server_port}/v1beta",
                        "apiKey": "decision-fixture-only",
                    },
                }
            }
            (agent / "models.json").write_text(json.dumps(models))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True, "enableInstallTelemetry": False, "extensionsAutoUpgrade": False, "codemode": {"mode": "only", "inlineBudget": 0}}))
            env = dict(os.environ, HOME=str(root), PIG_HOME=str(root / "library"), OMPIG_SLIM_CODING_AGENT_DIR=str(agent), PI_OFFLINE="1", PI_TELEMETRY="0", PI_SKIP_VERSION_CHECK="1")
            for name in list(env):
                if name.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN")):
                    del env[name]
            ModelServer.stage, ModelServer.failure = 0, ""
            DecisionServer.failure = ""
            initial_decision_requests = DecisionServer.requests
            command = [str(binary), "--provider", "codemode-smoke", "--model", "codemode-smoke", "--no-extensions", "--print", "--no-session", "--offline", "--no-context-files", "--no-skills", "--no-prompt-templates", "--no-themes", "Exercise native Gemini classification in codemode."]
            result = subprocess.run(command, cwd=root, env=env, stdin=subprocess.DEVNULL, text=True, capture_output=True, timeout=30)
            if result.returncode or ModelServer.failure or DecisionServer.failure or "GEMINI_CODEMODE_OK" not in result.stdout:
                raise RuntimeError(f"codemode Gemini classification: exit {result.returncode}\nmodel server: {ModelServer.failure}\nDecisions server: {DecisionServer.failure}\nstdout:\n{result.stdout}\nstderr:\n{result.stderr}")
            if ModelServer.stage != 3 or DecisionServer.requests != initial_decision_requests + 1:
                raise RuntimeError(f"unexpected model calls: chat stages={ModelServer.stage}, Decisions requests={DecisionServer.requests - initial_decision_requests}")
            print("codemode Gemini Decisions: registry lookup, Google API-key auth, answer handling, and nested model call")
        finally:
            for server in (decision_server, model_server):
                server.shutdown()
                server.server_close()
            for thread in threads:
                thread.join()


if __name__ == "__main__":
    main()
