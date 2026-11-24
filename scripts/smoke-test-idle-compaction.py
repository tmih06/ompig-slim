#!/usr/bin/env python3
"""Verify cache-prefix idle compaction and continuation through the real RPC binary."""
import argparse
import http.server
import json
from pathlib import Path
import queue
import subprocess
import tempfile
import threading
import time

from benchmark import isolated_env

SUMMARY = "IDLE_CHECKPOINT saved the older fixture work."
FINAL_REPLY = "FINAL_ASSISTANT_CONTEXT must survive the idle checkpoint."
CONTINUE = "CONTINUE_AFTER_IDLE"


def without_cache_markers(value):
    if isinstance(value, dict):
        return {key: without_cache_markers(item) for key, item in value.items() if key != "cache_control"}
    if isinstance(value, list):
        return [without_cache_markers(item) for item in value]
    return value


def run(binary, cache_metadata, api, preempt=False):
    requests = []
    request_lock = threading.Lock()
    summary_started = threading.Event()
    release_summary = threading.Event()

    class Model(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            with request_lock:
                requests.append(body)
                number = len(requests)
            payload = json.dumps(body["messages"])
            if CONTINUE in payload:
                answer = "CONTINUED_FROM_CHECKPOINT"
            elif number > 2:
                answer = SUMMARY
                summary_started.set()
                if preempt:
                    release_summary.wait(timeout=10)
            else:
                answer = FINAL_REPLY + " fixture-result" * 300
            chunks = [
                {"choices": [{"index": 0, "delta": {"role": "assistant", "content": answer}, "finish_reason": None}]},
                {"choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
                 "usage": {"prompt_tokens": 4000, "completion_tokens": 400,
                           "prompt_tokens_details": {"cached_tokens": 3000}}},
            ]
            wire = "".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n"
            if api == "anthropic-messages":
                chunks = [
                    {"type": "message_start", "message": {"id": "fixture", "type": "message", "role": "assistant",
                     "model": "chat", "content": [], "stop_reason": None,
                     "usage": {"input_tokens": 1000, "output_tokens": 0, "cache_read_input_tokens": 3000}}},
                    {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
                    {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": answer}},
                    {"type": "content_block_stop", "index": 0},
                    {"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "usage": {"output_tokens": 400}},
                    {"type": "message_stop"},
                ]
                wire = "".join(f"event: {chunk['type']}\ndata: {json.dumps(chunk)}\n\n" for chunk in chunks)
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(wire.encode())))
            self.end_headers()
            try:
                self.wfile.write(wire.encode())
            except (BrokenPipeError, ConnectionResetError):
                pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Model)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-idle-compact-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            model = {"id": "chat", "contextWindow": 128000, "maxTokens": 1024}
            if cache_metadata:
                model["promptCache"] = {"short": 11}
            (agent / "models.json").write_text(json.dumps({"providers": {"idle-fixture": {
                "api": api, "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "fixture", "compat": {"promptCacheKeyMode": "enabled"},
                "models": [model],
            }}}))
            (agent / "settings.json").write_text(json.dumps({
                "compaction": {"idleEnabled": True, "idleDelaySeconds": 1,
                               "keepRecentTokens": 100, "reserveTokens": 1024},
                "cacheWarming": "off", "retry": {"enabled": False},
            }))
            env = isolated_env(root, agent)
            process = subprocess.Popen([str(binary), "--no-extensions", "--no-context-files", "--no-skills",
                                        "--offline", "--mode", "rpc", "--model", "idle-fixture/chat"],
                                       cwd=root, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                       stderr=subprocess.PIPE, text=True)
            events = queue.Queue()
            seen = []

            def event_labels():
                return [(event.get("type"), event.get("command"), event.get("reason")) for event in seen[-8:]]

            def read_events():
                for line in process.stdout:
                    try:
                        events.put(json.loads(line))
                    except json.JSONDecodeError:
                        pass

            reader = threading.Thread(target=read_events, daemon=True)
            reader.start()

            def command(kind, **fields):
                process.stdin.write(json.dumps({"id": kind, "type": kind, **fields}) + "\n")
                process.stdin.flush()

            def wait_for(predicate, timeout=20):
                deadline = time.monotonic() + timeout
                while True:
                    remaining = deadline - time.monotonic()
                    if remaining <= 0:
                        raise AssertionError(f"event missing; requests={len(requests)}; events={event_labels()}")
                    try:
                        event = events.get(timeout=remaining)
                    except queue.Empty as error:
                        raise AssertionError(f"event missing; requests={len(requests)}; events={event_labels()}") from error
                    seen.append(event)
                    assert not (event.get("type") == "response" and event.get("success") is False), event
                    if predicate(event):
                        return event

            try:
                for index in range(2):
                    command("prompt", message=f"OLD_FIXTURE_{index} " + "remember-fixture " * 300)
                    wait_for(lambda event: event.get("type") == "agent_end")
                command("get_state")
                state = wait_for(lambda event: event.get("command") == "get_state")
                if preempt:
                    assert summary_started.wait(timeout=10), "idle summary never started"
                    command("prompt", message=CONTINUE)
                    ended = wait_for(lambda event: event.get("type") == "compaction_end" and event.get("reason") == "idle")
                    release_summary.set()
                    assert ended.get("aborted"), ended
                    wait_for(lambda event: event.get("type") == "agent_end")
                    entries = [json.loads(line) for line in Path(state["data"]["sessionFile"]).read_text().splitlines()]
                    assert not any(entry.get("type") == "compaction" for entry in entries), "cancelled summary committed"
                    with request_lock:
                        assert CONTINUE in json.dumps(requests[-1]["messages"]), "incoming RPC prompt was lost"
                    print(f"PASS: {api} incoming RPC prompt cancelled the idle request without losing input or committing a stale checkpoint")
                    return
                ended = wait_for(lambda event: event.get("type") == "compaction_end" and event.get("reason") == "idle")
                assert ended.get("result", {}).get("summary") == SUMMARY and not ended.get("aborted") and not ended.get("errorMessage"), ended
                with request_lock:
                    assert len(requests) == 3, f"expected two turns and one idle summary, got {len(requests)}"
                    before, summary = requests[1], requests[2]
                assert summary["model"] == before["model"], (before["model"], summary["model"])
                prefix = summary["messages"][:len(before["messages"])]
                assert without_cache_markers(prefix) == without_cache_markers(before["messages"]), "idle summary rewrote the cached prefix"
                for field in ("tools", "system", "prompt_cache_key", "prompt_cache_retention", "reasoning_effort", "max_tokens"):
                    assert without_cache_markers(summary.get(field)) == without_cache_markers(before.get(field)), f"idle summary changed {field}"
                if api == "openai-completions":
                    assert before.get("prompt_cache_key"), "fixture must exercise session affinity"
                appended = summary["messages"][len(before["messages"]):]
                assert any(message.get("role") == "assistant" and FINAL_REPLY in json.dumps(message)
                           for message in appended), "idle summary omitted the final assistant response"
                path = Path(state["data"]["sessionFile"])
                entries = [json.loads(line) for line in path.read_text().splitlines()]
                compactions = [entry for entry in entries if entry.get("type") == "compaction"]
                assert len(compactions) == 1 and compactions[0]["summary"] == SUMMARY, compactions
                command("prompt", message=CONTINUE)
                wait_for(lambda event: event.get("type") == "agent_end")
                with request_lock:
                    continuation = requests[-1]
                payload = json.dumps(continuation["messages"])
                assert SUMMARY in payload and CONTINUE in payload, payload
                assert "OLD_FIXTURE_0" not in payload, "continuation reloaded summarized history"
                timing = "cache metadata" if cache_metadata else "configured fallback"
                print(f"PASS: {api} {timing} idle checkpoint preserved the prefix, saved the final response, and reduced continuation context")
            finally:
                release_summary.set()
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                reader.join(timeout=5)
                process.stdin.close()
                process.stdout.close()
                process.stderr.close()
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    for api in ("openai-completions", "anthropic-messages"):
        for cache_metadata in (True, False):
            run(args.binary.resolve(), cache_metadata, api)
    run(args.binary.resolve(), False, "openai-completions", preempt=True)
