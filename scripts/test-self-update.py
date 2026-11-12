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
import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest

# Release fixtures live in release_fixture.py so this suite and
# test-install.py prove the same archive layout.
import release_fixture

INSTALLER = pathlib.Path(__file__).resolve().with_name("install.sh")
def platform_suffix():
    """Return this host's release platform suffix, e.g. linux-x64.

    The updater picks its archive from the Go runtime's GOOS/GOARCH, so unlike
    the installer suite this one cannot pin a platform: it must name the archive
    the running executable will actually ask for.
    """
    system = subprocess.run(["uname", "-s"], capture_output=True, text=True).stdout.strip()
    machine = subprocess.run(["uname", "-m"], capture_output=True, text=True).stdout.strip()
    goos = {"Linux": "linux", "Darwin": "darwin"}.get(system)
    goarch = {"x86_64": "x64", "aarch64": "arm64", "arm64": "arm64"}.get(machine)
    if goos is None or goarch is None:
        return None
    return f"{goos}-{goarch}"


# The executable under test is tens of megabytes, so an archive is encoded once
# per payload and reused: re-gzipping it per test would dominate the run time
# without testing anything different. Keyed by payload digest.
_ARCHIVES = {}


def release_archive(payload):
    """Wrap one executable payload in the release archive layout."""
    key = release_fixture.sha256_bytes(payload)
    archive = _ARCHIVES.get(key)
    if archive is None:
        # compresslevel=1 keeps encoding cheap; the archive is oversized for its
        # contents either way, which is all these tests need.
        archive = release_fixture.archive_bytes(payload, compresslevel=1)
        _ARCHIVES[key] = archive
    return archive


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
        self.server = release_fixture.ReleaseServer(self.root / "channel", platform_suffix())
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
        self.server.publish(tag or f"v{self.version}", release_archive(self.payload), make_latest=True)
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
        self.server.publish(f"v{version}", release_fixture.archive_bytes(release_fixture.script_payload(version)),
                            make_latest=True)
        return version

    def test_update_replaces_an_archive_install(self):
        self.install()
        newer = self.publish_newer()
        result = self.run_binary("update")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.installed_version(), newer)
        self.assertIn("Updated ompig-slim", result.stdout)

    def test_beta_selection_is_remembered(self):
        """A second plain update uses beta even with a newer stable available."""
        self.install()
        self.server.publish("v99.0.0-beta.9", release_archive(self.payload))
        result = self.run_binary("upgrade", "--channel", "beta")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("channel=beta", (self.home / ".pi/ompig-slim-install").read_text())
        self.publish_newer("100.0.0")
        self.server.publish("v99.0.0-beta.10", release_fixture.archive_bytes(
            release_fixture.script_payload("99.0.0-beta.10")))
        result = self.run_binary("update")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.installed_version(), "99.0.0-beta.10")

    def test_explicit_stable_overrides_saved_beta(self):
        """Switching to stable persists the selection after replacement."""
        self.install()
        marker = self.home / ".pi/ompig-slim-install"
        marker.write_text(marker.read_text() + "channel=beta\n")
        self.publish_newer()
        self.server.publish("v100.0.0-beta.1", release_fixture.archive_bytes(
            release_fixture.script_payload("100.0.0-beta.1")))
        result = self.run_binary("update", "--channel=stable")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.installed_version(), "99.0.0")
        self.assertIn("channel=stable", marker.read_text())
        self.assertNotIn("channel=beta", marker.read_text())

    def test_failed_switch_does_not_save_channel(self):
        """No published beta must not change the default stable selection."""
        self.install()
        marker = self.home / ".pi/ompig-slim-install"
        before = marker.read_bytes()
        self.server.documents.clear()
        result = self.run_binary("update", "--channel", "beta")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(marker.read_bytes(), before)

    def test_upgrade_does_the_same_thing_as_update(self):
        self.install()
        newer = self.publish_newer()
        result = self.run_binary("upgrade")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.installed_version(), newer)
        self.assertIn("Updated ompig-slim", result.stdout)

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
        # CI also runs this suite against beta archives: request that channel
        # when checking currency rather than intentionally switching to stable.
        channel = "beta" if "-beta." in self.version else "stable"
        result = self.run_binary("update", "--channel", channel)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("up to date", result.stdout)
        self.assertEqual(pathlib.Path(target).read_bytes(), before)

    def test_force_reinstalls_the_current_release(self):
        self.install()
        channel = "beta" if "-beta." in self.version else "stable"
        result = self.run_binary("update", "--force", "--channel", channel)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Updated ompig-slim", result.stdout)

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
        self.server.publish("v99.0.0",
                            release_fixture.archive_bytes(release_fixture.script_payload("99.0.0")),
                            make_latest=True, checksum="wrong")
        result = self.run_binary("update")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertEqual(pathlib.Path(target).read_bytes(), before)

    def test_release_without_the_executable_leaves_the_install_untouched(self):
        target = self.install()
        before = pathlib.Path(target).read_bytes()
        # A well-formed archive that simply lacks the executable: verification
        # passes and extraction is what must refuse.
        archive = release_fixture.archive_of([
            release_fixture.file_member("ompig-slim/README.md", b"doc", 0o644),
        ])
        directory = self.server.releases / "download" / "v99.0.0"
        directory.mkdir(parents=True, exist_ok=True)
        name = f"ompig-slim-{self.server.platform}.tar.gz"
        (directory / name).write_bytes(archive)
        digest = release_fixture.sha256_bytes(archive)
        pathlib.Path(str(directory / name) + ".sha256").write_text(f"{digest}  {name}\n")
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
