#!/usr/bin/env python3
"""Prove every /goal command paints its feedback in both TUI modes.

The model never calls the goal tool, so each state in this run is reachable only
through the slash command. Each assertion reads the painted renderer frame, not
bare output bytes, so the check observes what a user sees.

Every assertion therefore reads a frame, the region between the renderer's
synchronized-update markers, and never a bare byte match. A repaint is forced by
changing the terminal width and raising SIGWINCH, which makes both the fullscreen
and the main-screen renderer redraw the screen; a write outside a frame is not
part of that redraw, so it does not survive and is reported as unpainted. The
toggle key is not the oracle because it repaints only the widget region in one of
the two modes.

Each reporting command runs in its own session, because a goal one command
started would still be in the next one's transcript.

The objective is typed on two lines in mixed case. Case must survive verbatim;
whitespace is compared normalized because the command joins its arguments.

State tokens are the compact status fragments goalext renders today (`· active ·`,
`· paused ·`, `· complete ·`). A drop leaves no state to print, so the test accepts
either the past-tense notice or the tracker's empty-state line.
"""
import contextlib
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time

BEGIN, END = b"\x1b[?2026h", b"\x1b[?2026l"
IDLE = "No active goal."
ACTIVE, PAUSED, COMPLETE = "· active ·", "· paused ·", "· complete ·"
DROPPED = ("dropped", IDLE)
OBJECTIVE_LINES = ("Fix The Parser", "and Keep CASING")
OBJECTIVE = " ".join(OBJECTIVE_LINES)

requests = 0
lock = threading.Lock()
release = threading.Event()


class ModelServer(http.server.BaseHTTPRequestHandler):
    """Answer every request with text, holding the first until released."""

    objective = None

    def log_message(self, *_):
        """Stay quiet; assertions report failures."""

    def do_POST(self):
        """Count the request, then answer with an OpenAI-compatible SSE stream."""
        global requests
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        with lock:
            index = requests
            requests += 1
        if index == 0:
            type(self).objective = first_user_text(body)
            release.wait(120)
        chunks = [
            {"id": "goal", "choices": [{"index": 0, "delta": {"role": "assistant",
                "content": "Fixture turn finished."}, "finish_reason": None}]},
            {"id": "goal", "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n"
        try:
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(wire.encode())))
            self.end_headers()
            self.wfile.write(wire.encode())
        except OSError:
            pass


def first_user_text(body):
    """Return the text of the request's first user message."""
    for message in body.get("messages") or []:
        if message.get("role") != "user":
            continue
        content = message.get("content")
        if isinstance(content, list) and content:
            content = content[0].get("text")
        return content
    return None


def request_count():
    """Return how many provider requests have arrived so far."""
    with lock:
        return requests


def reset_requests():
    """Start counting from zero for a new session."""
    global requests
    with lock:
        requests = 0


def frames(data):
    """Return the bodies of the frames the renderer wrote."""
    out, cursor = [], 0
    while True:
        start = data.find(BEGIN, cursor)
        if start < 0:
            return out
        stop = data.find(END, start)
        if stop < 0:
            return out
        out.append(data[start + len(BEGIN):stop])
        cursor = stop + len(END)


def plain(data):
    """Strip escape sequences and flatten text to what a reader sees."""
    data = re.sub(rb"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)", b"", data)
    data = re.sub(rb"\x1b[_P^][^\x07\x1b]*(?:\x07|\x1b\\)", b"", data)
    data = re.sub(rb"\x1b\[[0-9;]*[Hf]", b"\n", data)
    data = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", data)
    return " ".join(data.decode("utf-8", "replace").split())


def painted(data):
    """Return the text the renderer painted in a window.

    A repaint window holds the full redraw and then one small diff frame per
    spinner tick, so the frames are joined: the tick frames carry almost nothing.
    A write outside every frame is what this excludes, and that is exactly the
    text a repaint does not reproduce.
    """
    return " ".join(text for text in (plain(body) for body in frames(data)) if text)


def unpainted(data):
    """Return the text written outside every frame."""
    chunks, cursor = [], 0
    while True:
        start = data.find(BEGIN, cursor)
        chunks.append(data[cursor:start if start >= 0 else len(data)])
        if start < 0:
            break
        stop = data.find(END, start)
        if stop < 0:
            break
        cursor = stop + len(END)
    return " ".join(text for text in (plain(chunk) for chunk in chunks) if text)


