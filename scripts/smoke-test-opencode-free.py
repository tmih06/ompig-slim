#!/usr/bin/env python3
"""Check the complete native free-model catalog and hub; optionally test live tools."""

import argparse
import fcntl
import os
from pathlib import Path
import pty
import re
import secrets
import select
import struct
import subprocess
import tempfile
import termios
import time


CHAT_MODELS = {
    "big-pickle": "Big Pickle",
    "mimo-v2.6-flash-free": "MiMo-V2.6-Flash Free",
    "deepseek-v4.1-flash-free": "DeepSeek V4.1 Flash Free",
    "glm-5.3-flash-free": "GLM-5.3-Flash Free",
    "iceman-free": "Iceman Free",
    "inkling-2-small-free": "Inkling 2 Small Free",
    "beam-free": "Beam Free",
    "flint-40b-free": "Flint 40B Free",
    "longcat-2.5-free": "LongCat 2.5 Free",
    "hy4-flash-free": "Hy4 flash Free",
    "nemotron-3-ultra-free": "Nemotron 3 Ultra Free",
    "nemotron-3.5-lightning-free": "Nemotron 3.5 Lightning Free",
    "nemotron-3.5-super-vl-free": "Nemotron 3.5 Super VL Free",
    "minimax-m3.1-flash-free": "MiniMax M3.1 Flash Free",
}


def verify_model_hub(binary, root, env):
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    process = subprocess.Popen(
        [str(binary), "--offline", "-ne", "--model", "test-faux/model"],
        cwd=root, env={**env, "TERM": "xterm-256color", "PIG_TEST_FAUX": "1",
                       "PIG_STARTUP_TRACE": "1"},
        stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)

    def read_until(predicate, label):
        captured = bytearray()
        end = time.monotonic() + 20
        while time.monotonic() < end:
            if select.select([master], [], [], 0.05)[0]:
                try:
                    captured.extend(os.read(master, 262144))
                except OSError:
                    break
            visible = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", bytes(captured))
            if predicate(visible):
                return
            if process.poll() is not None:
                break
        raise AssertionError(f"{label} not observed: {visible[-2000:]!r}")

    try:
        read_until(lambda data: b"interactive-ready" in data, "startup")
        os.write(master, b"/model\r")
        read_until(lambda data: b"All models" in data and b"Models" in data, "model hub")
        os.write(master, b"\x1b[C")
        for model, name in {**CHAT_MODELS, "jev-1.13-free": "Jev 1.13 Free"}.items():
            query = ("opencode-free/" + model).encode()
            os.write(master, b"\x15" + query)
            read_until(lambda data: query in data and name.encode() in data,
                       "OpenCode Free model row " + model)
        print("PASS /model hub search displays all 14 chat models and Jev judge")
    finally:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
        os.close(master)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--live", action="store_true", help="contact OpenCode's free inference endpoint")
    args = parser.parse_args()
    binary = args.binary.resolve()
    with tempfile.TemporaryDirectory(prefix="ompig-native-free-") as root:
        env = {
            "HOME": root,
            "PIG_HOME": root + "/pig",
            "PI_HOME": root + "/pig",
            "OMPIG_SLIM_CODING_AGENT_DIR": root + "/agent",
            "PIG_CODING_AGENT_DIR": root + "/agent",
            "PI_CODING_AGENT_DIR": root + "/agent",
            "PATH": root + "/empty-path",
            "PI_OFFLINE": "1",
            "PI_TELEMETRY": "0",
            "PI_SKIP_VERSION_CHECK": "1",
        }
        def run(arguments, timeout=30):
            result = subprocess.run([str(binary), "--offline", *arguments], cwd=root,
                                    env=env, capture_output=True, text=True, timeout=timeout)
            assert result.returncode == 0, (result.stdout, result.stderr)
            return result.stdout

        listing = run(["--list-models", "opencode-free"])
        assert "opencode-free" in listing, listing
        for model in CHAT_MODELS:
            assert model in listing, (model, listing)
        print("PASS native free-model discovery without credentials or other executables")
        verify_model_hub(binary, root, env)
        if args.live:
            for model in CHAT_MODELS:
                token = "native-free-" + secrets.token_hex(16)
                Path(root, "token.txt").write_text(token + "\n")
                arguments = ["--no-session", "--model", "opencode-free/" + model,
                             "--tools", "read", "-p",
                             "Read token.txt using the read tool. Reply only with its exact contents. "
                             "Do not guess the contents."]
                result = subprocess.run([str(binary), "--offline", *arguments], cwd=root,
                                        env=env, capture_output=True, text=True, timeout=240)
                if model == "nemotron-3-ultra-free" and result.returncode != 0 and "Model is unavailable" in result.stderr:
                    print("UNAVAILABLE nemotron-3-ultra-free: upstream rejects its documented route")
                    continue
                assert result.returncode == 0 and token in result.stdout, (model, result.stdout, result.stderr)
                print("PASS live native read-tool round trip: " + model)


if __name__ == "__main__":
    main()
