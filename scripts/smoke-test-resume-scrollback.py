#!/usr/bin/env python3
"""Verify resume scroll-back reaches pre-compaction history without feeding it to the model."""
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

TS = "2026-01-01T00:00:00.000Z"
CONTINUATION = "CONTINUATION-PROBE-7F3"
DONE_TOKEN = "RESUME-CONTINUED-OK"
SUMMARY_ONE = "COMPACTION-SUMMARY-ONE earlier work condensed"
SUMMARY_TWO = "COMPACTION-SUMMARY-TWO recent work condensed"

# MARKER_ORDER is branch order: OLD, compaction one's kept MID range, INTER, then
# compaction two's kept TAIL, the only messages a resume renders initially.
def turn_markers(family, turn):
    return f"MARKER-{family}-USER-{turn}", f"MARKER-{family}-ASSISTANT-{turn}"

OLD_TURNS, MID_TURNS, INTER_TURNS, TAIL_TURNS = 3, 2, 2, 3
MARKER_ORDER = []
for family, count in (("OLD", OLD_TURNS), ("MID", MID_TURNS), ("INTER", INTER_TURNS), ("TAIL", TAIL_TURNS)):
    for turn in range(count):
        MARKER_ORDER.extend(turn_markers(family, turn))
OFFBRANCH_MARKER = "MARKER-OFFBRANCH-USER-0"
OLDEST_MARKERS = turn_markers("OLD", 0)


def user_wire(text):
    return {"role": "user", "content": text, "timestamp": 1}


def assistant_wire(marker):
    text = marker + "\n" + "\n".join(f"fixture scrollback padding line {line}" for line in range(15))
    return {"role": "assistant", "api": "openai-completions", "provider": "scroll-fixture",
            "model": "scroll-fixture", "content": [{"type": "text", "text": text}],
            "stopReason": "stop", "timestamp": 1}


def write_session(path, cwd):
    """Write the two-compaction session fixture."""
    parent = None
    lines = [json.dumps({"type": "session", "version": 3, "id": "scrollback-fixture",
                         "cwd": str(cwd), "timestamp": TS})]

    def emit(entry_id, message):
        nonlocal parent
        lines.append(json.dumps({"type": "message", "id": entry_id, "parentId": parent,
                                 "timestamp": TS, "message": message}))
        parent = entry_id

    def emit_turn(family, turn):
        user_marker, assistant_marker = turn_markers(family, turn)
        emit(f"{family.lower()}-u-{turn}", user_wire(f"Earlier user request {user_marker}"))
        emit(f"{family.lower()}-a-{turn}", assistant_wire(assistant_marker))

    def emit_compaction(entry_id, first_kept, summary):
        nonlocal parent
        lines.append(json.dumps({"type": "compaction", "id": entry_id, "parentId": parent,
                                 "timestamp": TS, "firstKeptEntryId": first_kept,
                                 "summary": summary, "tokensBefore": 90000}))
        parent = entry_id

    for turn in range(OLD_TURNS):
        emit_turn("OLD", turn)
    # Kept messages precede their compaction entry, matching real session logs.
    for turn in range(MID_TURNS):
        emit_turn("MID", turn)
    emit_compaction("compact-one", "mid-u-0", SUMMARY_ONE)
    for turn in range(INTER_TURNS):
        emit_turn("INTER", turn)
    # An abandoned sibling fork must sit mid-file: appended last it would be the leaf.
    lines.append(json.dumps({"type": "message", "id": "offbranch-u-0", "parentId": "inter-a-0",
                             "timestamp": TS, "message": user_wire(f"Abandoned fork {OFFBRANCH_MARKER}")}))
    for turn in range(TAIL_TURNS):
        emit_turn("TAIL", turn)
    emit_compaction("compact-two", "tail-u-0", SUMMARY_TWO)
    path.write_text("".join(line + "\n" for line in lines))


def visible_text(data):
    """Strip CSI sequences so assertions read what the user sees."""
    return re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", data)


