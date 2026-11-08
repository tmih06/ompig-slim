#!/usr/bin/env python3
"""Prove `ompig-slim update` and `ompig-slim upgrade` on the real executable.

The unit tests in internal/selfupdate cover version ordering and argument
handling. This script covers the contract only the real binary can show: that
the words `update` and `upgrade` reach the self-updater before PiG parses them,
that a claimed update replaces the exact file the installer put on PATH, and
that an installation the installer did not create keeps PiG's own refusal.

It serves fixture releases over loopback and builds nothing. The release the
test installs first contains the executable under test, so the running program
is genuinely self-updating; later fixture releases contain a stand-in script so
a replacement is observable through `--version`. That is the only command the
self-updater relies on.

Usage: scripts/test-self-update.py <path to ompig-slim>
"""
import hashlib
import http.server
import io
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tarfile
import tempfile
import threading
import unittest

INSTALLER = pathlib.Path(__file__).resolve().with_name("install.sh")
ARCHIVE_MEMBER = "ompig-slim/ompig-slim"


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def platform_suffix():
    """Return the release platform suffix for this host, e.g. linux-x64."""
    system = subprocess.run(["uname", "-s"], capture_output=True, text=True).stdout.strip()
    machine = subprocess.run(["uname", "-m"], capture_output=True, text=True).stdout.strip()
    goos = {"Linux": "linux", "Darwin": "darwin"}.get(system)
    goarch = {"x86_64": "x64", "aarch64": "arm64", "arm64": "arm64"}.get(machine)
    if goos is None or goarch is None:
        return None
    return f"{goos}-{goarch}"


def script_payload(version):
    """A stand-in executable that reports one version, for observing a replace."""
    return f"#!/bin/sh\nprintf '%s\\n' '{version}'\n".encode()


def release_archive(payload):
    """Wrap one executable payload in the release archive layout.

    Archives are memoized by payload digest: the executable under test is tens
    of megabytes, and re-gzipping it per test would dominate the suite's run
    time without testing anything different.
    """
    key = sha256(payload)
    cached = _ARCHIVES.get(key)
    if cached is not None:
        return cached
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz", compresslevel=1) as package:
        member = tarfile.TarInfo(ARCHIVE_MEMBER)
        member.size = len(payload)
        member.mode = 0o755
        package.addfile(member, io.BytesIO(payload))
    archive = buffer.getvalue()
    _ARCHIVES[key] = archive
    return archive


# _ARCHIVES caches encoded archives for the life of the process; see release_archive.
_ARCHIVES = {}


class ReleaseServer:
    """Serves GitHub's release layout plus the one API route the updater reads."""

    def __init__(self, root):
        self.root = root
        self.releases = root / "releases"
        self.releases.mkdir(parents=True)
        self.latest_tag = ""
        self.platform = platform_suffix()
        self.instance = None

    def publish(self, tag, payload, make_latest=False):
        directory = self.releases / "download" / tag
        directory.mkdir(parents=True, exist_ok=True)
        archive = release_archive(payload)
        name = f"ompig-slim-{self.platform}.tar.gz"
        (directory / name).write_bytes(archive)
        pathlib.Path(str(directory / name) + ".sha256").write_text(f"{sha256(archive)}  {name}\n")
        if make_latest:
            self.latest_tag = tag
            latest = self.releases / "latest" / "download"
            latest.mkdir(parents=True, exist_ok=True)
            (latest / name).write_bytes(archive)
            pathlib.Path(str(latest / name) + ".sha256").write_text(
                f"{sha256(archive)}  {name}\n")

    def publish_with_bad_checksum(self, tag, payload):
        """Publish an archive whose checksum does not describe it."""
        directory = self.releases / "download" / tag
        directory.mkdir(parents=True, exist_ok=True)
        name = f"ompig-slim-{self.platform}.tar.gz"
        (directory / name).write_bytes(release_archive(payload))
        pathlib.Path(str(directory / name) + ".sha256").write_text(f"{'0' * 64}  {name}\n")
        self.latest_tag = tag

    def start(self):
        server = self

        class Handler(http.server.SimpleHTTPRequestHandler):
            def do_GET(self):
                if self.path.endswith("/releases/latest"):
                    body = json.dumps({"tag_name": server.latest_tag}).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                super().do_GET()

            def log_message(self, *args):
                pass

        self.instance = http.server.ThreadingHTTPServer(
            ("127.0.0.1", 0), lambda *a, **k: Handler(*a, directory=str(self.root), **k))
        threading.Thread(target=self.instance.serve_forever, daemon=True).start()
        host, port = self.instance.server_address
        return f"http://{host}:{port}"

    def stop(self):
        if self.instance is not None:
            self.instance.shutdown()
            self.instance.server_close()


