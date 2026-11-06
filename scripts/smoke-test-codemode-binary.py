#!/usr/bin/env python3
"""Exercise the built binary's real QuickJS engine, session state, and nested tools."""
import http.server
import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile
import threading


class ModelServer(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    stage = 0
    nested = False
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
        if cls.stage == 0 and "codemode" not in offered:
            cls.failure = "codemode is not declared to the model"
        if cls.stage == 1 and not re.search(r"(?:^|\n)16(?:\n|$)", result_text):
            cls.failure = "QuickJS arithmetic result is missing: " + result_text
        if cls.stage == 2 and not re.search(r"(?:^|\n)32(?:\n|$)", result_text):
            cls.failure = "codemode store/load did not persist across calls: " + result_text

        if cls.failure or cls.stage >= 2:
            delta = {"role": "assistant", "content": "CODEMODE_FAILED" if cls.failure else "CODEMODE_RUNTIME_OK"}
            reason = "stop"
        else:
            if cls.stage == 0:
                code = 'const sum = [3, 5, 8].reduce((a, b) => a + b, 0); store("sum", sum); '
                if cls.nested:
                    code += 'await tools.write({path: "codemode-proof.txt", content: String(sum)}); '
                code += "return sum;"
            else:
                code = 'const sum = load("sum"); if (sum !== 16) throw new Error("lost session state"); '
                if cls.nested:
                    code += 'const content = await tools.read({path: "codemode-proof.txt"}); if (!content.includes(String(sum))) throw new Error("nested read differs"); '
                code += "return sum * 2;"
            delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": f"call_{cls.stage}", "type": "function", "function": {"name": "codemode", "arguments": json.dumps({"code": code})}}]}
            reason = "tool_calls"
        cls.stage += 1
        chunks = [
            {"id": "smoke", "object": "chat.completion.chunk", "created": 0, "model": "codemode-smoke", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "smoke", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())
        self.wfile.flush()


def main():
    if len(sys.argv) != 2:
        raise SystemExit("Usage: python3 scripts/smoke-test-codemode-binary.py <ompig-slim-binary>")
    binary = pathlib.Path(sys.argv[1]).resolve()
    with tempfile.TemporaryDirectory(prefix="ompig-codemode-smoke-") as directory:
        root = pathlib.Path(directory)
        agent = root / "agent"
        agent.mkdir()
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            (agent / "models.json").write_text(json.dumps({"providers": {"codemode-smoke": {"api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1", "apiKey": "local-fixture-only", "models": [{"id": "codemode-smoke"}]}}}))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True, "enableInstallTelemetry": False, "extensionsAutoUpgrade": False}))
            env = dict(os.environ, HOME=str(root), PIG_HOME=str(root / "library"), OMPIG_SLIM_CODING_AGENT_DIR=str(agent), PI_OFFLINE="1", PI_TELEMETRY="0", PI_SKIP_VERSION_CHECK="1")
            for name in list(env):
                if name.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN")):
                    del env[name]
            for name, flags, nested in [
                ("explicit codemode", ["--no-extensions", "--tools", "codemode"], False),
                ("default codemode without extensions", ["--no-extensions"], True),
            ]:
                ModelServer.stage, ModelServer.nested, ModelServer.failure = 0, nested, ""
                command = [str(binary), "--provider", "codemode-smoke", "--model", "codemode-smoke", *flags, "--print", "--no-session", "--offline", "--no-context-files", "--no-skills", "--no-prompt-templates", "--no-themes", "Exercise the compiled codemode runtime."]
                result = subprocess.run(command, cwd=root, env=env, stdin=subprocess.DEVNULL, text=True, capture_output=True, timeout=30)
                if result.returncode or ModelServer.failure or "CODEMODE_RUNTIME_OK" not in result.stdout:
                    raise RuntimeError(f"{name}: exit {result.returncode}\n{ModelServer.failure}\nstdout:\n{result.stdout}\nstderr:\n{result.stderr}")
                if nested and (root / "codemode-proof.txt").read_text() != "16":
                    raise RuntimeError("nested codemode tool did not write its computed result")
                print(name + ": QuickJS arithmetic, persistent store/load" + (", nested write/read" if nested else ""))
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


if __name__ == "__main__":
    main()
