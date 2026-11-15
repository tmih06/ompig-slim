#!/usr/bin/env python3
"""Prove tool-contract recovery through a loopback model and the built binary."""
import http.server
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading


SOURCE = b'package demo\r\nfunc alpha() {\r\n  call("old")\r\n}\r\nfunc beta() {\r\n  call("old")\r\n}\r\n'


def call(name, **arguments):
    return {"name": name, "arguments": json.dumps(arguments)}


def script(code):
    return call("codemode", code=code)


STAGES = [
    (script('await tools.todo({op: "view"});'), ["todo", "direct"], ["searchTools"]),
    (call("todo", op="init", list=[{"phase": "Fix", "items": ["Verify recovery"]}]), ["Verify recovery"], []),
    (call("todo", op="done", phase="Fix", task="wrong task"), ["Verify recovery"], []),
    (call("todo", op="done", phase="Fix", task="Verify recovery"), ["Verify recovery"], []),
    (call("grep", pattern='call("', path="source.go"), ["regex parse error", "literal"], []),
    (call("grep", pattern='call("', path="source.go", literal=True), ["source.go:3:", "source.go:6:"], []),
    (script('text(await tools.grep({pattern: "func (alpha|beta)", path: "source.go"}));'), ["func alpha", "func beta"], []),
    (call("edit", path="source.go", edits=[{"oldText": "package demo", "newText": "package changed"}, {"oldText": 'func alpha() {\n  missing()\n}', "newText": "replacement"}]), ["edits[1]", "source.go:2", "candidate"], []),
    (script('await tools.edit({path: "source.go", edits: [{oldText: \'call("old")\', newText: "replacement"}]});'), ["occurrences", "source.go:3", "source.go:6"], []),
    (call("edit", path="source.go", edits=[{"oldText": "func alpha() {", "newText": "replacement"}, {"oldText": "alpha()", "newText": "other"}]), ["edits[0]", "edits[1]", "overlap"], []),
    (call("edit", path="source.go", edits=[{"oldText": "nothing remotely similar", "newText": "replacement"}]), ["No credible candidate"], []),
    (script('text(await tools.edit({path: "source.go", edits: [{oldText: \'func alpha() {\\n  call("old")\\n}\', newText: \'func alpha() {\\n  call("new")\\n}\'}]}));'), ["Successfully replaced"], []),
    (call("todo", op="view"), ["Verify recovery"], []),
]


class ModelServer(http.server.BaseHTTPRequestHandler):
    stage = 0
    failure = ""
    results = []
    budget = None
    root = None

    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        cls = type(self)
        try:
            offered = {item["function"]["name"]: item["function"] for item in body.get("tools", [])}
            guidance = offered["codemode"]["description"]
            assert "todo" in guidance and "direct-only" in guidance, "codemode guidance omits direct-only todo"
            assert "verbatim" in offered["todo"]["description"], "todo lost verbatim task guidance"
            if cls.stage:
                result = next(m for m in reversed(body["messages"]) if m["role"] == "tool")
                value = result.get("content", "")
                text = value if isinstance(value, str) else json.dumps(value)
                cls.results.append(text)
                _, required, forbidden = STAGES[cls.stage - 1]
                for phrase in required:
                    assert phrase.lower() in text.lower(), f"stage {cls.stage - 1} missing {phrase!r}: {text}"
                for phrase in forbidden:
                    assert phrase not in text, f"stage {cls.stage - 1} misleading {phrase!r}: {text}"
                if 8 <= cls.stage <= 11:
                    assert (cls.root / "source.go").read_bytes() == SOURCE, "rejected edit changed file bytes"
                if cls.stage == 12:
                    assert (cls.root / "source.go").read_bytes() == SOURCE.replace(b'  call("old")', b'  call("new")', 1), "corrected edit differs"
        except (AssertionError, KeyError, StopIteration) as error:
            cls.failure = str(error)

        if cls.failure or cls.stage >= len(STAGES):
            delta = {"role": "assistant", "content": "RECOVERY_FAILED" if cls.failure else "RECOVERY_OK"}
            reason = "stop"
        else:
            delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": f"recovery-{cls.stage}", "type": "function", "function": STAGES[cls.stage][0]}]}
            reason = "tool_calls"
        cls.stage += 1
        chunks = [
            {"id": "recovery", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "recovery", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())


def main():
    binary = Path(sys.argv[1]).resolve()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        for budget in (None, 0):
            with tempfile.TemporaryDirectory(prefix="ompig-recovery-") as directory:
                root = Path(directory)
                agent = root / "agent"
                agent.mkdir()
                (root / "source.go").write_bytes(SOURCE)
                (agent / "models.json").write_text(json.dumps({"providers": {"recovery": {
                    "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                    "apiKey": "local-fixture-only", "models": [{"id": "recovery"}]}}}))
                settings = {"quietStartup": True, "enableInstallTelemetry": False, "extensionsAutoUpgrade": False}
                if budget is not None:
                    settings["codemode"] = {"mode": "on", "inlineBudget": budget}
                (agent / "settings.json").write_text(json.dumps(settings))
                env = dict(os.environ, HOME=directory, PIG_HOME=directory, PI_HOME=directory,
                           PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
                for key in list(env):
                    if key.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN")):
                        del env[key]
                for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                    env[key] = str(agent)
                ModelServer.stage, ModelServer.failure, ModelServer.results, ModelServer.root = 0, "", [], root
                result = subprocess.run([str(binary), "--provider", "recovery", "--model", "recovery",
                    "--no-extensions", "--print", "--no-session", "--offline", "--no-context-files",
                    "--no-skills", "--no-prompt-templates", "--no-themes", "Exercise tool recovery."],
                    cwd=root, env=env, stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=90)
                assert result.returncode == 0 and not ModelServer.failure and "RECOVERY_OK" in result.stdout, \
                    f"budget={budget}: {ModelServer.failure}\n{result.stdout}\n{result.stderr}"
                print(f"PASS budget={budget}: model sees direct-only todo; direct and nested recovery; rejected edits preserve bytes")
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main()