class SelfUpdateTests(unittest.TestCase):
    binary = None

    @classmethod
    def setUpClass(cls):
        if not cls.binary:
            raise unittest.SkipTest("no executable under test")
        if platform_suffix() is None:
            raise unittest.SkipTest("no release archive for this platform")
        cls.payload = pathlib.Path(cls.binary).read_bytes()
        cls.version = subprocess.run([cls.binary, "--version"], capture_output=True,
                                     text=True).stdout.strip()

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="ompig self-update ")
        self.addCleanup(self.temporary.cleanup)
        self.root = pathlib.Path(self.temporary.name)
        self.home = self.root / "home"
        self.home.mkdir()
        self.server = ReleaseServer(self.root / "channel")
        self.base = self.server.start()
        self.addCleanup(self.server.stop)
        self.install_dir = self.home / ".local" / "bin"

    def env(self, **overrides):
        env = {
            "HOME": str(self.home),
            "PATH": f"{self.install_dir}:/usr/local/bin:/usr/bin:/bin",
            "OMPIG_SLIM_DOWNLOAD_BASE": f"{self.base}/releases",
            # The updater reads this to learn the latest tag, so it must point at
            # the fixture rather than api.github.com.
            "OMPIG_SLIM_RELEASES_API": f"{self.base}/releases/latest",
            "PI_OFFLINE": "1",
            "PI_SKIP_VERSION_CHECK": "1",
        }
        env.update(overrides)
        return env

    def install(self, tag=None):
        """Install the executable under test through scripts/install.sh.

        Each test gets its own HOME, so every install copies the whole
        executable. That is intentional: it keeps tests independent, and the
        file is removed with the temporary directory.
        """
        self.server.publish(tag or f"v{self.version}", self.payload, make_latest=True)
        result = subprocess.run(["bash", str(INSTALLER)], env=self.env(),
                                text=True, capture_output=True, timeout=180)
        self.assertEqual(result.returncode, 0, result.stderr)
        return self.install_dir / "ompig-slim"

    def run_binary(self, *arguments, **env):
        return subprocess.run([str(self.install_dir / "ompig-slim"), *arguments],
                              env=self.env(**env), text=True, capture_output=True, timeout=180)

    def installed_version(self):
        return self.run_binary("--version").stdout.strip()

    def publish_newer(self, version="99.0.0"):
        self.server.publish(f"v{version}", script_payload(version), make_latest=True)
        return version

    def test_update_replaces_an_archive_install(self):
        self.install()
        newer = self.publish_newer()
        result = self.run_binary("update")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.installed_version(), newer)
        self.assertIn("Updated to ompig-slim", result.stdout)

    def test_upgrade_does_the_same_thing_as_update(self):
        self.install()
        newer = self.publish_newer()
        result = self.run_binary("upgrade")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.installed_version(), newer)
        self.assertIn("Updated to ompig-slim", result.stdout)

    def test_upgrade_is_not_read_as_a_prompt(self):
        # The regression: `upgrade` used to reach PiG as a prompt and fail with
        # "No API key found for the selected model." on a machine with no model.
        self.install()
        self.publish_newer()
        result = self.run_binary("upgrade")
        self.assertNotIn("No API key found", result.stdout + result.stderr)

    def test_update_is_a_no_op_when_current(self):
        target = self.install()
        before = pathlib.Path(target).read_bytes()
        result = self.run_binary("update")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("up to date", result.stdout)
        self.assertEqual(pathlib.Path(target).read_bytes(), before)

    def test_force_reinstalls_the_current_release(self):
        self.install()
        result = self.run_binary("update", "--force")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Updated to ompig-slim", result.stdout)

    def test_explicit_self_targets_still_update(self):
        for target in ("self", "ompig-slim", "pig"):
            with self.subTest(target=target):
                self.install(tag="v1.0.0")
                newer = self.publish_newer(version="99.0.0")
                result = self.run_binary("update", target)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(self.installed_version(), newer)

    def test_checksum_mismatch_leaves_the_install_untouched(self):
        target = self.install()
        before = pathlib.Path(target).read_bytes()
        self.server.publish_with_bad_checksum("v99.0.0", script_payload("99.0.0"))
        result = self.run_binary("update")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertEqual(pathlib.Path(target).read_bytes(), before)

    def test_release_without_the_executable_leaves_the_install_untouched(self):
        target = self.install()
        before = pathlib.Path(target).read_bytes()
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w:gz") as package:
            member = tarfile.TarInfo("ompig-slim/README.md")
            member.size = 3
            package.addfile(member, io.BytesIO(b"doc"))
        archive = buffer.getvalue()
        directory = self.server.releases / "download" / "v99.0.0"
        directory.mkdir(parents=True, exist_ok=True)
        name = f"ompig-slim-{self.server.platform}.tar.gz"
        (directory / name).write_bytes(archive)
        pathlib.Path(str(directory / name) + ".sha256").write_text(f"{sha256(archive)}  {name}\n")
        self.server.latest_tag = "v99.0.0"
        result = self.run_binary("update")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("does not contain", result.stderr)
        self.assertEqual(pathlib.Path(target).read_bytes(), before)

    def test_package_forms_still_belong_to_the_package_manager(self):
        self.install()
        # --models and a positional source are the package manager's, so the
        # self-updater must not print its own progress for them.
        models = self.run_binary("update", "--models")
        self.assertNotIn("Updating ompig-slim", models.stdout)
        package = self.run_binary("update", "npm:@example/pkg")
        self.assertNotIn("Updating ompig-slim", package.stdout)
        self.assertEqual(self.installed_version(), self.version)

    def test_help_is_still_pigs(self):
        self.install()
        result = self.run_binary("update", "--help")
        self.assertIn("Usage:", result.stdout)
        self.assertNotIn("Updating ompig-slim", result.stdout)

    def test_a_build_beside_an_installed_copy_is_not_replaced(self):
        # The marker names the installed file. A repository build that happens to
        # run against the same HOME must not be overwritten by a release archive;
        # PiG keeps that decision.
        target = self.install()
        before = pathlib.Path(target).read_bytes()
        self.publish_newer()
        elsewhere = self.root / "checkout" / "ompig-slim"
        elsewhere.parent.mkdir()
        shutil.copyfile(target, elsewhere)
        elsewhere.chmod(0o755)
        result = subprocess.run([str(elsewhere), "update"], env=self.env(),
                                text=True, capture_output=True, timeout=180)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("cannot self-update", result.stderr)
        self.assertEqual(pathlib.Path(target).read_bytes(), before)

    def test_a_binary_without_the_marker_keeps_pigs_refusal(self):
        # Removing the marker models an npm, go-install, or container install.
        target = self.install()
        marker = self.home / ".pi" / "ompig-slim-install"
        self.assertTrue(marker.exists())
        marker.unlink()
        self.publish_newer()
        before = pathlib.Path(target).read_bytes()
        result = self.run_binary("update")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("cannot self-update", result.stderr)
        self.assertEqual(pathlib.Path(target).read_bytes(), before)


def main():
    if len(sys.argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    binary = pathlib.Path(sys.argv[1]).resolve()
    if not binary.is_file():
        print(f"not a file: {binary}", file=sys.stderr)
        return 2
    SelfUpdateTests.binary = str(binary)
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(SelfUpdateTests)
    return 0 if unittest.TextTestRunner(verbosity=2).run(suite).wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(main())
