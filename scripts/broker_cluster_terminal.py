"""Drive a long-lived interactive client inside a container over a pseudo-terminal.

The broker suites otherwise run the client one command at a time, which is why a
stale live session went unnoticed: every one-shot invocation re-reads
settings.json at startup and therefore passes whether or not a *running* session
follows a synchronized document. This module keeps one client interactive and
lets a check assert what its fullscreen UI shows.
"""

import os
import pty
import re
import select
import subprocess
import time


class InteractiveClient:
    """One `docker exec --tty` session on a client container's terminal."""

    def __init__(self, container, model="test-faux/faux-1"):
        self.container = container
        self.master, slave = pty.openpty()
        # -ne loads no extensions and the fixture provider needs no credentials,
        # so the session reaches its prompt without contacting a provider.
        # PIG_STARTUP_TRACE makes the harness print its readiness mark, which is how
        # scripts/smoke-test-model-menu.py also knows the session is interactive.
        self.process = subprocess.Popen(
            ["docker", "exec", "-i", "--tty", "-e", "PIG_STARTUP_TRACE=1", container,
             "ompig-slim", "-ne", "--model", model],
            stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        os.close(slave)
        self.captured = bytearray()

    def send(self, data):
        """Type into the session through the same path as a user."""
        os.write(self.master, data)

    def read_until(self, predicate, label, timeout=60):
        """Pump output until predicate holds for the bytes seen during this call."""
        end = time.monotonic() + timeout
        observed = bytearray()
        while time.monotonic() < end:
            if select.select([self.master], [], [], 0.05)[0]:
                try:
                    chunk = os.read(self.master, 262144)
                except OSError:
                    break
                observed.extend(chunk)
                self.captured.extend(chunk)
            if predicate(bytes(observed)):
                return bytes(observed)
            if self.process.poll() is not None:
                break
        raise AssertionError(
            f"{label} never appeared in {self.container}; exit={self.process.poll()}\n"
            f"terminal tail: {visible(bytes(self.captured[-6000:]))[-2500:]!r}")

    def close(self):
        if self.process.poll() is None:
            self.process.terminate()
        try:
            self.process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait()
        os.close(self.master)


def visible(data):
    """Strip terminal control sequences so assertions read what a user reads.

    The manager paints a role and its model with separate theme tokens, so words
    that sit side by side on screen are separated by SGR bytes on the wire.
    """
    return re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", data).decode("utf-8", "replace")


def open_role_manager(live, label):
    """Enter the fullscreen manager and land on its Roles pane.

    `/model` opens the manager on All models; one Up moves the sidebar onto
    Roles. The returned bytes are this call's output, which is what the caller
    asserts against.
    """
    live.send(b"/model\r")
    live.read_until(lambda data: b"All models" in data and b"Models" in data, f"{label} manager")
    live.send(b"\x1b[A")
    return live.read_until(lambda data: b"Model roles" in data, f"{label} Roles pane")


def close_role_manager(live, label):
    """Leave the manager, restoring the main screen for the next command."""
    live.send(b"\x1b")
    live.read_until(lambda data: b"\x1b[?1049l" in data, f"{label} main screen")
