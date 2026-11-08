#!/usr/bin/env python3
"""Exercise scripts/install.sh against a loopback release server.

The installer is the only part of this repository that runs on a user's machine
before any code is reviewed, so its contract is tested as a command: given a
release layout, environment, and arguments, what does it print, what does it
leave in the destination directory, and what exit status does it report?

Fixtures are served over loopback with a shell script standing in for the
executable. That script is portable across the Linux and macOS runners and keeps
the checks independent of the host architecture; no real release, network, or
Go toolchain is involved.
"""
import hashlib
import http.server
import io
import os
import pathlib
import shutil
import socket
import subprocess
import tarfile
import tempfile
import threading
import unittest


INSTALLER = pathlib.Path(__file__).resolve().with_name("install.sh")
FAKE_VERSION = "ompig-slim 9.9.9-test"


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class Fixture:
    """A writable release tree plus an HTTP server that exposes it."""

    def __init__(self, root):
        self.root = root
        self.releases = root / "releases"
        self.releases.mkdir(parents=True)
        self.server = None
        self.thread = None

    def stage(self, tag, content=None, verify=True, fake_version=FAKE_VERSION):
        """Write ompig-slim (and its checksum) for one release tag."""
        directory = self.releases / ("latest/download" if tag == "latest" else f"download/{tag}")
        directory.mkdir(parents=True, exist_ok=True)
        archive = directory / "ompig-slim-linux-x64.tar.gz"
        archive.write_bytes(self.archive_bytes(content, fake_version))
        checksum = pathlib.Path(str(archive) + ".sha256")
        checksum.write_text(f"{sha256(archive)}  {archive.name}\n" if verify else "not a digest\n")
        return directory

    @staticmethod
    def archive_bytes(content, fake_version):
        """Build the same member layout scripts/build-binaries.sh publishes."""
        # The executable is a shell script so the per-architecture binary does
        # not matter; only the archive contract and the --version contract do.
        executable = content if content is not None else (
            "#!/bin/sh\nprintf '%s\\n' '" + fake_version + "'\n"
        )
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w:gz") as package:
            payload = executable.encode()
            member = tarfile.TarInfo("ompig-slim/ompig-slim")
            member.size = len(payload)
            member.mode = 0o755
            package.addfile(member, io.BytesIO(payload))
            notice = tarfile.TarInfo("ompig-slim/README.md")
            notice.size = 3
            notice.mode = 0o644
            package.addfile(notice, io.BytesIO(b"doc"))
        return buffer.getvalue()

    def start(self):
        handler = lambda *args, **kwargs: http.server.SimpleHTTPRequestHandler(
            *args, directory=str(self.root), **kwargs
        )
        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        host, port = self.server.server_address
        return f"http://{host}:{port}/releases"

    def stop(self):
        if self.server is not None:
            self.server.shutdown()
            self.server.server_close()


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="ompig install tests ")
        self.addCleanup(self.temporary.cleanup)
        self.root = pathlib.Path(self.temporary.name)
        self.home = self.root / "home"
        self.home.mkdir()
        self.fixture = Fixture(self.root / "release")
        self.base = self.fixture.start()
        self.addCleanup(self.fixture.stop)
        # A PATH with no ompig-slim directory keeps the "not on PATH" hint
        # deterministic, and shims/ holds a fake uname so every test selects the
        # same release platform regardless of the machine running the suite.
        self.shims = self.root / "shims"
        self.shims.mkdir()
        self.path = f"{self.shims}:/usr/local/bin:/usr/bin:/bin"
        self.use_platform("Linux", "x86_64")

    def env(self, **overrides):
        env = {
            "HOME": str(self.home),
            "PATH": self.path,
            "OMPIG_SLIM_DOWNLOAD_BASE": self.base,
        }
        env.update(overrides)
        return env

    def run_installer(self, *arguments, **env):
        return subprocess.run(
            ["bash", str(INSTALLER), *arguments],
            env=self.env(**env), text=True, capture_output=True, timeout=60,
        )

    def install_dir(self):
        return self.home / ".local" / "bin"

    def shim(self, name, body):
        path = self.shims / name
        path.write_text("#!/bin/sh\n" + body)
        path.chmod(0o755)
        return path

    def use_platform(self, system, machine):
        """Make `uname` report one release platform for the installer."""
        self.shim(
            "uname",
            f'if [ "${{1:-}}" = "-s" ]; then echo {system}; else echo {machine}; fi\n',
        )

    def test_latest_release_installs_into_local_bin(self):
        self.fixture.stage("latest")
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        target = self.install_dir() / "ompig-slim"
        self.assertTrue(os.access(target, os.X_OK))
        self.assertIn(FAKE_VERSION, target.read_text())
        # The summary names the resolved version; the install directory is
        # announced by the phase line above it.
        self.assertIn("installed ompig-slim", result.stdout)
        self.assertIn(FAKE_VERSION, result.stdout)
        self.assertIn(str(self.install_dir()), result.stdout)
        # Nothing else from the archive lands on PATH, including partial files.
        self.assertEqual(sorted(os.listdir(self.install_dir())), ["ompig-slim"])

    def test_tagged_release_uses_the_tag_path(self):
        self.fixture.stage("v1.2.3")
        result = self.run_installer("--version", "v1.2.3")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/download/v1.2.3/", result.stdout)
        self.assertTrue((self.install_dir() / "ompig-slim").exists())

    def test_tag_comes_from_the_environment(self):
        self.fixture.stage("v1.2.3")
        result = self.run_installer(OMPIG_SLIM_VERSION="v1.2.3")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/download/v1.2.3/", result.stdout)

    def test_latest_spelling_resolves_to_the_latest_alias(self):
        self.fixture.stage("latest")
        result = self.run_installer("--version", "latest")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/latest/download/", result.stdout)

    def test_flag_overrides_environment(self):
        self.fixture.stage("v1.2.3")
        result = self.run_installer("--version", "v1.2.3", OMPIG_SLIM_VERSION="v9.9.9")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/download/v1.2.3/", result.stdout)
        self.assertTrue((self.install_dir() / "ompig-slim").exists())

    def test_directory_override_and_environment_fallback(self):
        self.fixture.stage("latest")
        explicit = self.root / "explicit bin"
        result = self.run_installer("--dir", str(explicit))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((explicit / "ompig-slim").exists())
        from_env = self.root / "env bin"
        result = self.run_installer(OMPIG_SLIM_INSTALL_DIR=str(from_env))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((from_env / "ompig-slim").exists())

    def test_ownership_marker_names_pig_homes_config_root(self):
        # `ompig-slim update` only replaces a binary the installer claims, and it
        # finds that claim under the same config root PiG uses: PIG_HOME, else
        # ~/.pi, with a leading ~/ expanded.
        self.fixture.stage("latest")
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        marker = self.home / ".pi" / "ompig-slim-install"
        fields = dict(line.split("=", 1) for line in marker.read_text().splitlines() if "=" in line)
        self.assertEqual(fields["kind"], "archive")
        self.assertEqual(fields["executable"], str(self.install_dir() / "ompig-slim"))
        self.assertEqual(fields["download-base"], self.base)
        self.assertEqual(oct(marker.stat().st_mode & 0o777), "0o600")

        # A leading ~/ expands against HOME, as PiG expands PIG_HOME.
        result = self.run_installer("--dir", str(self.root / "bin2"), PIG_HOME="~/pig")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.home / "pig" / "ompig-slim-install").exists())

    def test_unwritable_config_root_warns_but_keeps_the_install(self):
        # The executable is already installed by then; a marker that cannot be
        # written only costs in-place updates, so it must not fail the install.
        self.fixture.stage("latest")
        blocker = self.root / "blocker"
        blocker.write_text("a file, not a directory")
        result = self.run_installer(PIG_HOME=str(blocker / "nested"))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("could not record the install", result.stderr)
        self.assertTrue((self.install_dir() / "ompig-slim").exists())

    def test_upgrade_replaces_the_previous_executable(self):
        self.fixture.stage("latest", fake_version="ompig-slim 1.0.0-first")
        self.assertEqual(self.run_installer().returncode, 0)
        first = (self.install_dir() / "ompig-slim").read_text()
        self.fixture.stage("latest", fake_version="ompig-slim 2.0.0-second")
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        second = (self.install_dir() / "ompig-slim").read_text()
        self.assertNotEqual(first, second)
        self.assertIn("2.0.0-second", second)
        self.assertEqual(sorted(os.listdir(self.install_dir())), ["ompig-slim"])

    def test_checksum_mismatch_keeps_the_previous_install(self):
        self.fixture.stage("latest", fake_version="ompig-slim 1.0.0-first")
        self.assertEqual(self.run_installer().returncode, 0)
        installed = self.install_dir() / "ompig-slim"
        before = installed.read_bytes()
        # Publish a different payload under the old checksum file.
        directory = self.fixture.stage("latest", fake_version="ompig-slim 2.0.0-second")
        archive = directory / "ompig-slim-linux-x64.tar.gz"
        archive.write_bytes(self.fixture.archive_bytes(None, "ompig-slim 3.0.0-tampered"))
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertEqual(installed.read_bytes(), before)

    def test_malformed_checksum_is_rejected(self):
        self.fixture.stage("latest", verify=False)
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("not a valid checksum file", result.stderr)
        self.assertFalse(self.install_dir().exists())

    def test_missing_release_fails_without_creating_the_directory(self):
        result = self.run_installer("--version", "v9.9.9")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.install_dir().exists())

    def test_archive_without_the_executable_is_rejected(self):
        self.fixture.stage("latest")
        directory = self.fixture.releases / "latest" / "download"
        archive = directory / "ompig-slim-linux-x64.tar.gz"
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w:gz") as package:
            member = tarfile.TarInfo("ompig-slim/README.md")
            member.size = 3
            package.addfile(member, io.BytesIO(b"doc"))
        archive.write_bytes(buffer.getvalue())
        pathlib.Path(str(archive) + ".sha256").write_text(f"{sha256(archive)}  {archive.name}\n")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("does not contain ompig-slim/ompig-slim", result.stderr)
        self.assertFalse(self.install_dir().exists())

    def test_symlinked_executable_is_rejected(self):
        self.fixture.stage("latest")
        directory = self.fixture.releases / "latest" / "download"
        archive = directory / "ompig-slim-linux-x64.tar.gz"
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w:gz") as package:
            member = tarfile.TarInfo("ompig-slim/ompig-slim")
            member.type = tarfile.SYMTYPE
            member.linkname = "/bin/sh"
            package.addfile(member)
        archive.write_bytes(buffer.getvalue())
        pathlib.Path(str(archive) + ".sha256").write_text(f"{sha256(archive)}  {archive.name}\n")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no regular ompig-slim/ompig-slim file", result.stderr)
        self.assertFalse(self.install_dir().exists())

    def test_destination_that_is_not_a_directory_is_reported(self):
        self.fixture.stage("latest")
        blocker = self.root / "blocker"
        blocker.write_text("a file, not a directory")
        result = self.run_installer("--dir", str(blocker))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("cannot write to", result.stderr)
        self.assertEqual(blocker.read_text(), "a file, not a directory")

    def test_running_under_a_non_bash_shell_names_the_requirement(self):
        shell = shutil.which("dash") or shutil.which("sh")
        if shell is None:
            self.skipTest("no POSIX shell available")
        self.fixture.stage("latest")
        result = subprocess.run(
            [shell, str(INSTALLER)],
            env=self.env(), text=True, capture_output=True, timeout=60,
        )
        # Some images link /bin/sh to bash; the guarantee is about the message
        # when it is not bash, so only assert the refusal when it applies.
        if subprocess.run([shell, "-c", 'echo "$BASH_VERSION"'], capture_output=True,
                          text=True).stdout.strip():
            self.skipTest("sh is bash on this host")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("bash is required", result.stderr)
        self.assertFalse(self.install_dir().exists())

    def test_unsupported_platforms_are_refused_before_downloading(self):
        self.fixture.stage("latest")
        self.use_platform("FreeBSD", "x86_64")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported operating system", result.stderr)
        self.assertNotIn("Downloading", result.stdout)
        self.use_platform("Linux", "riscv64")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported CPU", result.stderr)
        self.assertNotIn("Downloading", result.stdout)
        self.assertFalse(self.install_dir().exists())

    def test_missing_home_is_reported_unless_a_directory_is_given(self):
        self.fixture.stage("latest")
        env = self.env()
        del env["HOME"]
        result = subprocess.run(["bash", str(INSTALLER)], env=env,
                                text=True, capture_output=True, timeout=60)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("HOME must be set", result.stderr)
        # The message must not leak the script path and line number of a ${...:?}
        # expansion, which is what a piped user would otherwise see.
        self.assertNotIn("line ", result.stderr)
        directory = self.root / "no-home"
        result = self.run_installer("--dir", str(directory), HOME="")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((directory / "ompig-slim").exists())

    def test_unusable_temporary_directory_is_reported(self):
        self.fixture.stage("latest")
        result = self.run_installer(TMPDIR=str(self.root / "absent"))
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.install_dir().exists())

    def test_banner_is_printed_and_stays_aligned_without_color(self):
        # The mark is plain text with optional color runs, so redirected output
        # keeps the same geometry and no escape sequence reaches a log file.
        self.fixture.stage("latest")
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("in Go \u00b7 slim harness", result.stdout)
        art = [line for line in result.stdout.splitlines() if "\u2588" in line or "\u255a" in line]
        self.assertEqual(len(art), 5, result.stdout)
        # Every row of the mark is the same width once color is stripped.
        widths = {len(line) for line in art}
        self.assertEqual(len(widths), 1, f"logo rows differ in width: {art}")
        self.assertNotIn("\x1b", result.stdout)

    def test_a_refused_platform_prints_no_banner(self):
        # The mark announces an install that is about to happen, so a refusal
        # before that point prints only its own message.
        self.fixture.stage("latest")
        self.use_platform("FreeBSD", "x86_64")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("in Go", result.stdout)
        self.assertNotIn("\u2588", result.stdout)

    def test_usage_help_stays_free_of_the_mark(self):
        result = self.run_installer("--help")
        self.assertIn("Usage: install.sh", result.stdout)
        self.assertNotIn("in Go", result.stdout)

    def test_usage_errors_exit_two(self):
        for arguments in (["--nope"], ["--dir"], ["--version"]):
            with self.subTest(arguments=arguments):
                result = self.run_installer(*arguments)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("Usage: install.sh", result.stderr)

    def test_help_prints_usage_and_installs_nothing(self):
        result = self.run_installer("--help")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Usage: install.sh", result.stdout)
        self.assertFalse(self.install_dir().exists())

    def test_path_hint_is_printed_only_when_needed(self):
        self.fixture.stage("latest")
        result = self.run_installer()
        self.assertIn("is not on your PATH", result.stdout)
        self.assertIn(str(self.install_dir()), result.stdout)
        # With the destination on PATH the hint disappears.
        result = self.run_installer(PATH=f"{self.install_dir()}:{self.path}")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("is not on your PATH", result.stdout)

    def test_executable_that_fails_to_run_is_not_installed(self):
        self.fixture.stage("latest", content="#!/bin/sh\necho broken >&2\nexit 3\n")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("did not run on this machine", result.stderr)
        self.assertFalse((self.install_dir() / "ompig-slim").exists())
        # A failed smoke check must not leave the staged copy behind.
        self.assertEqual(os.listdir(self.install_dir()), [])

    def test_wget_is_used_when_curl_is_absent(self):
        if shutil.which("wget") is None:
            self.skipTest("wget is not installed on this host")
        self.fixture.stage("latest")
        # Hide curl by building a PATH that contains every other tool. The shims
        # directory stays first so the fixture's uname still selects the release.
        bindir = self.root / "no-curl"
        bindir.mkdir()
        for tool in ("bash", "tar", "gzip", "mktemp", "awk", "cp", "mv", "chmod", "rm", "mkdir",
                     "uname", "sha256sum", "shasum", "wget"):
            found = shutil.which(tool)
            if found:
                (bindir / tool).symlink_to(found)
        result = self.run_installer(PATH=f"{self.shims}:{bindir}")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(os.access(self.install_dir() / "ompig-slim", os.X_OK))

    def test_missing_http_client_is_reported(self):
        if shutil.which("curl") is None:
            self.skipTest("curl is not installed on this host")
        self.fixture.stage("latest")
        bindir = self.root / "no-network"
        bindir.mkdir()
        for tool in ("bash", "tar", "gzip", "mktemp", "awk", "cp", "mv", "chmod", "rm", "mkdir",
                     "uname", "sha256sum", "shasum"):
            found = shutil.which(tool)
            if found:
                (bindir / tool).symlink_to(found)
        result = self.run_installer(PATH=f"{self.shims}:{bindir}")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("curl or wget is required", result.stderr)

    def test_missing_digest_tool_is_reported(self):
        self.fixture.stage("latest")
        bindir = self.root / "no-digest"
        bindir.mkdir()
        for tool in ("bash", "tar", "gzip", "mktemp", "awk", "cp", "mv", "chmod", "rm", "mkdir",
                     "uname", "curl"):
            found = shutil.which(tool)
            if found:
                (bindir / tool).symlink_to(found)
        result = self.run_installer(PATH=f"{self.shims}:{bindir}")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("sha256sum or shasum is required", result.stderr)
        self.assertFalse(self.install_dir().exists())

    def test_missing_gzip_is_reported_for_gnu_tar(self):
        # bsdtar (macOS) decompresses gzip internally, so the requirement only
        # holds where tar shells out to the helper.
        tar = shutil.which("tar")
        if tar is None:
            self.skipTest("tar is not installed on this host")
        banner = subprocess.run([tar, "--version"], capture_output=True, text=True).stdout.lower()
        if "bsdtar" in banner:
            self.skipTest("bsdtar decompresses gzip itself")
        self.fixture.stage("latest")
        bindir = self.root / "no-gzip"
        bindir.mkdir()
        for tool in ("bash", "tar", "mktemp", "awk", "cp", "mv", "chmod", "rm", "mkdir",
                     "uname", "sha256sum", "shasum", "curl"):
            found = shutil.which(tool)
            if found:
                (bindir / tool).symlink_to(found)
        result = self.run_installer(PATH=f"{self.shims}:{bindir}")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("gzip is required", result.stderr)


if __name__ == "__main__":
    unittest.main()
