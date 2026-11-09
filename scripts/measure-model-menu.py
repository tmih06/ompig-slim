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


def wait_for(master, process, marker, timeout=60, complete_frame=False):
    """Return observed bytes and the first marker time, or fail with the PTY tail.

    PTY reads do not preserve renderer writes (notably on macOS). Menu assertions
    require the synchronized-output end after the title, not an arbitrary read
    containing it. Keep the marker time separate so frame validation does not
    inflate command-to-first-visible-title measurements. Other callers only need
    their literal marker and do not wait for a renderer frame.
    """
    received = bytearray()
    deadline = time.monotonic() + timeout
    visible_at = None
    marker_end = 0
    while time.monotonic() < deadline:
        if select.select([master], [], [], 0.02)[0]:
            try:
                chunk = os.read(master, 1048576)
            except OSError:
                break
            if not chunk:
                break
            received.extend(chunk)
            for query, response in ((b"\x1b[6n", b"\x1b[1;1R"),
                                    (b"\x1b[c", b"\x1b[?1;2c"),
                                    (b"\x1b]11;?", b"\x1b]11;rgb:1010/1010/1010\x1b\\")):
                if query in chunk:
                    os.write(master, response)
            if visible_at is None and marker in received:
                visible_at = time.monotonic()
                marker_end = received.index(marker) + len(marker)
            if visible_at is not None:
                if not complete_frame or b"\x1b[?2026l" in received[marker_end:]:
                    return bytes(received), visible_at
        if process.poll() is not None:
            break
    label = f"complete frame after {marker!r}" if complete_frame else repr(marker)
    raise AssertionError(f"missing {label}; exit={process.poll()}: {received[-2000:]!r}")


def measure(binary, metadata, legacy_settings, stale_cycle):
    """Return cold/reopen latencies; dummy auth never sends inference requests."""
    with tempfile.TemporaryDirectory(prefix="omp-model-latency-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        settings = {"quietStartup": True, "tuiMode": "regular",
                    "modelRoles": {"default": "test-faux/model", "review": "test-faux/model"},
                    "cycleOrder": ["default", "review"]}
        if legacy_settings:
            settings["enabledModels"] = ["openai/gpt-6.1-sol"]
        if stale_cycle:
            settings["modelRoles"] = {"default": "openai/gpt-4o", "smol": "openai/gpt-4o-mini"}
            settings["cycleOrder"] = ["scoped-1", "scoped-2", "scoped-3", "scoped-4"]
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
        if legacy_settings:
            retired = subprocess.run([str(binary), "--models", "openai/*", "--no-extensions",
                                      "--mode", "json", "--no-session"], cwd=root, env=env,
                                     stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=20)
            assert retired.returncode != 0 and "Unknown option: --models" in retired.stderr, "retired CLI selection flag still accepted"
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        model = "openai/gpt-4o" if stale_cycle else "test-faux/model"
        command = [str(binary), "-ne", "--model", model, "--no-session"]
        process = subprocess.Popen(command,
                                   cwd=root, env=env, stdin=slave, stdout=slave, stderr=slave,
                                   start_new_session=True)
        os.close(slave)

        try:
            wait_for(master, process, b"interactive-ready")
            if stale_cycle:
                # Exercise the real Ctrl+P binding, not only Session.CycleModel.
                # Dummy env auth permits switching but no prompt is ever sent.
                for _ in range(2):
                    os.write(master, b"\x10")
                    wait_for(master, process, b"Switched to GPT-4o mini", timeout=15)
                    os.write(master, b"\x10")
                    frame, _ = wait_for(master, process, b"Switched to GPT-4o", timeout=15)
                    assert b"Switched to GPT-4o mini" not in frame, "Default was skipped"
                assert json.loads((agent / "settings.json").read_text())["modelRoles"] == settings["modelRoles"], "cycling changed assignments"
            timings = []
            for _ in range(2):
                # Drain pending startup/refresh output, outside the timed interval.
                until = time.monotonic() + 0.5
                while time.monotonic() < until:
                    if select.select([master], [], [], 0.02)[0]:
                        os.read(master, 1048576)
                started = time.monotonic()
                os.write(master, b"/model\r")
                frame, visible_at = wait_for(master, process, b"All available models", complete_frame=True)
                timings.append((visible_at - started) * 1000)
                saved = json.loads((agent / "settings.json").read_text())
                assert saved["modelRoles"] == settings["modelRoles"], "obsolete settings created or changed roles"
                assert b"--models scope" not in frame, "obsolete model filter still active"
                assert b"openai" in frame, "authenticated provider missing"
                os.write(master, b"\x1b")
                wait_for(master, process, b"\x1b[?1049l")
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
    parser.add_argument("--legacy-settings", action="store_true", help="Assert obsolete settings are ignored, never migrated into roles")
    parser.add_argument("--stale-cycle", action="store_true")
    args = parser.parse_args()
    results = [measure(args.binary.resolve(), args.metadata, args.legacy_settings, args.stale_cycle) for _ in range(args.runs)]
    print(json.dumps({"samples_ms": results, "cold_median_ms": statistics.median(r[0] for r in results),
                      "reopen_median_ms": statistics.median(r[1] for r in results)}, indent=2))


if __name__ == "__main__":
    main()
