#!/usr/bin/env python3
"""Exercise the shipped durable CLI with a loopback model and killed processes.

All configuration, database files and workspaces are temporary. No external
credentials, inference services, or Kubernetes cluster are required.
"""

import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def stream(w, delta, finish):
    w.send_response(200)
    w.send_header("Content-Type", "text/event-stream")
    w.end_headers()
    chunk = {"choices": [{"index": 0, "delta": delta, "finish_reason": None}]}
    stop = {"choices": [{"index": 0, "delta": {}, "finish_reason": finish}]}
    w.wfile.write(("data: " + json.dumps(chunk) + "\n\ndata: " + json.dumps(stop) +
                   "\n\ndata: [DONE]\n\n").encode())


def answer(w, text):
    stream(w, {"role": "assistant", "content": text}, "stop")


def tool_call(w, name, arguments):
    stream(w, {"role": "assistant", "tool_calls": [{
        "index": 0, "id": "fixture-call", "type": "function",
        "function": {"name": name, "arguments": json.dumps(arguments)}}]}, "tool_calls")


def request_text(body):
    texts = []
    for message in body["messages"]:
        content = message.get("content")
        if isinstance(content, str):
            texts.append(content)
        elif isinstance(content, list):
            texts.extend(part.get("text", "") for part in content)
    return "\n".join(texts)


def crash(binary):
    """Kill during a model call, resume, and prove completed admission deduplication."""
    reached = threading.Event()
    release = threading.Event()
    calls = []

    class Handler(BaseHTTPRequestHandler):
        """Implement only the OpenAI streaming endpoint used by the fixture model."""

        def log_message(self, *_):
            """Keep successful smoke output deterministic and free of HTTP logs."""

        def do_POST(self):
            """Hold the first request at the crash point and answer its recovery."""
            body = self.rfile.read(int(self.headers["Content-Length"]))
            calls.append(json.loads(body))
            if len(calls) == 1:
                reached.set()
                release.wait(30)
                return
            answer(self, "Recovered workdesk")

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-durable-") as directory:
            root = Path(directory)
            profile, workspace, state = root / "profile", root / "repo", root / "state"
            profile.mkdir()
            workspace.mkdir()
            (workspace / "preserved.txt").write_text("uncommitted work\n")
            (profile / "models.json").write_text(json.dumps({"providers": {"desk-fixture": {
                "api": "openai-completions",
                "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "fixture-key",
                "models": [{"id": "fixture", "name": "fixture", "contextWindow": 128000, "maxTokens": 1024}],
            }}}))
            env = {**os.environ, "PIG_HOME": str(root / "home"),
                   "OMPIG_SLIM_CODING_AGENT_DIR": str(profile), "PI_OFFLINE": "1"}
            # Do not inherit a live broker connection or bootstrap secret.
            for key in list(env):
                if key.startswith("OMPIG_BROKER_"):
                    del env[key]
            command = [binary, "durable", "run", "--state", str(state), "--cwd", str(workspace),
                       "--model", "desk-fixture/fixture", "--request-id", "smoke-1", "Finish the work"]
            with subprocess.Popen(command, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE) as process:
                try:
                    assert reached.wait(20), "workdesk never reached model request"
                    process.send_signal(signal.SIGKILL)
                    process.communicate(timeout=10)
                finally:
                    release.set()
                    if process.poll() is None:
                        process.kill()
                        process.communicate(timeout=10)
            for args in ([binary, "durable", "resume", "--state", str(state)], command,
                         [binary, "durable", "status", "--state", str(state)]):
                result = subprocess.run(args, env=env, capture_output=True, text=True, timeout=25)
                assert result.returncode == 0, result.stderr
                receipt = json.loads(result.stdout)
                assert receipt["status"] == "done", receipt
                assert receipt["answer"] == "Recovered workdesk", receipt
            assert len(calls) == 2, f"duplicate work: {len(calls)} requests"
            assert (workspace / "preserved.txt").read_text() == "uncommitted work\n"
    finally:
        release.set()
        server.shutdown()
        server.server_close()
        thread.join()


