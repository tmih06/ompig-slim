#!/usr/bin/env python3
"""Check that Enter accepts an absolute path without submitting the prompt."""
import fcntl
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import time


def run(binary, root):
    for name in ("candidate-one.txt", "candidate-two.txt"):
        (root / name).touch()
    env = {**os.environ, "HOME": str(root), "PI_HOME": str(root), "PIG_HOME": str(root),
           "PIG_TEST_FAUX": "1", "PI_OFFLINE": "1", "PIG_STARTUP_TRACE": "1", "TERM": "xterm-256color"}
    for key in list(env):
        if key.startswith("OMPIG_BROKER_"):
            del env[key]
    for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
        env[key] = str(root)
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 35, 150, 0, 0))
    child = subprocess.Popen([binary, "--offline", "--no-extensions", "--no-session", "--model", "test-faux/faux"],
                             cwd=root, env=env, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    captured = bytearray()

    def wait_for(token, offset=0):
        deadline = time.monotonic() + 10
        while token not in captured[offset:] and time.monotonic() < deadline:
            if select.select([master], [], [], .05)[0]:
                try:
                    captured.extend(os.read(master, 65536))
                except OSError:
                    break
        assert token in captured[offset:], f"missing {token!r}: {captured[-1400:]!r}"

    try:
        wait_for(b"interactive-ready")
        os.write(master, f"look at {root}/cand\t".encode())
        wait_for(b"candidate-two.txt")
        offset = len(captured)
        os.write(master, b"\r")
        wait_for(b"candidate-one.txt", offset)
        os.write(master, b"\x15")
        os.write(master, b"/quit\r")
        wait_for(b"Session summary")
        summary = captured.index(b"Session summary")
        wait_for(b"tool calls", summary)
        assert b"0 user, 0 assistant" in captured[summary:], "Enter submitted the path completion"
        assert child.wait(timeout=5) == 0
    finally:
        if child.poll() is None:
            child.kill()
            child.wait()
        os.close(master)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("Usage: smoke-test-terminal-autocomplete.py <ompig-slim-binary>")
    with tempfile.TemporaryDirectory(prefix="ompig-tab-") as directory:
        run(str(Path(sys.argv[1]).resolve()), Path(directory))
    print("PASS: Enter accepted the absolute path and left the prompt unsent")
