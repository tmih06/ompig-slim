#!/usr/bin/env python3
"""Check slash suggestions: purpose keywords rank commands, and an unknown command names the closest match."""
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
        # A purpose word ranks the command whose keyword names it, and the row shows the tag.
        # Tab accepts the row, which closes the menu for a command without argument options.
        os.write(master, b"/clea")
        wait_for(b"new (clear, fresh)")
        os.write(master, b"\t")
        os.write(master, b"\x15")
        os.write(master, b"/exi")
        wait_for(b"quit (exit, close)")
        os.write(master, b"\t")
        os.write(master, b"\x15")
        # An unknown command keeps its draft, starts no turn, and names the closest match.
        os.write(master, b"/mew\r")
        wait_for(b"Unknown slash command: /mew. Did you mean /new?")
        os.write(master, b"\x15")
        os.write(master, b"/zzzzz\r")
        wait_for(b"Unknown slash command: /zzzzz")
        rejected = captured.index(b"Unknown slash command: /zzzzz")
        assert b"Did you mean" not in captured[rejected:rejected + 120], "stray input named a command"
        os.write(master, b"\x15")
        os.write(master, b"/quit\r")
        wait_for(b"Session summary")
        summary = captured.index(b"Session summary")
        wait_for(b"tool calls", summary)
        assert b"0 user, 0 assistant" in captured[summary:], "a rejected command reached the model"
        assert child.wait(timeout=5) == 0
    finally:
        if child.poll() is None:
            child.kill()
            child.wait()
        os.close(master)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("Usage: smoke-test-slash-suggestions.py <ompig-slim-binary>")
    with tempfile.TemporaryDirectory(prefix="ompig-slash-") as directory:
        run(str(Path(sys.argv[1]).resolve()), Path(directory))
    print("PASS: keyword tags rank slash suggestions and unknown commands name the closest match")