def expect(frame, tokens, label, mode, window=None):
    """Assert the painted frame carries every token.

    window, when given, holds the raw bytes, so a token that reached the terminal
    outside a frame is reported as that instead of as a missing feature.
    """
    missing = [token for token in tokens if token.lower() not in frame.lower()]
    if not missing:
        return
    escaped = [token for token in missing
               if window is not None and token.lower() in unpainted(window).lower()]
    cause = f"{escaped} was written outside the frame, so it is not painted UI" if escaped \
        else "nothing painted it"
    detail = f"\nunpainted: {unpainted(window)[-600:]!r}" if window is not None else ""
    raise AssertionError(f"{mode}: {label}: no frame showed {missing}; {cause}\n"
                         f"painted: {frame[-1000:]!r}{detail}")


def expect_any(frame, tokens, label, mode, invert=False):
    """Assert the painted frame carries one of tokens, or with invert none."""
    found = any(token.lower() in frame.lower() for token in tokens)
    if found != invert:
        return
    wanted = "none of" if invert else "any of"
    raise AssertionError(f"{mode}: {label}: the frame did not show {wanted} {tokens}\n"
                         f"painted: {frame[-1000:]!r}")


def expect_fresh(window, after, tokens, label, mode):
    """Assert that the command's result remains visible in a painted frame.

    Goal state is a persistent widget, so later reads can repaint the same state.
    """
    expect(after, tokens, label, mode, window)


class Session:
    """One isolated profile driving the real binary over a pseudo-terminal."""

    def __init__(self, binary, mode, port, root):
        self.mode, self.width = mode, 120
        agent = root / "agent"
        agent.mkdir()
        (agent / "models.json").write_text(json.dumps({"providers": {"goal-ui": {
            "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{port}/v1",
            "apiKey": "fixture", "models": [{"id": "goal-ui", "name": "goal-ui",
                                             "contextWindow": 128000, "maxTokens": 1024}]}}}))
        (agent / "settings.json").write_text(json.dumps({"quietStartup": True,
            "tuiMode": mode, "enableInstallTelemetry": False}))
        env = dict(os.environ, HOME=str(root), PIG_HOME=str(root), PI_HOME=str(root),
                   PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                   PIG_STARTUP_TRACE="1", TERM="xterm-256color")
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        for key in [key for key in env if key.startswith("OMPIG_BROKER_")]:
            del env[key]
        self.master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, self.width, 0, 0))
        self.process = subprocess.Popen(
            [str(binary), "--no-extensions", "--provider", "goal-ui", "--model", "goal-ui",
             "--no-context-files", "--no-skills", "--offline"],
            stdin=slave, stdout=slave, stderr=slave, cwd=root, env=env, start_new_session=True)
        os.close(slave)
        self.captured = bytearray()
        self.frame = ""

    def pump(self, seconds, quiesce=None, settled=None):
        """Read for seconds, until quiesce seconds pass quietly, or until settled.

        settled is checked against everything read since this call started, so a
        caller can stop on the frame it is waiting for instead of waiting out a
        spinner that never stops repainting.
        """
        mark = len(self.captured)
        deadline = time.monotonic() + seconds
        quiet_since = time.monotonic()
        while time.monotonic() < deadline:
            if select.select([self.master], [], [], 0.05)[0]:
                try:
                    self.captured.extend(os.read(self.master, 262144))
                except OSError:
                    return
                quiet_since = time.monotonic()
                if settled is not None and settled(bytes(self.captured[mark:])):
                    return
            elif quiesce is not None and time.monotonic() - quiet_since >= quiesce:
                return

    def wait_ready(self, seconds=90):
        """Wait for the startup mark the harness prints once the prompt is live."""
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            self.pump(0.1)
            if b"interactive-ready" in self.captured:
                return
            if self.process.poll() is not None:
                break
        raise AssertionError(f"{self.mode}: the prompt never became ready; "
                             f"exit={self.process.poll()}\n{self.tail()}")

    def repaint(self, seconds=6):
        """Force a repaint, adopt its frame, and return every byte it wrote."""
        mark = len(self.captured)
        self.width = 100 if self.width == 120 else 120
        fcntl.ioctl(self.master, termios.TIOCSWINSZ, struct.pack("HHHH", 40, self.width, 0, 0))
        try:
            os.kill(self.process.pid, signal.SIGWINCH)
        except ProcessLookupError:
            pass
        # A full repaint is one frame; waiting for it beats waiting out the
        # spinner ticks that follow it.
        self.pump(seconds, settled=lambda data: len(frames(data)) > 0)
        self.pump(1, quiesce=0.3)
        window = bytes(self.captured[mark:])
        self.frame = painted(window)
        return window

    def command(self, line, seconds=6):
        """Type one slash command; return its bytes and the frame before and after."""
        before = self.frame
        mark = len(self.captured)
        os.write(self.master, line.encode() + b"\r")
        self.pump(seconds, quiesce=0.5)
        window = bytes(self.captured[mark:]) + self.repaint()
        return window, before, self.frame

    def submit_lines(self, prefix, lines):
        """Type a command whose argument spans lines, joined with the newline key."""
        os.write(self.master, (prefix + lines[0]).encode())
        for line in lines[1:]:
            self.pump(0.5)
            os.write(self.master, b"\x1b\r")
            self.pump(0.5)
            os.write(self.master, line.encode())
        os.write(self.master, b"\r")

    def close(self):
        """Terminate the child this session started and release the model."""
        release.set()
        if self.process.poll() is None:
            self.process.terminate()
        try:
            self.process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait()
        os.close(self.master)

    def tail(self, size=4000):
        """Return the end of the raw stream for a failure message."""
        return repr(bytes(self.captured[-size:]))


