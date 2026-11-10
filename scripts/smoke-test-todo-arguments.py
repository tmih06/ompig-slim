#!/usr/bin/env python3
"""Prove the two todo-argument fixes on a built binary through a loopback model.

The reported failure was `Validation failed for tool "todo": op: missing`, reached
two ways: a call the schema rejected before the tool could repair an omitted `op`,
and a call whose argument text was not JSON at all (reported as a missing member of
the empty object that text decays to). This drives both through the real CLI and
reads the session transcript for what the model was actually told.

Print mode does not echo tool results, so the assertions read the session log the
run writes. That is also what a resumed session sees, so the check covers what the
model carries forward, not just what scrolled past.
"""
import http.server
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading


# The scripted calls replay the observed failures, then stop for a handoff.
STAGES = [
    # 1. Op-less init: what three of the four logged failures sent.
    {"name": "todo", "arguments": json.dumps(
        {"list": [{"phase": "Fix", "items": ["Parse the payload", "Land the list"]}]})},
    # 2. Argument text that is not JSON at all.
    {"name": "todo", "arguments": '{"init": list" /><parameter name="phase">investigate}'},
    # 3. A bare call, which can only mean "show me the list".
    {"name": "todo", "arguments": "{}"},
    # 4. A flat-init `items` list with no phase: append would reject a missing
    # phase, so init is the only op that can succeed.
    {"name": "todo", "arguments": json.dumps({"items": ["Flat init task"]})},
]


class ModelServer(http.server.BaseHTTPRequestHandler):
    """Serve one scripted tool call per request, then a final answer."""

    stage = 0

    def log_message(self, *_):
        """Stay quiet; assertions report failures."""

    def do_POST(self):
        """Answer with an OpenAI-compatible SSE stream."""
        self.rfile.read(int(self.headers["Content-Length"]))
        if type(self).stage < len(STAGES):
            call = STAGES[type(self).stage]
            delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": f"op-{type(self).stage}",
                "type": "function", "function": call}]}
            reason = "tool_calls"
        else:
            delta = {"role": "assistant", "content": "ARGS_DONE."}
            reason = "stop"
        type(self).stage += 1
        chunks = [
            {"id": "args", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "args", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())


def todo_results(agent):
    """Return the text and details of every todo tool result, in call order."""
    results = []
    for session in sorted((agent / "sessions").rglob("*.jsonl")):
        for line in session.read_text().splitlines():
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue
            message = record.get("message") or {}
            if message.get("role") != "toolResult" or message.get("toolName") != "todo":
                continue
            text = "".join(block.get("text", "") for block in message.get("content") or [])
            results.append({"text": text, "details": message.get("details") or {},
                "isError": bool(message.get("isError"))})
    return results


def main():
    """Run the scripted turns and assert what the model was told."""
    if len(sys.argv) != 2:
        raise SystemExit("Usage: smoke-test-todo-arguments.py <ompig-slim-binary>")
    binary = Path(sys.argv[1]).resolve()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    with tempfile.TemporaryDirectory(prefix="ompig-args-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        (agent / "models.json").write_text(json.dumps({"providers": {"args-proof": {
            "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
            "apiKey": "fixture", "models": [{"id": "args-proof"}]}}}))
        (agent / "settings.json").write_text(json.dumps({"quietStartup": True,
            "enableInstallTelemetry": False}))
        env = dict(os.environ, HOME=directory, PIG_HOME=directory, PI_HOME=directory,
                   PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0")
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        finished = subprocess.run(
            [str(binary), "-p", "Exercise the todo argument repair.", "--no-extensions",
             "--provider", "args-proof", "--model", "args-proof", "--no-context-files",
             "--no-skills", "--offline"],
            cwd=root, env=env, capture_output=True, timeout=120, check=False)
        out = finished.stdout.decode("utf-8", "replace") + finished.stderr.decode("utf-8", "replace")
        assert finished.returncode == 0, f"exit={finished.returncode}\n{out[-4000:]}"
        assert "ARGS_DONE" in out, f"the run did not finish:\n{out[-4000:]}"

        results = todo_results(agent)
        assert len(results) == len(STAGES), \
            f"expected {len(STAGES)} todo results, got {len(results)}: {results}"
        assert "op: missing" not in "".join(r["text"] for r in results), \
            f"an op-less call was still rejected: {[r['text'] for r in results]}"

        # 1. The op-less init was repaired and applied.
        init = results[0]
        assert not init["isError"], f"the op-less init failed: {init['text']}"
        assert init["details"].get("op") == "init", f"op = {init['details'].get('op')}, want init"
        assert len(init["details"].get("phases") or []) == 1, f"the list did not land: {init['details']}"

        # 2. The unparseable call reports its parse error, not a missing member.
        parse = results[1]
        assert parse["isError"], "a call with unparseable arguments must fail"
        assert "Invalid arguments for tool \"todo\"" in parse["text"], \
            f"the parse failure was not named: {parse['text']}"
        assert "is not valid JSON" in parse["text"], \
            f"the parse error text was lost: {parse['text']}"

        # 3. The bare call reads the list instead of failing.
        view = results[2]
        assert not view["isError"], f"a bare todo call failed: {view['text']}"
        assert view["details"].get("op") == "view", f"op = {view['details'].get('op')}, want view"
        assert view["details"].get("phases") == init["details"].get("phases"), \
            "the bare call did not report the list the init landed"

        # 4. The flat init was repaired and replaced the list.
        flat = results[3]
        assert not flat["isError"], f"a flat items init failed: {flat['text']}"
        assert flat["details"].get("op") == "init", f"op = {flat['details'].get('op')}, want init"
        phases = flat["details"].get("phases") or []
        assert len(phases) == 1 and [t["content"] for t in phases[0]["tasks"]] == ["Flat init task"], \
            f"the flat init did not land: {phases}"

        print("PASS: op-less init and flat init repaired, unparseable arguments report "
              "the parse error, bare call reads the list")


if __name__ == "__main__":
    main()
