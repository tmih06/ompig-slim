#!/usr/bin/env python3
"""Prove a Mistral key's Vibe Code allowance reaches the compiled /usage board.

The build reads `https://api.mistral.ai/v1/billing/budget` with no configuration,
so the only way to exercise the real endpoint without a subscription is to
intercept it: a loopback CONNECT proxy terminates TLS for that host with a
throwaway CA the process trusts through SSL_CERT_FILE. Nothing leaves the
machine and no credential outside the fixture is sent.
"""
import argparse
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import socket
import sqlite3
import ssl
import struct
import subprocess
import tempfile
import termios
import threading
import time

HOST = "api.mistral.ai"

# The dashboard billing_api_public shape, as mistral-vibe reads it.
BUDGET = {
    "vibe_budget": {
        "usage_percentage": 42.5,
        "initial_budget": 300.0,
        "currency": "EUR",
        "reset_at": "2027-01-01T00:00:00Z",
        "payg_enabled": False,
    },
    "api_budget": {"usage_percentage": 3.0, "initial_budget": 50.0},
}


def mint_certificate(directory):
    """Create a CA and a leaf for HOST, both readable by the probe."""
    ca_key, ca_cert = directory / "ca.key", directory / "ca.pem"
    leaf_key, leaf_csr, leaf_cert = directory / "leaf.key", directory / "leaf.csr", directory / "leaf.pem"
    ext = directory / "leaf.ext"
    ext.write_text(f"subjectAltName=DNS:{HOST}\nbasicConstraints=CA:FALSE\n")
    run = lambda *args: subprocess.run(["openssl", *args], check=True,
                                       capture_output=True)
    run("req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2",
        "-keyout", str(ca_key), "-out", str(ca_cert), "-subj", "/CN=ompig-usage-fixture-ca")
    run("req", "-newkey", "rsa:2048", "-nodes", "-keyout", str(leaf_key),
        "-out", str(leaf_csr), "-subj", f"/CN={HOST}")
    run("x509", "-req", "-in", str(leaf_csr), "-CA", str(ca_cert),
        "-CAkey", str(ca_key), "-CAcreateserial", "-days", "2",
        "-out", str(leaf_cert), "-extfile", str(ext))
    return ca_cert, leaf_cert, leaf_key


class Fixture:
    """A CONNECT proxy that answers the billing route for one host."""

    def __init__(self, leaf_cert, leaf_key):
        self.context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        self.context.load_cert_chain(str(leaf_cert), str(leaf_key))
        self.listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.listener.bind(("127.0.0.1", 0))
        self.listener.listen(16)
        self.port = self.listener.getsockname()[1]
        self.requests = []
        self.lock = threading.Lock()
        self.stopping = threading.Event()
        threading.Thread(target=self.serve, daemon=True).start()

    def serve(self):
        """Accept CONNECT tunnels and answer each one."""
        while not self.stopping.is_set():
            try:
                client, _ = self.listener.accept()
            except OSError:
                return
            threading.Thread(target=self.answer, args=(client,), daemon=True).start()

    def answer(self, client):
        """Read the CONNECT line, upgrade to TLS, serve one request."""
        try:
            head = b""
            while b"\r\n\r\n" not in head:
                chunk = client.recv(4096)
                if not chunk:
                    return
                head += chunk
            if not head.startswith(f"CONNECT {HOST}:443 ".encode()):
                client.sendall(b"HTTP/1.1 403 Forbidden\r\n\r\n")
                return
            client.sendall(b"HTTP/1.1 200 Connection established\r\n\r\n")
            with self.context.wrap_socket(client, server_side=True) as tunnel:
                request = b""
                while b"\r\n\r\n" not in request:
                    chunk = tunnel.recv(4096)
                    if not chunk:
                        return
                    request += chunk
                lines = request.split(b"\r\n")
                path = lines[0].split(b" ")[1].decode()
                headers = {}
                for line in lines[1:]:
                    if b":" in line:
                        name, value = line.split(b":", 1)
                        headers[name.strip().lower().decode()] = value.strip().decode()
                with self.lock:
                    self.requests.append((path, headers.get("authorization")))
                if not path.endswith("/billing/budget"):
                    body, status = b'{"detail":"not found"}', b"404 Not Found"
                elif headers.get("authorization") != "Bearer fixture-key":
                    body, status = b'{"detail":"unauthorized"}', b"401 Unauthorized"
                else:
                    body, status = json.dumps(BUDGET).encode(), b"200 OK"
                tunnel.sendall(b"HTTP/1.1 " + status + b"\r\nContent-Type: application/json\r\nContent-Length: "
                               + str(len(body)).encode() + b"\r\nConnection: close\r\n\r\n" + body)
        except (OSError, ssl.SSLError):
            pass
        finally:
            try:
                client.close()
            except OSError:
                pass

    def stop(self):
        self.stopping.set()
        self.listener.close()


