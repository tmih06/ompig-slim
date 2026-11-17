#!/usr/bin/env python3
"""Verify live subagent identity and thinking through a PTY and local SSE model."""
import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time


class ModelServer(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    advance = [threading.Event() for _ in range(3)]
    finished = threading.Event()
    spawned = False

    def log_message(self, *_):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        child = any("<active_agent name=" in str(m.get("content", ""))
                    for m in body["messages"] if m["role"] in ("system", "developer"))
        if child and body["model"] == "ui-primary":
            self.send_response(404)
            self.send_header("Content-Type", "application/json")
            self.send_header("Connection", "close")
            self.end_headers()
            self.close_connection = True
            self.wfile.write(json.dumps({"error": {"message": "Model unavailable", "type": "not_found"}}).encode())
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()
        self.close_connection = True
        try:
            if child:
                self.chunk({"role": "assistant", "reasoning_content": "Earlier context. " * 30 + "THOUGHT_WIDGET_FIRST"})
                for index, text in enumerate((" THOUGHT_HUB_SECOND", " THOUGHT_VIEWER_THIRD", " THOUGHT_WIDGET_FOURTH")):
                    if not type(self).advance[index].wait(30):
                        return
                    self.chunk({"reasoning_content": text})
                if not type(self).finished.wait(30):
                    return
                self.chunk({"content": "CHILD_UI_DONE"})
                self.chunk({}, "stop")
            elif not type(self).spawned:
                type(self).spawned = True
                args = {"prompt": "Think through the fixture.", "description": "Inspect live thought stream",
                        "role": "@slow", "tool": ["read"], "name": "named-ui-worker"}
                self.chunk({"role": "assistant", "tool_calls": [{"index": 0, "id": "ui-agent",
                    "type": "function", "function": {"name": "Agent", "arguments": json.dumps(args)}}]})
                self.chunk({}, "tool_calls")
            else:
                self.chunk({"role": "assistant", "content": "PARENT_UI_READY. Should I continue?"})
                self.chunk({}, "stop")
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

    def chunk(self, delta, reason=None):
        data = {"id": "subagent-ui", "choices": [{"index": 0, "delta": delta, "finish_reason": reason}]}
        self.wfile.write(("data: " + json.dumps(data) + "\n\n").encode())
        self.wfile.flush()


def plain(data):
    return re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", data)


def run(binary, mode):
    ModelServer.advance = [threading.Event() for _ in range(3)]
    ModelServer.finished = threading.Event()
    ModelServer.spawned = False
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ModelServer)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="ompig-subagent-ui-") as directory:
            root = Path(directory)
            agent = root / "agent"
            agent.mkdir()
            (root / ".pi").mkdir()
            (root / ".pi" / "subagents.json").write_text(json.dumps({"enabled": True}))
            (agent / "models.json").write_text(json.dumps({"providers": {"ui-fixture": {
                "api": "openai-completions", "baseUrl": f"http://127.0.0.1:{server.server_port}/v1",
                "apiKey": "local-fixture", "models": [
                    {"id": "ui-primary", "reasoning": True}, {"id": "ui-fixture", "reasoning": True}]}}}))
            (agent / "settings.json").write_text(json.dumps({"quietStartup": True, "tuiMode": mode,
                "enableInstallTelemetry": False, "extensionsAutoUpgrade": False,
                "modelRoles": {"default": "ui-fixture/ui-fixture", "slow": "ui-fixture/ui-primary"},
                "retry": {"maxRetries": 0, "modelFallback": True, "fallbackOn": "any",
                          "fallbackChains": {"slow": ["ui-fixture/ui-fixture"]}},
                "modelTags": {"slow": {"color": "error"}}}))
            env = {k: v for k, v in os.environ.items() if not k.endswith(("_API_KEY", "_AUTH_TOKEN", "_OAUTH_TOKEN"))}
            env.update(HOME=directory, PIG_HOME=directory, PI_HOME=directory, PI_OFFLINE="1",
                       PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0", PIG_STARTUP_TRACE="1", TERM="xterm-256color")
            for key in ("PIG_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                env[key] = str(agent)
            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 48, 160, 0, 0))
            process = subprocess.Popen([str(binary), "--no-extensions", "--model", "@slow", "--offline",
                "--no-context-files", "--no-skills"], cwd=root, env=env,
                stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
            os.close(slave)
            captured = bytearray()

            def wait(token):
                observed = bytearray()
                deadline = time.monotonic() + 25
                while time.monotonic() < deadline:
                    if select.select([master], [], [], .05)[0]:
                        try:
                            data = os.read(master, 262144)
                        except OSError:
                            break
                        observed.extend(data)
                        captured.extend(data)
                    if token in plain(bytes(observed)):
                        settle = time.monotonic() + .3
                        while time.monotonic() < settle and select.select([master], [], [], .05)[0]:
                            data = os.read(master, 262144)
                            observed.extend(data)
                            captured.extend(data)
                        return bytes(observed)
                    if process.poll() is not None:
                        break
                raise AssertionError(f"{mode}: missing {token!r}; exit={process.poll()}\n{bytes(captured[-8000:])!r}")

            try:
                startup = wait(b"interactive-ready")
                os.write(master, b"Exercise the live subagent UI.\r")
                widget = wait(b"THOUGHT_WIDGET_FIRST")
                assert re.search("[├└]─.*@slow named-ui-worker  Inspect live thought stream.*turns".encode(), plain(widget)), "tree row must show role, name, task, and usage"
                role_pattern = rb"(\x1b\[[0-9;]*m)@slow"
                parent_role = re.findall(role_pattern, startup + widget)
                assert parent_role, "main role color was not rendered"
                os.write(master, b"\x1ba")
                hub = wait(b"Agent Hub")
                assert b"named-ui-worker" in plain(hub), "hub lost the subagent name"
                assert b"ui-fixture/ui-fixture" in plain(hub), "hub must show the active fallback model, not ui-primary"
                hub_role = re.findall(role_pattern, hub)
                assert parent_role[-1] in hub_role, "hub role color differs from the main role catalog color"
                ModelServer.advance[0].set()
                wait(b"THOUGHT_HUB_SECOND")
                os.write(master, b"\r")
                wait(b"THOUGHT_WIDGET_FIRST")
                ModelServer.advance[1].set()
                wait(b"THOUGHT_VIEWER_THIRD")
                os.write(master, b"\x1b")
                wait(b"named-ui-worker")
                ModelServer.advance[2].set()
                wait(b"THOUGHT_WIDGET_FOURTH")
                assert not ModelServer.finished.is_set(), "proof ran after child completion"
                ModelServer.finished.set()
                print(f"{mode}: fallback model in hub, named tree row, catalog role color, live hub/inspector/main thinking before completion: OK")
            finally:
                ModelServer.finished.set()
                for event in ModelServer.advance:
                    event.set()
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
                os.close(master)
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    binary = Path(sys.argv[1]).resolve()
    for mode in ("fullscreen", "regular"):
        run(binary, mode)
