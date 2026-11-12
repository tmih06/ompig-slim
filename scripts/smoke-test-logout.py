#!/usr/bin/env python3
"""Prove single-account /logout and cancellation through the compiled TUI."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import sqlite3
import struct
import subprocess
import tempfile
import termios
import time


def run(binary):
    """Use fixture accounts in an isolated profile; never contact a provider."""
    with tempfile.TemporaryDirectory(prefix="omp-logout-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        (agent / "settings.json").write_text(json.dumps({
            "quietStartup": True, "tuiMode": "regular",
            "modelRoles": {"default": "test-faux/model"},
        }))
        database = agent / "agent.db"
        with sqlite3.connect(database) as db:
            db.execute("""CREATE TABLE auth_credentials (
                id INTEGER PRIMARY KEY AUTOINCREMENT, provider TEXT NOT NULL,
                credential_type TEXT NOT NULL, data TEXT NOT NULL,
                disabled_cause TEXT, identity_key TEXT,
                created_at INTEGER NOT NULL DEFAULT 0,
                updated_at INTEGER NOT NULL DEFAULT 0)""")
            for provider, key in (("openai", None), ("openai", "remove-me"),
                                  ("openai", "keep-me"), ("anthropic", "other")):
                db.execute("""INSERT INTO auth_credentials
                    (provider, credential_type, data, identity_key)
                    VALUES (?, 'api_key', ?, ?)""",
                           (provider, json.dumps({"key": "fixture-secret"}), key))
        env = dict(os.environ, PIG_TEST_FAUX="1", PIG_STARTUP_TRACE="1",
                   PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                   PIG_HOME=directory, PI_HOME=directory, TERM="xterm-256color")
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        # A caller's remote connection must never redirect this local fixture.
        for key in ("OMPIG_BROKER_URL", "OMPIG_BROKER_TOKEN", "OMPIG_BROKER_TOKEN_FILE"):
            env.pop(key, None)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        process = subprocess.Popen([str(binary), "-ne", "--model", "test-faux/model"],
                                   stdin=slave, stdout=slave, stderr=slave,
                                   cwd=root, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()

        def wait_for(text):
            """Wait for new rendered output, failing with the terminal tail."""
            observed = bytearray()
            deadline = time.monotonic() + 20
            while time.monotonic() < deadline:
                if select.select([master], [], [], 0.05)[0]:
                    try:
                        data = os.read(master, 262144)
                    except OSError:
                        break
                    captured.extend(data)
                    observed.extend(data)
                if text in observed:
                    return
                if process.poll() is not None:
                    break
            raise AssertionError(f"missing {text!r}; exit={process.poll()}; tail={captured[-3000:]!r}")

        def accounts():
            """Read the observable durable enabled identities after an interaction."""
            with sqlite3.connect(database) as db:
                return db.execute("""SELECT provider, identity_key FROM auth_credentials
                    WHERE disabled_cause IS NULL ORDER BY id""").fetchall()

        def open_accounts():
            """Drive the real provider picker into the account picker."""
            os.write(master, b"/logout\r")
            wait_for(b"Select provider to logout:")
            os.write(master, b"openai\r")
            wait_for(b"Select account to log out of")

        try:
            wait_for(b"interactive-ready")
            original = accounts()
            open_accounts()
            os.write(master, b"\x1b")
            # Wait for Escape decoding and the restored editor before typing:
            # sending immediately would encode Alt+/ rather than a new command.
            wait_for(b"Widgets")
            open_accounts()
            assert accounts() == original, "cancelling removed credentials"
            os.write(master, b"\x1b[B\r")  # Choose the non-active named account.
            wait_for(b"Removed one stored account")
            assert accounts() == [("openai", None), ("openai", "keep-me"),
                                  ("anthropic", "other")], "logout changed siblings"
            open_accounts()
            os.write(master, b"\r")  # The NULL identity is active and listed first.
            wait_for(b"Removed one stored account")
            assert accounts() == [("openai", "keep-me"), ("anthropic", "other")], \
                "identity-less logout removed a sibling"
            assert b"fixture-secret" not in captured, "picker leaked an API key"
            print("PASS: account picker, Escape cancellation, named/NULL logout, sibling preservation, no secret display")
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
    """Resolve the caller's compiled binary; fixtures live only in a temp profile."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    run(args.binary.resolve())


if __name__ == "__main__":
    main()
