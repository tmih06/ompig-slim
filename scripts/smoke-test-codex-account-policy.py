#!/usr/bin/env python3
import argparse
import base64
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
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

HOST = "chatgpt.com"
PROVIDER = "openai-codex"
LUNA = "gpt-6-luna"
PAID = "gpt-6.1-sol"
MESSAGE = "Codex account policy fixture passed."
USAGE_PATH = "/backend-api/wham/usage"
VERIFIED_PATH = "/backend-api/accounts/verified_access"


def mint_certificate(directory):
    ca_key, ca_cert = directory / "ca.key", directory / "ca.pem"
    leaf_key = directory / "leaf.key"
    leaf_csr, leaf_cert = directory / "leaf.csr", directory / "leaf.pem"
    extensions = directory / "leaf.ext"
    extensions.write_text(
        f"subjectAltName=DNS:{HOST}\nbasicConstraints=CA:FALSE\n"
        "keyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n")
    run = lambda *args: subprocess.run(["openssl", *args], check=True, capture_output=True)
    run("req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2",
        "-keyout", str(ca_key), "-out", str(ca_cert), "-subj", "/CN=omp-codex-policy-ca")
    run("req", "-newkey", "rsa:2048", "-nodes", "-keyout", str(leaf_key),
        "-out", str(leaf_csr), "-subj", f"/CN={HOST}")
    run("x509", "-req", "-in", str(leaf_csr), "-CA", str(ca_cert),
        "-CAkey", str(ca_key), "-CAcreateserial", "-days", "2",
        "-out", str(leaf_cert), "-extfile", str(extensions))
    return ca_cert, leaf_cert, leaf_key


def read_headers(connection):
    request = bytearray()
    while b"\r\n\r\n" not in request:
        chunk = connection.recv(4096)
        if not chunk:
            return None
        request.extend(chunk)
        if len(request) > 65536:
            return None
    return bytes(request).split(b"\r\n\r\n", 1)[0]


