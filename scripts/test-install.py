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
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

# Release fixtures live in release_fixture.py so this suite and
# test-self-update.py prove the same archive layout.
import release_fixture

INSTALLER = pathlib.Path(__file__).resolve().with_name("install.sh")
FAKE_VERSION = "ompig-slim 9.9.9-test"
# TEST_PLATFORM is what the suite's uname shim reports, so one archive name
# serves every host.
TEST_PLATFORM = "linux-x64"


def stage(fixture, tag, content=None, checksum="correct", fake_version=FAKE_VERSION):
    """Publish the stand-in executable for one release tag.

    `content` replaces the generated script, which is how the suite proves an
    executable that fails to start is rejected. A "latest" tag also refreshes
    the alias, matching how a release publishes both.
    """
    payload = content.encode() if content is not None else release_fixture.script_payload(fake_version)
    archive = release_fixture.archive_bytes(payload)
    return fixture.publish(tag, archive, make_latest=(tag == "latest"), checksum=checksum)


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="ompig install tests ")
        self.addCleanup(self.temporary.cleanup)
        self.root = pathlib.Path(self.temporary.name)
        self.home = self.root / "home"
        self.home.mkdir()
        self.fixture = release_fixture.ReleaseServer(self.root / "release", TEST_PLATFORM)
        # The installer takes the release root, which sits under /releases.
        self.base = f"{self.fixture.start()}/releases"
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
        stage(self.fixture, "latest")
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
        stage(self.fixture, "v1.2.3")
        result = self.run_installer("--version", "v1.2.3")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/download/v1.2.3/", result.stdout)
        self.assertTrue((self.install_dir() / "ompig-slim").exists())

    def test_tag_comes_from_the_environment(self):
        stage(self.fixture, "v1.2.3")
        result = self.run_installer(OMPIG_SLIM_VERSION="v1.2.3")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/download/v1.2.3/", result.stdout)

    def test_latest_spelling_resolves_to_the_latest_alias(self):
        stage(self.fixture, "latest")
        result = self.run_installer("--version", "latest")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/latest/download/", result.stdout)

    def test_flag_overrides_environment(self):
        stage(self.fixture, "v1.2.3")
        result = self.run_installer("--version", "v1.2.3", OMPIG_SLIM_VERSION="v9.9.9")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/download/v1.2.3/", result.stdout)
        self.assertTrue((self.install_dir() / "ompig-slim").exists())

    def test_directory_override_and_environment_fallback(self):
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
        blocker = self.root / "blocker"
        blocker.write_text("a file, not a directory")
        result = self.run_installer(PIG_HOME=str(blocker / "nested"))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("could not record the install", result.stderr)
        self.assertTrue((self.install_dir() / "ompig-slim").exists())

    def test_upgrade_replaces_the_previous_executable(self):
        stage(self.fixture, "latest", fake_version="ompig-slim 1.0.0-first")
        self.assertEqual(self.run_installer().returncode, 0)
        first = (self.install_dir() / "ompig-slim").read_text()
        stage(self.fixture, "latest", fake_version="ompig-slim 2.0.0-second")
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        second = (self.install_dir() / "ompig-slim").read_text()
        self.assertNotEqual(first, second)
        self.assertIn("2.0.0-second", second)
        self.assertEqual(sorted(os.listdir(self.install_dir())), ["ompig-slim"])

    def test_checksum_mismatch_keeps_the_previous_install(self):
        stage(self.fixture, "latest", fake_version="ompig-slim 1.0.0-first")
        self.assertEqual(self.run_installer().returncode, 0)
        installed = self.install_dir() / "ompig-slim"
        before = installed.read_bytes()
        # Republish with a digest that does not describe the archive: the
        # download is rejected, and the working install stays byte-identical.
        stage(self.fixture, "latest", fake_version="ompig-slim 2.0.0-second", checksum="wrong")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertEqual(installed.read_bytes(), before)

    def test_malformed_checksum_is_rejected(self):
        stage(self.fixture, "latest", checksum="garbage")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("not a valid checksum file", result.stderr)
        self.assertFalse(self.install_dir().exists())

    def test_missing_release_fails_without_creating_the_directory(self):
        result = self.run_installer("--version", "v9.9.9")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.install_dir().exists())

    def test_archive_without_the_executable_is_rejected(self):
        stage(self.fixture, "latest")
        directory = self.fixture.releases / "latest" / "download"
        archive = directory / "ompig-slim-linux-x64.tar.gz"
        archive.write_bytes(release_fixture.archive_of([
            release_fixture.file_member("ompig-slim/README.md", b"doc", 0o644),
        ]))
        checksum = release_fixture.sha256_bytes(archive.read_bytes())
        pathlib.Path(str(archive) + ".sha256").write_text(f"{checksum}  {archive.name}\n")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("does not contain ompig-slim/ompig-slim", result.stderr)
        self.assertFalse(self.install_dir().exists())

    def test_symlinked_executable_is_rejected(self):
        stage(self.fixture, "latest")
        directory = self.fixture.releases / "latest" / "download"
        archive = directory / "ompig-slim-linux-x64.tar.gz"
        archive.write_bytes(release_fixture.archive_of([
            release_fixture.symlink_member("ompig-slim/ompig-slim", "/bin/sh"),
        ]))
        checksum = release_fixture.sha256_bytes(archive.read_bytes())
        pathlib.Path(str(archive) + ".sha256").write_text(f"{checksum}  {archive.name}\n")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no regular ompig-slim/ompig-slim file", result.stderr)
        self.assertFalse(self.install_dir().exists())

    def test_destination_that_is_not_a_directory_is_reported(self):
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
        result = self.run_installer(TMPDIR=str(self.root / "absent"))
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.install_dir().exists())

    def test_banner_matches_the_startup_mark_without_color(self):
        # The installer uses the same two-line half-block mark as the startup
        # TUI, so the covered halves of the M and the G stay visible as half
        # blocks instead of the solid letters an earlier copy drew. Redirected
        # output drops color escapes but preserves the geometry.
        stage(self.fixture, "latest")
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("█▀█ █▄ ▄█ ▀▀█     █▀▀ █▀█ -slim", result.stdout)
        self.assertIn("█▄█ █ ▀ █ █▀ █ in █▄█ █▄█", result.stdout)
        self.assertNotIn("in Go · slim harness", result.stdout)
        self.assertNotIn("\x1b", result.stdout)

    def test_banner_colors_match_the_startup_mark(self):
        # The mark's colors are the startup header's brand fills, not the 16-color
        # ANSI palette an earlier copy used, and they follow the same truecolor
        # and 256-color decision the TUI makes. The expected sequences were
        # captured from `ompig-slim -ne` drawing its header in a pty, which is
        # the value internal/selfupdate/ui_test.go pins as its golden; the two
        # suites therefore reject the same drift from either surface.
        #
        # Extract the installer's own functions so this cannot pass against a
        # copy that has drifted from the script it ships.
        script = pathlib.Path(INSTALLER).read_text()
        start = script.index("bar_width=32")
        end = script.index("# human_bytes sets REPLY")
        body = script[start:end].replace(
            'interactive=0\nif [[ -t 1 ]]; then interactive=1; fi', "interactive=1")
        source = pathlib.Path(self.root / "mark.sh")
        source.write_text(body)
        cases = {
            # COLORTERM advertises 24-bit color for the terminal.
            "TERM=xterm-256color COLORTERM=truecolor": (
                "\x1b[38;2;236;236;236m█▀█ █▄ ▄█\x1b[0m \x1b[38;2;228;138;122m\x1b[48;2;79;142;179m▀\x1b[0m"
                "\x1b[38;2;228;138;122m▀█\x1b[0m     \x1b[38;2;0;173;216m█▀▀ █▀█\x1b[0m \x1b[38;2;198;160;246m-slim\x1b[0m"),
            # No hint: downsample to the 256-color palette, as the TUI does.
            "TERM=xterm-256color": (
                "\x1b[38;5;255m█▀█ █▄ ▄█\x1b[0m \x1b[38;5;174m\x1b[48;5;67m▀\x1b[0m"
                "\x1b[38;5;174m▀█\x1b[0m     \x1b[38;5;38m█▀▀ █▀█\x1b[0m \x1b[38;5;183m-slim\x1b[0m"),
            # PI_TRUE_COLOR overrides detection in both directions.
            "TERM=xterm-256color COLORTERM=truecolor PI_TRUE_COLOR=0": (
                "\x1b[38;5;255m█▀█ █▄ ▄█\x1b[0m \x1b[38;5;174m\x1b[48;5;67m▀\x1b[0m"
                "\x1b[38;5;174m▀█\x1b[0m     \x1b[38;5;38m█▀▀ █▀█\x1b[0m \x1b[38;5;183m-slim\x1b[0m"),
        }
        for environment, top in cases.items():
            with self.subTest(environment=environment):
                result = subprocess.run(
                    ["bash", "-c", f"set -euo pipefail; . '{source}'; print_banner"],
                    env=dict({"PATH": "/usr/local/bin:/usr/bin:/bin"},
                             **dict(pair.split("=", 1) for pair in environment.split())),
                    text=True, capture_output=True, timeout=60)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(top, result.stdout)

    def test_a_refused_platform_prints_no_banner(self):
        # The mark announces an install that is about to happen, so a refusal
        # before that point prints only its own message.
        stage(self.fixture, "latest")
        self.use_platform("FreeBSD", "x86_64")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("in █▄█", result.stdout)
        self.assertNotIn("\u2588", result.stdout)

    def test_usage_help_stays_free_of_the_mark(self):
        result = self.run_installer("--help")
        self.assertIn("Usage: install.sh", result.stdout)
        self.assertNotIn("in █▄█", result.stdout)

    def test_byte_count_formatting_matches_the_go_meter(self):
        # The installer formats progress itself (it cannot call Go), so the two
        # meters would otherwise drift. Pin the exact strings the Go side prints,
        # including the half-to-even rounding of a trailing 5.
        cases = {
            "0": "0 B",
            "1023": "1023 B",
            "1024": "1.0 KB",
            "1536": "1.5 KB",
            "1016064": "992.2 KB",     # 992.25 KB rounds half-to-even
            "1048576": "1.0 MB",
            "34000000": "32.4 MB",
            "52428800": "50.0 MB",
            "1073741824": "1.0 GB",
            "5368709120": "5.0 GB",
        }
        # Extract the function the installer actually defines, so the test
        # cannot pass against a copy that has drifted.
        script = pathlib.Path(INSTALLER).read_text()
        start = script.index("human_bytes() {")
        end = script.index("\n}\n", start) + len("\n}\n")
        source = pathlib.Path(self.root / "human_bytes.sh")
        source.write_text(script[start:end])
        calls = "".join(f'human_bytes {n}; printf "%s\\n" "$REPLY"\n' for n in cases)
        result = subprocess.run(["bash", "-c", f"set -euo pipefail; . '{source}'; {calls}"],
                                text=True, capture_output=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), list(cases.values()))

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
        stage(self.fixture, "latest")
        result = self.run_installer()
        self.assertIn("is not on your PATH", result.stdout)
        self.assertIn(str(self.install_dir()), result.stdout)
        # With the destination on PATH the hint disappears.
        result = self.run_installer(PATH=f"{self.install_dir()}:{self.path}")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("is not on your PATH", result.stdout)

    def test_executable_that_fails_to_run_is_not_installed(self):
        stage(self.fixture, "latest", content="#!/bin/sh\necho broken >&2\nexit 3\n")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("did not run on this machine", result.stderr)
        self.assertFalse((self.install_dir() / "ompig-slim").exists())
        # A failed smoke check must not leave the staged copy behind.
        self.assertEqual(os.listdir(self.install_dir()), [])

    def test_wget_is_used_when_curl_is_absent(self):
        if shutil.which("wget") is None:
            self.skipTest("wget is not installed on this host")
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
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
        stage(self.fixture, "latest")
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
