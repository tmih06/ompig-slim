#!/usr/bin/env python3
"""Verify idle countdowns and persisted compaction cards through an isolated PTY."""
import argparse
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import struct
import subprocess
import tempfile
import termios
import threading
import time

from benchmark import isolated_env

SUMMARY = "COMPACTION_INFO_CHECKPOINT"
PROVIDER = "compact-info"
STAMP = "2026-01-01T00:00:00.000Z"


def visible(data):
    return re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", data).decode(errors="replace")


def write_history(path, root):
    entries = [{"type": "session", "version": 3, "id": "compaction-info",
                "cwd": str(root), "timestamp": STAMP}]
    parent = None
    for turn in range(3):
        for role in ("user", "assistant"):
            entry_id = f"{role}-{turn}"
            # A real session keeps the model-identity message the modelcontext
            # extension injects ahead of the turn's user message; without it the
            # first turn rewrites its context prefix and the idle offer
            # legitimately declines.
            if turn == 2 and role == "user":
                entries.append({"type": "custom_message", "customType": "model-identity",
                                "content": "Selected provider: compact-info\nSelected model: chat",
                                "display": False, "id": "identity", "parentId": parent,
                                "timestamp": STAMP})
                parent = "identity"
            text = f"HISTORY_{turn} " + "remember previous work " * 200
            message = {"role": role, "content": text, "timestamp": 1}
            if role == "assistant":
                message.update(api="openai-completions", provider=PROVIDER, model="chat",
                               content=[{"type": "text", "text": text}], stopReason="stop")
            entries.append({"type": "message", "id": entry_id, "parentId": parent,
                            "timestamp": STAMP, "message": message})
            parent = entry_id
    path.write_text("".join(json.dumps(entry) + "\n" for entry in entries))


class Terminal:
    def __init__(self, binary, root, agent, session):
        env = isolated_env(root, agent)
        env.update(TERM="xterm-256color", PIG_STARTUP_TRACE="1")
        self.master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 140, 0, 0))
        self.process = subprocess.Popen(
            [str(binary), "--no-extensions", "--no-context-files", "--no-skills", "--offline",
             "--model", f"{PROVIDER}/chat", "--session", str(session)],
            stdin=slave, stdout=slave, stderr=slave, cwd=root, env=env, start_new_session=True)
        os.close(slave)
        self.captured = bytearray()
        self.query_tail = b""

    def send(self, packet):
        os.write(self.master, packet)

    def pump(self, seconds):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            if not select.select([self.master], [], [], min(.05, max(0, end - time.monotonic())))[0]:
                continue
            try:
                data = os.read(self.master, 262144)
            except OSError:
                return
            if not data:
                return
            query = self.query_tail + data
            for _ in range(query.count(b"\x1b[c")):
                self.send(b"\x1b[?1;2c")
            self.query_tail = query[-2:]
            self.captured.extend(data)

    def until(self, predicate, label, start=0, timeout=20):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            text = visible(bytes(self.captured[start:]))
            if predicate(text):
                return text
            if self.process.poll() is not None:
                break
            self.pump(.05)
        raise AssertionError(f"{label}; exit={self.process.poll()}; "
                             f"terminal tail={visible(bytes(self.captured[-4500:]))!r}")

    def close(self):
        if self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
        os.close(self.master)


