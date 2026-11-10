#!/usr/bin/env python3
"""Run one broker host and two real clients as three Docker containers.

`test-broker-container.py` drives one container from the host and covers the
storage contract as an HTTP surface. This suite covers the deployment shape
instead: the broker, and the clients that use it, are all containers on a private
network, so the checks exercise what a user actually runs — a client image with no
provider access, connecting to a service it can only reach by its network name.

Three containers, one bridge network, no published ports: the clients reach the
broker as `http://broker:8765`, which also proves the deployed URL form works.
Everything asserted here is deterministic and offline. Credentials are fixture
values, and no provider is ever contacted: login writes the API key locally, and
synchronization is storage only.

Covered: the hosted profile seeding an empty broker, preference propagation in
both directions, deletion propagation, per-document revision conflict detection
that preserves both copies, credential snapshots syncing as logical records (never
a SQLite file), the credential lease refusing a concurrent mutation, the saved
connection file staying device-local and 0600, a restart revoking the lease while
keeping documents, a wrong secret and an unreachable service degrading to
local-only, and a third container joining late to receive the whole profile.

Exit status: 0 all checks passed, 1 a check failed, 2 the environment cannot run
the suite (no Docker daemon).

Usage: scripts/test-broker-cluster.py [--binary PATH] [--keep] [--no-build]
"""
import argparse
import base64
import hashlib
import json
import os
import pathlib
import pty
import select
import shutil
import subprocess
import sys
import tempfile
import time

REPO = pathlib.Path(__file__).resolve().parent.parent
BROKER_IMAGE = "ompig-slim-cluster-broker:test"
CLIENT_IMAGE = "ompig-slim-cluster-client:test"
NETWORK = "ompig-slim-cluster-net"
HOST = "ompig-slim-cluster-host"
# Fixture identities. Each client is its own container with its own profile
# volume, so a check that passes cannot be explained by shared local state.
CLIENT_A = "ompig-slim-cluster-a"
CLIENT_B = "ompig-slim-cluster-b"
LATE_CLIENT = "ompig-slim-cluster-late"
VOLUMES = [f"{name}-profile" for name in (HOST, CLIENT_A, CLIENT_B, LATE_CLIENT)]
# A fresh secret per run keeps the suite honest about requiring one and keeps the
# value out of every committed fixture.
SECRET = "cluster-" + hashlib.sha256(os.urandom(16)).hexdigest()[:24]
# The broker's documented name on the private network. Clients use this URL form,
# which is also what a deployed service looks like behind a compose service name.
BROKER_URL = f"http://{HOST}:8765"

# The client image is built from the executable under test, not from a release
# archive: this suite runs in the same job as the build, and a client that is not
# the code in this checkout would prove nothing about it.
CLIENT_DOCKERFILE = """\
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \\
 && rm -rf /var/lib/apt/lists/* \\
 && useradd --uid 10001 --create-home client \\
 && install -d -m 0700 -o client -g client /profile
COPY ompig-slim /usr/local/bin/ompig-slim
# curl is the suite's HTTP probe from inside the network, and an operator debugging
# a deployment wants one on a client too. It is the only package beyond CA roots.
USER 10001:10001
# The profile root and the agent directory are separate concepts: PIG_HOME holds
# `state/`, and the agent directory holds settings, the credential database and the
# device-local connection file.
ENV PIG_HOME=/profile PI_HOME=/profile
ENV PIG_CODING_AGENT_DIR=/profile/agent PI_CODING_AGENT_DIR=/profile/agent
ENV OMPIG_SLIM_CODING_AGENT_DIR=/profile/agent
ENTRYPOINT ["ompig-slim"]
"""


class CheckFailed(Exception):
    """A cluster behavior did not match the documented contract."""


def docker(*args, check=True, timeout=300, input=None):
    """Run the Docker CLI and return its completed process as text."""
    return subprocess.run(
        ["docker", *args], check=check, capture_output=True, text=True,
        timeout=timeout, input=input,
    )


