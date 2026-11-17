#!/usr/bin/env python3
"""Measure real PTY startup through the first rendered edit, including exec time."""
import argparse
import fcntl
import json
import os
import pathlib
import pty
import select
import signal
import struct
import subprocess
import tempfile
import termios
import time

from benchmark import isolated_env


def measure(binary, agent_dir, cwd=None, model=None, environment=None, profile_dir=None):
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 130, 0, 0))
    env = dict(os.environ if environment is None else environment, PIG_STARTUP_TRACE="1")
    if profile_dir:
        env.update(PIG_PROFILE="cpu,allocs,trace", PIG_PROFILE_DIR=str(profile_dir.resolve()))
    if agent_dir:
        for name in ("OMPIG_SLIM_CODING_AGENT_DIR", "PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR"):
            env[name] = str(agent_dir)
    started = time.perf_counter()
    command = [str(binary), "-ne"]
    if model:
        command += ["--model", model]
    process = subprocess.Popen(command, stdin=slave, stdout=slave,
                               stderr=slave, env=env, cwd=cwd, start_new_session=True)
    os.close(slave)
    captured = bytearray()
    query_tail = b""
    typed = False
    usable = initialized = None
    exiting = False
    marker = b"startup-edit-probe"
    try:
        while time.perf_counter() - started < 15:
            if select.select([master], [], [], 0.01)[0]:
                try:
                    data = os.read(master, 131072)
                except OSError:
                    break
                if not data:
                    break
                captured.extend(data)
                query_scan = query_tail + data
                for _ in range(query_scan.count(b"\x1b[c")):
                    os.write(master, b"\x1b[?1;2c")
                query_tail = query_scan[-2:]
                if not typed and b"raw-mode-entered" in captured:
                    os.write(master, marker)
                    typed = True
                if usable is None and typed and marker in captured:
                    usable = (time.perf_counter() - started) * 1000
                if initialized is None and b"interactive-ready" in captured:
                    initialized = (time.perf_counter() - started) * 1000
                if usable is not None and initialized is not None and not exiting:
                    os.write(master, b"\x03\x04")
                    exiting = True
            if process.poll() is not None:
                break
    finally:
        if process.poll() is None:
            process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        os.close(master)
    if usable is None or initialized is None or process.returncode:
        raise RuntimeError(
            f"startup did not reach an editable TUI and exit cleanly "
            f"(exit={process.returncode}, typed={typed}, typing_ms={usable}, initialized_ms={initialized})\n"
            f"terminal tail: {bytes(captured[-4000:])!r}"
        )
    return {"typing_ms": round(usable, 2), "initialized_ms": round(initialized, 2)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=pathlib.Path)
    parser.add_argument("--agent-dir", type=pathlib.Path, help="Use this profile; default uses normal CLI profile resolution")
    parser.add_argument("--cwd", type=pathlib.Path, help="Launch in this directory; use an empty fixture for noninteractive CI")
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--max-ms", type=float, help="Fail if any rendered edit exceeds this elapsed time")
    parser.add_argument("--isolated", action="store_true", help="Use a temporary profile, saved model role, and fixture credentials")
    parser.add_argument("--model", help="Exercise an explicit model or role instead of the saved default")
    parser.add_argument("--compare", type=pathlib.Path, help="Alternate runs with this second binary")
    parser.add_argument("--profile-dir", type=pathlib.Path, help="Write attribution profiles; do not compare profiled timings")
    args = parser.parse_args()
    if args.runs < 1:
        parser.error("--runs must be positive")
    if args.isolated and (args.agent_dir or args.cwd):
        parser.error("--isolated cannot use an existing profile or working directory")
    binaries = [args.binary.resolve()]
    if args.compare:
        binaries.append(args.compare.resolve())
    for run in range(args.runs):
        for binary in binaries:
            with tempfile.TemporaryDirectory(prefix="ompig-startup-") as directory:
                root = pathlib.Path(directory)
                agent, cwd, env = args.agent_dir, args.cwd, None
                if args.isolated:
                    agent, cwd = root / "agent", root
                    agent.mkdir()
                    (agent / "settings.json").write_text(json.dumps({
                        "quietStartup": True, "tuiMode": "regular",
                        "defaultProvider": "openai", "defaultModel": "gpt-4o",
                        "modelRoles": {"default": "openai/gpt-4o"}}))
                    env = isolated_env(root, agent)
                    env["OPENAI_API_KEY"] = "offline-fixture"
                result = measure(binary, agent.resolve() if agent else None,
                                 cwd.resolve() if cwd else None, args.model, env, args.profile_dir)
            print(json.dumps({"run": run + 1, "binary": str(binary),
                              "profiled": bool(args.profile_dir), **result}), flush=True)
            if args.max_ms is not None and result["typing_ms"] >= args.max_ms:
                raise SystemExit(f"editable startup exceeded {args.max_ms} ms")


if __name__ == "__main__":
    main()
