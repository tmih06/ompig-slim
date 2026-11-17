#!/usr/bin/env python3
"""Measure role cycling and session-model selection with an isolated delayed broker."""
import argparse
import base64
from collections import Counter
from contextlib import contextmanager
import fcntl
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import json
import os
from pathlib import Path
import pty
import select
import statistics
import struct
import subprocess
import tempfile
import termios
import threading
import time

import settings_fixture

SPEC = importlib.util.spec_from_file_location(
    "measure_model_menu", Path(__file__).with_name("measure-model-menu.py"))
MENU = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MENU)


class FixtureBroker(BaseHTTPRequestHandler):
    """Serve empty fixture credentials and preferences, with one delay per exchange."""

    def log_message(self, *_args):
        """Keep the evidence free of request headers and fixture credentials."""

    def do_GET(self):
        """Implement only the read contracts this offline client exercises."""
        self.server.requests.append(self.path)
        time.sleep(self.server.rtt)
        if self.path.startswith("/v1/items/"):
            data = self.server.items.get(self.path)
            raw = base64.b64decode(data) if data else b""
            payload = {"data": data, "revision": hashlib.sha256(raw).hexdigest()}
        elif self.path in ("/v1/accounts", "/v1/snapshot", "/v1/themes"):
            payload = []
        elif self.path.startswith("/v1/provider?"):
            payload = False
        elif self.path.startswith("/v1/account-count?"):
            payload = 0
        else:
            self.send_error(404)
            return
        self.answer(payload)

    def do_PUT(self):
        """Retain uploaded fixture preferences for idle reconciliation."""
        self.server.requests.append(self.path)
        time.sleep(self.server.rtt)
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.server.items[self.path] = request.get("data")
        self.answer(None)

    def do_POST(self):
        """Answer quota reads without mutating the fixture credential view."""
        self.server.requests.append(self.path)
        time.sleep(self.server.rtt)
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        if self.path != "/v1/credential" or request.get("operation") != "blocked":
            self.send_error(400)
            return
        self.answer(None)

    def answer(self, payload):
        """An exact child shutdown may cancel a background quota request."""
        data = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        try:
            self.wfile.write(data)
        except (BrokenPipeError, ConnectionResetError):
            pass


@contextmanager
def fixture_broker(rtt):
    """Own a loopback-only fixture with no provider traffic or real user state."""
    server = ThreadingHTTPServer(("127.0.0.1", 0), FixtureBroker)
    server.rtt, server.requests, server.items = rtt, [], {}
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        yield server
    finally:
        server.shutdown()
        server.server_close()
        worker.join()


def settle(master, seconds=2.2):
    """Drain outside the timer and let the broker's one-second credential view expire."""
    until = time.monotonic() + seconds
    while time.monotonic() < until:
        if select.select([master], [], [], 0.02)[0]:
            os.read(master, 1048576)