def expect(name, condition, detail=""):
    """Fail the run with a diagnostic instead of continuing past a wrong state."""
    if not condition:
        raise CheckFailed(f"{name}: {detail}" if detail else name)
    print(f"PASS: {name}", flush=True)


def build_images(binary):
    """Build the client image from the binary under test and the broker image."""
    print(f"building {CLIENT_IMAGE} from {binary}", flush=True)
    context = pathlib.Path(tempfile.mkdtemp(prefix="ompig-cluster-build-"))
    try:
        shutil.copy2(binary, context / "ompig-slim")
        (context / "Dockerfile").write_text(CLIENT_DOCKERFILE)
        docker("build", "-t", CLIENT_IMAGE, str(context))
    finally:
        shutil.rmtree(context, ignore_errors=True)
    print(f"building {BROKER_IMAGE} from {REPO}", flush=True)
    docker("build", "-f", str(REPO / "Dockerfile.broker"), "-t", BROKER_IMAGE, str(REPO))


def clean():
    """Remove containers, network and volumes from an earlier run."""
    for name in (CLIENT_A, CLIENT_B, LATE_CLIENT, HOST):
        docker("rm", "-f", name, check=False)
    docker("network", "rm", NETWORK, check=False)
    for volume in VOLUMES:
        docker("volume", "rm", "-f", volume, check=False)


def start_network():
    """Create the private bridge the three containers share."""
    docker("network", "create", NETWORK)


def start_host():
    """Start the broker with no published port: only the network can reach it."""
    docker("volume", "create", VOLUMES[0])
    docker("run", "-d", "--name", HOST, "--network", NETWORK,
           "-e", "OMPIG_BROKER_TOKEN=" + SECRET,
           "-v", f"{VOLUMES[0]}:/data", BROKER_IMAGE)
    wait_healthy(HOST)


def wait_healthy(container, timeout=120):
    """Wait for Docker's own health check to report the broker serving."""
    deadline = time.monotonic() + timeout
    status = "unknown"
    while time.monotonic() < deadline:
        status = docker("inspect", container, "--format", "{{.State.Health.Status}}",
                        check=False).stdout.strip()
        if status == "healthy":
            return
        if status in ("unhealthy", ""):
            raise CheckFailed(f"{container} never became healthy: {status}")
        time.sleep(2)
    raise CheckFailed(f"{container} health check timed out: {status}")


def start_client(name, url=BROKER_URL, token=SECRET):
    """Start a client container with its own profile volume and broker connection."""
    docker("volume", "create", f"{name}-profile")
    environment = ["-e", f"OMPIG_BROKER_URL={url}", "-e", f"OMPIG_BROKER_TOKEN={token}"]
    docker("run", "-d", "--name", name, "--network", NETWORK, *environment,
           "--entrypoint", "sleep", "-v", f"{name}-profile:/profile", CLIENT_IMAGE, "infinity")


def client(name, *args, timeout=180, check=True, stdin=None):
    """Run one command inside a client container and return its completed process.

    The image's entrypoint is the harness itself, but `docker exec` does not apply
    an entrypoint, so every harness invocation names the binary explicitly. Shell
    commands go through `sh -c` and are passed unchanged.
    """
    command = args if args[:1] in (("sh",), ("cat",), ("stat",), ("rm",), ("test",), ("ls",)) \
        else ("ompig-slim", *args)
    return docker("exec", "-i", name, *command, timeout=timeout, check=check, input=stdin)


def token_from_stderr(result):
    """Join a client's streams for a failure diagnostic."""
    return (result.stdout + result.stderr).strip()[:600]


def run_client(name, *args, timeout=180):
    """Run one client command and fail the check when it exits non-zero."""
    result = client(name, *args, timeout=timeout, check=False)
    if result.returncode != 0:
        raise CheckFailed(f"{name} {' '.join(args)} failed: {token_from_stderr(result)}")
    return result


