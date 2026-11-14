#!/usr/bin/env python3
"""Measure old-session resume through editing and a history-checked model request."""
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


class ResumeModel(http.server.BaseHTTPRequestHandler):
    """Accept one continuation only when the saved conversation reached the model."""

    def log_message(self, *_):
        """Keep HTTP logging out of terminal timings."""

    def do_POST(self):
        """Record arrival, validate history, and return an immediate SSE answer."""
        self.server.requested = time.perf_counter()
        body = self.rfile.read(int(self.headers["Content-Length"]))
        messages = json.loads(body)["messages"]
        history = json.dumps(messages[:-1])
        seen = {int(turn) for turn in re.findall(r"RESUME_HISTORY_(\d+)", history)}
        self.server.failures = []
        if seen != set(range(self.server.first_turn, self.server.turns)):
            self.server.failures.append("saved conversation is incomplete")
        if "resume-edit-probe" not in json.dumps(messages[-1]):
            self.server.failures.append("continuation prompt missing")
        chunks = [
            {"choices": [{"index": 0, "delta": {"role": "assistant", "content": "RESUME_CONTINUED"}, "finish_reason": None}]},
            {"choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())


def write_session(path, cwd, turns, kept_turns=None):
    """Write matched calls/results, optionally compacting all but kept_turns.

    The first kept turn precedes its compaction entry, matching real session
    logs; every later turn remains on that same branch.
    """
    parent = None
    with path.open("w") as stream:
        stream.write(json.dumps({"type": "session", "version": 3, "id": "resume-fixture",
                                 "cwd": str(cwd), "timestamp": "2026-01-01T00:00:00.000Z"}) + "\n")
        for turn in range(turns):
            messages = [
                {"role": "user", "content": f"Inspect fixture {turn}.", "timestamp": 1},
                {"role": "assistant", "api": "openai-completions", "provider": "resume-proof", "model": "resume-proof",
                 "content": [{"type": "toolCall", "id": f"read-{turn}", "name": "read", "arguments": {"path": "fixture.go"}}],
                 "stopReason": "toolUse", "timestamp": 1},
                {"role": "toolResult", "toolCallId": f"read-{turn}", "toolName": "read", "isError": False,
                 "content": [{"type": "text", "text": "package fixture\n" + "// historical read output\n" * 160}], "timestamp": 1},
                {"role": "assistant", "api": "openai-completions", "provider": "resume-proof", "model": "resume-proof",
                 "content": [{"type": "text", "text": f"RESUME_HISTORY_{turn}\n\n" + "Review **saved history** and `fixture.go`.\n\n" * 24}],
                 "stopReason": "stop", "timestamp": 1},
            ]
            for index, message in enumerate(messages):
                entry_id = f"{turn * 4 + index:08x}"
                stream.write(json.dumps({"type": "message", "id": entry_id, "parentId": parent,
                                         "timestamp": "2026-01-01T00:00:00.000Z", "message": message}) + "\n")
                parent = entry_id
            if kept_turns is not None and turn == turns - kept_turns:
                stream.write(json.dumps({"type": "compaction", "id": "compacted", "parentId": parent,
                    "timestamp": "2026-01-01T00:00:00.000Z", "firstKeptEntryId": f"{turn * 4:08x}",
                    "summary": "Earlier fixture work was summarized.", "tokensBefore": 100000}) + "\n")
                parent = "compacted"


def measure(binary, mode, turns, timeout, via_picker=False, kept_turns=None):
    """Time real edits and the first request, isolated from profiles and credentials.

    Direct resume starts at exec; picker resume starts when /resume is entered
    and includes listing/selection. Fixture generation and clean shutdown are
    excluded. A missing history marker or timeout fails rather than timing an
    incomplete continuation. Only synthetic fixture output is captured.
    """
    with tempfile.TemporaryDirectory(prefix="ompig-resume-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        sessions = root / "sessions"
        sessions.mkdir()
        path = sessions / "history.jsonl"
        write_session(path, root, turns, kept_turns)
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ResumeModel)
        server.turns, server.requested, server.failures = turns, None, []
        server.first_turn = turns - kept_turns if kept_turns is not None else 0
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        (agent / "models.json").write_text(json.dumps({"providers": {"resume-proof": {
            "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
            "apiKey": "fixture", "models": [{"id": "resume-proof", "contextWindow": 10000000, "maxTokens": 1024}]}}}))
        (agent / "settings.json").write_text(json.dumps({"quietStartup": True, "tuiMode": mode,
            "enableInstallTelemetry": False, "compaction": {"enabled": False}}))
        env = isolated_env(root, agent)
        env.update(PIG_STARTUP_TRACE="1", TERM="xterm-256color")
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        started = time.perf_counter()
        command = [str(binary), "--no-extensions", "--no-context-files", "--no-skills", "--offline",
                   "--provider", "resume-proof", "--model", "resume-proof", "--session-dir", str(sessions)]
        if not via_picker:
            command += ["--session", str(path)]
        process = subprocess.Popen(command,
            stdin=slave, stdout=slave, stderr=slave, cwd=root, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()
        terminal_bytes = 0
        checkpoints, checkpoint_seen, trace_tail = [], set(), b""
        typed = submitted = picker_opened = selected = False
        editable = ready = completed = None
        try:
            while time.perf_counter() - started < timeout:
                if select.select([master], [], [], .005)[0]:
                    try:
                        data = os.read(master, 262144)
                    except OSError:
                        break
                    if not data:
                        break
                    terminal_bytes += len(data)
                    # Keep token searches bounded: rescanning the accumulated
                    # scrollback on every PTY packet would throttle the child
                    # and measure this reader instead of resume latency.
                    captured.extend(data)
                    del captured[:-65536]
                    trace_scan = trace_tail + data
                    for checkpoint in re.findall(rb"\[startup\][^\r\n]*?\d+ms", trace_scan):
                        if checkpoint not in checkpoint_seen:
                            checkpoints.append(checkpoint.decode())
                            checkpoint_seen.add(checkpoint)
                    trace_tail = trace_scan[-128:]
                    now = time.perf_counter()
                    if via_picker:
                        if not picker_opened and b"interactive-ready" in captured:
                            started = now
                            captured.clear()
                            terminal_bytes = 0
                            checkpoints.clear()
                            checkpoint_seen.clear()
                            trace_tail = b""
                            os.write(master, b"/resume\r")
                            picker_opened = True
                        elif picker_opened and not selected and b"Inspect fixture 0." in captured:
                            os.write(master, b"\r")
                            selected = True
                        if not selected or (ready is None and b"Resumed session" not in captured):
                            continue
                        if ready is None:
                            ready = now
                    if not typed and (via_picker or b"raw-mode-entered" in captured):
                        os.write(master, b"resume-edit-probe")
                        typed = True
                    if typed and editable is None and b"resume-edit-probe" in captured:
                        editable = now
                    if ready is None and b"interactive-ready" in captured:
                        ready = now
                    # Submit only after init, so measuring a pre-init buffered
                    # prompt cannot be mistaken for a usable continuation.
                    if editable is not None and ready is not None and not submitted:
                        os.write(master, b"\r")
                        submitted = True
                    if b"RESUME_CONTINUED" in captured:
                        completed = now
                        break
                if process.poll() is not None:
                    break
        finally:
            if completed is not None and process.poll() is None:
                os.write(master, b"\x03\x04")
                deadline = time.monotonic() + 5
                while process.poll() is None and time.monotonic() < deadline:
                    if select.select([master], [], [], .01)[0]:
                        try:
                            if not os.read(master, 262144):
                                break
                        except OSError:
                            break
            if process.poll() is None:
                process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            os.close(master)
            server.shutdown()
            server.server_close()
            thread.join()
        if completed is None or server.requested is None or server.failures:
            raise AssertionError(f"resume failed: editable={editable is not None}, ready={ready is not None}, "
                                 f"history failures={server.failures}; terminal tail={bytes(captured[-3000:])!r}")
        return {"mode": mode, "via_picker": via_picker, "turns": turns, "kept_turns": kept_turns,
                "session_bytes": path.stat().st_size,
                "editable_ms": round((editable - started) * 1000, 2),
                "ready_ms": round((ready - started) * 1000, 2),
                "request_ms": round((server.requested - started) * 1000, 2),
                "completed_ms": round((completed - started) * 1000, 2),
                "terminal_bytes": terminal_bytes,
                "checkpoints": checkpoints}


def main():
    """Print repeatable measurements; an optional budget fails slow continuations."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--mode", choices=["regular", "fullscreen"], default="fullscreen")
    parser.add_argument("--turns", type=int, default=500)
    parser.add_argument("--kept-turns", type=int, help="Compact history, keeping this many recent turns")
    parser.add_argument("--via-picker", action="store_true", help="Time /resume selection in an already running TUI instead of exec")
    parser.add_argument("--runs", type=int, default=3)
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("--max-ms", type=float, help="Fail if the first model request exceeds this elapsed time")
    args = parser.parse_args()
    if min(args.turns, args.runs, args.timeout) <= 0:
        parser.error("turns, runs and timeout must be positive")
    if args.kept_turns is not None and not 1 <= args.kept_turns <= args.turns:
        parser.error("kept-turns must be between 1 and turns")
    for run in range(args.runs):
        result = measure(args.binary.resolve(), args.mode, args.turns, args.timeout, args.via_picker, args.kept_turns)
        print(json.dumps({"run": run + 1, **result}), flush=True)
        if args.max_ms is not None and result["request_ms"] > args.max_ms:
            raise SystemExit(f"resume-to-request exceeded {args.max_ms} ms")


if __name__ == "__main__":
    main()
