#!/usr/bin/env python3
"""Run the broker container and prove vault and preference contracts end to end.

The image under test owns SQLite and refreshes OAuth, but never proxies inference.
All credentials are fixtures; loopback HTTP checks cover bearer admission, secret
redaction, removed raw-download routes, preference CAS, restart durability and
real clients without local credential mirrors. No real provider is contacted.

Exit status: 0 passed, 1 failed, 2 missing Docker/image.
Usage: scripts/test-broker-container.py [--image NAME] [--binary PATH] [--keep]
"""
import argparse
import base64
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

REPO = pathlib.Path(__file__).resolve().parent.parent
CONTAINER = "ompig-slim-broker-test"
VOLUME = "ompig-slim-broker-test-data"
BUILD_TAG = "ompig-slim-broker:ci"
# A fresh secret per run keeps the suite honest about requiring one and keeps the
# value out of any committed fixture.
SECRET = "container-test-" + hashlib.sha256(os.urandom(16)).hexdigest()[:24]
# An opener that never follows a redirect, so a redirecting service is reported
# instead of silently exercised.
OPENER = urllib.request.build_opener(urllib.request.HTTPRedirectHandler())


class CheckFailed(Exception):
    """A container behavior did not match the documented contract."""


def docker(*args, check=True, timeout=300):
    """Run the Docker CLI and return its completed process as text."""
    return subprocess.run(
        ["docker", *args], check=check, capture_output=True, text=True, timeout=timeout
    )


def http(base, method, path, token=None, body=None, headers=None, timeout=10):
    """Send one JSON request and return (status, decoded body or None, headers).

    HTTPError is captured rather than raised because the suite's point is often
    the exact refusal status.
    """
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(base + path, data=data, method=method)
    request.add_header("Content-Type", "application/json")
    if token is not None:
        request.add_header("Authorization", "Bearer " + token)
    for name, value in (headers or {}).items():
        request.add_header(name, value)
    try:
        with OPENER.open(request, timeout=timeout) as response:
            status, raw, received = response.status, response.read(), response.headers
    except urllib.error.HTTPError as error:
        status, raw, received = error.code, error.read(), error.headers
    try:
        payload = json.loads(raw) if raw else None
    except json.JSONDecodeError:
        payload = raw.decode(errors="replace")
    return status, payload, dict(received)


def expect(name, condition, detail=""):
    """Fail the run with a diagnostic instead of continuing past a wrong state."""
    if not condition:
        raise CheckFailed(f"{name}: {detail}" if detail else name)
    print(f"PASS: {name}", flush=True)


def empty_revision():
    """The revision a service reports for a document that does not exist yet."""
    return hashlib.sha256(b"").hexdigest()


def encode(text):
    """Encode document bytes the way the Go client's []byte field does."""
    return base64.b64encode(text.encode()).decode()


def decode(payload):
    """Decode a document's bytes from its wire base64, or None for a tombstone."""
    if payload is None:
        return None
    return base64.b64decode(payload).decode()


def host_port(container):
    """Resolve the ephemeral host port Docker assigned to the service."""
    mapping = docker("port", container, "8765/tcp").stdout.strip().splitlines()
    if not mapping:
        raise CheckFailed("container published no port")
    return int(mapping[0].rsplit(":", 1)[1])


