#!/usr/bin/env python3
"""Compare real CLIs against a deterministic local model; Linux, stdlib only.

No API credentials or user profiles are inherited. Runs are serial and rotated
between implementations; the fixture server and sampler are outside the measured
process tree. Failures invalidate a run rather than producing a fast result.
"""
import argparse
import gzip
import hashlib
import http.server
import json
import os
import pathlib
import platform
import queue
import random
import resource
import shutil
import signal
import statistics
import subprocess
import threading
import time
import tempfile
import zlib


class Fixture(http.server.BaseHTTPRequestHandler):
    """Serve streamed chat completions and assert each real tool result arrives."""

    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        """Keep fixture logging out of timing and CI output."""

    def do_POST(self):
        """Produce one tool call then a chunked answer per numbered user turn."""
        try:
            length = int(self.headers["Content-Length"])
            body = json.loads(self.rfile.read(length))
            messages = body["messages"]
            user = max(i for i, m in enumerate(messages) if m["role"] == "user")
            content = messages[user]["content"]
            if isinstance(content, list):
                content = "".join(c.get("text", "") for c in content)
            turn = int(content.split("BENCH_TURN=")[1].split()[0])
            history = str(messages[:user])
            if turn and f"BENCH_OK_{turn - 1}\\n" not in history and f"BENCH_OK_{turn - 1}\n" not in history:
                raise AssertionError(f"turn {turn}: prior assistant history missing (including after resume)")
            tools = [m for m in messages[user + 1:] if m["role"] == "tool"]
            operation = turn % 3
            expected = ("def fixture_", f"proof-{turn}", "BENCH_SHELL_OK")[operation]
            if tools:
                if operation == 1:
                    if (self.server.project / "proof.txt").read_text() != expected:
                        raise AssertionError(f"turn {turn}: write contents differ")
                elif expected not in str(tools[-1]["content"]):
                    raise AssertionError(f"turn {turn}: missing tool proof {expected}")
                chunks = [{"role": "assistant"}]
                text = f"BENCH_OK_{turn}\n" + self.server.answer
                chunks += [{"content": text[i:i + 128]} for i in range(0, len(text), 128)]
                reason = "stop"
                self.server.completed.append(turn)
            else:
                offered = {t["function"]["name"] for t in body.get("tools", [])}
                name = ("read", "write", "bash")[operation]
                if name not in offered:
                    raise AssertionError(f"{name} not offered: {offered}")
                args = ({"path": f"src/file_{turn % 100:03}.py"},
                        {"path": "proof.txt", "content": f"proof-{turn}"},
                        {"command": "printf BENCH_SHELL_OK"})[operation]
                chunks = [{"role": "assistant", "tool_calls": [{"index": 0,
                           "id": f"call_{turn}", "type": "function",
                           "function": {"name": name, "arguments": json.dumps(args)}}]}]
                reason = "tool_calls"
            wire = []
            for delta in chunks:
                wire.append({"id": f"bench-{turn}", "object": "chat.completion.chunk",
                             "created": 0, "model": "bench", "choices": [
                                 {"index": 0, "delta": delta, "finish_reason": None}]})
            wire.append({"id": f"bench-{turn}", "choices": [
                {"index": 0, "delta": {}, "finish_reason": reason}],
                "usage": {"prompt_tokens": 100 + turn * 200,
                          "completion_tokens": len(self.server.answer) // 4}})
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Connection", "close")
            self.end_headers()
            for chunk in wire:
                self.wfile.write(("data: " + json.dumps(chunk) + "\n\n").encode())
                self.wfile.flush()
            self.wfile.write(b"data: [DONE]\n\n")
            self.close_connection = True
            self.server.requests += 1
            self.server.request_bytes += length
        except Exception as exc:
            self.server.failures.append(repr(exc))
            self.close_connection = True


def tree_sample(pid):
    """Sum live Linux process-tree counters; tolerate children exiting mid-read.

    RSS includes shared pages per process (not PSS). Short-lived children can fall
    between samples. CPU totals instead use wait-based resource accounting.
    """
    totals = dict(rss_bytes=0, threads=0, fds=0, read_bytes=0, write_bytes=0, cpu_seconds=0)
    ticks = os.sysconf("SC_CLK_TCK")
    pending, seen = [pid], set()
    while pending:
        child = pending.pop()
        if child in seen:
            continue
        seen.add(child)
        root = pathlib.Path(f"/proc/{child}")
        try:
            status = dict(line.split(":", 1) for line in (root / "status").read_text().splitlines())
            totals["rss_bytes"] += int(status.get("VmRSS", "0 kB").split()[0]) * 1024
            totals["threads"] += int(status["Threads"])
            # comm can contain spaces or parentheses; fields after its final ')'
            # begin at stat field 3. utime/stime are fields 14/15, in clock ticks.
            stat = (root / "stat").read_text().rsplit(")", 1)[1].split()
            totals["cpu_seconds"] += (int(stat[11]) + int(stat[12])) / ticks
            totals["fds"] += len(list((root / "fd").iterdir()))
            for line in (root / "io").read_text().splitlines():
                key, value = line.split(":")
                if key in ("read_bytes", "write_bytes"):
                    totals[key] += int(value)
            # Children belong to the spawning thread, not necessarily the leader.
            for task in (root / "task").iterdir():
                pending.extend(map(int, (task / "children").read_text().split()))
        except (FileNotFoundError, ProcessLookupError, PermissionError):
            continue
    return totals


def isolated_env(root, agent):
    """Allowlist environment variables so credentials and real profiles cannot leak."""
    env = {k: os.environ[k] for k in ("PATH", "LANG", "LC_ALL", "TERM", "SystemRoot") if k in os.environ}
    env.update(HOME=str(root), XDG_CONFIG_HOME=str(root / "config"),
               XDG_CACHE_HOME=str(root / "cache"), TMPDIR=str(root / "tmp"),
               PI_CODING_AGENT_DIR=str(agent), PIG_CODING_AGENT_DIR=str(agent),
               OMPIG_SLIM_CODING_AGENT_DIR=str(agent), OMP_CODING_AGENT_DIR=str(agent),
               PI_HOME=str(root), PIG_HOME=str(root), PI_OFFLINE="1",
               PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0", NO_COLOR="1")
    (root / "tmp").mkdir(exist_ok=True)
    return env


class RPC:
    """Own a CLI process group, bounded event queue and out-of-process samples."""

    def __init__(self, command, cwd, env, stderr, interval):
        """Start the measured CLI and drain stdout continuously, without history copies."""
        self.started = time.perf_counter()
        self.before = resource.getrusage(resource.RUSAGE_CHILDREN)
        self.process = subprocess.Popen(command, cwd=cwd, env=env, stdin=subprocess.PIPE,
                                        stdout=subprocess.PIPE, stderr=stderr, text=True,
                                        start_new_session=True)
        self.events = queue.Queue(maxsize=4096)
        self.samples = []
        self.stop = threading.Event()
        self.interval = interval
        self.reader = threading.Thread(target=self._read, daemon=True)
        self.sampler = threading.Thread(target=self._sample, daemon=True)
        self.reader.start()
        self.sampler.start()

    def _read(self):
        """Publish JSON events or a terminal error; ignore non-JSON startup notices."""
        try:
            for line in self.process.stdout:
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                if isinstance(event, dict):
                    self.events.put(event, timeout=5)
        finally:
            self.events.put({"type": "benchmark_eof"}, timeout=5)

    def _sample(self):
        """Sample the process tree at a fixed interval until the owner stops it."""
        while not self.stop.is_set():
            self.samples.append(dict(seconds=time.perf_counter() - self.started,
                                     **tree_sample(self.process.pid)))
            self.stop.wait(self.interval)

    def send(self, kind, **fields):
        """Write one JSONL command; broken pipes invalidate the run."""
        self.process.stdin.write(json.dumps(dict(type=kind, **fields)) + "\n")
        self.process.stdin.flush()

    def until(self, predicate, timeout):
        """Wait with a deadline and reject RPC errors, failed tools and early EOF."""
        deadline = time.monotonic() + timeout
        while True:
            try:
                event = self.events.get(timeout=max(0.001, deadline - time.monotonic()))
            except queue.Empty:
                raise TimeoutError(f"no RPC event before {timeout}s") from None
            if event.get("success") is False or event.get("isError") or event.get("type") == "benchmark_eof":
                raise RuntimeError(f"CLI failed: {event}")
            if event.get("type") == "message_end":
                message = event.get("message", {})
                if message.get("stopReason") in ("error", "aborted"):
                    raise RuntimeError(f"model failed: {message}")
            if predicate(event):
                return event
            if time.monotonic() >= deadline:
                raise TimeoutError("RPC event deadline exceeded")

    def close(self):
        """Close stdin, reap the CLI and kill its entire group on failure or timeout."""
        try:
            self.process.stdin.close()
            self.process.wait(timeout=10)
        except (subprocess.TimeoutExpired, BrokenPipeError):
            os.killpg(self.process.pid, signal.SIGKILL)
            self.process.wait()
        finally:
            # Also remove descendants that outlive the CLI leader.
            try:
                os.killpg(self.process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            self.stop.set()
            self.sampler.join()
            # Drain so a reader blocked on a full queue can observe EOF and exit.
            while True:
                try:
                    self.events.get_nowait()
                except queue.Empty:
                    break
            self.reader.join(timeout=6)
            self.process.stdout.close()
        after = resource.getrusage(resource.RUSAGE_CHILDREN)
        return dict(cpu_user_seconds=after.ru_utime - self.before.ru_utime,
                    cpu_system_seconds=after.ru_stime - self.before.ru_stime,
                    voluntary_context_switches=after.ru_nvcsw - self.before.ru_nvcsw,
                    involuntary_context_switches=after.ru_nivcsw - self.before.ru_nivcsw,
                    minor_faults=after.ru_minflt - self.before.ru_minflt,
                    major_faults=after.ru_majflt - self.before.ru_majflt)


def rpc_flags(name, agent, session=None):
    """Adapt only CLI syntax; all implementations get the same tools and workload."""
    flags = ["--mode", "rpc", "--provider", "bench", "--model", "bench", "--thinking", "off",
             "--tools", "read,write,bash", "--no-extensions", "--no-skills", "--no-context-files",
             "--session-dir", str(agent / "sessions")]
    if name == "omp":
        flags += ["--no-lsp", "--no-title", "--no-rules", "--no-pty", "--auto-approve"]
        if session:
            flags += ["--resume", str(session)]
    else:
        flags += ["--offline", "--no-prompt-templates", "--no-themes"]
        if session:
            flags += ["--session", str(session)]
    return flags


def run_session(name, command, root, agent, env, turns, start, timeout, interval, log, session=None):
    """Measure startup, streaming turns, growing history, idle and clean shutdown."""
    latencies, first_tokens, boundaries = [], [], []
    with log.open("w") as stderr:
        rpc = RPC(command + rpc_flags(name, agent, session), root / "project", env, stderr, interval)
        try:
            rpc.send("get_state", id="ready")
            state = rpc.until(lambda e: e.get("id") == "ready", timeout)
            ready = time.perf_counter() - rpc.started
            path = state.get("data", {}).get("sessionFile")
            for turn in range(start, start + turns):
                began = time.perf_counter()
                rpc.send("prompt", id=f"turn-{turn}", message=f"BENCH_TURN={turn} Complete this coding task.")
                rpc.until(lambda e: e["type"] == "message_update" and
                          e.get("assistantMessageEvent", {}).get("type") == "text_delta", timeout)
                first_tokens.append(time.perf_counter() - began)
                ended = rpc.until(lambda e: e["type"] == "agent_end", timeout)
                if f"BENCH_OK_{turn}\\n" not in str(ended) and f"BENCH_OK_{turn}\n" not in str(ended):
                    raise AssertionError(f"{name}: final assistant proof missing for turn {turn}")
                latencies.append(time.perf_counter() - began)
                boundaries.append(tree_sample(rpc.process.pid)["rss_bytes"])
            rpc.send("get_state", id="final")
            state = rpc.until(lambda e: e.get("id") == "final", timeout)
            path = state.get("data", {}).get("sessionFile", path)
            # A settled-idle sample is useful, but this is not a leak proof or forced GC.
            idle_start = tree_sample(rpc.process.pid)
            idle_began = time.perf_counter()
            time.sleep(0.25)
            idle_end = tree_sample(rpc.process.pid)
            idle_seconds = time.perf_counter() - idle_began
        finally:
            usage = rpc.close()
        if rpc.process.returncode:
            raise RuntimeError(f"{name} exit {rpc.process.returncode}; see {log}")
    elapsed = time.perf_counter() - rpc.started
    samples = rpc.samples
    metrics = dict(startup_seconds=ready, wall_seconds=elapsed,
                   turn_median_seconds=statistics.median(latencies),
                   turn_p95_seconds=sorted(latencies)[max(0, (95 * len(latencies) + 99) // 100 - 1)],
                   first_text_median_seconds=statistics.median(first_tokens),
                   turns_per_second=turns / sum(latencies),
                   idle_rss_bytes=idle_end["rss_bytes"],
                   idle_rss_change_bytes=idle_end["rss_bytes"] - idle_start["rss_bytes"],
                   idle_cpu_percent_one_core=100 * max(0, idle_end["cpu_seconds"] - idle_start["cpu_seconds"]) / idle_seconds,
                   rss_growth_bytes=boundaries[-1] - boundaries[0],
                   **usage)
    metrics["cpu_percent_one_core"] = 100 * (usage["cpu_user_seconds"] + usage["cpu_system_seconds"]) / elapsed
    for key in ("rss_bytes", "threads", "fds", "read_bytes", "write_bytes"):
        metrics[f"peak_{key}"] = max((s[key] for s in samples), default=0)
    return dict(metrics=metrics, turn_seconds=latencies, first_text_seconds=first_tokens,
                turn_rss_bytes=boundaries, samples=samples), path


def prepare(root, port, answer_bytes):
    """Create identical synthetic source files and isolated provider configuration."""
    agent = root / "agent"
    agent.mkdir()
    project = root / "project"
    (project / "src").mkdir(parents=True)
    for index in range(100):
        (project / "src" / f"file_{index:03}.py").write_text(
            "".join(f"def fixture_{index}_{line}(): return {line}\n" for line in range(256)))
    model = dict(id="bench", name="bench", reasoning=False, input=["text"],
                 contextWindow=10000000, maxTokens=max(16384, answer_bytes),
                 cost=dict(input=0, output=0, cacheRead=0, cacheWrite=0))
    config = {"providers": {"bench": {"api": "openai-completions",
              "baseUrl": f"http://127.0.0.1:{port}/v1", "apiKey": "local-fixture-only", "models": [model]}}}
    (agent / "models.json").write_text(json.dumps(config))
    # JSON is valid YAML: OMP uses models.yml while Pi and PiG use models.json.
    (agent / "models.yml").write_text(json.dumps(config))
    settings = dict(quietStartup=True, enableInstallTelemetry=False, extensionsAutoUpgrade=False,
                    compaction={"enabled": False}, retry={"enabled": False})
    (agent / "settings.json").write_text(json.dumps(settings))
    return agent


def footprint(paths):
    """Return raw and gzip payload sizes for a distribution.

    Files are deduplicated by resolved path so one install is not counted twice,
    and symlinks are skipped so a tree cannot pull in unrelated bytes. gzip is
    streamed over the same files in sorted order at level 6, which approximates
    the archive a user downloads without writing a temporary tarball. Shared
    system libraries and the interpreter are outside these paths.
    """
    files = set()
    for path in paths:
        path = pathlib.Path(path).resolve()
        if path.is_dir():
            files.update(p.resolve() for p in path.rglob("*") if p.is_file() and not p.is_symlink())
        else:
            files.add(path)
    raw = 0
    compressor = zlib.compressobj(6)
    packed = 0
    for file in sorted(files):
        with file.open("rb") as handle:
            while chunk := handle.read(1 << 20):
                raw += len(chunk)
                packed += len(compressor.compress(chunk))
    return dict(raw=raw, gzip=packed + len(compressor.flush()))


def summarize(report):
    """Render medians across whole runs, retaining individual samples in JSON."""
    rows = ["# CLI benchmark", "", "Medians across measured runs (warmups excluded). Lower is better except turns/s.", "",
            "| CLI | Phase | Ready ms | Turn ms | p95 ms | First text ms | Turns/s | Peak RSS MiB | CPU s | Idle MiB | Session MiB |",
            "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|"]
    report["medians"] = {}
    for name, runs in report["runs"].items():
        report["medians"][name] = {}
        for phase in ("long", "resume"):
            metrics = {key: statistics.median(r[phase]["metrics"][key] for r in runs)
                       for key in runs[0][phase]["metrics"]}
            report["medians"][name][phase] = metrics
            rows.append(f"| {name} | {phase} | {metrics['startup_seconds'] * 1000:.1f} | "
                        f"{metrics['turn_median_seconds'] * 1000:.1f} | {metrics['turn_p95_seconds'] * 1000:.1f} | "
                        f"{metrics['first_text_median_seconds'] * 1000:.1f} | {metrics['turns_per_second']:.2f} | "
                        f"{metrics['peak_rss_bytes'] / 2**20:.1f} | "
                        f"{metrics['cpu_user_seconds'] + metrics['cpu_system_seconds']:.2f} | "
                        f"{metrics['idle_rss_bytes'] / 2**20:.1f} | {metrics['session_bytes'] / 2**20:.2f} |")
    rows += ["", "## Footprint", "",
             "| CLI | Entrypoint B | Entrypoint gzip B | Installed B | Installed gzip B | Runtime B | Total B | Total gzip B |",
             "|---|---:|---:|---:|---:|---:|---:|---:|"]
    for name, entry in report["implementations"].items():
        total = entry["entry_bytes"] + entry["installed_raw"] + entry["runtime_bytes"]
        total_gzip = entry["entry_gzip_bytes"] + entry["installed_gzip"] + entry["runtime_gzip_bytes"]
        rows.append(f"| {name} | {entry['entry_bytes']} | {entry['entry_gzip_bytes']} | {entry['installed_raw']} | "
                    f"{entry['installed_gzip']} | {entry['runtime_bytes']} | {total} | {total_gzip} |")
    rows += ["", "Total = entrypoint + installed dependencies + interpreter executable. It excludes shared system libraries "
             "and does not claim identical feature sets or a single-file distribution."]
    rows += ["", "See `results.json` for versions, hashes, machine metadata, per-run counters, latency arrays and RSS timelines.",
             "", "These are synthetic headless RPC measurements, not model quality, terminal rendering, cold-cache, or leak claims.",
             "RSS is sampled for the live process tree; shared pages are counted per process. CPU includes reaped children.",
             "First text includes the preceding tool round-trip. Disk counters are sampled kernel physical I/O, not logical I/O.",
             "Entrypoints are not equivalent distributions: Pi needs Node; OMP needs Bun and installed dependencies."]
    return "\n".join(rows) + "\n"


def main():
    """Run warmups and rotated repetitions; persist partial evidence even on failure."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--slim", required=True)
    parser.add_argument("--baselines", default="benchmarks")
    parser.add_argument("--only", nargs="+", choices=["ompig-slim", "omp", "pi"], default=["ompig-slim", "omp", "pi"])
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--warmups", type=int, default=1)
    parser.add_argument("--turns", type=int, default=120)
    parser.add_argument("--resume-turns", type=int, default=12)
    parser.add_argument("--answer-bytes", type=int, default=4096)
    parser.add_argument("--timeout", type=float, default=60)
    parser.add_argument("--sample-interval", type=float, default=0.02)
    parser.add_argument("--output", default="benchmark-results")
    parser.add_argument("--profile", nargs="+", choices=["cpu", "heap", "allocs", "metrics"],
                        help="opt-in Go profiles for --only ompig-slim; profiling adds measurement overhead")
    args = parser.parse_args()
    if platform.system() != "Linux":
        parser.error("process-tree measurements require Linux /proc")
    if min(args.runs, args.turns, args.resume_turns, args.answer_bytes) < 1 or args.warmups < 0 or min(args.timeout, args.sample_interval) <= 0:
        parser.error("counts and intervals must be positive; warmups may be zero")
    if args.profile and args.only != ["ompig-slim"]:
        parser.error("--profile requires --only ompig-slim; do not compare instrumented and uninstrumented CLIs")
    output = pathlib.Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    base = pathlib.Path(args.baselines).resolve()
    commands = {"ompig-slim": [str(pathlib.Path(args.slim).resolve())],
                "omp": [shutil.which("bun"), str(base / "omp/node_modules/@oh-my-pi/pi-coding-agent/dist/cli.js")],
                "pi": [shutil.which("node"), str(base / "pi/node_modules/@earendil-works/pi-coding-agent/dist/cli.js")]}
    for name in args.only:
        command = commands[name]
        if not command[0] or not pathlib.Path(command[-1]).is_file():
            parser.error(f"{name} is unavailable: {' '.join(str(part) for part in command)}; run npm ci --prefix benchmarks/{name}"
                         if name != "ompig-slim" else f"binary not found: {command[0]}")
    report = dict(schema=1, parameters=vars(args), machine=dict(platform=platform.platform(),
                  cpu_count=os.cpu_count(), cpu=pathlib.Path("/proc/cpuinfo").read_text(),
                  memory=pathlib.Path("/proc/meminfo").read_text()), implementations={},
                  runs={name: [] for name in args.only}, order=[])
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
    server.answer = "".join(f"Step {i:06}: inspect fixture_{i}, preserve behavior and run test_{i}.\n"
                            for i in range(args.answer_bytes // 60 + 2))[:args.answer_bytes]
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        for name in args.only:
            command = commands[name]
            entry = pathlib.Path(command[-1])
            data = entry.read_bytes()
            with tempfile.TemporaryDirectory(prefix="cli-version-") as directory:
                root = pathlib.Path(directory)
                version_result = subprocess.run(command + ["--version"], env=isolated_env(root, root / "agent"),
                                                cwd=root, capture_output=True, text=True, check=True, timeout=args.timeout)
                version = (version_result.stdout + version_result.stderr).strip()
            package = base / name / "node_modules" if name != "ompig-slim" else entry
            # The measured binary is the whole distribution for ompig-slim; only the
            # JS CLIs have a separate dependency payload beside their entrypoint.
            installed = footprint([package]) if name != "ompig-slim" else dict(raw=0, gzip=0)
            lock = base / name / "package-lock.json" if name != "ompig-slim" else None
            report["implementations"][name] = dict(command=command, version=version,
                sha256=hashlib.sha256(data).hexdigest(),
                dependency_lock_sha256=hashlib.sha256(lock.read_bytes()).hexdigest() if lock else None,
                runtime_version=subprocess.check_output([command[0], "--version"], text=True).strip() if len(command) > 1 else None,
                entry_bytes=len(data),
                entry_gzip_bytes=len(gzip.compress(data, mtime=0)),
                **{"installed_" + key: value for key, value in installed.items()},
                runtime_bytes=0 if len(command) == 1 else pathlib.Path(command[0]).stat().st_size,
                runtime_gzip_bytes=0 if len(command) == 1 else footprint([command[0]])["gzip"])
        for repetition in range(-args.warmups, args.runs):
            order = args.only[:]
            random.Random(781 + repetition).shuffle(order)
            report["order"].append(dict(repetition=repetition, implementations=order))
            for name in order:
                print(f"{name}: {'warmup' if repetition < 0 else 'run'} {repetition}", flush=True)
                with tempfile.TemporaryDirectory(prefix="cli-bench-") as directory:
                    root = pathlib.Path(directory)
                    agent = prepare(root, server.server_port, args.answer_bytes)
                    env = isolated_env(root, agent)
                    server.project = root / "project"
                    logs = output / "logs"
                    logs.mkdir(exist_ok=True)
                    server.completed, server.failures, server.requests, server.request_bytes = [], [], 0, 0
                    result, path = {}, None
                    for phase, count, start in [("long", args.turns, 0),
                                                ("resume", args.resume_turns, args.turns)]:
                        if args.profile:
                            # Profiles survive the temporary HOME and are separate per process;
                            # inherited profiling settings still never enter ordinary runs.
                            profile_dir = output / "profiles" / f"{repetition}-{phase}"
                            profile_dir.mkdir(parents=True, exist_ok=True)
                            env.update(PIG_PROFILE=",".join(args.profile), PIG_PROFILE_DIR=str(profile_dir))
                        result[phase], path = run_session(name, commands[name], root, agent, env, count,
                            start, args.timeout, args.sample_interval, logs / f"{name}-{repetition}-{phase}.stderr",
                            path)
                        result[phase]["metrics"]["session_bytes"] = sum(p.stat().st_size for p in agent.rglob("*.jsonl"))
                        if not path or not pathlib.Path(path).is_file():
                            raise AssertionError(f"{name}: persisted session missing: {path}")
                    expected = list(range(args.turns + args.resume_turns))
                    if server.completed != expected or server.failures or server.requests != 2 * len(expected):
                        raise AssertionError(f"fixture mismatch: {server.completed}, {server.failures}, requests={server.requests}")
                    written = [t for t in expected if t % 3 == 1]
                    if written and (root / "project/proof.txt").read_text() != f"proof-{written[-1]}":
                        raise AssertionError("write tool did not persist expected contents")
                    result.update(requests=server.requests, request_bytes=server.request_bytes)
                    if repetition >= 0:
                        report["runs"][name].append(result)
        summary = summarize(report)
        (output / "summary.md").write_text(summary)
        print(summary)
    except Exception as exc:
        report["error"] = repr(exc)
        report["fixture_failures"] = getattr(server, "failures", [])
        raise
    finally:
        (output / "results.json").write_text(json.dumps(report, indent=2) + "\n")
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    main()