class ScrollbackModel(http.server.BaseHTTPRequestHandler):
    """Answer exactly one continuation and validate the history it carries."""

    def log_message(self, *_):
        """Keep fixture logging out of the terminal capture."""

    def do_POST(self):
        self.server.requests += 1
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        messages = body["messages"]
        payload = json.dumps(messages)
        failures = self.server.failures
        encoded = [json.dumps(m) for m in messages]
        continuation = next((i for i, m in enumerate(encoded)
                             if messages[i].get("role") == "user" and CONTINUATION in m), None)
        if continuation is None:
            failures.append("continuation prompt missing")
        if SUMMARY_TWO not in payload:
            failures.append("newest compaction summary missing from context")
        for kept in turn_markers("TAIL", 0):
            index = next((i for i, m in enumerate(encoded) if kept in m), None)
            if index is None:
                failures.append(f"kept tail marker {kept} missing from context")
            elif continuation is not None and index > continuation:
                failures.append(f"kept tail marker {kept} appears after the continuation")
        for marker in MARKER_ORDER:
            if marker.startswith("MARKER-TAIL"):
                continue
            if marker in payload:
                failures.append(f"summarized marker {marker} leaked into context")
        if OFFBRANCH_MARKER in payload:
            failures.append("off-branch fork leaked into context")
        if SUMMARY_ONE in payload:
            failures.append("superseded compaction summary leaked into context")
        chunks = [
            {"choices": [{"index": 0, "delta": {"role": "assistant", "content": DONE_TOKEN}, "finish_reason": None}]},
            {"choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())


def run(binary, timeout):
    with tempfile.TemporaryDirectory(prefix="ompig-scrollback-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        sessions = root / "sessions"
        sessions.mkdir()
        path = sessions / "history.jsonl"
        write_session(path, root)
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ScrollbackModel)
        server.requests, server.failures = 0, []
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        (agent / "models.json").write_text(json.dumps({"providers": {"scroll-fixture": {
            "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
            "apiKey": "fixture", "models": [{"id": "scroll-fixture", "contextWindow": 10000000, "maxTokens": 1024}]}}}))
        (agent / "settings.json").write_text(json.dumps({"quietStartup": True, "tuiMode": "fullscreen",
            "enableInstallTelemetry": False, "compaction": {"enabled": False}}))
        env = isolated_env(root, agent)
        env.update(PIG_STARTUP_TRACE="1", TERM="xterm-256color")
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        process = subprocess.Popen([str(binary), "--no-extensions", "--no-context-files", "--no-skills",
                                    "--offline", "--provider", "scroll-fixture", "--model", "scroll-fixture",
                                    "--session-dir", str(sessions), "--session", str(path)],
                                   stdin=slave, stdout=slave, stderr=slave,
                                   cwd=root, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()
        query_tail = b""

        def pump(duration):
            """Drain the PTY for duration, answering device-attribute queries."""
            nonlocal query_tail
            end = time.monotonic() + duration
            while time.monotonic() < end:
                if not select.select([master], [], [], 0.02)[0]:
                    continue
                try:
                    data = os.read(master, 262144)
                except OSError:
                    return
                if not data:
                    return
                query_scan = query_tail + data
                for _ in range(query_scan.count(b"\x1b[c")):
                    os.write(master, b"\x1b[?1;2c")
                query_tail = query_scan[-2:]
                captured.extend(data)

        def until(token, deadline, label):
            while token not in captured:
                if time.monotonic() > deadline:
                    raise AssertionError(f"{label}: {token!r} never appeared; "
                                         f"exit={process.poll()}; tail={bytes(captured[-3000:])!r}")
                if process.poll() is not None:
                    raise AssertionError(f"{label}: exited {process.returncode} before {token!r}; "
                                         f"tail={bytes(captured[-3000:])!r}")
                pump(0.05)
            return len(captured)

        try:
            deadline = time.monotonic() + timeout
            until(b"interactive-ready", deadline, "startup")
            pump(0.5)
            # No kept tail here would mean the fixture, not the loader, is broken.
            tail_u, tail_a = turn_markers("TAIL", TAIL_TURNS - 1)
            for marker in (tail_u, tail_a):
                until(marker.encode(), deadline, "initial kept-tail render")

            # PageUp reaches an incremental loader's older chunks; the ctrl+home
            # bytes cover a loader that fills history only at the top.
            oldest = tuple(m.encode() for m in OLDEST_MARKERS)
            attempts = stall = 0
            seen_count = len([m for m in MARKER_ORDER if m.encode() in captured])
            while any(marker not in captured for marker in oldest):
                found = len([m for m in MARKER_ORDER if m.encode() in captured])
                stall = 0 if found > seen_count else stall + 1
                seen_count = max(seen_count, found)
                if attempts >= 40 or stall >= 12 or time.monotonic() > deadline:
                    seen = [m for m in MARKER_ORDER if m.encode() in captured]
                    raise AssertionError(
                        f"scroll-back never reached {OLDEST_MARKERS[0]}: "
                        f"{attempts} PageUp/ctrl+home attempts, markers seen={seen}, "
                        f"tail={bytes(visible_text(bytes(captured[-4000:])))!r}")
                os.write(master, b"\x1b[5~\x1b[5~\x1b[7^")
                pump(0.35)
                attempts += 1

            # One repaint of the top: each marker it holds appears once, in order.
            before = len(captured)
            os.write(master, b"\x1b[7^")
            pump(0.5)
            frame = visible_text(bytes(captured[before:]))
            shown = [m for m in MARKER_ORDER if m.encode() in frame]
            positions = [frame.index(m.encode()) for m in shown]
            assert positions == sorted(positions), \
                f"markers out of document order in top frame: {shown}"
            for marker in shown:
                assert frame.count(marker.encode()) == 1, f"{marker} duplicated in top frame"

            # Descend so a jump-to-top loader also paints the regions it skipped.
            missing = [m for m in MARKER_ORDER if m.encode() not in captured]
            descents = 0
            while missing and descents < 40 and time.monotonic() < deadline:
                os.write(master, b"\x1b[6~")
                pump(0.25)
                descents += 1
                missing = [m for m in MARKER_ORDER if m.encode() not in captured]
            assert not missing, f"scroll-back never painted {missing}"
            assert OFFBRANCH_MARKER.encode() not in captured, "off-branch fork rendered"

            # The scrolled-back history is view-only: the model must receive
            # only the kept tail and the newest summary.
            os.write(master, b"\x1b[8^")
            pump(0.3)
            os.write(master, CONTINUATION.encode() + b" confirm context")
            pump(0.2)
            os.write(master, b"\r")
            until(DONE_TOKEN.encode(), deadline, "continuation")
            assert server.requests == 1, f"expected one model request, got {server.requests}"
            assert not server.failures, f"model context violations: {server.failures}"

            os.write(master, b"\x03\x04")
            exit_deadline = time.monotonic() + 5
            while process.poll() is None and time.monotonic() < exit_deadline:
                pump(0.05)
        finally:
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
        assert process.returncode == 0, f"unclean exit {process.returncode}"


def main():
    parser = argparse.ArgumentParser(description=__doc__,
                                   formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--timeout", type=float, default=60)
    args = parser.parse_args()
    run(str(args.binary.resolve()), args.timeout)
    print("PASS: scroll-back revealed pre-compaction transcript in order, "
          "kept the fork out, and the continuation sent only kept context")


if __name__ == "__main__":
    main()
