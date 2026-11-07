#!/usr/bin/env python3
"""Prove model declaration, free search fallback, and fetch in the real binary."""
import http.server
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import threading


class Fixture(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    stage = 0
    failure = ""
    excluded = False
    requests_seen = []

    def log_message(self, *_):
        pass

    def reply(self, body, content_type="text/html", status=200):
        wire = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(wire)))
        self.end_headers()
        self.wfile.write(wire)

    def do_GET(self):
        cls = type(self)
        cls.requests_seen.append(self.path)
        if self.path.startswith("/ddg"):
            self.reply('<html><body><div class="anomaly-modal">Challenge</div></body></html>')
        elif self.path == "/article":
            self.reply("<html><head><title>Local web fixture</title></head><body><nav>Menu</nav><article><h1>Free web tools proof</h1><p>The compiled harness fetched this article through the native reader. Local fixture evidence proves that readable content reaches the assistant without a browser, API key, or paid service.</p><p>FETCH_NATIVE_PROOF: the river flows north. This second substantial paragraph distinguishes the article from a short navigation page and exercises the native quality gate.</p></article><script>UNSAFE_SCRIPT_PROOF</script></body></html>")
        else:
            self.send_error(404)

    def do_POST(self):
        cls = type(self)
        raw = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        if self.path.startswith("/ddg"):
            cls.requests_seen.append(self.path)
            self.reply('<html><body><div class="anomaly-modal">Challenge</div></body></html>')
            return
        if self.path == "/firecrawl":
            cls.requests_seen.append(self.path)
            self.reply(json.dumps({"success": True, "data": {"web": [{"url": f"http://127.0.0.1:{self.server.server_port}/article", "title": "Free web tools proof", "description": "SEARCH_FALLBACK_PROOF: a locally served article."}]}}), "application/json")
            return
        if not self.path.endswith("/chat/completions"):
            self.send_error(404)
            return
        body = json.loads(raw)
        offered = {tool["function"]["name"] for tool in body.get("tools", [])}
        expected = {"web_search", "web_fetch"}
        if cls.stage == 0:
            if cls.excluded and offered & expected:
                cls.failure = "excluded web tools remained declared: " + repr(offered)
            elif not cls.excluded and not expected <= offered:
                cls.failure = "web tools are not declared: " + repr(offered)
        result = next((m.get("content", "") for m in reversed(body["messages"]) if m["role"] == "tool"), "")
        result = result if isinstance(result, str) else json.dumps(result)
        if cls.stage == 1 and "SEARCH_FALLBACK_PROOF" not in result:
            cls.failure = "search fallback result missing: " + result
        if cls.stage == 2 and ("FETCH_NATIVE_PROOF" not in result or "UNSAFE_SCRIPT_PROOF" in result):
            cls.failure = "native article conversion failed: " + result
        if cls.failure or cls.excluded or cls.stage >= 2:
            delta = {"role": "assistant", "content": "WEB_TOOLS_FAILED" if cls.failure else "WEB_TOOLS_RUNTIME_OK"}
            reason = "stop"
        else:
            name, args = ("web_search", {"query": "free web tools", "limit": 3}) if cls.stage == 0 else ("web_fetch", {"url": f"http://127.0.0.1:{self.server.server_port}/article"})
            delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": f"web_{cls.stage}", "type": "function", "function": {"name": name, "arguments": json.dumps(args)}}]}
            reason = "tool_calls"
        cls.stage += 1
        chunks = [
            {"id": "web-smoke", "object": "chat.completion.chunk", "created": 0, "model": "web-smoke", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "web-smoke", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        self.reply("".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n", "text/event-stream")


def main():
    if len(sys.argv) != 2:
        raise SystemExit("Usage: python3 scripts/smoke-test-web-tools-binary.py <ompig-slim-binary>")
    binary = pathlib.Path(sys.argv[1]).resolve()
    with tempfile.TemporaryDirectory(prefix="ompig-web-smoke-") as directory:
        root = pathlib.Path(directory)
        agent = root / "agent"
        agent.mkdir()
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            base = f"http://127.0.0.1:{server.server_port}"
            (agent / "models.json").write_text(json.dumps({"providers": {"web-smoke": {"api": "openai-completions", "baseUrl": base + "/v1", "apiKey": "local-fixture-only", "models": [{"id": "web-smoke"}]}}}))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True, "enableInstallTelemetry": False, "extensionsAutoUpgrade": False, "webSearch": {"chain": ["duckduckgo", "firecrawl"], "endpoints": {"duckduckgo": base + "/ddg", "firecrawl": base + "/firecrawl"}}, "webFetch": {"reader": "native"}}))
            env = dict(os.environ, HOME=str(root), PIG_HOME=str(root / "library"), OMPIG_SLIM_CODING_AGENT_DIR=str(agent), PI_OFFLINE="1", PI_TELEMETRY="0", PI_SKIP_VERSION_CHECK="1")
            for key in list(env):
                if key.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN")):
                    del env[key]
            scenarios = [
                ("default builtins without extensions", ["--no-extensions"], False),
                ("explicit web tool allowlist", ["--no-extensions", "--tools", "web_search,web_fetch"], False),
                ("excluded web tools", ["--no-extensions", "--exclude-tools", "web_search,web_fetch"], True),
                ("disabled builtins", ["--no-extensions", "--no-builtin-tools"], True),
            ]
            for name, flags, excluded in scenarios:
                Fixture.stage, Fixture.failure, Fixture.excluded, Fixture.requests_seen = 0, "", excluded, []
                command = [str(binary), "--provider", "web-smoke", "--model", "web-smoke", *flags, "--print", "--no-session", "--offline", "--no-context-files", "--no-skills", "--no-prompt-templates", "--no-themes", "Exercise the compiled web tools."]
                result = subprocess.run(command, cwd=root, env=env, stdin=subprocess.DEVNULL, text=True, capture_output=True, timeout=30)
                if result.returncode or Fixture.failure or "WEB_TOOLS_RUNTIME_OK" not in result.stdout:
                    raise RuntimeError(f"{name}: exit {result.returncode}\n{Fixture.failure}\nstdout:\n{result.stdout}\nstderr:\n{result.stderr}")
                if not excluded and (not any(p.startswith("/ddg") for p in Fixture.requests_seen) or "/firecrawl" not in Fixture.requests_seen or "/article" not in Fixture.requests_seen):
                    raise RuntimeError("fallback and fetch transports were not exercised: " + repr(Fixture.requests_seen))
                print(name + ": " + ("not declared to model" if excluded else "model declaration, bot-wall fallback, cited search, native fetch"))
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


if __name__ == "__main__":
    main()
