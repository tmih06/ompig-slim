#!/usr/bin/env python3
"""Exercise model-management PTY flows with a fixture profile and no model calls."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import tempfile
import termios
import time


def run(binary):
    """Mount real fullscreen/compact menus and assert durable edits in isolation."""
    with tempfile.TemporaryDirectory(prefix="omp-model-menu-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        settings = agent / "settings.json"
        settings.write_text(json.dumps({
            "quietStartup": True,
            "tuiMode": "regular",
            "modelRoles": {"default": "test-faux/model", "review": "test-faux/model"},
            "cycleOrder": ["default", "review"],
            "modelTags": {"review": {"name": "Reviewer"}},
            "retry": {"fallbackChains": {"review": ["test-faux/model"]}},
        }))
        env = dict(os.environ, PIG_TEST_FAUX="1", PIG_STARTUP_TRACE="1",
                   PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                   PIG_HOME=directory, PI_HOME=directory, TERM="xterm-256color")
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        process = subprocess.Popen([str(binary), "-ne", "--model", "test-faux/model"],
                                   stdin=slave, stdout=slave, stderr=slave,
                                   cwd=root, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()

        def read_until(predicate, label, timeout=15):
            """Pump output until an observable condition, not a guessed render delay."""
            end = time.monotonic() + timeout
            observed = bytearray()
            while time.monotonic() < end:
                if select.select([master], [], [], 0.05)[0]:
                    try:
                        data = os.read(master, 262144)
                    except OSError:
                        break
                    observed.extend(data)
                    captured.extend(data)
                if predicate(bytes(observed)):
                    return bytes(observed)
                if process.poll() is not None:
                    break
            raise AssertionError(f"{label} did not settle; exit={process.poll()}\n"
                                 f"terminal tail: {bytes(captured[-3000:])!r}")

        def send(data):
            """Write an input packet through the same terminal path as user typing."""
            os.write(master, data)

        try:
            read_until(lambda data: b"interactive-ready" in data, "startup")
            send(b"/model\r")
            read_until(lambda data: b"All models" in data and b"Models" in data, "model hub")
            send(b"\x1b[A")
            read_until(lambda data: b"DEFAULT" in data, "Roles view")
            send(b"\x1b[C")
            send(b"c")
            read_until(lambda _: json.loads(settings.read_text()).get("cycleOrder") == ["review"],
                       "cycle order persistence")
            saved_roles = json.loads(settings.read_text())["modelRoles"]
            send(b"\x1b")
            read_until(lambda data: b"\x1b[?1049l" in data, "restore main screen")
            send(b"\x1bp")
            read_until(lambda data: b"Switch Model" in data, "compact picker")
            send(b"@review")
            read_until(lambda data: b"@review" in data, "quick-role search")
            send(b"\r")
            read_until(lambda data: b"Model:" in data or b"test-faux/model" in data,
                       "role selection")
            assert json.loads(settings.read_text())["modelRoles"] == saved_roles, "picker changed saved roles"
            send(b"/model\r")
            read_until(lambda data: b"All models" in data, "reopen manager")
            send(b"\x1b[A")
            read_until(lambda data: b"Model roles" in data, "reopen Roles")
            send(b"\x1b[C")
            # The stable built-in chat-role order has eleven rows; review is
            # the first custom chat role, before the kind-role separator.
            send(b"\x1bf")  # Alt+Right selects the chat-only role tab.
            for _ in range(11):
                send(b"\x1b[B")
            send(b"D")
            read_until(lambda data: b"Delete @review (global)" in data, "delete confirmation")
            send(b"\r")  # Cancel is selected by default.
            assert "review" in json.loads(settings.read_text())["modelRoles"], "default confirmation deleted"
            send(b"D")
            read_until(lambda data: b"Delete @review (global)" in data, "second delete confirmation")
            send(b"\x1b[C\r")
            read_until(lambda _: "review" not in json.loads(settings.read_text()).get("modelRoles", {}),
                       "delete role persistence")
            deleted = json.loads(settings.read_text())
            assert "review" not in deleted.get("modelTags", {}), "role tag survived"
            assert "review" not in deleted.get("cycleOrder", []), "role cycle survived"
            assert "review" not in deleted.get("retry", {}).get("fallbackChains", {}), "role fallback survived"
            assert deleted["modelRoles"]["default"] == saved_roles["default"], "unrelated role changed"
            print("PASS: fullscreen hub, Roles, cycle edit, main-screen restore, session-only @role picker, confirmed custom-role deletion")
        finally:
            if process.poll() is None:
                process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            os.close(master)


def main():
    """Resolve a caller-selected binary; tests never modify the real user profile."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    run(args.binary.resolve())


if __name__ == "__main__":
    main()
