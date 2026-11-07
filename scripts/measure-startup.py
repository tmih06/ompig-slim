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
import termios
import time


def measure(binary, agent_dir, cwd=None):
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 130, 0, 0))
    env = dict(os.environ, PIG_STARTUP_TRACE="1")
    if agent_dir:
        for name in ("OMPIG_SLIM_CODING_AGENT_DIR", "PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR"):
            env[name] = str(agent_dir)
    started = time.perf_counter()
    process = subprocess.Popen([str(binary), "-ne"], stdin=slave, stdout=slave,
                               stderr=slave, env=env, cwd=cwd, start_new_session=True)
    os.close(slave)
    captured = bytearray()
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
    args = parser.parse_args()
    if args.runs < 1:
        parser.error("--runs must be positive")
    for run in range(args.runs):
        result = measure(args.binary.resolve(), args.agent_dir.resolve() if args.agent_dir else None,
                         args.cwd.resolve() if args.cwd else None)
        print(json.dumps({"run": run + 1, **result}), flush=True)
        if args.max_ms is not None and result["typing_ms"] >= args.max_ms:
            raise SystemExit(f"editable startup exceeded {args.max_ms} ms")


if __name__ == "__main__":
    main()
