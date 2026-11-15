#!/usr/bin/env python3
"""Prove the goal blocker policy through the shipped binary and a loopback model.

The model alternates a goal tool call with a text-only answer, so every report
ends its own turn and the settle boundary decides whether to poke. That is the
boundary the policy exists for; a single run that calls the tool repeatedly would
never reach it.

The scripted order is two reports of one blocker, an explicit progress report, a
different blocker, then three reports of the first blocker again. Only those last
three are consecutive, so the goal must pause exactly there.

What the run proves, read from the built binary rather than a helper:

1. The provider sees one request pair per turn and none after the goal pauses, so
   a paused goal is not poked again.
2. The pause notice is persisted once, with the report that crossed the threshold.
   Two status reads append no outcome and reach no provider.
3. Status reports the blocker and its consecutive count.
4. Explicit resume starts work again and keeps the blocker as the explanation of
   the pause, with the streak cleared.

The terminal proves the notice a human sees. The persisted transcript proves the
state machine, because it records every outcome in order.
"""
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time

BLOCKER = "needs the staging API token"
OTHER_BLOCKER = "needs a decision on the migration order"
NOTICE = f"Goal paused after 3 reports of the same blocker: {BLOCKER}"

# One entry per goal outcome, each followed by the text that ends its turn. The
# progress report keeps a read-only turn: it runs a shell command that writes no
# file, which must still count as progress rather than as a missing diff.
OUTCOMES = [
    {"blocked": BLOCKER},
    {"blocked": BLOCKER},
    {"progress": True, "read_only": True},
    {"blocked": BLOCKER},
    {"blocked": OTHER_BLOCKER},
    {"blocked": BLOCKER},
    {"blocked": BLOCKER},
    {"blocked": BLOCKER},
]
# After the resume a human turn starts the run, and it completes the goal so the
# session settles instead of poking.
RESUMED = {"complete": True}


def build_script():
    """Expand the outcomes into one provider response per request."""
    script = []
    for number, outcome in enumerate(OUTCOMES):
        if outcome.get("read_only"):
            # A read-only inspection first: it proves progress does not need a diff.
            script.append({"call": {"name": "bash", "arguments": json.dumps(
                {"command": "sed -n 1,3p README.md"})}})
            script.append({"text": f"Inspected the README for context {number}."})
        script.append(dict(outcome))
        script.append({"text": f"BLOCKER_TURN_{number}. Anything else?"})
    script.append(dict(RESUMED))
    script.append({"text": "BLOCKERS_DONE. Anything else?"})
    return script


SCRIPT = build_script()
# Every scripted call before the pause is an automatic turn except the objective's
# own turn, which the human's `/goal` prompt starts and no poke is counted for.
POKES_BEFORE_PAUSE = sum(1 for turn in SCRIPT[:2 * len(OUTCOMES) + 1]
                         if "text" not in turn) - 1


class ModelServer(http.server.BaseHTTPRequestHandler):
    """Serve the scripted turns in order, then repeat the last answer."""

    stage = 0
    requests = 0
    systems = []
    lock = threading.Lock()

    def log_message(self, *_):
        """Stay quiet; assertions report failures."""

    def do_POST(self):
        """Answer with an OpenAI-compatible SSE stream."""
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        with type(self).lock:
            for message in body.get("messages") or []:
                if message.get("role") == "system":
                    type(self).systems.append(message.get("content") or "")
            stage = type(self).stage
            type(self).stage += 1
            type(self).requests += 1
        turn = SCRIPT[min(stage, len(SCRIPT) - 1)]
        if turn.get("blocked"):
            arguments = {"op": "blocked", "reason": turn["blocked"]}
        elif turn.get("progress"):
            arguments = {"op": "progress"}
        elif turn.get("complete"):
            arguments = {"op": "complete"}
        else:
            arguments = None
        if turn.get("call"):
            tool_call = turn["call"]
        elif arguments:
            tool_call = {"name": "goal", "arguments": json.dumps(arguments)}
        else:
            tool_call = None
        if tool_call:
            delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": f"goal-{stage}",
                "type": "function", "function": tool_call}]}
            reason = "tool_calls"
        else:
            delta = {"role": "assistant", "content": turn.get("text", "Anything else?")}
            reason = "stop"
        chunks = [
            {"id": "goal", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
            {"id": "goal", "choices": [{"index": 0, "delta": {}, "finish_reason": reason}],
             "usage": {"prompt_tokens": 1, "completion_tokens": 1}},
        ]
        wire = "".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(wire.encode())))
        self.end_headers()
        self.wfile.write(wire.encode())


def goal_results(agent):
    """Return the text of every persisted goal tool result, in call order."""
    results = []
    for session in sorted((agent / "sessions").rglob("*.jsonl")):
        for line in session.read_text().splitlines():
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue
            message = record.get("message") or {}
            if message.get("role") != "toolResult" or message.get("toolName") != "goal":
                continue
            results.append("".join(block.get("text", "")
                                   for block in message.get("content") or []))
    return results


def reported_count(text):
    """Return the consecutive count a goal status printed, or 0 when it omitted it."""
    marker = text.rfind("(reported ")
    if marker < 0:
        return 0
    digits = text[marker + len("(reported "):].split("×", 1)[0]
    return int(digits) if digits.isdigit() else 0