def measure(binary, rtt, profile_dir):
    """Time real shortcut inputs to their resulting UI, not internal setters."""
    with tempfile.TemporaryDirectory(prefix="omp-model-switch-") as directory, fixture_broker(rtt) as broker:
        root = Path(directory)
        agent = root / "agent"
        agent.mkdir()
        settings = {"quietStartup": True, "tuiMode": "regular",
                    "modelRoles": {"default": "anthropic/claude-sonnet-4-5",
                                   "smol": "anthropic/claude-haiku-4-5"},
                    "cycleOrder": ["default", "smol"]}
        path = agent / "settings.json"
        path.write_text(json.dumps(settings))
        env = {"PATH": os.environ["PATH"], "HOME": directory, "TERM": "xterm-256color",
               "PIG_HOME": directory, "PI_HOME": directory, "PIG_TEST_FAUX": "1",
               "PI_OFFLINE": "1", "PI_SKIP_VERSION_CHECK": "1", "PIG_STARTUP_TRACE": "1",
               "ANTHROPIC_API_KEY": "offline-fixture",
               "OMPIG_BROKER_URL": f"http://127.0.0.1:{broker.server_port}",
               "OMPIG_BROKER_TOKEN": "isolated-fixture"}
        for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
            env[key] = str(agent)
        if profile_dir:
            env.update(PIG_PROFILE="cpu,block,trace", PIG_PROFILE_DIR=str(profile_dir.resolve()))
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
        process = subprocess.Popen([str(binary), "-ne", "--model", "@default", "--no-session"],
                                   cwd=root, env=env, stdin=slave, stdout=slave, stderr=slave,
                                   start_new_session=True)
        os.close(slave)
        timings, traffic = {}, {}

        def action(name, keys, marker):
            """Count all broker traffic during the measured UI operation."""
            before = len(broker.requests)
            started = time.perf_counter()
            os.write(master, keys)
            MENU.wait_for(master, process, marker, timeout=30)
            timings[name] = (time.perf_counter() - started) * 1000
            traffic[name] = dict(Counter(broker.requests[before:]))

        try:
            MENU.wait_for(master, process, b"interactive-ready")
            settle(master)
            action("cycle_ms", b"\x10", b"Switched to Claude Haiku 4.5")
            settle(master)
            action("picker_open_ms", b"\x1bp", b"Switch Model")
            os.write(master, b"@default")
            MENU.wait_for(master, process, b"@default")
            settle(master)
            action("picker_select_ms", b"\r", b"Model: @default")
            saved = settings_fixture.settled_settings(path)
            assert saved["modelRoles"] == settings["modelRoles"], "selection changed saved roles"
            assert saved["cycleOrder"] == settings["cycleOrder"], "selection changed cycle order"
            settle(master, 0.2)
            os.write(master, b"/quit\r")
            quit_output = bytearray()
            deadline = time.monotonic() + 20
            while process.poll() is None and time.monotonic() < deadline:
                if select.select([master], [], [], 0.02)[0]:
                    try:
                        quit_output.extend(os.read(master, 1048576))
                    except OSError:
                        break
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired as error:
                raise AssertionError(f"quit did not settle: {quit_output[-4000:]!r}") from error
            assert process.returncode == 0, f"child exit={process.returncode}"
            return {"timings": timings, "broker_requests": traffic, "errors": 0,
                    "completed_switches": 2, "saved_assignments_unchanged": True}
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
    """Alternate binaries so shared-machine load does not favor either version."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--compare", type=Path)
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--rtt-ms", type=float, default=150)
    parser.add_argument("--profile-dir", type=Path, help="Attribution only; do not report profiled timings")
    parser.add_argument("--check-ui", action="store_true", help="Reject credential snapshots during cycle display or picker opening")
    args = parser.parse_args()
    binaries = [args.binary.resolve()]
    if args.compare:
        binaries.append(args.compare.resolve())
    results = {str(binary): [] for binary in binaries}
    for _ in range(args.runs):
        for binary in binaries:
            results[str(binary)].append(measure(binary, args.rtt_ms / 1000, args.profile_dir))
    if args.check_ui:
        for runs in results.values():
            for run in runs:
                for action in ("cycle_ms", "picker_open_ms"):
                    assert run["broker_requests"][action].get("/v1/snapshot", 0) == 0, f"{action} waited for credential I/O"
    summaries = {}
    for binary, runs in results.items():
        summaries[binary] = {}
        for metric in runs[0]["timings"]:
            values = [run["timings"][metric] for run in runs]
            summaries[binary][metric] = {"median": statistics.median(values),
                                         "range": [min(values), max(values)]}
    print(json.dumps({"fixture_broker_rtt_ms": args.rtt_ms, "runs_per_binary": args.runs,
                      "profiled": bool(args.profile_dir), "summary_ms": summaries,
                      "runs": results}, indent=2))


if __name__ == "__main__":
    main()
