#!/usr/bin/env python3
"""Prove streaming-rule reminders, blocks, and rule-poke cards with a loopback model."""
import http.server
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading


CATALOG = {
    "catalog-go": {
        "path": "catalog.go",
        "content": 'package probe\nimport ("crypto/tls"; "encoding/json")\nvar config = tls.Config{InsecureSkipVerify: true}\nfunc decode(data []byte) { var result any; _ = json.Unmarshal(data, &result) }\n',
        "rules": {
            "go-tls-skip-verify": "TLS certificate verification is disabled. Restore verification or prove the custom verifier.",
            "go-ignored-json-error": "A JSON decode error is discarded. Handle invalid input before using the result.",
        },
    },
    "catalog-js": {
        "path": "catalog.ts",
        "content": 'const job = new Promise(async (resolve) => { resolve(await load()); });\nitems.forEach(async item => { await save(item); });\nconst all = items.reduce((acc, item) => [...acc, item], []);\n',
        "rules": {
            "js-async-promise-executor": "An async Promise executor can lose errors. Use an async function or a synchronous executor.",
            "js-async-foreach": "Array.forEach does not await async callbacks. Make completion and failure handling explicit.",
            "js-reduce-spread": "Spreading a growing reduce accumulator repeatedly can cause quadratic copying.",
        },
    },
    "catalog-py": {
        "path": "catalog.py",
        "content": 'def append(value, items=[]):\n    items.append(value)\n    return items\nresponse = requests.get(url)\n',
        "rules": {
            "py-mutable-default": "A mutable default argument is shared between Python calls. Allocate per call when state should be fresh.",
            "py-requests-timeout": "A Requests call has no explicit timeout. Verify a bounded network wait.",
        },
    },
    "catalog-rs": {
        "path": "catalog.rs",
        "content": 'fn example(bytes: &[u8]) {\n    let text = unsafe { std::str::from_utf8_unchecked(bytes) };\n    let count = (0..10).collect::<Vec<_>>().len();\n}\n',
        "rules": {
            "rs-unchecked-utf8": "Unchecked UTF-8 conversion requires valid bytes. Validate external data before creating strings.",
            "rs-collect-count": "Collecting a Vec only to count it allocates unnecessarily. Check whether Iterator.count preserves semantics.",
        },
    },
}
TUI_CATALOG = tuple(CATALOG)
CATALOG["catalog-disabled"] = {**CATALOG["catalog-go"], "ttsr": {"builtinRules": False}}
CATALOG["catalog-one-disabled"] = {**CATALOG["catalog-go"], "ttsr": {"disabledRules": ["go-tls-skip-verify"]}}


def verify_catalog(root, scenario, requests):
    case = CATALOG[scenario]
    settings = case.get("ttsr", {})
    active = {name: summary for name, summary in case["rules"].items()
              if settings.get("builtinRules", True) and name not in settings.get("disabledRules", [])}
    assert len(requests) == 3, (scenario, len(requests))
    for name in case["rules"]:
        assert reminder_count(requests[0], name) == 0, "rule leaked into initial prompt"
        for request in requests[1:]:
            assert reminder_count(request, name) == (1 if name in active else 0), (scenario, name, request)
    for target in (case["path"], "second-" + case["path"]):
        assert (root / target).read_text() == case["content"], "advisory rule changed or blocked source"
    entries = [json.loads(line) for file in (root / "agent/sessions").rglob("*.jsonl") for line in file.read_text().splitlines()]
    pokes = [entry for entry in entries if entry.get("customType") == "streaming-rule-poke"]
    assert len(pokes) == (1 if active else 0), (scenario, pokes)
    if pokes:
        assert pokes[0]["type"] == "custom" and not pokes[0]["data"]["blocked"], pokes
        assert {rule["name"]: rule["summary"] for rule in pokes[0]["data"]["rules"]} == active, pokes
    assert all("streaming-rule-poke" not in json.dumps(request["messages"]) for request in requests), "poke entered model context"
    return active


