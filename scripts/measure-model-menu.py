#!/usr/bin/env python3
"""Measure /model command-to-first-visible-tab latency in an isolated real PTY."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import statistics
import struct
import subprocess
import tempfile
import termios
import time


def measure(binary, metadata, legacy_scope, explicit_scope):
    """Return cold/reopen latencies; dummy auth never sends inference requests."""
    with tempfile.TemporaryDirectory(prefix="omp-model-latency-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        settings = {"quietStartup": True, "tuiMode": "regular",
                    "modelRoles": {"default": "test-faux/model", "review": "test-faux/model"},
                    "cycleOrder": ["default", "review"]}
        if legacy_scope:
            settings["enabledModels"] = ["openai/gpt-6.1-sol"]
        (agent / "settings.json").write_text(json.dumps(settings))
        if metadata:
            (agent / "models-metadata.json").write_bytes(metadata.read_bytes())
        env = {"PATH": os.environ["PATH"], "HOME": directory, "TERM": "xterm-256color",
               "PIG_HOME": directory, "PI_HOME": directory, "PIG_TEST_FAUX": "1",
               "PI_OFFLINE": "1", "PI_SKIP_VERSION_CHECK": "1", "PIG_STARTUP_TRACE": "1",
               "OPENAI_API_KEY": "offline-fixture", "ANTHROPIC_API_KEY": "offline-fixture",
               "GOOGLE_API_KEY": "offline-fixture", "OPENROUTER_API_KEY": "offline-fixture"}
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        command = [str(binary), "-ne", "--model", "test-faux/model", "--no-session"]
        if explicit_scope:
            command += ["--models", "openai/gpt-6.1-sol"]
        process = subprocess.Popen(command,
                                   cwd=root, env=env, stdin=slave, stdout=slave, stderr=slave,
                                   start_new_session=True)
        os.close(slave)

        def wait_for(marker):
            """Answer terminal queries and wait for a literal first-frame marker."""
            received = bytearray()
            deadline = time.monotonic() + 60
            while time.monotonic() < deadline:
                if select.select([master], [], [], 0.02)[0]:
                    chunk = os.read(master, 1048576)
                    received.extend(chunk)
                    for query, response in ((b"\x1b[6n", b"\x1b[1;1R"),
                                            (b"\x1b[c", b"\x1b[?1;2c"),
                                            (b"\x1b]11;?", b"\x1b]11;rgb:1010/1010/1010\x1b\\")):
                        if query in chunk:
                            os.write(master, response)
                    if marker in received:
                        return bytes(received)
                if process.poll() is not None:
                    break
            raise AssertionError(f"missing {marker!r}: {received[-2000:]!r}")

        try:
            wait_for(b"interactive-ready")
            timings = []
            for _ in range(2):
                # Drain pending startup/refresh output, outside the timed interval.
                until = time.monotonic() + 0.5
                while time.monotonic() < until:
                    if select.select([master], [], [], 0.02)[0]:
                        os.read(master, 1048576)
                started = time.monotonic()
                os.write(master, b"/model\r")
                frame = wait_for(b"All available models")
                timings.append((time.monotonic() - started) * 1000)
                if legacy_scope:
                    saved = json.loads((agent / "settings.json").read_text())
                    assert saved.get("enabledModels") == [], f"legacy scope still hides models: {saved}"
                    assert saved["modelRoles"]["default"] == "test-faux/model", "role overwritten"
                if explicit_scope:
                    assert b"--models scope" in frame, "explicit CLI scope lost"
                else:
                    assert b"--models scope" not in frame, "unexpected persisted scope"
                    assert b"openai" in frame, "authenticated provider missing"
                os.write(master, b"\x1b")
                wait_for(b"\x1b[?1049l")
            return timings
        finally:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            os.close(master)


def main():
    """Print repeatable cold/warm observations without touching the user's profile."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--metadata", type=Path)
    parser.add_argument("--runs", type=int, default=3)
    parser.add_argument("--legacy-scope", action="store_true")
    parser.add_argument("--explicit-scope", action="store_true")
    args = parser.parse_args()
    results = [measure(args.binary.resolve(), args.metadata, args.legacy_scope, args.explicit_scope) for _ in range(args.runs)]
    print(json.dumps({"samples_ms": results, "cold_median_ms": statistics.median(r[0] for r in results),
                      "reopen_median_ms": statistics.median(r[1] for r in results)}, indent=2))


if __name__ == "__main__":
    main()