@contextlib.contextmanager
def session_for(binary, mode, server):
    """Open an isolated session, then remove its temporary profile."""
    with tempfile.TemporaryDirectory(prefix=f"ompig-goal-ui-{mode}-") as directory:
        reset_requests()
        release.clear()
        ModelServer.objective = None
        session = Session(binary, mode, server.server_port, Path(directory))
        try:
            session.wait_ready()
            session.repaint()
            expect(session.frame, ["Widgets"], "startup frame", mode)
            yield session
        finally:
            session.close()


REPORTING = (
    ("/goal", [IDLE], "idle /goal status"),
    ("/goal status", [IDLE], "/goal status alias"),
    ("/goal get", [IDLE], "/goal get alias"),
    ("/goal resume", ["no active goal"], "resume without a goal"),
    ("/goal help", ["Usage", "/goal", "pause", "resume", "complete", "drop"], "/goal help usage"),
)


def check_reporting(binary, mode, server):
    """Prove each reporting command paints its answer and contacts no provider.

    Each command gets its own session: a goal started by one command would still
    be in the next session's transcript, and a word already on screen cannot show
    that the second command painted anything.
    """
    for line, tokens, label in REPORTING:
        with session_for(binary, mode, server) as session:
            window, before, after = session.command(line)
            expect_fresh(window, after, tokens, label, mode)
            assert request_count() == 0, f"{mode}: {label} contacted a provider"
    print(f"PASS: {mode} frames showed {IDLE!r} for /goal, status, and get, "
          f"the usage line, and a visible resume error, each painted and with no "
          f"provider request")


def check_lifecycle(binary, mode, server):
    """Prove each lifecycle transition paints its own feedback, and a pause stops work."""
    with session_for(binary, mode, server) as session:
        before = session.frame
        session.submit_lines("/goal set ", OBJECTIVE_LINES)
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline and request_count() == 0:
            session.pump(0.2)
        assert request_count() == 1, f"{mode}: /goal set made {request_count()} provider requests"
        received = " ".join((ModelServer.objective or "").split())
        assert received == OBJECTIVE, \
            f"{mode}: the objective lost its case or its words: {ModelServer.objective!r}"
        window = session.repaint()
        expect_fresh(window, session.frame, [ACTIVE, OBJECTIVE],
                     "an active goal is shown", mode)

        window, before, after = session.command("/goal pause", seconds=6)
        expect_fresh(window, after, [PAUSED], "a paused goal is shown", mode)
        release.set()
        session.pump(6, quiesce=0.5)
        settled = request_count()
        session.pump(6, quiesce=0.5)
        assert settled == request_count() == 1, \
            f"{mode}: a paused goal kept working: {settled} -> {request_count()} requests"

        window, before, after = session.command("/goal complete")
        expect_fresh(window, after, [COMPLETE], "a completed goal is shown", mode)

        window, before, after = session.command("/goal drop")
        expect_any(after, DROPPED, "a dropped goal is announced", mode)
        expect_any(before, DROPPED, "a dropped goal is announced", mode, invert=True)
        assert request_count() == 1, f"{mode}: a command after the pause started work"

        print(f"PASS: {mode} frames showed {ACTIVE!r} with the objective, {PAUSED!r} "
              f"before the turn settled, {COMPLETE!r}, a new drop notice, no turn "
              f"after the pause, and the objective {OBJECTIVE!r} kept its case "
              f"across two lines")


def main():
    """Run both TUI modes against the caller's built binary, without paid APIs."""
    if len(sys.argv) != 2:
        raise SystemExit("usage: smoke-test-goal-ui.py <ompig-slim-binary>")
    binary = Path(sys.argv[1]).resolve()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        for mode in ("fullscreen", "regular"):
            check_reporting(binary, mode, server)
            check_lifecycle(binary, mode, server)
    finally:
        release.set()
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main()