class Fixture(http.server.BaseHTTPRequestHandler):
    stage = 0
    scenario = ""
    requests_seen = []

    def log_message(self, *_):
        pass

    def do_POST(self):
        cls = type(self)
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        cls.requests_seen.append(body)
        stage = cls.stage
        cls.stage += 1
        if cls.scenario in CATALOG and stage < 2:
            case = CATALOG[cls.scenario]
            name, args = "write", {"path": ("second-" if stage else "") + case["path"], "content": case["content"]}
        elif stage == 0:
            if cls.scenario == "custom":
                name, args = "write", {"path": "blocked.go", "content": "BAD_CODE"}
            elif cls.scenario == "nested":
                name = "codemode"
                args = {"code": 'await tools.write({path: "nested.go", content: \'package main\\nimport "io/ioutil"\\n\'});'}
            elif cls.scenario == "removal":
                name = "edit"
                args = {"path": "old.go", "edits": [{"oldText": '"io/ioutil"', "newText": '"io"'}]}
            else:
                name, args = "write", {"path": "first.go", "content": 'package main\nimport "io/ioutil"\n'}
        elif stage == 1 and cls.scenario in ("builtin", "custom"):
            name, args = "write", {"path": "second.go", "content": 'package main\nimport "io/ioutil"\n'}
        else:
            name, args = None, None
        if name:
            arguments = json.dumps(args)
            parts = [arguments[:len(arguments) // 2], arguments[len(arguments) // 2:]]
            deltas = [{"role": "assistant", "tool_calls": [{"index": 0, "id": f"call-{stage}", "type": "function", "function": {"name": name, "arguments": parts[0]}}]},
                      {"tool_calls": [{"index": 0, "function": {"arguments": parts[1]}}]}]
            reason = "tool_calls"
        else:
            deltas = [{"role": "assistant", "content": "STREAMING_RULES_OK"}]
            reason = "stop"
        chunks = [{"id": "rules", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]} for delta in deltas]
        chunks.append({"id": "rules", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}})
        wire = ("".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n").encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire)))
        self.end_headers()
        self.wfile.write(wire)


def reminder_count(request, name):
    return json.dumps(request["messages"]).count(f'rule=\\"{name}\\"')


def run_tui(binary):
    spec = importlib.util.spec_from_file_location("goal_ui", Path(__file__).with_name("smoke-test-goal-ui.py"))
    ui = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(ui)
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        for mode in ("fullscreen", "regular"):
            for scenario in ("builtin", "custom", "nested", "removal", *TUI_CATALOG):
                with tempfile.TemporaryDirectory(prefix=f"ompig-rules-ui-{mode}-") as directory:
                    root = Path(directory)
                    (root / ".git").mkdir()
                    Fixture.stage, Fixture.scenario, Fixture.requests_seen = 0, scenario, []
                    session = ui.Session(binary, mode, server.server_port, root)
                    try:
                        if scenario == "custom":
                            rules = root / ".pi/rules"
                            rules.mkdir(parents=True)
                            (rules / "no-bad.md").write_text('---\ncondition: BAD_CODE\nscope: "tool:write(*.go)"\n---\nWrite GOOD_CODE instead.\n')
                        if scenario == "removal":
                            (root / "old.go").write_text('package main\nimport "io/ioutil"\n')
                        session.wait_ready()
                        window, _, frame = session.command("Exercise streaming rules.", seconds=10)
                        expected = 3 if scenario in ("builtin", "custom") or scenario in CATALOG else 2
                        assert len(Fixture.requests_seen) == expected, (mode, scenario, len(Fixture.requests_seen), session.tail())
                        ui.expect(frame, ["STREAMING_RULES_OK"], scenario, mode, window)
                        if scenario in CATALOG:
                            active = verify_catalog(root, scenario, Fixture.requests_seen)
                            ui.expect(frame, ["◇ Rule poke", "Reminder sent", *active, *active.values()], scenario, mode, window)
                            assert frame.count("◇ Rule poke") == 1, (mode, scenario, frame)
                        elif scenario in ("builtin", "nested"):
                            ui.expect(frame, ["◇ Rule poke", "Reminder sent", "go-ioutil", "Deprecated io/ioutil import. Use io and os."], scenario, mode, window)
                            assert frame.count("◇ Rule poke") == 1, (mode, scenario, frame)
                            target = "nested.go" if scenario == "nested" else "first.go"
                            assert (root / target).exists()
                            assert reminder_count(Fixture.requests_seen[1], "go-ioutil") == 1
                        elif scenario == "custom":
                            ui.expect(frame, ["◇ Rule poke", "Tool call blocked", "no-bad", "Write GOOD_CODE instead."], scenario, mode, window)
                            assert not (root / "blocked.go").exists()
                            assert (root / "second.go").exists()
                        else:
                            assert "◇ Rule poke" not in frame, (mode, scenario, frame)
                            assert reminder_count(Fixture.requests_seen[1], "go-ioutil") == 0
                        assert "Streaming rules sent" not in frame and "Streaming rules blocked" not in frame, (mode, scenario, frame)
                        if scenario != "custom":
                            assert "system-reminder" not in frame and "rule_violation" not in frame, (mode, scenario, frame)
                        entries = [json.loads(line) for file in (root / "agent/sessions").rglob("*.jsonl") for line in file.read_text().splitlines()]
                        pokes = [entry for entry in entries if entry.get("customType") == "streaming-rule-poke"]
                        assert len(pokes) == (2 if scenario == "custom" else 0 if scenario == "removal" else 1), (mode, scenario, pokes)
                        assert all(entry["type"] == "custom" for entry in pokes), pokes
                        assert all("streaming-rule-poke" not in json.dumps(request["messages"]) and "Deprecated io/ioutil import. Use io and os." not in json.dumps(request["messages"]) for request in Fixture.requests_seen), "display-only poke entered model context"
                        if scenario == "builtin":
                            session_file = next((root / "agent/sessions").rglob("*.jsonl"))
                            with tempfile.TemporaryDirectory(prefix="ompig-rules-resume-") as resumed_directory:
                                session.close()
                                session = None
                                resumed = ui.Session(binary, mode, server.server_port, Path(resumed_directory), extra_args=("--session", str(session_file)))
                                try:
                                    resumed.wait_ready()
                                    resumed_window = resumed.repaint()
                                    ui.expect(resumed.frame, ["◇ Rule poke", "Reminder sent", "go-ioutil", "Deprecated io/ioutil import. Use io and os."], "resumed poke", mode, resumed_window)
                                    assert len(Fixture.requests_seen) == expected, "resume started an unwanted model turn"
                                finally:
                                    resumed.close()
                        print(f"PASS {mode} {scenario}. Rule-poke card, summary, and saved entry verified.")
                    finally:
                        if session is not None:
                            session.close()
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def main():
    if len(sys.argv) not in (2, 3) or (len(sys.argv) == 3 and sys.argv[2] != "--tui"):
        raise SystemExit("Usage: python3 scripts/smoke-test-streaming-rules.py <binary> [--tui]")
    binary = Path(sys.argv[1]).resolve()
    if len(sys.argv) == 3:
        run_tui(binary)
        return
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        for scenario in ("builtin", "custom", "nested", "disabled", "removal", *CATALOG):
            with tempfile.TemporaryDirectory(prefix="ompig-streaming-rules-") as directory:
                root = Path(directory)
                agent = root / "agent"
                agent.mkdir()
                (root / ".git").mkdir()
                (agent / "models.json").write_text(json.dumps({"providers": {"rule-proof": {
                    "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                    "apiKey": "local-fixture", "models": [{"id": "rule-proof"}]}}}))
                settings = {"quietStartup": True, "enableInstallTelemetry": False, "extensionsAutoUpgrade": False}
                if scenario == "disabled":
                    settings["ttsr"] = {"enabled": False}
                elif scenario in CATALOG:
                    settings["ttsr"] = CATALOG[scenario].get("ttsr", {})
                (agent / "settings.json").write_text(json.dumps(settings))
                if scenario == "custom":
                    rules = root / ".pi/rules"
                    rules.mkdir(parents=True)
                    (rules / "no-bad.md").write_text('---\ncondition: BAD_CODE\nscope: "tool:write(*.go)"\n---\nRULE_CORRECTION_PROOF. Write GOOD_CODE instead.\n')
                if scenario == "removal":
                    (root / "old.go").write_text('package main\nimport "io/ioutil"\n')
                Fixture.stage, Fixture.scenario, Fixture.requests_seen = 0, scenario, []
                env = dict(os.environ, HOME=str(root), PIG_HOME=str(root / "library"), PI_HOME=str(root / "library"),
                           PIG_CODING_AGENT_DIR=str(agent), PI_CODING_AGENT_DIR=str(agent), OMPIG_SLIM_CODING_AGENT_DIR=str(agent),
                           PI_OFFLINE="1", PI_TELEMETRY="0", PI_SKIP_VERSION_CHECK="1")
                for key in list(env):
                    if key.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN")) or key.startswith("OMPIG_BROKER_"):
                        del env[key]
                command = [str(binary), "--provider", "rule-proof", "--model", "rule-proof", "--print",
                           "--offline", "--no-extensions", "--no-context-files", "--no-skills", "--no-prompt-templates",
                           "--no-themes", "Exercise streaming rules."]
                result = subprocess.run(command, cwd=root, env=env, stdin=subprocess.DEVNULL, text=True, capture_output=True, timeout=45)
                assert result.returncode == 0 and "STREAMING_RULES_OK" in result.stdout, (scenario, result.stdout, result.stderr)
                requests = Fixture.requests_seen
                assert len(requests) == (3 if scenario in ("builtin", "custom") or scenario in CATALOG else 2), (scenario, len(requests))
                assert reminder_count(requests[0], "go-ioutil") == 0, "rule leaked into the initial prompt"
                if scenario in CATALOG:
                    verify_catalog(root, scenario, requests)
                    assert "◇ Rule poke" not in result.stdout, "display-only poke appeared in print mode"
                elif scenario in ("builtin", "nested"):
                    assert reminder_count(requests[1], "go-ioutil") == 1, (scenario, requests[1])
                    target = "nested.go" if scenario == "nested" else "first.go"
                    assert '"io/ioutil"' in (root / target).read_text(), "advisory rule blocked a write"
                    if scenario == "builtin":
                        assert reminder_count(requests[2], "go-ioutil") == 1, "rule fired twice in one run"
                        assert (root / "second.go").exists()
                    sessions = list((agent / "sessions").rglob("*.jsonl"))
                    assert sessions, "session was not persisted"
                    entries = [json.loads(line) for file in sessions for line in file.read_text().splitlines()]
                    assert sum(entry.get("customType") == "streaming-rule-reminder" for entry in entries) == 1, entries
                elif scenario == "custom":
                    assert not (root / "blocked.go").exists(), "blocked call wrote a file"
                    assert "RULE_CORRECTION_PROOF" in json.dumps(requests[1]["messages"]), requests[1]
                    assert (root / "second.go").exists(), "blocking ended the agent run"
                else:
                    assert reminder_count(requests[1], "go-ioutil") == 0, (scenario, requests[1])
                    if scenario == "removal":
                        assert '"io"' in (root / "old.go").read_text()
                    else:
                        assert (root / "first.go").exists()
                print(f"PASS {scenario}. Provider context and filesystem effects verified.")
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main()
