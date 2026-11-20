#!/usr/bin/env python3
"""Check Pi 1.5 and 1.6 filesystem tool behavior through the real CLI."""

import http.server
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import threading


CALLS = [
    ("grep", {"path": "repo", "glob": "./migrations/main/*.js", "pattern": "needle"}),
    ("find", {"path": "repo", "pattern": "./src/*.ts"}),
    ("find", {"path": "repo", "pattern": "src/*.ts"}),
    ("find", {"path": "repo/src", "pattern": "*.ts", "limit": 1}),
    ("find", {"path": "repo/src", "pattern": "*.ts", "limit": 0}),
    ("ls", {"path": "repo/src", "limit": 0}),
    ("edit", {"path": "mixed.txt", "edits": [{"oldText": "before", "newText": "after"}]}),
]


class FixtureModel(http.server.BaseHTTPRequestHandler):
    step = 0

    def log_message(self, *_):
        pass

    def do_POST(self):
        self.rfile.read(int(self.headers["Content-Length"]))
        step = type(self).step
        if step < len(CALLS):
            name, args = CALLS[step]
            delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": f"call_{step}",
                     "type": "function", "function": {"name": name, "arguments": json.dumps(args)}}]}
            reason = "tool_calls"
        else:
            delta = {"role": "assistant", "content": "PARITY_FILES_DONE"}
            reason = "stop"
        type(self).step += 1
        chunks = [
            {"id": "parity", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "parity", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = ("".join(f"data: {json.dumps(chunk)}\n\n" for chunk in chunks) + "data: [DONE]\n\n").encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire)))
        self.end_headers()
        self.wfile.write(wire)


def exercise(binary, root, port):
    agent = root / "agent"
    agent.mkdir()
    bin_dir = root / "bin"
    bin_dir.mkdir()
    if not shutil.which("fd") and shutil.which("fdfind"):
        (bin_dir / "fd").symlink_to(shutil.which("fdfind"))
    if not (shutil.which("fd") or (bin_dir / "fd").exists()) or not shutil.which("rg"):
        raise SystemExit("This smoke test requires fd and ripgrep on PATH.")
    (agent / "models.json").write_text(json.dumps({"providers": {"parity-fixture": {
        "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{port}/v1",
        "apiKey": "fixture", "models": [{"id": "parity-fixture"}],
    }}}))
    for fixture, contents in {
        "repo/migrations/main/a.js": b"needle\n",
        "repo/src/main.ts": b"export {};\n",
        "repo/pkg/src/nested.ts": b"export {};\n",
        "mixed.txt": b"one\r\nbefore\ntwo\r\n",
    }.items():
        path = root / fixture
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(contents)
    env = dict(os.environ, HOME=str(root), PIG_HOME=str(root), PI_HOME=str(root),
               PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
               PATH=os.pathsep.join((str(bin_dir), os.environ.get("PATH", ""))))
    for key in list(env):
        if key.startswith("OMPIG_BROKER_") or key.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN")):
            del env[key]
    for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
        env[key] = str(agent)
    result = subprocess.run(
        [str(binary), "-p", "Run the parity fixture calls.", "--no-extensions", "--no-context-files",
         "--no-skills", "--offline", "--tools", "grep,find,ls,edit", "--model", "parity-fixture/parity-fixture"],
        cwd=root, env=env, capture_output=True, text=True, timeout=30, check=False)
    if result.returncode != 0 or "PARITY_FILES_DONE" not in result.stdout:
        raise SystemExit(f"CLI exit {result.returncode}\n{result.stdout}\n{result.stderr}")
    results = []
    for session in (agent / "sessions").rglob("*.jsonl"):
        for line in session.read_text().splitlines():
            message = json.loads(line).get("message") or {}
            if message.get("role") == "toolResult":
                text = "".join(block.get("text", "") for block in message.get("content", []))
                results.append((message.get("toolName"), text.strip(), message.get("isError", False)))
    assert [name for name, _, _ in results] == [name for name, _ in CALLS], results
    return results


def main():
    if len(sys.argv) != 2:
        raise SystemExit("Usage: smoke-test-pi-parity-files.py <ompig-slim-binary>")
    binary = Path(sys.argv[1]).resolve()
    with http.server.ThreadingHTTPServer(("127.0.0.1", 0), FixtureModel) as server:
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            with tempfile.TemporaryDirectory(prefix="ompig-pi-parity-") as directory:
                root = Path(directory)
                out = exercise(binary, root, server.server_port)
                checks = {
                    "grep glob relative to path": "migrations/main/a.js:1: needle" in out[0][1] and not out[0][2],
                    "find leading ./": out[1][1] == "src/main.ts" and not out[1][2],
                    "find path anchored to the search root": out[2][1] == "src/main.ts" and not out[2][2],
                    "find exact limit without truncation notice": out[3][1] == "main.ts" and not out[3][2],
                    "find limit zero clamped to one": out[4][1] == "main.ts" and not out[4][2],
                    "ls limit zero clamped to one": out[5][1] == "main.ts" and not out[5][2],
                    "edit preserves untouched line endings": (root / "mixed.txt").read_bytes() == b"one\r\nafter\ntwo\r\n" and not out[6][2],
                }
                for title, passed in checks.items():
                    print(f"{'PASS' if passed else 'FAIL'} {title}")
                if not all(checks.values()):
                    print(out, file=sys.stderr)
                    return 1
        finally:
            server.shutdown()
            thread.join()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