def compaction(binary):
    published = threading.Event()
    release = threading.Event()
    calls = []
    constraint = "never repeat a completed mutation"
    summary = ("## Goal\nFinish the work.\n\n## Constraints & Preferences\n- (none)\n\n"
               "## Progress\n### Done\n- [x] everything the assignment asked for\n\n"
               "## Next Steps\n1. Report completion\n")
    steps = {"progress": False, "wrote": False, "held": False, "implemented": False, "checked": False}
    fillers = []

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            """Keep successful smoke output deterministic and free of HTTP logs."""

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            calls.append(body)
            text = request_text(body)
            if "context summarization assistant" in text:
                answer(self, summary)
                return
            assert text.count("<ompig_progress>") <= 1, "duplicate progress sections"
            if not steps["progress"]:
                steps["progress"] = True
                tool_call(self, "update_progress", {
                    "criteria": ["counter.txt holds one mark", "result.txt holds fixed"], "completedWork": [],
                    "currentStep": "write counter.txt", "constraints": [constraint],
                    "decisions": [], "blockers": ["result.txt is not implemented"],
                    "nextAction": "implement result.txt then check both files"})
                return
            if not steps["wrote"]:
                steps["wrote"] = True
                tool_call(self, "bash", {"command": "printf x >> counter.txt"})
                return
            if "<summary>" in text:
                assert constraint in text, "the committed constraint left the model context"
                assert '"revision":1' in text, "the committed checkpoint left the model context"
                if not steps["held"]:
                    steps["held"] = True
                    published.set()
                    release.wait(60)
                    return
                if not steps["implemented"]:
                    steps["implemented"] = True
                    tool_call(self, "write", {"path": "result.txt", "content": "fixed\n"})
                    return
                if not steps["checked"]:
                    steps["checked"] = True
                    tool_call(self, "bash", {"command": 'test "$(cat counter.txt)" = x && '
                                            'test "$(cat result.txt)" = fixed && printf checked > checked.txt'})
                    return
                answer(self, "result.txt holds fixed")
                return
            fillers.append(len(text))
            tool_call(self, "bash", {"command":
                                     'for ((i=0;i<1200;i++)); do echo "filler %d line $i '
                                     'padding padding padding padding padding"; done' % len(fillers)})

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-durable-") as directory:
            root = Path(directory)
            profile, workspace, state = root / "profile", root / "repo", root / "state"
            profile.mkdir()
            workspace.mkdir()
            (profile / "models.json").write_text(json.dumps({"providers": {"desk-fixture": {
                "api": "openai-completions",
                "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "fixture-key",
                "models": [{"id": "fixture", "name": "fixture", "contextWindow": 64000, "maxTokens": 1024}],
            }}}))
            env = {**os.environ, "PIG_HOME": str(root / "home"),
                   "OMPIG_SLIM_CODING_AGENT_DIR": str(profile), "PI_OFFLINE": "1"}
            for key in list(env):
                if key.startswith("OMPIG_BROKER_"):
                    del env[key]
            command = [binary, "durable", "run", "--state", str(state), "--cwd", str(workspace),
                       "--model", "desk-fixture/fixture", "--request-id", "compact-1",
                       "Create counter.txt with one x, implement result.txt containing fixed, and check both files. "
                       "Never repeat a completed mutation."]
            with subprocess.Popen(command, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE) as process:
                try:
                    assert published.wait(90), "no request carried the committed summary"
                    assert fillers, "the fixture never crossed the compaction threshold"
                    process.send_signal(signal.SIGKILL)
                    process.communicate(timeout=10)
                finally:
                    release.set()
                    if process.poll() is None:
                        process.kill()
                        process.communicate(timeout=10)
            result = subprocess.run([binary, "durable", "resume", "--state", str(state)],
                                    env=env, capture_output=True, text=True, timeout=60)
            assert result.returncode == 0, result.stderr
            receipt = json.loads(result.stdout)
            assert receipt["status"] == "done", receipt
            assert receipt["answer"] == "result.txt holds fixed", receipt
            assert receipt["progress"]["checkpoint"]["constraints"] == [constraint], receipt
            assert (workspace / "counter.txt").read_text() == "x", "the mutation ran twice or not at all"
            assert (workspace / "result.txt").read_text() == "fixed\n", "remaining implementation was lost"
            assert (workspace / "checked.txt").read_text() == "checked", "remaining checks did not pass"
            final = request_text(calls[-1])
            assert "<summary>" in final, "the committed summary left the context"
            assert constraint in final, "the resumed request dropped the constraint"
            assert final.count("<ompig_progress>") == 1, "duplicate progress sections"
            assert "only a later update_progress commit replaces this revision" in final.lower()
            assert "reopen work this revision supersedes" in final
            assert "promote completed work to verified acceptance" in final
            print("durable CLI: compaction, SIGKILL resume and committed progress passed")
    finally:
        release.set()
        server.shutdown()
        server.server_close()
        thread.join()


def run(binary):
    crash(binary)
    print("durable CLI: SIGKILL recovery, deduplication and status passed")
    compaction(binary)
    version = subprocess.check_output([binary, "--version"], text=True, timeout=10).strip()
    assert version and version[0].isdigit(), version


if __name__ == "__main__":
    run(str(Path(sys.argv[1]).resolve()))
