#!/usr/bin/env python3
"""Capture real OMP/slim PTYs against isolated model fixtures (requires pyte).

No prompts are sent. Both fixture provider endpoints are unreachable loopback;
no real credentials, settings, or session files are read. Outputs include raw
ANSI and display-cell text so layout and color can be inspected separately.
"""
import argparse
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import signal
import struct
import subprocess
import termios
import time

import pyte


def capture(binary, name, output, columns, rows):
    """Boot an actual CLI, visit manager/provider/roles/pickers, and retain frames."""
    profile = output / f"{name}-{columns}x{rows}-profile"
    agent = profile / "agent"
    agent.mkdir(parents=True, exist_ok=True)
    roles = {"default": "fixture/alpha:high", "review": "fixture/beta"}
    settings = {"quietStartup": True, "tuiMode": "regular", "setupVersion": 999,
                "defaultProvider": "fixture", "defaultModel": "alpha",
                "defaultThinkingLevel": "high", "modelRoles": roles,
                "cycleOrder": ["default", "review"], "disabledProviders": ["local", "web"]}
    # JSON is also valid YAML: give each CLI its own native config entry point.
    for filename in ("settings.json", "config.yml"):
        (agent / filename).write_text(json.dumps(settings))
    models = {"providers": {"fixture": {
        "baseUrl": "http://127.0.0.1:1/v1", "api": "openai-completions",
        "apiKey": "offline-fixture", "models": [
            {"id": "alpha", "name": "Alpha", "reasoning": False,
             "input": ["text", "image"], "contextWindow": 128000, "maxTokens": 16000,
             "cost": {"input": 2.5, "output": 10, "cacheRead": 1.25, "cacheWrite": 0}},
            {"id": "beta", "name": "Beta", "reasoning": False,
             "input": ["text"], "contextWindow": 64000, "maxTokens": 8000,
             "cost": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0}},
        ]}}}
    for filename in ("models.json", "models.yml"):
        (agent / filename).write_text(json.dumps(models))
    env = {"PATH": os.environ["PATH"], "HOME": str(profile), "TERM": "xterm-256color",
           "COLORTERM": "truecolor", "PI_OFFLINE": "1", "PI_SKIP_VERSION_CHECK": "1",
           "PI_TELEMETRY": "0", "PI_HOME": str(profile), "PIG_HOME": str(profile),
           "PI_CODING_AGENT_DIR": str(agent), "PIG_CODING_AGENT_DIR": str(agent),
           "OMPIG_SLIM_CODING_AGENT_DIR": str(agent)}
    args = [str(binary), "--model", "fixture/alpha", "--models", "fixture/*",
            "--no-session", "--no-extensions"]
    if name == "omp":
        args += ["--no-title", "--no-skills", "--no-rules", "--no-lsp", "--no-context-files"]
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", rows, columns, 0, 0))
    process = subprocess.Popen(args, cwd=profile, env=env, stdin=slave,
                               stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    screen = pyte.Screen(columns, rows)
    stream = pyte.ByteStream(screen)
    raw = bytearray()

    def pump(seconds):
        """Feed real terminal output, answering capability/color/cursor queries only."""
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            if not select.select([master], [], [], 0.05)[0]:
                continue
            try:
                data = os.read(master, 1048576)
            except OSError:
                break
            raw.extend(data)
            stream.feed(data)
            for query, response in (
                (b"\x1b[6n", b"\x1b[1;1R"),
                (b"\x1b[c", b"\x1b[?1;2c"),
                (b"\x1b[0c", b"\x1b[?1;2c"),
                (b"\x1b]11;?", b"\x1b]11;rgb:1010/1010/1010\x1b\\"),
                (b"\x1b]10;?", b"\x1b]10;rgb:dddd/dddd/dddd\x1b\\"),
            ):
                if query in data:
                    os.write(master, response)

    def save(view, expected):
        """Fail on wrong screens and store text, cell styles, and unmodified ANSI."""
        lines = screen.display
        text = "\n".join(lines) + "\n"
        if expected not in text:
            raise AssertionError(f"{name}/{view}: expected {expected!r}; exit={process.poll()}\n{text}")
        prefix = output / f"{name}-{columns}x{rows}-{view}"
        prefix.with_suffix(".txt").write_text(text)
        prefix.with_suffix(".ansi").write_bytes(bytes(raw))
        cells = [[{"text": screen.buffer[y][x].data, "fg": screen.buffer[y][x].fg,
                   "bg": screen.buffer[y][x].bg, "bold": screen.buffer[y][x].bold}
                  for x in range(columns)] for y in range(rows)]
        prefix.with_suffix(".cells.json").write_text(json.dumps(cells))

    def send(keys, pause=1):
        """Send keys through the PTY, never a model prompt or acceptance action."""
        os.write(master, keys)
        pump(pause)

    try:
        pump(10)
        save("startup", "")
        send(b"/models\r" if name == "omp" else b"/model\r", 3)
        save("hub", "All available models")
        send(b"\x1b[B")
        save("provider", "fixture")
        send(b"\x1b[A")
        send(b"\x1b[A")
        save("roles", "Model roles")
        send(b"\x1b[C")
        send(b"\r")
        save("assignment", "Assign")
        send(b"\x1b")
        send(b"\x1b")
        send(b"\x1bp", 2)
        save("picker", "Switch Model")
        send(b"@")
        save("quick-roles", "@review")
        send(b"\x1b")
        send(b"\x1bp")
        save("task", "Switch Task Model")
        print(f"Captured {name} at {columns}x{rows}: {output}")
    finally:
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
        os.close(master)


def compare_frames(output, columns, rows):
    """Compare full managers and cropped sheets, including nonblank cell styling."""
    for view in ("hub", "provider", "roles", "assignment", "picker", "quick-roles", "task"):
        height = max(14, int(max(16, rows) * 0.4)) if view in ("picker", "quick-roles", "task") else rows
        frames = [json.loads((output / f"{name}-{columns}x{rows}-{view}.cells.json").read_text())[-height:]
                  for name in ("omp", "slim")]
        differences = []
        for y, (reference, actual) in enumerate(zip(*frames)):
            for x, (expected, got) in enumerate(zip(reference, actual)):
                if expected["text"] != got["text"] or (expected["text"].strip() and expected != got):
                    differences.append((y, x, expected, got))
        print(f"{view}: {len(differences)} differing display/style cells")
        if differences:
            raise AssertionError(f"{view} differs from OMP: {differences[:8]}")


def main():
    """Select local binaries and output paths; capture both at identical geometry."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--omp", type=Path, required=True)
    parser.add_argument("--slim", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--columns", type=int, default=120)
    parser.add_argument("--rows", type=int, default=40)
    args = parser.parse_args()
    if args.columns < 40 or args.rows < 16:
        parser.error("comparison requires at least 40 columns and 16 rows")
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    for name, binary in (("omp", args.omp), ("slim", args.slim)):
        binary = binary.resolve()
        version = subprocess.run([str(binary), "--version"], capture_output=True, text=True, check=True)
        (args.output / f"{name}-version.txt").write_text(version.stdout)
        capture(binary, name, args.output, args.columns, args.rows)
    compare_frames(args.output, args.columns, args.rows)


if __name__ == "__main__":
    main()
