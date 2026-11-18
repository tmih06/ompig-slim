#!/usr/bin/env python3
"""Drive /usage, Tab with a real loopback broker and synthetic Codex OAuth accounts."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import pty
import re
import runpy
import socket
import struct
import subprocess
import tempfile
import termios
import time
import urllib.error
import urllib.request

fixture_api = runpy.run_path(str(Path(__file__).with_name("smoke-test-codex-account-policy.py")))
CASES = (("openai-codex", fixture_api["LUNA"], "fixture-free"),
         ("codex-json", fixture_api["PAID"], "fixture-plus"))
TOKEN = "local-codex-usage-fixture"
ANSI = re.compile(rb"\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07")


def check(client, root, fixture, ca, url, provider, model, expected):
    root.mkdir()
    agent = root / "agent"
    agent.mkdir()
    (agent / "models.json").write_text(json.dumps({"providers": {provider: {
        "api": "openai-codex-responses", "baseUrl": fixture.inference_url,
        "models": [{"id": model, "name": model, "contextWindow": 128000, "maxTokens": 1024}],
    }}}))
    (agent / "settings.json").write_text('{"quietStartup":true,"tuiMode":"regular"}')
    env = fixture_api["clean_env"](root, agent, fixture, ca)
    env.update(OMPIG_BROKER_URL=url, OMPIG_BROKER_TOKEN=TOKEN)
    args = [str(client), "-ne", "--no-tools", "--model", f"{provider}/{model}"]
    offset = len(fixture.model_calls())
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    proc = subprocess.Popen(args, cwd=root, env=env, stdin=slave, stdout=slave, stderr=slave,
                            start_new_session=True)
    os.close(slave)
    captured = bytearray()

    def wait(predicate, what):
        fixture_api["wait_for"](master, proc, captured, predicate, what, timeout=20)

    try:
        wait(lambda data: b"interactive-ready" in data, "interactive startup")
        os.write(master, b"/usage\r")
        wait(lambda data: b"tab/enter details" in data, "/usage dashboard")
        captured.clear()
        os.write(master, b"\t")
        wait(lambda data: b"Usage \xc2\xb7 Details" in ANSI.sub(b"", data), "Tab details")
        wait(lambda data: f"in use by this session: {expected}".encode() in ANSI.sub(b"", data),
             f"{provider}/{model} account label {expected}")
        assert len(fixture.model_calls()) == offset, "account label required an inference request"
        if provider == "codex-json":
            print(f"PASS {provider}/{model} preview showed {expected}", flush=True)
            return
        captured.clear()
        os.write(master, b"\x1b")
        wait(lambda data: b"tab/enter details" in data, "back to usage dashboard")
        captured.clear()
        os.write(master, b"\x1b")
        wait(lambda data: b"\x1b[?1049l" in data, "return to the regular prompt screen")
        os.write(master, b"Say hello.\r")
        marker = f"{fixture_api['MESSAGE']} {offset + 1}".encode()
        wait(lambda data: marker in ANSI.sub(b"", data), "fixture inference response")
        assert [label for label, _ in fixture.model_calls()[offset:]] == [f"{provider}/{expected}"]
        print(f"PASS {root.name}: {provider}/{model} preview and request used {expected}", flush=True)
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
        os.close(master)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("client", type=Path)
    parser.add_argument("broker", type=Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="ompig-broker-usage-") as directory:
        root = Path(directory)
        ca, cert, key = fixture_api["mint_certificate"](root)
        fixture = fixture_api["CodexFixture"](cert, key)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            bind = f"127.0.0.1:{sock.getsockname()[1]}"
        url = f"http://{bind}"
        env = fixture_api["clean_env"](root, root / "unused", fixture, ca)
        env["OMPIG_BROKER_TOKEN"] = TOKEN
        broker = subprocess.Popen([str(args.broker.resolve()), "--bind", bind, "--data-dir", str(root / "vault")],
                                  env=env, cwd=root)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

        def vault(**body):
            request = urllib.request.Request(url + "/v1/credential", json.dumps(body).encode(), headers={
                "Authorization": "Bearer " + TOKEN, "Content-Type": "application/json",
            })
            with opener.open(request, timeout=5) as response:
                return json.load(response)

        try:
            deadline = time.monotonic() + 5
            while True:
                try:
                    with opener.open(url + "/healthz", timeout=1) as response:
                        assert response.read() == b"ok\n"
                    break
                except urllib.error.URLError:
                    if broker.poll() is not None or time.monotonic() >= deadline:
                        raise
                    time.sleep(0.05)
            for provider, _, _ in CASES:
                for identity, plan, used in (("fixture-free", "free", 0),
                                             ("fixture-free-low", "free", 20),
                                             ("fixture-plus", "plus", 0)):
                    account = fixture.add_account(f"{provider}/{identity}", identity, "plus", plan, used, 3600)
                    account["secondary_window"] = {
                        "used_percent": 70 if identity == "fixture-free" else 20,
                        "limit_window_seconds": 7 * 24 * 3600,
                        "reset_at": account["reset_at"] + 24 * 3600,
                    }
                    vault(operation="save", provider=provider, accountKey=identity, credential={
                        "type": "oauth", "access": account["token"], "accountId": identity,
                        "expires": int((time.time() + 3600) * 1000), "refresh": "fixture-refresh",
                    })
                vault(operation="select", provider=provider, accountKey="fixture-plus")
            for provider, model, expected in CASES:
                check(args.client.resolve(), root / provider, fixture, ca, url, provider, model, expected)
            provider, model, expected = CASES[0]
            check(args.client.resolve(), root / "second-luna-session", fixture, ca, url, provider, model, expected)
        finally:
            broker.terminate()
            try:
                broker.wait(timeout=5)
            except subprocess.TimeoutExpired:
                broker.kill()
                broker.wait()
            fixture.close()


if __name__ == "__main__":
    main()