def wait_ready(base, timeout=60):
    """Poll the public health route until the service answers or time runs out."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            status, payload, _ = http(base, "GET", "/healthz", timeout=2)
            if status == 200 and payload == "ok\n":
                return
        except OSError:
            pass
        time.sleep(0.25)
    raise CheckFailed("service never became ready; see its log")


def build_image(tag):
    """Build the deployable image from this checkout, refusing a stale context."""
    print(f"building {tag} from {REPO}")
    docker("build", "-f", str(REPO / "Dockerfile.broker"), "-t", tag, str(REPO))


def check_image(image):
    """Assert the published image's identity, entrypoint and notices.

    The license tree is part of the distribution contract for every published
    artifact in this repository, so a runtime image without it is a defect.
    """
    info = json.loads(docker("image", "inspect", image).stdout)[0]
    config = info["Config"]
    expect("image runs as the unprivileged broker user", config.get("User") == "10001:10001", config.get("User"))
    expect("image entrypoint is the storage service", config.get("Entrypoint") == ["ompig-slim-broker"], config.get("Entrypoint"))
    expect("image declares the service port", "8765/tcp" in (config.get("ExposedPorts") or {}), config.get("ExposedPorts"))
    # A published image is deployed unattended, so Docker must be able to report
    # its health without an authenticated request.
    expect("image declares a health check", bool(config.get("Healthcheck", {}).get("Test")), config.get("Healthcheck"))
    listing = docker("run", "--rm", "--entrypoint", "sh", image, "-c",
                     "test -s /usr/share/doc/ompig-slim-broker/LICENSE && "
                     "test -s /usr/share/doc/ompig-slim-broker/NOTICE && "
                     "test -s /usr/share/doc/ompig-slim-broker/vendor/github.com/MichaelKinsy/PiG/LICENSE").returncode
    expect("image carries its attribution and license files", listing == 0)
    # An operator must be able to tell a pulled image from a local build.
    reported = docker("run", "--rm", "--entrypoint", "ompig-slim-broker", image, "--version").stdout.strip()
    expect("image reports its build version", reported.startswith("ompig-slim-broker "), reported)


def check_refuses_without_secret(image):
    """A service with no bearer must refuse to start rather than serve openly."""
    result = docker("run", "--rm", image, check=False, timeout=60)
    expect("a missing secret stops the service",
           result.returncode != 0 and "OMPIG_BROKER_TOKEN" in result.stderr,
           f"exit={result.returncode} stderr={result.stderr.strip()[:200]}")


def start_container(image):
    """Start the service on an ephemeral loopback port with a persistent volume."""
    docker("volume", "create", VOLUME)
    docker("run", "-d", "--name", CONTAINER,
           "-e", "OMPIG_BROKER_TOKEN=" + SECRET,
           "-p", "127.0.0.1::8765",
           "-v", f"{VOLUME}:/data",
           image)
    base = "http://127.0.0.1:" + str(host_port(CONTAINER))
    wait_ready(base)
    return base


def check_health(base, image):
    """The declared health check must actually turn healthy against this build."""
    deadline = time.monotonic() + 90
    status = "unknown"
    while time.monotonic() < deadline:
        status = docker("inspect", CONTAINER, "--format", "{{.State.Health.Status}}", check=False).stdout.strip()
        if status == "healthy":
            print("PASS: the declared health check reports healthy", flush=True)
            return
        if status == "unhealthy":
            break
        time.sleep(2)
    raise CheckFailed(f"health check never reported healthy: {status}")


def check_identity():
    """The service must not run as root and must keep its data directory private."""
    uid = docker("exec", CONTAINER, "id", "-u").stdout.strip()
    expect("service process is unprivileged", uid == "10001", uid)
    mode = docker("exec", CONTAINER, "stat", "-c", "%a", "/data").stdout.strip()
    expect("service data directory is private", mode in ("700", "0700"), mode)


def check_authorization(base):
    """Every data route requires the bearer; only health is public."""
    for path in ("/v1/items/settings.json", "/v1/snapshot", "/v1/credential", "/v1/credential/refresh"):
        status, _, _ = http(base, "GET", path)
        expect(f"{path} refuses an unauthenticated read", status == 401, status)
        status, _, _ = http(base, "GET", path, token="not-the-secret")
        expect(f"{path} refuses a wrong secret", status == 401, status)
        status, _, _ = http(base, "PUT", path, body={"data": encode("{}")})
        expect(f"{path} refuses an unauthenticated write", status == 401, status)
    status, _, _ = http(base, "POST", "/v1/credential", token="not-the-secret")
    expect("the credential vault refuses a wrong secret", status == 401, status)


def check_no_inference_route(base):
    """The image must not be usable as a model gateway under any spelling."""
    for path in ("/v1/messages", "/v1/chat/completions", "/v1/responses", "/v1/embeddings"):
        status, _, _ = http(base, "POST", path, token=SECRET, body={"model": "x", "messages": []})
        expect(f"{path} does not exist", status == 404, status)


def check_allowlist(base):
    """Only portable profile documents are addressable; the database never is.

    A name that escapes the data directory is answered by the mux's own path
    cleaning before any handler runs, so a redirect is an acceptable refusal —
    the allowlist, not the mux, is what makes traversal impossible. That is
    asserted directly against the volume contents.
    """
    revision = empty_revision()
    for name in ("agent.db", "auth.json", "sessions.jsonl", "../escape"):
        status, _, _ = http(base, "GET", f"/v1/items/{name}", token=SECRET)
        expect(f"{name} is not a synchronized document", status in (404, 400, 307), status)
        status, _, _ = http(base, "PUT", f"/v1/items/{name}", token=SECRET,
                            body={"data": encode("{}")},
                            headers={"If-Match": f'"{revision}"'})
        expect(f"{name} cannot be written", status in (404, 400, 307), status)


def check_documents(base):
    """Documents round-trip, and a stale writer loses instead of overwriting.

    The last-writer-wins alternative is exactly the failure this contract exists
    to prevent, so the check asserts the stored bytes did not change.
    """
    first = json.dumps({"modelRoles": {"default": "test-faux/model", "smol": "test-faux/model"}})
    status, payload, headers = http(base, "GET", "/v1/items/settings.json", token=SECRET)
    expect("an absent document reports an empty tombstone", status == 200 and payload["data"] is None, payload)
    revision = payload["revision"]
    expect("an absent document has the empty-content revision", revision == empty_revision(), revision)

    status, _, _ = http(base, "PUT", "/v1/items/settings.json", token=SECRET,
                        body={"data": encode(first)}, headers={"If-Match": f'"{revision}"'})
    expect("a fresh document is accepted", status == 204, status)

    status, payload, headers = http(base, "GET", "/v1/items/settings.json", token=SECRET)
    expect("the stored document is returned verbatim", decode(payload["data"]) == first, payload)
    # net/http canonicalizes the header name, so read it back the same way.
    expect("the document exposes a revision",
           headers.get("Etag") == f'"{payload["revision"]}"', headers.get("Etag"))

    status, _, _ = http(base, "PUT", "/v1/items/settings.json", token=SECRET,
                        body={"data": encode('{"lost":true}')}, headers={"If-Match": f'"{revision}"'})
    expect("a stale revision is rejected", status == 412, status)
    status, payload, _ = http(base, "GET", "/v1/items/settings.json", token=SECRET)
    expect("a rejected write leaves the document intact", decode(payload["data"]) == first, payload)

    status, _, _ = http(base, "PUT", "/v1/items/keybindings.json", token=SECRET,
                        body={"data": encode('{"app.interrupt":"ctrl+c"}')},
                        headers={"If-Match": f'"{empty_revision()}"'})
    expect("an independent document is accepted", status == 204, status)
    status, payload, _ = http(base, "GET", "/v1/items/settings.json", token=SECRET)
    expect("documents do not overwrite each other", decode(payload["data"]) == first, payload)

    status, keybindings, _ = http(base, "GET", "/v1/items/keybindings.json", token=SECRET)
    status, _, _ = http(base, "PUT", "/v1/items/keybindings.json", token=SECRET,
                        body={"data": None}, headers={"If-Match": f'"{revision}"'})
    expect("a deletion needs the current revision", status == 412, status)
    status, _, _ = http(base, "PUT", "/v1/items/keybindings.json", token=SECRET,
                        body={"data": None}, headers={"If-Match": f'"{keybindings["revision"]}"'})
    expect("a deletion is accepted at the current revision", status == 204, status)
    status, keybindings, _ = http(base, "GET", "/v1/items/keybindings.json", token=SECRET)
    expect("a deletion leaves a tombstone", status == 200 and keybindings["data"] is None, keybindings)
    status, _, _ = http(base, "PUT", "/v1/items/keybindings.json", token=SECRET,
                        body={"data": encode('{"app.interrupt":"ctrl+c"}')},
                        headers={"If-Match": f'"{keybindings["revision"]}"'})
    expect("a tombstone can be republished", status == 204, status)


def check_vault(base):
    """Credentials persist in SQLite; refresh material never returns to clients."""
    credential = {"type": "oauth", "access": "fixture-access", "refresh": "fixture-private-refresh",
                  "expires": 4102444800000}
    status, _, _ = http(base, "POST", "/v1/credential", token=SECRET,
                        body={"operation": "save", "provider": "fixture", "accountKey": None,
                              "explicit": True, "credential": credential})
    expect("a credential is persisted by the vault", status == 200, status)
    status, snapshot, _ = http(base, "GET", "/v1/snapshot", token=SECRET)
    expect("the snapshot exposes access credentials", status == 200 and snapshot[0]["credential"]["access"] == "fixture-access", snapshot)
    expect("the snapshot never exposes refresh material", "fixture-private-refresh" not in json.dumps(snapshot))
    expect("the snapshot marks remote refresh ownership", snapshot[0]["credential"]["refresh"] == "__OMPIG_REMOTE_REFRESH__")

def check_volume_contents():
    """The volume holds preferences, the private vault and the ownership lock."""
    listing = set(docker("exec", CONTAINER, "ls", "-A", "/data").stdout.split())
    unexpected = listing - {"settings.json", "keybindings.json", "agent.db", "agent.db-wal", "agent.db-shm", "server.lock"}
    expect("the volume holds only synchronized state", not unexpected, sorted(unexpected))


def check_restart(base):
    """Preferences and redacted credentials survive a broker restart.

    Docker reassigns an ephemeral host port when a stopped container starts
    again, so the address is re-resolved rather than assumed stable.
    """
    docker("stop", "-t", "5", CONTAINER)
    docker("start", CONTAINER)
    base = "http://127.0.0.1:" + str(host_port(CONTAINER))
    wait_ready(base)
    status, payload, _ = http(base, "GET", "/v1/items/settings.json", token=SECRET)
    expect("documents survive a restart",
           status == 200 and json.loads(decode(payload["data"]))["modelRoles"]["smol"] == "test-faux/model",
           payload)
    status, snapshot, _ = http(base, "GET", "/v1/snapshot", token=SECRET)
    expect("vault credentials survive a restart", status == 200 and snapshot[0]["credential"]["access"] == "fixture-access", snapshot)
    expect("restart does not expose refresh material", "fixture-private-refresh" not in json.dumps(snapshot))
    return base


def check_clients(base, binary):
    """Drive two real clients through the container, with no provider involved.

    Each profile is a temporary directory, and the client's own exit flush is
    what publishes the edit, so this covers startup pull, exit publish and the
    second machine's pull in one pass.
    """
    work = pathlib.Path(tempfile.mkdtemp(prefix="ompig-broker-clients-"))
    try:
        profiles = {}
        for name in ("a", "b"):
            profile = work / name
            profile.mkdir()
            (profile / "settings.json").write_text(json.dumps(
                {"quietStartup": True, "modelRoles": {"default": "test-faux/model"}}))
            profiles[name] = profile
        environment = dict(os.environ,
                           PI_OFFLINE="1", PI_SKIP_VERSION_CHECK="1", PI_TELEMETRY="0",
                           PIG_HOME=str(work), PI_HOME=str(work),
                           OMPIG_BROKER_URL=base, OMPIG_BROKER_TOKEN=SECRET)

        def run(name, *args):
            """Run the client against one profile and assert it succeeded."""
            profile = str(profiles[name])
            env = dict(environment, PIG_CODING_AGENT_DIR=profile, PI_CODING_AGENT_DIR=profile,
                       OMPIG_SLIM_CODING_AGENT_DIR=profile)
            result = subprocess.run([binary, *args], capture_output=True, text=True,
                                    env=env, cwd=str(work), timeout=180)
            if result.returncode != 0:
                raise CheckFailed(f"client {name} failed: {result.stderr.strip()[:400]}")
            return result

        run("a", "--version")
        edited = json.loads((profiles["a"] / "settings.json").read_text())
        edited["modelRoles"]["smol"] = "test-faux/model"
        (profiles["a"] / "settings.json").write_text(json.dumps(edited))
        run("a", "--version")
        run("b", "--version")
        pulled = json.loads((profiles["b"] / "settings.json").read_text())
        expect("a second machine pulls a preference edit", pulled.get("modelRoles", {}).get("smol") == "test-faux/model", pulled)

        status, payload, _ = http(base, "GET", "/v1/snapshot", token=SECRET)
        expect("clients use the redacted vault snapshot", status == 200 and len(payload) == 1, payload)
        expect("the client never uploads its connection secret", SECRET not in json.dumps(payload))
        for profile in profiles.values():
            expect("remote clients never create a local credential mirror", not (profile / "agent.db").exists())
    finally:
        shutil.rmtree(work, ignore_errors=True)


def run(image, binary, keep):
    """Execute the whole suite against one image, cleaning up unless kept."""
    if image is None:
        build_image(BUILD_TAG)
        image = BUILD_TAG
    docker("image", "inspect", image)

    check_image(image)
    check_refuses_without_secret(image)

    docker("rm", "-f", CONTAINER, check=False)
    docker("volume", "rm", "-f", VOLUME, check=False)
    try:
        base = start_container(image)
        check_health(base, image)
        check_identity()
        check_authorization(base)
        check_no_inference_route(base)
        check_allowlist(base)
        check_documents(base)
        check_vault(base)
        check_volume_contents()
        # A restart reassigns the ephemeral port, so later checks use the
        # address the container reports now.
        base = check_restart(base)
        if binary is not None:
            check_clients(base, binary)
        else:
            print("SKIP: client synchronization needs --binary")
    except CheckFailed as failure:
        print(f"FAIL: {failure}", file=sys.stderr)
        log = docker("logs", CONTAINER, check=False).stdout + docker("logs", CONTAINER, check=False).stderr
        print("--- container log ---\n" + log[-4000:], file=sys.stderr)
        return 1
    finally:
        if keep:
            print(f"kept {CONTAINER} (data volume {VOLUME}) on port {host_port(CONTAINER)}")
        else:
            docker("rm", "-f", CONTAINER, check=False)
            docker("volume", "rm", "-f", VOLUME, check=False)
    print("PASS: broker container storage contract")
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--image", help="test an already-built image instead of building one")
    parser.add_argument("--binary", type=pathlib.Path, help="ompig-slim executable for the client checks")
    parser.add_argument("--keep", action="store_true", help="leave the container running for inspection")
    args = parser.parse_args()

    if shutil.which("docker") is None:
        print("Docker is required to run this suite", file=sys.stderr)
        return 2
    if docker("info", check=False).returncode != 0:
        print("no Docker daemon is reachable", file=sys.stderr)
        return 2
    binary = None
    if args.binary is not None:
        binary = str(args.binary.resolve())
        if not pathlib.Path(binary).is_file():
            print(f"not a file: {binary}", file=sys.stderr)
            return 2
    try:
        return run(args.image, binary, args.keep)
    except subprocess.CalledProcessError as error:
        print(f"Docker command failed: {error.cmd}\n{error.stderr}", file=sys.stderr)
        return 1
    except subprocess.TimeoutExpired as error:
        print(f"Docker command timed out: {error.cmd}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