class CodexFixture:
    def __init__(self, leaf_cert, leaf_key):
        self.lock = threading.Lock()
        self.accounts = {}
        self.usage_requests = []
        self.model_requests = []
        self.denied_hosts = []
        self.unexpected_paths = []
        self.proxy_errors = []

        fixture = self

        class Inference(http.server.BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length", "0")))
                account = fixture.account_for(self.headers.get("Authorization"))
                if account is None:
                    self.send_error(401)
                    return
                with fixture.lock:
                    fixture.model_requests.append((account["label"], self.path))
                    response_text = f"{MESSAGE} {len(fixture.model_requests)}"
                events = [
                    {"type": "response.created", "response": {"id": "fixture", "status": "in_progress"}},
                    {"type": "response.output_item.added", "output_index": 0,
                     "item": {"type": "message", "id": "fixture-message", "role": "assistant", "content": []}},
                    {"type": "response.content_part.added", "output_index": 0, "content_index": 0,
                     "part": {"type": "output_text", "text": ""}},
                    {"type": "response.output_text.delta", "output_index": 0, "content_index": 0,
                     "delta": response_text},
                    {"type": "response.completed", "response": {"id": "fixture", "status": "completed",
                     "output": [], "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}},
                ]
                wire = "".join("data: " + json.dumps(event) + "\n\n" for event in events)
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Content-Length", str(len(wire.encode())))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(wire.encode())

            def do_GET(self):
                self.send_error(403)

        self.inference = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Inference)
        self.inference.daemon_threads = True
        self.inference_thread = threading.Thread(target=self.inference.serve_forever, daemon=True)
        self.inference_thread.start()

        self.tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        self.tls.load_cert_chain(str(leaf_cert), str(leaf_key))
        self.listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.listener.bind(("127.0.0.1", 0))
        self.listener.listen(16)
        self.listener.settimeout(0.2)
        self.proxy_port = self.listener.getsockname()[1]
        self.stopping = threading.Event()
        self.proxy_thread = threading.Thread(target=self.serve_proxy, daemon=True)
        self.proxy_thread.start()

    @property
    def inference_url(self):
        return f"http://127.0.0.1:{self.inference.server_port}"

    @property
    def proxy_url(self):
        return f"http://127.0.0.1:{self.proxy_port}"

    def add_account(self, label, identity, jwt_plan, usage_plan, used_percent, reset_seconds):
        auth = {"chatgpt_account_id": identity}
        if jwt_plan is not None:
            auth["chatgpt_plan_type"] = jwt_plan
        claims = {"exp": int(time.time()) + 3600, "https://api.openai.com/auth": auth}
        payload = base64.urlsafe_b64encode(json.dumps(claims).encode()).decode().rstrip("=")
        token = f"e30.{payload}.fixture-{label}"
        account = {"label": label, "identity": identity, "token": token,
                   "usage_plan": usage_plan, "used_percent": used_percent,
                   "reset_at": int(time.time() + reset_seconds)}
        self.accounts[token] = account
        return account

    def account_for(self, authorization):
        if not authorization or not authorization.startswith("Bearer "):
            return None
        return self.accounts.get(authorization.removeprefix("Bearer "))

    def serve_proxy(self):
        while not self.stopping.is_set():
            try:
                client, _ = self.listener.accept()
            except socket.timeout:
                continue
            except OSError:
                return
            threading.Thread(target=self.answer_proxy, args=(client,), daemon=True).start()

    def answer_proxy(self, client):
        try:
            client.settimeout(15)
            head = read_headers(client)
            if head is None:
                return
            parts = head.split(b"\r\n", 1)[0].split()
            target = parts[1].decode("ascii", "replace") if len(parts) > 1 else "unknown"
            if target.lower() != f"{HOST}:443":
                with self.lock:
                    self.denied_hosts.append(target)
                client.sendall(b"HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
                return
            client.sendall(b"HTTP/1.1 200 Connection established\r\n\r\n")
            with self.tls.wrap_socket(client, server_side=True) as tunnel:
                request = read_headers(tunnel)
                if request is None:
                    return
                lines = request.split(b"\r\n")
                request_line = lines[0].decode("ascii", "replace").split()
                path = request_line[1] if len(request_line) > 1 else ""
                headers = {}
                for line in lines[1:]:
                    if b":" in line:
                        name, value = line.split(b":", 1)
                        headers[name.strip().lower().decode("ascii", "replace")] = value.strip().decode("utf8", "replace")
                account = self.account_for(headers.get("authorization"))
                if account is None:
                    status, body = "401 Unauthorized", b'{"error":"fixture token required"}'
                elif path.split("?", 1)[0] == USAGE_PATH:
                    body = json.dumps({
                        "plan_type": account["usage_plan"],
                        "rate_limit": {
                            "allowed": True,
                            "limit_reached": False,
                            "primary_window": {
                                "used_percent": account["used_percent"],
                                "limit_window_seconds": 18000,
                                "reset_at": account["reset_at"],
                            },
                            "secondary_window": account.get("secondary_window"),
                        },
                    }).encode()
                    status = "200 OK"
                    with self.lock:
                        self.usage_requests.append((account["label"], path))
                elif path.split("?", 1)[0] == VERIFIED_PATH:
                    body, status = b'{"has_access":false}', "200 OK"
                else:
                    body, status = b'{"error":"fixture route not found"}', "404 Not Found"
                    with self.lock:
                        self.unexpected_paths.append(path)
                tunnel.sendall(
                    f"HTTP/1.1 {status}\r\nContent-Type: application/json\r\n"
                    f"Content-Length: {len(body)}\r\nConnection: close\r\n\r\n".encode() + body)
        except (OSError, ssl.SSLError) as error:
            with self.lock:
                self.proxy_errors.append(type(error).__name__)
        finally:
            try:
                client.close()
            except OSError:
                pass

    def wait_for_usage(self, labels, offset, timeout=45):
        deadline = time.monotonic() + timeout
        expected = set(labels)
        while time.monotonic() < deadline:
            with self.lock:
                seen = {label for label, path in self.usage_requests[offset:]
                        if path.split("?", 1)[0] == USAGE_PATH}
            if expected <= seen:
                return
            time.sleep(0.05)
        raise AssertionError(f"Codex usage fixture did not receive /wham/usage for {sorted(expected - seen)}")

    def model_calls(self):
        with self.lock:
            return list(self.model_requests)

    def usage_count(self):
        with self.lock:
            return len(self.usage_requests)

    def close(self):
        self.stopping.set()
        self.listener.close()
        self.inference.shutdown()
        self.inference.server_close()


def clean_env(root, agent, fixture, ca_cert):
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(root),
        "TMPDIR": str(root),
        "PIG_HOME": str(root),
        "PI_HOME": str(root),
        "PI_SKIP_VERSION_CHECK": "1",
        "PI_TELEMETRY": "0",
        "PIG_STARTUP_TRACE": "1",
        "TERM": "xterm-256color",
        "SSL_CERT_FILE": str(ca_cert),
        "HTTPS_PROXY": fixture.proxy_url,
        "https_proxy": fixture.proxy_url,
        "HTTP_PROXY": fixture.proxy_url,
        "http_proxy": fixture.proxy_url,
        "ALL_PROXY": fixture.proxy_url,
        "all_proxy": fixture.proxy_url,
        "NO_PROXY": "127.0.0.1,localhost",
        "no_proxy": "127.0.0.1,localhost",
    }
    for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
        env[key] = str(agent)
    assert "PI_OFFLINE" not in env and "PIG_OFFLINE" not in env
    return env


def configure_profile(binary, root, fixture, ca_cert, accounts):
    agent = root / "agent"
    agent.mkdir(parents=True)
    models = {
        "providers": {
            PROVIDER: {
                "api": "openai-codex-responses",
                "baseUrl": fixture.inference_url,
                "models": [
                    {"id": LUNA, "name": "GPT-6 Luna fixture", "contextWindow": 128000, "maxTokens": 1024},
                    {"id": PAID, "name": "GPT-6.1 Sol fixture", "contextWindow": 128000, "maxTokens": 1024},
                ],
            },
        },
    }
    (agent / "models.json").write_text(json.dumps(models))
    (agent / "settings.json").write_text(json.dumps({
        "quietStartup": True,
        "modelRoles": {"default": f"{PROVIDER}/{LUNA}"},
        "retry": {"maxRetries": 0},
    }))
    env = clean_env(root, agent, fixture, ca_cert)

    def run(*args):
        return subprocess.run([str(binary), *args], cwd=root, env=env,
                              capture_output=True, text=True, timeout=45, check=True)

    run("auth", "accounts", "--json")
    with sqlite3.connect(agent / "agent.db") as db:
        for account in accounts:
            credential = {
                "access": account["token"],
                "accountId": account["identity"],
                "expires": int((time.time() + 3600) * 1000),
            }
            db.execute("""INSERT INTO auth_credentials
                (provider, credential_type, data, identity_key)
                VALUES (?, 'oauth', ?, ?)""",
                       (PROVIDER, json.dumps(credential), account["identity"]))
    run("auth", "use", PROVIDER, "plus")
    return agent, env


def wait_for(master, process, captured, predicate, label, timeout=45):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if select.select([master], [], [], 0.05)[0]:
            try:
                data = os.read(master, 262144)
            except OSError:
                break
            captured.extend(data)
        if predicate(bytes(captured)):
            return
        if process.poll() is not None:
            break
    raise AssertionError(f"{label} not observed; exit={process.poll()}; tail={bytes(captured[-3000:])!r}")


def prompt(binary, root, agent, fixture, ca_cert, model, session, usage_labels, resume=False):
    env = clean_env(root, agent, fixture, ca_cert)
    args = [str(binary), "-ne", "--no-tools", "--model", f"{PROVIDER}/{model}",
            "--session-dir", str(root / "sessions" / session)]
    if resume:
        args.append("--continue")
    usage_offset = fixture.usage_count()
    model_offset = len(fixture.model_calls())
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    process = subprocess.Popen(args, cwd=root, env=env, stdin=slave, stdout=slave, stderr=slave,
                               start_new_session=True)
    os.close(slave)
    captured = bytearray()
    try:
        wait_for(master, process, captured, lambda data: b"interactive-ready" in data, "interactive startup")
        fixture.wait_for_usage(usage_labels, usage_offset)
        os.write(master, b"Say hello.\r")
        response_marker = f"{MESSAGE} {model_offset + 1}".encode()
        wait_for(master, process, captured,
                 lambda data: len(fixture.model_calls()) > model_offset and response_marker in data,
                 "Codex fixture response")
    finally:
        if process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        os.close(master)
    calls = fixture.model_calls()[model_offset:]
    if len(calls) != 1:
        raise AssertionError(f"expected one local inference request, got {calls}")
    return calls[0][0]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    if not binary.is_file():
        parser.error(f"CLI binary does not exist: {binary}")

    with tempfile.TemporaryDirectory(prefix="omp-codex-policy-ca-") as ca_directory:
        ca_cert, leaf_cert, leaf_key = mint_certificate(Path(ca_directory))
        fixture = CodexFixture(leaf_cert, leaf_key)
        try:
            main_accounts = [
                fixture.add_account("main/plus", "plus", "plus", "plus", 20, 4 * 3600),
                fixture.add_account("main/b", "b", None, "free", 70, 3 * 3600),
                fixture.add_account("main/a", "a", None, "free", 0, 5 * 3600),
            ]
            conflict_accounts = [
                fixture.add_account("conflict/plus", "plus", "plus", "plus", 20, 4 * 3600),
                fixture.add_account("conflict/free", "free-conflict", "plus", "free", 0, 5 * 3600),
            ]
            with tempfile.TemporaryDirectory(prefix="omp-codex-policy-main-") as directory:
                root = Path(directory)
                agent, _ = configure_profile(binary, root, fixture, ca_cert, main_accounts)
                labels = [account["label"] for account in main_accounts]
                first = prompt(binary, root, agent, fixture, ca_cert, LUNA, "one", labels)
                second = prompt(binary, root, agent, fixture, ca_cert, LUNA, "two", labels)
                resumed = prompt(binary, root, agent, fixture, ca_cert, LUNA, "one", labels, resume=True)
                paid = prompt(binary, root, agent, fixture, ca_cert, PAID, "paid", labels)
                third = prompt(binary, root, agent, fixture, ca_cert, LUNA, "three", labels)
                fourth = prompt(binary, root, agent, fixture, ca_cert, LUNA, "four", labels)
                main_calls = [first, second, resumed, paid, third, fourth]

            with tempfile.TemporaryDirectory(prefix="omp-codex-policy-conflict-") as directory:
                root = Path(directory)
                agent, _ = configure_profile(binary, root, fixture, ca_cert, conflict_accounts)
                conflict = prompt(binary, root, agent, fixture, ca_cert, LUNA, "conflict",
                                  [account["label"] for account in conflict_accounts])

            if fixture.unexpected_paths or fixture.proxy_errors:
                raise AssertionError(
                    f"usage fixture rejected unexpected canonical-host traffic: "
                    f"paths={fixture.unexpected_paths}, proxy_errors={fixture.proxy_errors}")

            expected = ["main/a", "main/a", "main/a", "main/plus", "main/a", "main/a"]
            if main_calls != expected:
                raise AssertionError(
                    f"model accounts were {main_calls}, want most remaining quota, sticky resume, "
                    f"and paid-only routing {expected}; "
                    f"conflicting usage case selected {conflict}")
            if conflict != "conflict/free":
                raise AssertionError(
                    f"fresh free usage plan selected {conflict}, want conflict/free despite its plus JWT")
            print("PASS: GPT-6 Luna prefers the Free account with the most quota left, "
                  "resumes stickily, and keeps free accounts off paid-only models")
            print("PASS: fresh /wham/usage free plan overrides a conflicting plus JWT")
        finally:
            fixture.close()


if __name__ == "__main__":
    main()