def run(binary):
    """Drive one session: blocked outcomes, a pause, then an explicit resume."""
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-goal-blockers-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (agent / "models.json").write_text(json.dumps({"providers": {"goal-proof": {
                "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "fixture", "models": [{"id": "goal-proof"}]}}}))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True,
                "enableInstallTelemetry": False}))
            # The read-only turn inspects this file and must not change it.
            workspace_file = root / "README.md"
            contents = "# fixture objective\nnotes for the goal\n"
            workspace_file.write_text(contents)
            env = dict(os.environ, HOME=directory, PIG_HOME=directory, PI_HOME=directory,
                       PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                       PIG_STARTUP_TRACE="1", TERM="xterm-256color")
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)

            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
            process = subprocess.Popen(
                [str(binary), "--provider", "goal-proof", "--model", "goal-proof",
                 "--no-context-files", "--no-skills", "--offline"],
                stdin=slave, stdout=slave, stderr=slave, cwd=root, env=env, start_new_session=True)
            os.close(slave)
            captured = bytearray()

            def read_until(seconds):
                """Read output for at most seconds."""
                end = time.monotonic() + seconds
                while time.monotonic() < end:
                    if select.select([master], [], [], 0.05)[0]:
                        try:
                            captured.extend(os.read(master, 262144))
                        except OSError:
                            return
                return

            def wait_for(condition, seconds, describe):
                """Read output until condition holds, or fail with what was shown."""
                end = time.monotonic() + seconds
                while time.monotonic() < end:
                    if condition():
                        return
                    read_until(0.1)
                if condition():
                    return
                raise AssertionError(f"timed out waiting for {describe}; "
                                     f"requests={ModelServer.requests}\n"
                                     f"{bytes(captured[-4000:])!r}")

            try:
                wait_for(lambda: b"interactive-ready" in captured, 90, "the prompt")
                os.write(master, b"/goal ship the release\r")
                wait_for(lambda: NOTICE.encode() in captured, 120, "the pause notice")
                # The turn that reported the third blocker ends on its own. From
                # here a paused goal must not be poked, so the provider stays silent.
                read_until(25)
                paused_at = ModelServer.requests
                read_until(10)
                assert ModelServer.requests == paused_at, \
                    f"a paused goal was poked: {paused_at} -> {ModelServer.requests}"

                # The model is told how to report outcomes, which is what makes the
                # policy reachable at all: a tool the model never calls cannot pause.
                # The system message the provider received is that surface, and the
                # transcript does not persist it.
                assert ModelServer.systems, "the provider received no system message"
                for guidance in ("`blocked` names the missing input or access",
                                 "including read-only research, tests, or verification"):
                    assert any(guidance in system for system in ModelServer.systems), \
                        f"the goal guidance is missing: {guidance}"

                outcomes = goal_results(agent)
                assert len(outcomes) == len(OUTCOMES), \
                    f"persisted outcomes = {len(outcomes)}: {outcomes}"
                # The counts prove the identity rules: progress cleared the streak
                # and the second blocker started its own, so only the last three
                # reports of the first blocker count together.
                counts = [reported_count(outcome) for outcome in outcomes]
                assert counts == [1, 2, 0, 1, 1, 1, 2, 3], f"consecutive counts = {counts}: {outcomes}"
                assert BLOCKER in outcomes[0], outcomes[0]
                assert OTHER_BLOCKER in outcomes[4], outcomes[4]
                # The poke count proves the turns were separate runs: one poke per
                # automatic turn, including the read-only one.
                assert f"Pokes: {POKES_BEFORE_PAUSE}" in outcomes[7], outcomes[7]
                assert outcomes[7].count(NOTICE) == 1, \
                    f"the paused outcome showed {outcomes[7].count(NOTICE)} notices: {outcomes[7]}"
                assert "Status: paused" in outcomes[7], outcomes[7]
                # The progress report followed a read that wrote nothing, and the
                # run left the workspace file untouched: progress is explicit, never
                # inferred from a diff.
                assert workspace_file.read_text() == contents, \
                    f"the run changed the workspace: {workspace_file.read_text()!r}"

                # Two status reads append no outcome and reach no provider. The
                # pause notice is the transition's, so a read cannot repeat it.
                for _ in range(2):
                    os.write(master, b"/goal show\r")
                    read_until(3)
                assert ModelServer.requests == paused_at, \
                    f"a status read poked the paused goal: {ModelServer.requests}"
                assert goal_results(agent) == outcomes, "a status read changed the goal status"
                assert len(goal_results(agent)) == len(OUTCOMES), \
                    "a status read appended another outcome"

                # Explicit resume starts work again. The blocker stays as the
                # explanation of the pause while the streak restarts.
                os.write(master, b"/goal resume\r")
                os.write(master, b"carry on\r")
                wait_for(lambda: len(goal_results(agent)) > len(OUTCOMES), 60,
                         "the resumed turn to reach the goal")
                read_until(3)
                final = goal_results(agent)[-1]
                assert "Status: complete" in final, final
                assert BLOCKER in final, f"the completed status lost its blocker: {final}"
                assert not reported_count(final), f"resume did not clear the streak: {final}"
                assert NOTICE not in final, f"the pause notice repeated: {final}"
            finally:
                if process.poll() is None:
                    process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                os.close(master)

            print(f"PASS: {len(OUTCOMES)} blocked/progress outcomes, paused after 3 identical "
                  f"reports at {paused_at} provider requests, no poke while paused, one notice, "
                  "progress and a changed blocker reset the streak, resume restarted work")
    finally:
        server.shutdown()
        server.server_close()


def main():
    """Resolve the caller's built binary; every path stays under a temporary profile."""
    if len(sys.argv) != 2:
        print("usage: smoke-test-goal-blockers.py <binary>", file=sys.stderr)
        return 2
    run(Path(sys.argv[1]).resolve())
    return 0


if __name__ == "__main__":
    sys.exit(main())