def run(binary, mode, idle, metadata=False, compact_role=False):
    requests = []
    lock = threading.Lock()

    class Model(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            with lock:
                requests.append(body)
            model = body["model"]
            if model in ("primary", "second"):
                self.send_response(429)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b'{"error":{"message":"billing quota exceeded","type":"insufficient_quota"}}')
                return
            summary = model == "third" or "structured context checkpoint" in json.dumps(body["messages"])
            if summary:
                time.sleep(.35)
            answer = SUMMARY if summary else "CHAT_FINISHED " + "retained reply " * 200
            reported_model = "reported-" + model if summary else model
            chunks = [
                {"model": reported_model, "choices": [{"index": 0, "delta": {
                    "role": "assistant", "content": answer}, "finish_reason": None}]},
                {"model": reported_model, "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
                 "usage": {"prompt_tokens": 4000, "completion_tokens": 400}},
            ]
            wire = ("".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks)
                    + "data: [DONE]\n\n").encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(wire)))
            self.end_headers()
            try:
                self.wfile.write(wire)
            except (BrokenPipeError, ConnectionResetError):
                pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Model)
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-compaction-info-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            session = root / "history.jsonl"
            write_history(session, root)
            models = [{"id": name, "contextWindow": 128000, "maxTokens": 1024}
                      for name in ("chat", "primary", "second", "third")]
            if metadata:
                models[0]["promptCache"] = {"short": 15}
            (agent / "models.json").write_text(json.dumps({"providers": {PROVIDER: {
                "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "fixture", "models": models}}}))
            settings = {"quietStartup": True, "tuiMode": mode, "cacheWarming": "off",
                        "compaction": {"idleEnabled": idle, "idleDelaySeconds": 5,
                                       "keepRecentTokens": 100, "reserveTokens": 1024},
                        "retry": {"maxRetries": 0, "fallbackChains": {
                            "compact": [f"{PROVIDER}/second:off", f"{PROVIDER}/third:off"]}}}
            if compact_role or not idle:
                settings["modelRoles"] = {"default": f"{PROVIDER}/chat", "compact": f"{PROVIDER}/primary"}
            (agent / "settings.json").write_text(json.dumps(settings))
            terminal = Terminal(binary, root, agent, session)
            try:
                terminal.until(lambda text: "interactive-ready" in text, "startup")
                if idle:
                    terminal.send(b"OBSERVE_IDLE_ONE\r")
                    terminal.until(lambda text: "CHAT_FINISHED" in text, "first reply")
                    countdown_start = len(terminal.captured)
                    text = terminal.until(
                        lambda text: len(set(re.findall(r"Idle compaction check in (\d+)s", text))) >= 2,
                        "countdown did not decrease", start=countdown_start)
                    values = [int(value) for value in re.findall(r"Idle compaction check in (\d+)s", text)]
                    assert any(left > right for left, right in zip(values, values[1:])), values
                    reset_start = len(terminal.captured)
                    terminal.send(b"RESET_IDLE_COUNTDOWN\r")
                    terminal.until(lambda text: "CHAT_FINISHED" in text, "reset reply", start=reset_start)
                    terminal.until(lambda text: any(int(value) >= 4 for value in re.findall(
                        r"Idle compaction check in (\d+)s", text)), "new request did not reset countdown", start=reset_start)
                else:
                    terminal.pump(1.2)
                    assert "Idle compaction check" not in visible(bytes(terminal.captured)), "disabled idle policy showed a countdown"
                    terminal.send(b"/compact\r")
                completed = terminal.until(lambda text: "Compacted from" in text and "reported-" in text,
                                           "completed card missing model")
                entries = [json.loads(line) for line in session.read_text().splitlines()]
                compacted = [entry for entry in entries if entry.get("type") == "compaction"]
                assert len(compacted) == 1, compacted
                info = compacted[0].get("info")
                reason = "idle" if idle else "manual"
                model = f"{PROVIDER}/reported-{'chat' if idle and not compact_role else 'third'}"
                assert info and info["reason"] == reason and info["models"] == [model], info
                assert 300 <= info["elapsedMs"] < 30000, info
                assert reason in completed and model in completed, completed[-3000:]
                assert re.search(r"\d+(?:\.\d+)?s", completed), "card missing elapsed duration"
                with lock:
                    requested_models = [request["model"] for request in requests]
                # runDefaultCompaction makes one history call and one turn-prefix
                # call on the surviving fallback; the captured-prefix idle path
                # makes a single summary request.
                expected_models = ["primary", "second", "third", "third"]
                if idle:
                    expected_models = ["chat", "chat"] + (expected_models if compact_role else ["chat"])
                assert requested_models == expected_models, requested_models
                terminal.send(b"\x0f")
                # Fullscreen repaints only changed rows, so the already-drawn
                # metadata row is not emitted again; match the whole capture.
                terminal.until(lambda text: SUMMARY in text and model in text,
                               "expanded card lost metadata or summary")
                terminal.pump(1.2)
                settled_start = len(terminal.captured)
                terminal.pump(1.2)
                assert "Idle compaction check" not in visible(bytes(terminal.captured[settled_start:])), "consumed offer countdown returned"
            except AssertionError as error:
                with lock:
                    models_called = [request["model"] for request in requests]
                raise AssertionError(f"{error}; requested models={models_called}") from error
            finally:
                terminal.close()
            resumed = Terminal(binary, root, agent, session)
            try:
                resumed.until(lambda text: "interactive-ready" in text, "resumed startup")
                resumed.send(b"\x1b[7^")
                resumed.until(lambda text: model in text and reason in text,
                              "resumed card lost compaction info")
                saved = [json.loads(line) for line in session.read_text().splitlines()]
                assert [entry["info"] for entry in saved if entry.get("type") == "compaction"] == [info]
            finally:
                resumed.close()
            policy = "cache metadata" if metadata else "fallback delay"
            print(f"PASS: {mode} {reason} {policy} countdown/card, actual model, elapsed, expansion, and resume")
    finally:
        server.shutdown()
        server.server_close()
        server_thread.join(timeout=5)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--mode", choices=("regular", "fullscreen"))
    args = parser.parse_args()
    for mode in ([args.mode] if args.mode else ("regular", "fullscreen")):
        run(args.binary.resolve(), mode, idle=True)
        run(args.binary.resolve(), mode, idle=False)
    if args.mode is None:
        run(args.binary.resolve(), "fullscreen", idle=True, metadata=True)
        run(args.binary.resolve(), "fullscreen", idle=True, compact_role=True)


if __name__ == "__main__":
    main()