def run(binary, fixture, ca_cert):
    """Drive /usage in an isolated profile holding one Mistral account."""
    with tempfile.TemporaryDirectory(prefix="ompig-mistral-usage-") as directory:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        (agent / "settings.json").write_text(json.dumps({
            "quietStartup": True, "tuiMode": "regular"}))
        database = agent / "agent.db"
        with sqlite3.connect(database) as db:
            db.execute("""CREATE TABLE auth_credentials (
                id INTEGER PRIMARY KEY AUTOINCREMENT, provider TEXT NOT NULL,
                credential_type TEXT NOT NULL, data TEXT NOT NULL,
                disabled_cause TEXT, identity_key TEXT,
                created_at INTEGER NOT NULL DEFAULT 0,
                updated_at INTEGER NOT NULL DEFAULT 0)""")
            # The shape the Vibe sign-in stores: an oauth credential whose
            # access holds the minted key and whose refresh never rotates.
            db.execute("""INSERT INTO auth_credentials
                (provider, credential_type, data, identity_key)
                VALUES ('mistral', 'oauth', ?, 'vibe')""",
                       (json.dumps({"access": "fixture-key", "refresh": "fixture-key"}),))
        proxy = f"http://127.0.0.1:{fixture.port}"
        env = dict(os.environ, PIG_TEST_FAUX="1", PIG_STARTUP_TRACE="1",
                   PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                   PIG_HOME=directory, PI_HOME=directory, TERM="xterm-256color",
                   HTTPS_PROXY=proxy, https_proxy=proxy, SSL_CERT_FILE=str(ca_cert),
                   NO_PROXY="127.0.0.1,localhost", no_proxy="127.0.0.1,localhost")
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        for key in ("OMPIG_BROKER_URL", "OMPIG_BROKER_TOKEN", "OMPIG_BROKER_TOKEN_FILE",
                    "MISTRAL_API_KEY", "PI_OFFLINE", "PIG_OFFLINE", "HTTP_PROXY", "http_proxy"):
            env.pop(key, None)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        process = subprocess.Popen([str(binary), "--model", "mistral/devstral-medium-latest"],
                                   stdin=slave, stdout=slave, stderr=slave,
                                   cwd=root, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()

        def wait_for(text, timeout=45):
            """Wait for an observable frame, failing with the terminal tail."""
            observed = bytearray()
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if select.select([master], [], [], 0.05)[0]:
                    try:
                        data = os.read(master, 262144)
                    except OSError:
                        break
                    captured.extend(data)
                    observed.extend(data)
                if text in observed:
                    while select.select([master], [], [], 0.1)[0]:
                        data = os.read(master, 262144)
                        captured.extend(data)
                        observed.extend(data)
                    return bytes(observed)
                if process.poll() is not None:
                    break
            raise AssertionError(
                f"missing {text!r}; exit={process.poll()}; "
                f"requests={fixture.requests}; tail={bytes(captured[-3000:])!r}")

        try:
            wait_for(b"interactive-ready")
            os.write(master, b"/usage\r")
            wait_for(b"Vibe Credits", timeout=60)
            while select.select([master], [], [], 0.5)[0]:
                captured.extend(os.read(master, 262144))
            plain = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", bytes(captured)).decode("utf8", "replace")
            # The board reports the remaining share, so 42.5 percent used reads as
            # 57 percent free, beside the reset countdown the window carries.
            for token in ("Mistral", "Vibe Credits", "57%"):
                assert token in plain, f"/usage board missing {token!r}:\n{plain[-3000:]}"
            assert re.search(r"\b\d+d\d+h\b", plain), (
                f"/usage board missing the reset countdown:\n{plain[-3000:]}")
            assert "Usage unavailable" not in plain, "the board reported the probe as failed"
            assert "fixture-key" not in plain, "the board leaked the key"
            assert fixture.requests, "the probe never reached the billing route"
            for path, authorization in fixture.requests:
                assert path == "/v1/billing/budget", f"unexpected path {path}"
                assert authorization == "Bearer fixture-key", f"unexpected auth {authorization!r}"
            print("PASS: /usage paints the Vibe Code allowance read from "
                  f"https://{HOST}/v1/billing/budget ({len(fixture.requests)} request(s), "
                  "bearer sent, key not displayed)")
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
    """Mint the fixture certificate, start the proxy, drive the caller's binary."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="ompig-mistral-ca-") as directory:
        ca_cert, leaf_cert, leaf_key = mint_certificate(Path(directory))
        fixture = Fixture(leaf_cert, leaf_key)
        try:
            run(args.binary.resolve(), fixture, ca_cert)
        finally:
            fixture.stop()


if __name__ == "__main__":
    main()
