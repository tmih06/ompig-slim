#!/usr/bin/env python3
"""Verify external SIGTSTP restores the terminal before stopping ompig-slim."""
import fcntl
import os
from pathlib import Path
import pty
import select
import signal
import struct
import sys
import tempfile
import termios
import time


def supervise(binary, root):
    signal.signal(signal.SIGTTOU, signal.SIG_IGN)
    attrs = termios.tcgetattr(0)
    ready, start = os.pipe()
    child = os.fork()
    if child == 0:
        os.close(start)
        os.setpgid(0, 0)
        os.read(ready, 1)
        os.close(ready)
        signal.signal(signal.SIGTTOU, signal.SIG_DFL)
        os.chdir(root)
        env = {**os.environ, "HOME": root, "PI_HOME": root, "PIG_HOME": root,
               "PI_OFFLINE": "1", "PIG_TEST_FAUX": "1", "PIG_STARTUP_TRACE": "1",
               "TERM": "xterm-256color", "TERM_PROGRAM": "suspend-proof"}
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = root
        env["OMPIG_BROKER_URL"] = ""
        os.execve(binary, [binary, "--offline", "--no-extensions", "--no-session", "--model", "test-faux/faux"], env)
    os.close(ready)
    os.setpgid(child, child)
    os.tcsetpgrp(0, child)
    os.write(1, f"CHILD={child}\n".encode())
    os.write(start, b"1")
    os.close(start)
    for _ in range(2):
        _, status = os.waitpid(child, os.WUNTRACED)
        if not os.WIFSTOPPED(status):
            os._exit(1)
        restored = termios.tcgetattr(0) == attrs
        os.write(1, b"TERMINAL_RESTORED\n" if restored else b"TERMINAL_STILL_RAW\n")
        os.killpg(child, signal.SIGCONT)
    _, status = os.waitpid(child, 0)
    os._exit(os.waitstatus_to_exitcode(status))


def run(binary, root):
    Path(root, "settings.json").write_text('{"tuiMode":"fullscreen"}')
    supervisor, master = pty.fork()
    if supervisor == 0:
        fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 100, 0, 0))
        supervise(binary, root)
    captured = bytearray()
    child = None

    def wait_for(token, offset=0):
        deadline = time.monotonic() + 15
        while token not in captured[offset:] and time.monotonic() < deadline:
            if select.select([master], [], [], 0.1)[0]:
                try:
                    data = os.read(master, 65536)
                except OSError:
                    break
                if not data:
                    break
                captured.extend(data)
        assert token in captured[offset:], f"missing {token!r}: {captured[-3000:]!r}"
        return captured.index(token, offset) + len(token)

    try:
        wait_for(b"CHILD=")
        wait_for(b"\n", captured.index(b"CHILD="))
        child = int(captured.split(b"CHILD=", 1)[1].splitlines()[0])
        wait_for(b"interactive-ready")
        for _ in range(2):
            offset = len(captured)
            os.kill(child, signal.SIGTSTP)
            restored = wait_for(b"TERMINAL_RESTORED", offset)
            assert b"\x1b[?1049l" in captured[offset:restored], "alternate screen remained active while stopped"
            wait_for(b"\x1b[?1049h", restored)
        os.write(master, b"/quit\r")
        wait_for(b"Session summary")
        _, status = os.waitpid(supervisor, 0)
        supervisor = None
        child = None
        assert status == 0, status
    finally:
        if child is not None:
            try:
                os.kill(child, signal.SIGKILL)
            except ProcessLookupError:
                pass
        if supervisor is not None:
            os.kill(supervisor, signal.SIGKILL)
            os.waitpid(supervisor, 0)
        os.close(master)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("Usage: smoke-test-terminal-suspend.py <ompig-slim-binary>")
    with tempfile.TemporaryDirectory(prefix="ompig-suspend-") as root:
        run(str(Path(sys.argv[1]).resolve()), root)
    print("PASS: external SIGTSTP restored the terminal before both stops and redrew after SIGCONT")
