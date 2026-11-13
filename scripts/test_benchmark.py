#!/usr/bin/env python3
"""Deterministic tests for benchmark correctness, isolation and failed-run handling."""
import copy
import http.client
import http.server
import json
import os
import pathlib
import queue
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest import mock

import benchmark


class BenchmarkTests(unittest.TestCase):
    """Exercise the measurement infrastructure without installing external CLIs."""

    def test_environment_does_not_inherit_credentials_or_profiles(self):
        """An allowlist prevents real provider credentials and profile overrides leaking."""
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            with mock.patch.dict(os.environ, {"OPENAI_API_KEY": "secret", "OMP_AUTH_BROKER_TOKEN": "secret",
                                             "PI_HOME": "/real", "PI_PACKAGE_DIR": "/real/package",
                                             "OMPIG_BROKER_URL": "http://127.0.0.1:1",
                                             "OMPIG_BROKER_TOKEN": "real-broker-secret"}):
                env = benchmark.isolated_env(root, root / "agent")
            self.assertNotIn("OPENAI_API_KEY", env)
            self.assertNotIn("OMP_AUTH_BROKER_TOKEN", env)
            self.assertNotIn("PI_PACKAGE_DIR", env)
            self.assertNotIn("OMPIG_BROKER_URL", env)
            self.assertNotIn("OMPIG_BROKER_TOKEN", env)
            self.assertEqual(env["PI_HOME"], str(root))
            for key in ("PI_CODING_AGENT_DIR", "PIG_CODING_AGENT_DIR", "OMPIG_SLIM_CODING_AGENT_DIR"):
                self.assertEqual(env[key], str(root / "agent"))

    def test_profiling_rejects_mixed_implementations(self):
        """Instrumented native timings must never look like a fair CLI comparison."""
        result = subprocess.run([sys.executable, str(pathlib.Path(benchmark.__file__)),
                                 "--slim", "unused", "--profile", "metrics"],
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("--profile requires --only ompig-slim", result.stderr)

    def test_fixture_tools_history_and_sse(self):
        """The mock validates real tool output and prior history instead of trusting exit 0."""
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), benchmark.Fixture)
            server.completed, server.failures = [], []
            server.requests, server.request_bytes = 0, 0
            server.answer, server.project = "Unique fixture answer.", root
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                messages = [{"role": "user", "content": "BENCH_TURN=0 task"}]
                tool = self.request(server, messages)[0]["choices"][0]["delta"]["tool_calls"][0]
                self.assertEqual(tool["function"]["name"], "read")
                messages += [{"role": "assistant", "tool_calls": [tool]},
                             {"role": "tool", "content": "def fixture_0(): pass"}]
                chunks = self.request(server, messages)
                text = "".join(c["choices"][0]["delta"].get("content", "") for c in chunks)
                self.assertIn("BENCH_OK_0", text)
                self.assertEqual(server.completed, [0])
                messages += [{"role": "assistant", "content": text},
                             {"role": "user", "content": [{"type": "text", "text": "BENCH_TURN=1 task"}]}]
                tool = self.request(server, messages)[0]["choices"][0]["delta"]["tool_calls"][0]
                self.assertEqual(tool["function"]["name"], "write")
                (root / "proof.txt").write_text("proof-1")
                messages += [{"role": "assistant", "tool_calls": [tool]},
                             {"role": "tool", "content": "File written successfully"}]
                self.request(server, messages)
                self.assertEqual(server.completed, [0, 1])
                self.assertEqual(server.requests, 4)
                self.assertFalse(server.failures)
                with self.assertRaises(http.client.RemoteDisconnected):
                    self.request(server, [{"role": "user", "content": "BENCH_TURN=2 task"}])
                self.assertIn("history missing", server.failures[-1])
            finally:
                server.shutdown()
                server.server_close()
                thread.join()

    def request(self, server, messages):
        """Decode the fixture's SSE using HTTP, independently of provider adapters."""
        connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=5)
        try:
            tools = [{"function": {"name": name}} for name in ("read", "write", "bash")]
            connection.request("POST", "/v1/chat/completions", json.dumps(dict(messages=messages, tools=tools)),
                               {"Content-Type": "application/json"})
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            data = response.read().decode()
            self.assertTrue(data.endswith("data: [DONE]\n\n"))
            return [json.loads(line[6:]) for line in data.splitlines() if line.startswith("data: {")]
        finally:
            connection.close()

    def test_medians_exclude_outlier_and_keep_raw_runs(self):
        """Reports aggregate run-level measurements, not pooled event distributions."""
        keys = ["startup_seconds", "wall_seconds", "turn_median_seconds", "turn_p95_seconds",
                "first_text_median_seconds", "turns_per_second", "peak_rss_bytes",
                "cpu_user_seconds", "cpu_system_seconds", "idle_rss_bytes", "session_bytes"]
        runs = [{phase: {"metrics": dict.fromkeys(keys, value)} for phase in ("long", "resume")}
                for value in (1, 2, 99)]
        report = {"runs": {"fixture": runs}, "implementations": {}}
        original = copy.deepcopy(runs)
        summary = benchmark.summarize(report)
        self.assertEqual(report["medians"]["fixture"]["long"]["turn_median_seconds"], 2)
        self.assertIn("fixture", summary)
        self.assertEqual(report["runs"]["fixture"], original)

    def test_footprint_does_not_follow_directory_symlinks(self):
        """Separate installs count payloads once without escaping through symlinks."""
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            payload = root / "payload"
            payload.mkdir()
            (payload / "data").write_bytes(b"1234")
            (payload / "alias").symlink_to(payload / "data")
            outside = root / "outside"
            outside.mkdir()
            (outside / "data").write_bytes(b"not included")
            (payload / "external").symlink_to(outside, target_is_directory=True)
            self.assertEqual(benchmark.footprint([payload, payload / "data"])["raw"], 4)
            self.assertGreater(benchmark.footprint([payload])["gzip"], 0)

    @unittest.skipUnless(sys.platform == "linux", "resource sampler requires Linux /proc")
    def test_rpc_failure_timeout_and_cleanup(self):
        """Real child processes are sampled, failures reject results and EOF is reaped."""
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            script = root / "fixture.py"
            script.write_text("import json,sys\nfor line in sys.stdin:\n"
                              " c=json.loads(line)\n"
                              " if c['type']=='fail': print(json.dumps({'type':'response','success':False}),flush=True)\n"
                              " elif c['type']=='ok': print(json.dumps({'type':'response','id':c['id'],'success':True}),flush=True)\n")
            with (root / "stderr").open("w") as stderr:
                rpc = benchmark.RPC([sys.executable, str(script)], root, os.environ.copy(), stderr, 0.005)
                try:
                    rpc.send("ok", id="ready")
                    rpc.until(lambda e: e.get("id") == "ready", 5)
                    sample = benchmark.tree_sample(rpc.process.pid)
                    self.assertGreater(sample["rss_bytes"], 0)
                    self.assertGreater(sample["threads"], 0)
                    with self.assertRaises(TimeoutError):
                        rpc.until(lambda e: False, 0.02)
                    rpc.send("fail")
                    with self.assertRaisesRegex(RuntimeError, "CLI failed"):
                        rpc.until(lambda e: True, 5)
                finally:
                    usage = rpc.close()
                self.assertEqual(rpc.process.returncode, 0)
                self.assertGreaterEqual(usage["cpu_user_seconds"], 0)
                self.assertFalse(rpc.sampler.is_alive())
                self.assertFalse(rpc.reader.is_alive())
                self.assertTrue(rpc.samples)


if __name__ == "__main__":
    unittest.main()