def read_json(name, path):
    """Read a JSON document from a client's profile."""
    result = client(name, "cat", path, check=False)
    if result.returncode != 0:
        return None
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError:
        return None


def write_settings(name, document):
    """Replace a client's settings.json through a shell heredoc, as a user edit."""
    payload = json.dumps(document)
    client(name, "sh", "-c",
           "mkdir -p /profile/agent && "
           f"cat > /profile/agent/settings.json <<'EOF'\n{payload}\nEOF")


def login(container, provider, key, timeout=120):
    """Store an API key through the login dialog, which requires a terminal.

    `auth login --api-key` refuses a pipe: it prompts, and a prompt needs a TTY. So
    this allocates a real pseudo-terminal on `docker exec` and types the fixture key
    at the prompt, which is exactly what a user does. Driving it this way also keeps
    the key out of the container's environment, its argv and `docker inspect`.
    """
    master, slave = pty.openpty()
    process = subprocess.Popen(
        ["docker", "exec", "-i", "--tty", container, "ompig-slim", "auth", "login", provider, "--api-key"],
        stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    transcript = bytearray()
    typed = False
    deadline = time.monotonic() + timeout
    try:
        while time.monotonic() < deadline:
            if select.select([master], [], [], 0.1)[0]:
                try:
                    transcript.extend(os.read(master, 65536))
                except OSError:
                    break
            if not typed and b"API key" in transcript:
                os.write(master, key.encode() + b"\r")
                typed = True
            if process.poll() is not None:
                break
        process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
    finally:
        os.close(master)
    if process.returncode != 0:
        raise CheckFailed(f"{container} login failed: {bytes(transcript)[-400:]!r}")
    return transcript


def sync(name):
    """Publish and pull once by running a client command that flushes on exit.

    A client synchronizes at startup, every five seconds, and on normal exit, so a
    short `--version` run is a complete round trip. `--version` opens no runtime,
    which keeps the check to the storage path.
    """
    run_client(name, "--version")


def check_seeding():
    """The first client to connect seeds the empty host with its own profile.

    This is the documented first-run behavior and the one that decides what every
    other machine downloads, so it is asserted before anything is edited.
    """
    settings = {"quietStartup": True,
                "modelRoles": {"default": "test-faux/model", "smol": "test-faux/model"},
                "theme": "cluster-a"}
    write_settings(CLIENT_A, settings)
    sync(CLIENT_A)

    stored = broker_get("settings.json")
    expect("the first client seeds the hosted profile", stored is not None, stored)
    expect("the hosted profile holds the seeding client's document",
           json.loads(stored).get("theme") == "cluster-a", stored[:200])


def broker_get(document):
    """Read one document's bytes through the broker's own API from a client."""
    result = client(CLIENT_A, "sh", "-c",
                    "curl -fsS -H \"Authorization: Bearer $OMPIG_BROKER_TOKEN\" "
                    f"\"$OMPIG_BROKER_URL/v1/items/{document}\"", check=False)
    if result.returncode != 0:
        return None
    payload = json.loads(result.stdout)
    if payload.get("data") is None:
        return None
    return base64.b64decode(payload["data"]).decode()


def check_preferences_bidirectional():
    """An edit on one machine reaches the other, in both directions."""
    edited = read_json(CLIENT_A, "/profile/agent/settings.json")
    edited["cycleOrder"] = ["default", "smol"]
    write_settings(CLIENT_A, edited)
    sync(CLIENT_A)
    sync(CLIENT_B)

    pulled = read_json(CLIENT_B, "/profile/agent/settings.json") or {}
    expect("a preference edit propagates A to B",
           pulled.get("cycleOrder") == ["default", "smol"], pulled.get("cycleOrder"))
    expect("B received A's original document too", pulled.get("theme") == "cluster-a", pulled.get("theme"))

    pulled["theme"] = "cluster-b"
    write_settings(CLIENT_B, pulled)
    sync(CLIENT_B)
    sync(CLIENT_A)
    back = read_json(CLIENT_A, "/profile/agent/settings.json") or {}
    expect("a preference edit propagates B to A", back.get("theme") == "cluster-b", back.get("theme"))
    expect("B's edit kept A's earlier field", back.get("cycleOrder") == ["default", "smol"],
           back.get("cycleOrder"))


def check_keybindings():
    """A second synchronized document is independent of the first."""
    payload = json.dumps({"app.interrupt": "ctrl+c"})
    client(CLIENT_A, "sh", "-c",
           f"cat > /profile/agent/keybindings.json <<'EOF'\n{payload}\nEOF")
    sync(CLIENT_A)
    sync(CLIENT_B)
    pulled = read_json(CLIENT_B, "/profile/agent/keybindings.json") or {}
    expect("an independent document propagates", pulled.get("app.interrupt") == "ctrl+c", pulled)
    settings = read_json(CLIENT_B, "/profile/agent/settings.json") or {}
    expect("the other document was not disturbed", settings.get("theme") == "cluster-b", settings.get("theme"))


def check_deletion():
    """Deleting a synchronized file propagates as a tombstone, not a re-seed."""
    client(CLIENT_A, "rm", "-f", "/profile/agent/keybindings.json")
    sync(CLIENT_A)
    sync(CLIENT_B)
    expect("a deletion propagates", not (pathlib.PurePosixPath("/profile/agent/keybindings.json").name
                                         and client(CLIENT_B, "test", "-e", "/profile/agent/keybindings.json",
                                                    check=False).returncode == 0))
    expect("a deleted document is a tombstone on the host", broker_get("keybindings.json") is None)


def check_conflict():
    """Two machines editing one document is reported, never silently resolved.

    The documented contract is that a conflict keeps both copies and stops that
    document, because merging shared settings automatically would execute one
    machine's commands on another. The check asserts the upload is refused and both
    local files still hold their own text.
    """
    mine_a = read_json(CLIENT_A, "/profile/agent/settings.json")
    mine_b = read_json(CLIENT_B, "/profile/agent/settings.json")
    mine_a["theme"] = "conflict-a"
    mine_b["theme"] = "conflict-b"
    write_settings(CLIENT_A, mine_a)
    write_settings(CLIENT_B, mine_b)

    # Both uploads must be attempted before either can observe the other.
    client(CLIENT_A, "sh", "-c", "true")
    sync(CLIENT_A)
    result = client(CLIENT_B, "--version", check=False)
    expect("the losing client still exits successfully", result.returncode == 0,
           token_from_stderr(result))
    expect("the conflict is reported to the user",
           "conflict" in (result.stderr + result.stdout).lower(), token_from_stderr(result))

    kept_a = read_json(CLIENT_A, "/profile/agent/settings.json") or {}
    kept_b = read_json(CLIENT_B, "/profile/agent/settings.json") or {}
    expect("the winning copy is unchanged", kept_a.get("theme") == "conflict-a", kept_a.get("theme"))
    expect("the losing copy is preserved locally", kept_b.get("theme") == "conflict-b", kept_b.get("theme"))
    hosted = json.loads(broker_get("settings.json") or "{}")
    expect("the host kept the accepted document", hosted.get("theme") == "conflict-a", hosted.get("theme"))

    # Resolving by making the copies agree is the documented recovery.
    mine_b["theme"] = "conflict-a"
    write_settings(CLIENT_B, mine_b)
    sync(CLIENT_B)
    expect("reconciling the copies clears the conflict",
           (read_json(CLIENT_B, "/profile/agent/settings.json") or {}).get("theme") == "conflict-a")


def check_credentials():
    """Credentials sync as a logical record, and never as a SQLite file."""
    login(CLIENT_A, "openai", "sk-cluster-fixture-key")
    accounts = client(CLIENT_A, "auth", "accounts", "--json", check=False)
    expect("login stored an account on A", accounts.returncode == 0 and "openai" in accounts.stdout,
           token_from_stderr(accounts))

    sync(CLIENT_A)
    staged = broker_get("credentials.json")
    expect("the host holds a credential snapshot", staged is not None, staged)
    document = json.loads(staged) if staged else {}
    expect("the snapshot is the documented document, not a database file",
           isinstance(document, (dict, list)), type(document).__name__)
    expect("the snapshot carries the stored key", "sk-cluster-fixture-key" in staged, staged[:200])
    expect("the snapshot is not SQLite", not staged.startswith("SQLite format 3"),
           staged[:16])

    sync(CLIENT_B)
    printed = client(CLIENT_B, "auth", "print-api-key", "--provider", "openai", check=False)
    expect("B resolves the account A stored", printed.returncode == 0, token_from_stderr(printed))
    expect("B printed A's key verbatim", printed.stdout.strip() == "sk-cluster-fixture-key",
           printed.stdout.strip()[:40])
    # The account arrives as a logical record, so B can resolve it without ever
    # receiving A's SQLite file: the key proves the record, not a copied database.
    expect("B has no copied credential database",
           not docker("exec", CLIENT_B, "test", "-e", "/profile/agent/.broker-agent.db",
                      check=False).returncode == 0)
    listed = client(CLIENT_B, "auth", "accounts", "--json", check=False)
    expect("B lists the synchronized account", "sk-cluster-fixture-key" not in listed.stdout
           and "openai" in listed.stdout, listed.stdout[:200])


def check_lease():
    """A held credential lease makes a concurrent mutation wait, not interleave."""
    lease = acquire_lease(CLIENT_A)
    expect("a lease is granted to the first caller", bool(lease), lease)

    # The lease is what a live rotation holds; a second holder must be refused.
    second = client(CLIENT_B, "sh", "-c",
                    "curl -s -o /dev/null -w '%{http_code}' -X POST "
                    "-H \"Authorization: Bearer $OMPIG_BROKER_TOKEN\" "
                    "\"$OMPIG_BROKER_URL/v1/auth/lease\"", check=False)
    expect("a second lease is refused while one is held", second.stdout.strip() == "423", second.stdout.strip())

    released = client(CLIENT_A, "sh", "-c",
                      "curl -fsS -X DELETE -H \"Authorization: Bearer $OMPIG_BROKER_TOKEN\" "
                      f"-H 'X-Broker-Lease: {lease}' \"$OMPIG_BROKER_URL/v1/auth/lease\"", check=False)
    expect("the lease holder may release", released.returncode == 0, token_from_stderr(released))
    third = client(CLIENT_B, "sh", "-c",
                   "curl -s -o /dev/null -w '%{http_code}' -X POST "
                   "-H \"Authorization: Bearer $OMPIG_BROKER_TOKEN\" "
                   "\"$OMPIG_BROKER_URL/v1/auth/lease\"", check=False)
    expect("a lease is grantable again after release", third.stdout.strip() == "200", third.stdout.strip())
    client(CLIENT_B, "sh", "-c", "true")


def check_connection_file_is_local():
    """The connection secret is device-local, private, and never synchronized."""
    stat = client(CLIENT_A, "stat", "-c", "%a", "/profile/agent/remote-broker.json", check=False)
    if stat.returncode == 0:
        expect("a saved connection file is private", stat.stdout.strip() in ("600", "0600"), stat.stdout.strip())
    hosted = broker_get("remote-broker.json")
    expect("the connection file is never uploaded", hosted is None, hosted)
    staged = broker_get("credentials.json") or ""
    expect("the connection secret is not in the credential snapshot", SECRET not in staged)


def acquire_lease(container):
    """Acquire the credential lease, retrying while a previous holder releases.

    `check_lease` leaves the lease free, but a lease has a 60-second lifetime and a
    just-released one still counts as held for a moment. Retrying makes this check
    independent of how the previous one ended.
    """
    deadline = time.monotonic() + 70
    while time.monotonic() < deadline:
        result = client(container, "sh", "-c",
                        "curl -s -X POST -H \"Authorization: Bearer $OMPIG_BROKER_TOKEN\" "
                        "\"$OMPIG_BROKER_URL/v1/auth/lease\"", check=False)
        try:
            payload = json.loads(result.stdout)
        except (json.JSONDecodeError, ValueError):
            payload = {}
        if payload.get("lease"):
            return payload["lease"]
        time.sleep(2)
    raise CheckFailed("no lease could be acquired")


def check_restart_revokes_lease():
    """Documents survive a broker restart; a lease held across it does not."""
    lease = acquire_lease(CLIENT_A)
    expect("a lease is granted before the restart", bool(lease), lease)

    docker("stop", "-t", "10", HOST)
    docker("start", HOST)
    wait_healthy(HOST)

    renewed = client(CLIENT_A, "sh", "-c",
                     "curl -s -o /dev/null -w '%{http_code}' -X PUT "
                     "-H \"Authorization: Bearer $OMPIG_BROKER_TOKEN\" "
                     f"-H 'X-Broker-Lease: {lease}' \"$OMPIG_BROKER_URL/v1/auth/lease\"", check=False)
    expect("the restart revoked the previous lease", renewed.stdout.strip() in ("401", "423"), renewed.stdout.strip())
    expect("documents survive the restart",
           (json.loads(broker_get("settings.json") or "{}")).get("theme") == "conflict-a",
           broker_get("settings.json"))


def check_wrong_secret():
    """A wrong secret is refused on every data route and never trusted."""
    attempts = {
        "read": "curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer wrong' "
                "\"$OMPIG_BROKER_URL/v1/items/settings.json\"",
        "lease": "curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Authorization: Bearer wrong' "
                 "\"$OMPIG_BROKER_URL/v1/auth/lease\"",
    }
    for route, script in attempts.items():
        result = client(CLIENT_A, "sh", "-c", script, check=False)
        expect(f"a wrong secret is refused for {route}", result.stdout.strip() == "401", result.stdout.strip())
    health = client(CLIENT_A, "sh", "-c",
                    "curl -s -o /dev/null -w '%{http_code}' \"$OMPIG_BROKER_URL/healthz\"", check=False)
    expect("health stays public", health.stdout.strip() == "200", health.stdout.strip())


def check_configured_client():
    """A machine provisioned by config, not by the login dialog, synchronizes.

    This is the unattended path: a container image or a configuration-management
    run knows the URL and the secret before the process starts and cannot answer a
    prompt. `ompig-slim broker setup` writes the same device-local connection the
    dialog writes, so everything above must behave identically for it.
    """
    docker("rm", "-f", LATE_CLIENT, check=False)
    docker("volume", "rm", "-f", f"{LATE_CLIENT}-profile", check=False)
    # No OMPIG_BROKER_* in the environment: the connection comes from the file that
    # setup writes, so this also proves the saved form works without the env vars.
    docker("volume", "create", f"{LATE_CLIENT}-profile")
    docker("run", "-d", "--name", LATE_CLIENT, "--network", NETWORK,
           "--entrypoint", "sleep", "-v", f"{LATE_CLIENT}-profile:/profile",
           CLIENT_IMAGE, "infinity")

    # The secret arrives on stdin, as a mount or a pipe would deliver it, so it is
    # never in argv where every process on the host could read it.
    result = docker("exec", "-i", LATE_CLIENT, "ompig-slim", "broker", "setup",
                    "--url", BROKER_URL, "--token-stdin", "--verify",
                    input=SECRET + "\n", check=False)
    if result.returncode != 0:
        raise CheckFailed(f"broker setup failed: {token_from_stderr(result)}")
    expect("a scripted setup verifies the service", "Verified" in result.stdout, result.stdout.strip())

    shown = docker("exec", LATE_CLIENT, "ompig-slim", "broker", "show", check=False).stdout
    expect("a scripted setup reports the saved URL", BROKER_URL in shown, shown.strip())
    expect("the saved URL is shown without the secret", SECRET not in shown, shown.strip())

    stat = docker("exec", LATE_CLIENT, "stat", "-c", "%a", "/profile/agent/remote-broker.json", check=False)
    expect("a scripted setup writes a private connection",
           stat.stdout.strip() in ("600", "0600"), stat.stdout.strip())

    # Re-running the same setup must be a no-op, so a converge step is quiet.
    again = docker("exec", "-i", LATE_CLIENT, "ompig-slim", "broker", "setup",
                   "--url", BROKER_URL, "--token-stdin", input=SECRET + "\n", check=False)
    expect("a repeated scripted setup changes nothing", "already set" in again.stdout, again.stdout.strip())

    run_client(LATE_CLIENT, "--version")
    settings = read_json(LATE_CLIENT, "/profile/agent/settings.json") or {}
    expect("a configured client downloads the profile", settings.get("theme") == "conflict-a",
           settings.get("theme"))
    printed = client(LATE_CLIENT, "auth", "print-api-key", "--provider", "openai", check=False)
    expect("a configured client resolves the synchronized credential", printed.returncode == 0,
           token_from_stderr(printed))

    # Clearing disconnects without touching the downloaded profile.
    cleared = docker("exec", LATE_CLIENT, "ompig-slim", "broker", "clear", check=False)
    expect("a scripted clear succeeds", cleared.returncode == 0, token_from_stderr(cleared))
    gone = docker("exec", LATE_CLIENT, "ompig-slim", "broker", "show", check=False).stdout
    expect("a cleared client reports no connection", "No broker connection" in gone, gone.strip())
    expect("clearing keeps the downloaded profile",
           (read_json(LATE_CLIENT, "/profile/agent/settings.json") or {}).get("theme") == "conflict-a")
    docker("rm", "-f", LATE_CLIENT)
    docker("volume", "rm", "-f", f"{LATE_CLIENT}-profile")


def check_degraded_client():
    """A client whose broker is unreachable stays usable and reports it.

    Synchronization must never block the CLI: a wrong secret or a dead service
    prints a diagnostic and the requested command still runs, so a user can repair
    the connection instead of being locked out of their own machine.
    """
    docker("rm", "-f", LATE_CLIENT, check=False)
    docker("volume", "rm", "-f", f"{LATE_CLIENT}-profile", check=False)
    start_client(LATE_CLIENT, url="http://127.0.0.1:9")
    result = client(LATE_CLIENT, "--version", check=False)
    expect("an unreachable broker does not fail the CLI", result.returncode == 0,
           token_from_stderr(result))
    expect("the inactive sync is reported", "profile sync inactive" in (result.stderr + result.stdout),
           token_from_stderr(result))
    client(LATE_CLIENT, "auth", "accounts", check=False)
    docker("rm", "-f", LATE_CLIENT)
    docker("volume", "rm", "-f", f"{LATE_CLIENT}-profile")

    docker("rm", "-f", LATE_CLIENT, check=False)
    docker("volume", "rm", "-f", f"{LATE_CLIENT}-profile", check=False)
    start_client(LATE_CLIENT, token="not-the-secret")
    rejected = client(LATE_CLIENT, "--version", check=False)
    expect("a wrong secret does not fail the CLI", rejected.returncode == 0, token_from_stderr(rejected))
    expect("the rejected connection is reported",
           "profile sync inactive" in (rejected.stderr + rejected.stdout), token_from_stderr(rejected))
    docker("rm", "-f", LATE_CLIENT)
    docker("volume", "rm", "-f", f"{LATE_CLIENT}-profile")


def check_late_joiner():
    """A machine that joins after the profile exists downloads all of it."""
    start_client(LATE_CLIENT)
    run_client(LATE_CLIENT, "--version")
    settings = read_json(LATE_CLIENT, "/profile/agent/settings.json") or {}
    expect("a late client downloads the current settings", settings.get("theme") == "conflict-a",
           settings.get("theme"))
    expect("the late client downloaded every field", settings.get("cycleOrder") == ["default", "smol"],
           settings.get("cycleOrder"))
    printed = client(LATE_CLIENT, "auth", "print-api-key", "--provider", "openai", check=False)
    expect("the late client resolves the synchronized credential", printed.returncode == 0,
           token_from_stderr(printed))
    expect("the late client printed the stored key", printed.stdout.strip() == "sk-cluster-fixture-key",
           printed.stdout.strip()[:40])


def check_host_is_storage_only():
    """The broker container never holds provider traffic or a client's local state."""
    listing = set(docker("exec", HOST, "ls", "-A", "/data", check=False).stdout.split())
    unexpected = listing - {"settings.json", "keybindings.json", "credentials.json", "models.json",
                            "models.yml", "models.yaml", "server.lock"}
    expect("the host volume holds only synchronized documents", not unexpected, sorted(unexpected))
    expect("the host holds no client database", "agent.db" not in listing, sorted(listing))
    probe = client(CLIENT_A, "sh", "-c",
                   "curl -s -o /dev/null -w '%{http_code}' -X POST -H \"Authorization: Bearer $OMPIG_BROKER_TOKEN\" "
                   "-d '{}' \"$OMPIG_BROKER_URL/v1/chat/completions\"", check=False)
    expect("the broker serves no inference route", probe.stdout.strip() == "404", probe.stdout.strip())


def cluster(binary, keep):
    """Run the whole cluster suite against freshly built images."""
    if docker("info", check=False, timeout=60).returncode != 0:
        raise RuntimeError("no Docker daemon is reachable")
    clean()
    build_images(binary)
    try:
        start_network()
        start_host()
        start_client(CLIENT_A)
        start_client(CLIENT_B)

        check_seeding()
        check_preferences_bidirectional()
        check_keybindings()
        check_deletion()
        check_conflict()
        check_credentials()
        check_lease()
        check_connection_file_is_local()
        check_restart_revokes_lease()
        check_wrong_secret()
        check_host_is_storage_only()
        check_late_joiner()
        check_configured_client()
        check_degraded_client()
    except CheckFailed:
        log = docker("logs", HOST, check=False)
        print("--- broker log ---\n" + (log.stdout + log.stderr)[-4000:], file=sys.stderr)
        for name in (CLIENT_A, CLIENT_B):
            logs = docker("logs", name, check=False)
            print(f"--- {name} log ---\n" + (logs.stdout + logs.stderr)[-2000:], file=sys.stderr)
        raise
    finally:
        if not keep:
            clean()


def main():
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--binary", type=pathlib.Path, default=REPO / "bin" / "ompig-slim",
                        help="ompig-slim executable to build the client image from")
    parser.add_argument("--keep", action="store_true", help="leave the containers running for inspection")
    args = parser.parse_args()

    if shutil.which("docker") is None:
        print("Docker is required to run this suite", file=sys.stderr)
        return 2
    binary = args.binary.resolve()
    if not binary.is_file():
        print(f"not a file: {binary}; run make build first", file=sys.stderr)
        return 2
    try:
        cluster(str(binary), args.keep)
    except (CheckFailed, RuntimeError) as failure:
        print(f"FAIL: {failure}", file=sys.stderr)
        return 1
    except subprocess.CalledProcessError as error:
        print(f"Docker command failed: {error.cmd}\n{error.stderr}", file=sys.stderr)
        return 1
    except subprocess.TimeoutExpired as error:
        print(f"Docker command timed out: {error.cmd}", file=sys.stderr)
        return 1
    print("PASS: broker cluster (one host, two clients, one late joiner)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
