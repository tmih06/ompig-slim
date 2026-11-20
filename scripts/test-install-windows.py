#!/usr/bin/env python3
"""Exercise the native Windows installer with a real Windows executable.

Run this on Windows with the built executable that the release ZIP contains:

    python scripts/test-install-windows.py bin/native/ompig-slim/ompig-slim.exe

Every download is served from loopback. The suite never modifies the user's
PATH except in one test that saves and restores the original user PATH.
"""
import argparse
import base64
import ctypes
import hashlib
import http.server
import io
import json
import os
import pathlib
import re
import shutil
import stat
import struct
import subprocess
import sys
import tempfile
import threading
import urllib.parse
import unittest
import warnings
import zipfile

INSTALLER = pathlib.Path(__file__).resolve().with_name("install.ps1")
ARCHIVE_MEMBER = "ompig-slim/ompig-slim.exe"
LICENSE_MEMBER = "ompig-slim/LICENSE"
MAX_EXECUTABLE_SIZE = 256 * 1024 * 1024
BINARY = None
POWERSHELL = None
BINARY_VERSION = None
CURRENT_VERSION = None
BINARY_ARCH = None
NATIVE_ARCHIVE = None


def pe_architecture(path):
    with path.open("rb") as executable:
        header = executable.read(4096)
    if len(header) < 64 or header[:2] != b"MZ":
        raise ValueError(f"{path} is not a Windows PE executable")
    pe_offset = struct.unpack_from("<I", header, 0x3C)[0]
    if pe_offset + 6 > len(header) or header[pe_offset:pe_offset + 4] != b"PE\0\0":
        raise ValueError(f"{path} does not contain a valid PE header")
    machine = struct.unpack_from("<H", header, pe_offset + 4)[0]
    return {0x8664: "x64", 0xAA64: "arm64"}.get(machine)


def build_archive(executable=None, members=(), duplicate=False, symlink=False, declared_size=None,
                  include_executable=True):
    if executable is None:
        executable = BINARY.read_bytes()
    buffer = io.BytesIO()
    with warnings.catch_warnings():
        warnings.simplefilter("ignore", UserWarning)
        with zipfile.ZipFile(buffer, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            def add(name, payload, kind="file"):
                entry = zipfile.ZipInfo(name)
                entry.create_system = 3
                if kind == "symlink":
                    entry.external_attr = (stat.S_IFLNK | 0o777) << 16
                else:
                    entry.external_attr = (stat.S_IFREG | 0o755) << 16
                archive.writestr(entry, payload)

            if include_executable:
                add(ARCHIVE_MEMBER, b"not the target" if symlink else executable,
                    "symlink" if symlink else "file")
            if duplicate:
                add(ARCHIVE_MEMBER, executable)
            add(LICENSE_MEMBER, b"license fixture", "file")
            for member in members:
                add(*member)

    data = bytearray(buffer.getvalue())
    if declared_size is not None:
        with zipfile.ZipFile(io.BytesIO(data)) as parsed:
            position = parsed.start_dir
        changed = False
        while True:
            position = data.find(b"PK\x01\x02", position)
            if position < 0:
                break
            name_length = struct.unpack_from("<H", data, position + 28)[0]
            extra_length = struct.unpack_from("<H", data, position + 30)[0]
            comment_length = struct.unpack_from("<H", data, position + 32)[0]
            name_start = position + 46
            name = bytes(data[name_start:name_start + name_length]).decode("utf-8")
            if name == ARCHIVE_MEMBER:
                struct.pack_into("<I", data, position + 24, declared_size)
                local_header = struct.unpack_from("<I", data, position + 42)[0]
                struct.pack_into("<I", data, local_header + 22, declared_size)
                changed = True
                break
            position = name_start + name_length + extra_length + comment_length
        if not changed:
            raise AssertionError("could not find the executable ZIP entry")
    return bytes(data)


def checksum_for(archive, name):
    digest = hashlib.sha256(archive).hexdigest()
    return f"{digest}  {name}\n".encode("ascii")


def open_windows_file(path, access, sharing, disposition):
    kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel32.CreateFileW.argtypes = [ctypes.c_wchar_p, ctypes.c_uint32, ctypes.c_uint32,
                                    ctypes.c_void_p, ctypes.c_uint32, ctypes.c_uint32,
                                    ctypes.c_void_p]
    kernel32.CreateFileW.restype = ctypes.c_void_p
    handle = kernel32.CreateFileW(str(path), access, sharing, None, disposition, 0x80, None)
    if handle == ctypes.c_void_p(-1).value:
        raise ctypes.WinError(ctypes.get_last_error())
    kernel32.CloseHandle.argtypes = [ctypes.c_void_p]
    kernel32.CloseHandle.restype = ctypes.c_int
    return kernel32, handle


class Overlapped(ctypes.Structure):
    _fields_ = [("Internal", ctypes.c_void_p), ("InternalHigh", ctypes.c_void_p),
                ("Offset", ctypes.c_ulong), ("OffsetHigh", ctypes.c_ulong),
                ("hEvent", ctypes.c_void_p)]


def lock_windows_file_range(kernel32, handle):
    kernel32.LockFileEx.argtypes = [ctypes.c_void_p, ctypes.c_uint32, ctypes.c_uint32,
                                    ctypes.c_uint32, ctypes.c_uint32, ctypes.POINTER(Overlapped)]
    kernel32.LockFileEx.restype = ctypes.c_int
    overlapped = Overlapped()
    if not kernel32.LockFileEx(handle, 0x3, 0, 1, 0, ctypes.byref(overlapped)):
        raise ctypes.WinError(ctypes.get_last_error())
    return overlapped


def unlock_windows_file_range(kernel32, handle, overlapped):
    kernel32.UnlockFileEx.argtypes = [ctypes.c_void_p, ctypes.c_uint32, ctypes.c_uint32,
                                      ctypes.c_uint32, ctypes.POINTER(Overlapped)]
    kernel32.UnlockFileEx.restype = ctypes.c_int
    if not kernel32.UnlockFileEx(handle, 0, 1, 0, ctypes.byref(overlapped)):
        raise ctypes.WinError(ctypes.get_last_error())


class ReleaseServer:
    def __init__(self, platform):
        self.platform = platform
        self.latest_tag = None
        self.releases = {}
        self.requests = []
        self.lock = threading.Lock()
        fixture = self

        def release_document(tag):
            return {"tag_name": tag, "draft": False, "prerelease": "-beta." in tag}

        class Handler(http.server.BaseHTTPRequestHandler):
            def send_json(self, document):
                body = json.dumps(document).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_GET(self):
                parsed = urllib.parse.urlsplit(self.path)
                path = parsed.path
                with fixture.lock:
                    fixture.requests.append(self.path)
                    latest_tag = fixture.latest_tag
                    releases = dict(fixture.releases)
                if path == "/releases/latest":
                    if latest_tag is None:
                        self.send_error(404)
                        return
                    self.send_json(release_document(latest_tag))
                    return
                if path == "/releases":
                    self.send_json([release_document(tag) for tag in releases])
                    return
                prefix = "/releases/latest/download/"
                if path.startswith(prefix):
                    if latest_tag is None:
                        self.send_error(404)
                        return
                    asset = path[len(prefix):]
                    location = ("/releases/download/" + urllib.parse.quote(latest_tag, safe="")
                                + "/" + urllib.parse.quote(asset, safe=""))
                    self.send_response(302)
                    self.send_header("Location", location)
                    self.send_header("Content-Length", "0")
                    self.end_headers()
                    return

                parts = path.split("/")
                if len(parts) != 5 or parts[1:3] != ["releases", "download"]:
                    self.send_error(404)
                    return
                tag, asset = parts[3], parts[4]
                record = releases.get(tag)
                if record is None:
                    self.send_error(404)
                    return
                archive, sidecar, declared_length = record
                archive_name = f"ompig-slim-{fixture.platform}.zip"
                if asset == archive_name:
                    body = archive
                    content_length = len(body) if declared_length is None else declared_length
                elif asset == archive_name + ".sha256":
                    body = sidecar
                    content_length = len(body)
                else:
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Content-Type", "application/octet-stream")
                self.send_header("Content-Length", str(content_length))
                self.end_headers()
                if declared_length is None or asset != archive_name:
                    self.wfile.write(body)

            def log_message(self, *_args):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        host, port = self.server.server_address
        self.base = f"http://{host}:{port}/releases"

    def publish(self, tag, archive, latest=False, checksum=None, archive_content_length=None):
        archive_name = f"ompig-slim-{self.platform}.zip"
        sidecar = checksum_for(archive, archive_name) if checksum is None else checksum
        with self.lock:
            self.releases[tag] = (archive, sidecar, archive_content_length)
            if latest:
                self.latest_tag = tag

    def close(self):
        self.server.shutdown()
        self.server.server_close()


def power_shell_executable():
    for name in ("powershell.exe", "powershell", "pwsh.exe", "pwsh"):
        found = shutil.which(name)
        if found:
            return found
    system_root = os.environ.get("SystemRoot", r"C:\Windows")
    candidate = pathlib.Path(system_root) / "System32" / "WindowsPowerShell" / "v1.0" / "powershell.exe"
    if candidate.is_file():
        return str(candidate)
    raise ValueError("Windows PowerShell 5.1 or PowerShell 7 is required")


class WindowsInstallTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="ompig install windows ")
        self.addCleanup(self.temporary.cleanup)
        self.root = pathlib.Path(self.temporary.name)
        self.home = self.root / "profile 雪"
        self.home.mkdir()
        self.install_dir = self.root / "install directory 雪"
        self.pig_home = self.root / "PiG profile 雪"
        self.local_app_data = self.root / "Local App Data 雪"
        self.platform = f"windows-{BINARY_ARCH}"
        self.server = ReleaseServer(self.platform)
        self.addCleanup(self.server.close)

    def environment(self, **overrides):
        env = dict(os.environ)
        for key in list(env):
            if key.casefold() == "psmodulepath":
                del env[key]
        for key in ("OMPIG_SLIM_VERSION", "OMPIG_SLIM_INSTALL_DIR"):
            env.pop(key, None)
        env.update({
            "OMPIG_SLIM_DOWNLOAD_BASE": self.server.base,
            "OMPIG_SLIM_RELEASES_API": f"{self.server.base}/latest",
            "PIG_HOME": str(self.pig_home),
            "LOCALAPPDATA": str(self.local_app_data),
            "USERPROFILE": str(self.home),
            "PI_OFFLINE": "1",
            "PI_SKIP_VERSION_CHECK": "1",
        })
        env.update({key: str(value) for key, value in overrides.items()})
        return env

    def powershell(self, *arguments, env=None):
        return subprocess.run(
            [POWERSHELL, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
             *map(str, arguments)],
            env=self.environment() if env is None else env,
            text=True, capture_output=True, errors="replace", timeout=120, cwd=self.root,
        )

    def run_installer(self, *arguments, env=None):
        return self.powershell("-File", INSTALLER, *arguments, env=env)

    def stage(self, tag="v1.2.3", *, archive=None, latest=False, checksum=None,
              archive_content_length=None, **archive_options):
        if archive is None:
            archive = build_archive(**archive_options) if archive_options else NATIVE_ARCHIVE
        self.server.publish(tag, archive, latest=latest, checksum=checksum,
                            archive_content_length=archive_content_length)

    def target(self, directory=None):
        return (self.install_dir if directory is None else pathlib.Path(directory)) / "ompig-slim.exe"

    def marker(self):
        return self.pig_home / "ompig-slim-install"

    def assert_installed(self, result, target=None):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        path = self.target() if target is None else pathlib.Path(target)
        self.assertEqual(path.read_bytes(), BINARY.read_bytes())
        self.assertFalse(any(path.parent.glob(".ompig-slim.new.*.exe")))

    def assert_failed_cleanly(self, result, target=None):
        self.assertNotEqual(result.returncode, 0, result.stdout)
        path = self.target() if target is None else pathlib.Path(target)
        self.assertFalse(path.exists())
        self.assertFalse(self.marker().exists())
        self.assertFalse(any(path.parent.glob(".ompig-slim.new.*.exe")))

    def install_for_updates(self):
        self.stage(f"v{CURRENT_VERSION}", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        result = self.run_installer("-NoModifyPath", env=env)
        self.assert_installed(result)
        return self.target()

    def run_installed(self, *arguments):
        return subprocess.run(
            [str(self.target()), *arguments], env=self.environment(), cwd=self.root,
            text=True, capture_output=True, errors="replace", timeout=180,
        )

    def assert_update_preserved(self, before_binary, before_marker):
        self.assertEqual(self.target().read_bytes(), before_binary)
        self.assertEqual(self.marker().read_bytes(), before_marker)

    def test_latest_release_installs_and_repeat_install_is_idempotent(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_VERSION="latest", OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        first = self.run_installer("-NoModifyPath", env=env)
        self.assert_installed(first)
        before = self.marker().read_bytes()
        second = self.run_installer("-NoModifyPath", env=env)
        self.assert_installed(second)
        self.assertEqual(self.marker().read_bytes(), before)
        self.assertEqual({path.name for path in self.install_dir.iterdir()},
                         {"ompig-slim.exe", "ompig-slim.exe.update.lock"})
        self.assertIn("/releases/latest/download/ompig-slim-windows-", self.server.requests[0])
        self.assertFalse(any(path.split("?", 1)[0] == "/releases/latest" for path in self.server.requests))

    def test_install_does_not_need_bash_git_or_go_on_path(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        env.update({"PATH": "", "ProgramFiles": str(self.root / "empty programs"),
                    "ProgramFiles(x86)": ""})
        result = self.run_installer("-NoModifyPath", env=env)
        self.assert_installed(result)

    def test_pinned_version_and_directory_flags_override_environment(self):
        self.stage("v1.2.3")
        self.stage("v9.9.9", latest=True)
        directory = self.root / "chosen install 雪"
        env = self.environment(OMPIG_SLIM_VERSION="v9.9.9", OMPIG_SLIM_INSTALL_DIR=self.root / "wrong dir")
        result = self.run_installer("-Version", "v1.2.3", "-Dir", directory, "-NoModifyPath", env=env)
        self.assert_installed(result, self.target(directory))
        self.assertTrue(any("/releases/download/v1.2.3/ompig-slim-windows-" in path for path in self.server.requests))
        self.assertFalse((self.root / "wrong dir").exists())

    def test_relative_paths_follow_powershell_location(self):
        self.stage("v1.2.3", latest=True)
        location = self.root / "PowerShell location"
        location.mkdir()
        env = self.environment(INSTALLER_SCRIPT=INSTALLER, INSTALLER_LOCATION=location,
                               PIG_HOME="relative profile")
        result = self._run_powershell(
            "Push-Location -LiteralPath $env:INSTALLER_LOCATION; try { "
            "& $env:INSTALLER_SCRIPT -Version v1.2.3 -Dir 'relative install' -NoModifyPath "
            "} finally { Pop-Location }", env=env)
        target = location / "relative install" / "ompig-slim.exe"
        self.assert_installed(result, target)
        self.assertTrue((location / "relative profile" / "ompig-slim-install").is_file())
        self.assertFalse((self.root / "relative install").exists())

    def test_environment_version_directory_and_pig_home_overrides(self):
        self.stage("v2.0.0", latest=True)
        directory = self.root / "environment install 雪"
        custom_pig_home = self.root / "custom PiG home 雪"
        env = self.environment(OMPIG_SLIM_VERSION="v2.0.0", OMPIG_SLIM_INSTALL_DIR=directory,
                              PIG_HOME=custom_pig_home)
        result = self.run_installer("-NoModifyPath", env=env)
        self.assert_installed(result, self.target(directory))
        marker = custom_pig_home / "ompig-slim-install"
        self.assertTrue(marker.is_file())
        self.assertFalse(self.marker().exists())
        self.assertTrue(any("/releases/download/v2.0.0/" in path for path in self.server.requests))

    def test_pig_home_expands_only_supported_tilde_forms(self):
        self.stage("v1.2.3", latest=True)
        cases = (
            ("~", self.home),
            ("~/custom PiG root", self.home / "custom PiG root"),
            (r"~\literal PiG root", self.root / "~" / "literal PiG root"),
            ("relative PiG root", self.root / "relative PiG root"),
        )
        for value, marker_root in cases:
            with self.subTest(pig_home=value):
                env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir, PIG_HOME=value)
                self.assert_installed(self.run_installer("-NoModifyPath", env=env))
                self.assertTrue((marker_root / "ompig-slim-install").is_file())

    def test_empty_pig_home_defaults_to_user_pi_directory(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir, PIG_HOME="")
        self.assert_installed(self.run_installer("-NoModifyPath", env=env))
        self.assertTrue((self.home / ".pi" / "ompig-slim-install").is_file())
        self.assertFalse(self.marker().exists())

    def test_newline_pig_home_is_rejected_before_replacement(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        self.assert_installed(self.run_installer("-NoModifyPath", env=env))
        before_binary = self.target().read_bytes()
        before_marker = self.marker().read_bytes()
        self.stage("v2.0.0", latest=True)
        self.server.requests.clear()
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir, PIG_HOME=str(self.pig_home) + "\nignored")
        result = self.run_installer("-NoModifyPath", env=env)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("PIG_HOME must not contain a newline", result.stderr)
        self.assertFalse(self.server.requests)
        self.assert_update_preserved(before_binary, before_marker)

    def test_default_directory_uses_local_app_data(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_VERSION="latest")
        result = self.run_installer("-NoModifyPath", env=env)
        default_dir = self.local_app_data / "ompig-slim" / "bin"
        self.assert_installed(result, self.target(default_dir))

    def test_marker_is_utf8_without_bom_and_records_archive_provenance(self):
        self.stage("v1.2.3", latest=True)
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assert_installed(result)
        data = self.marker().read_bytes()
        self.assertFalse(data.startswith(b"\xef\xbb\xbf"))
        fields = dict(line.split("=", 1) for line in data.decode("utf-8").splitlines())
        self.assertEqual(fields["kind"], "archive")
        self.assertEqual(fields["executable"], str(self.target()))
        self.assertEqual(fields["download-base"], self.server.base)
        self.assertEqual(fields["installed-version"], BINARY_VERSION)
        self.assertEqual(fields["installer"], f"{self.server.base}/latest/download/ompig-slim-windows-{BINARY_ARCH}.zip")
        self.assertEqual(fields["channel"], "stable")

    def test_checksum_mismatch_preserves_existing_binary_and_marker(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        self.assert_installed(self.run_installer("-NoModifyPath", env=env))
        before_binary = self.target().read_bytes()
        before_marker = self.marker().read_bytes()
        archive = build_archive(executable=BINARY.read_bytes() + b"changed")
        self.stage("v2.0.0", archive=archive, latest=True, checksum=b"0" * 64)
        result = self.run_installer("-NoModifyPath", env=env)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("Checksum mismatch", result.stderr)
        self.assert_update_preserved(before_binary, before_marker)

    def test_oversized_archive_download_is_rejected_without_installing(self):
        self.stage("v1.2.3", latest=True, archive_content_length=512 * 1024 * 1024 + 1)
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("byte limit", result.stderr)
        self.assert_failed_cleanly(result)

    def test_oversized_checksum_download_is_rejected_without_installing(self):
        self.stage("v1.2.3", latest=True, checksum=b"x" * 4097)
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("byte limit", result.stderr)
        self.assert_failed_cleanly(result)

    def test_empty_checksum_is_rejected_without_installing(self):
        self.stage("v1.2.3", latest=True, checksum=b"")
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("not a valid checksum file", result.stderr)
        self.assert_failed_cleanly(result)

    def test_archive_without_exact_executable_is_rejected(self):
        archive = build_archive(include_executable=False,
                                members=[("ompig-slim/ompig-slim", b"wrong name", "file")])
        self.stage("v1.2.3", archive=archive, latest=True)
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assertIn("must contain exactly one", result.stderr)
        self.assert_failed_cleanly(result)

    def test_unsafe_archive_member_is_rejected_without_path_extraction(self):
        for name in ("../escaped.txt", "ompig-slim/trailing.", "ompig-slim/trailing ",
                     "ompig-slim/NUL.txt", "ompig-slim/stream:payload"):
            with self.subTest(name=name):
                archive = build_archive(members=[(name, b"not extracted", "file")])
                self.stage("v1.2.3", archive=archive, latest=True)
                result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
                self.assertIn("Unsafe ZIP member", result.stderr)
                self.assert_failed_cleanly(result)
                self.assertFalse((self.root / "escaped.txt").exists())

    def test_duplicate_executable_member_is_rejected(self):
        self.stage("v1.2.3", latest=True, duplicate=True)
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assertIn("Duplicate ZIP member", result.stderr)
        self.assert_failed_cleanly(result)

    def test_case_alias_executable_member_is_rejected(self):
        archive = build_archive(members=[("ompig-slim/OMPIG-SLIM.EXE", b"alias", "file")])
        self.stage("v1.2.3", archive=archive, latest=True)
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assertIn("Duplicate ZIP member", result.stderr)
        self.assert_failed_cleanly(result)

    def test_symlink_executable_member_is_rejected(self):
        self.stage("v1.2.3", latest=True, symlink=True)
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assertIn("symbolic link", result.stderr)
        self.assert_failed_cleanly(result)

    def test_oversized_executable_member_is_rejected(self):
        archive = build_archive(declared_size=MAX_EXECUTABLE_SIZE + 1)
        self.stage("v1.2.3", archive=archive, latest=True)
        result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        self.assertIn("invalid size", result.stderr)
        self.assert_failed_cleanly(result)

    def test_non_runnable_staged_executable_preserves_existing_install(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        self.assert_installed(self.run_installer("-NoModifyPath", env=env))
        before_binary = self.target().read_bytes()
        before_marker = self.marker().read_bytes()
        self.stage("v2.0.0", latest=True, executable=b"not a Windows executable")
        result = self.run_installer("-NoModifyPath", env=env)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assert_update_preserved(before_binary, before_marker)
        self.assertFalse(any(self.install_dir.glob(".ompig-slim.new.*.exe")))

    def test_backup_only_install_recovers_previous_executable_before_staging(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        self.assert_installed(self.run_installer("-NoModifyPath", env=env))
        before_binary = self.target().read_bytes()
        before_marker = self.marker().read_bytes()
        backup = self.install_dir / "ompig-slim.previous.exe"
        self.target().replace(backup)
        self.stage("v2.0.0", latest=True, executable=b"not a Windows executable")
        result = self.run_installer("-NoModifyPath", env=env)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertEqual(self.target().read_bytes(), before_binary)
        self.assertFalse(backup.exists())
        self.assertEqual(self.marker().read_bytes(), before_marker)

    def test_existing_previous_backup_is_not_deleted_when_target_exists(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        self.assert_installed(self.run_installer("-NoModifyPath", env=env))
        backup = self.install_dir / "ompig-slim.previous.exe"
        backup.write_bytes(b"preserve backup while target exists")
        self.stage("v2.0.0", latest=True)
        result = self.run_installer("-NoModifyPath", env=env)
        self.assert_installed(result)
        self.assertEqual(backup.read_bytes(), b"preserve backup while target exists")

    def test_marker_write_failure_warns_after_successful_install(self):
        self.stage("v1.2.3", latest=True)
        blocker = self.root / "not a directory"
        blocker.write_text("fixture", encoding="utf-8")
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir, PIG_HOME=blocker / "nested")
        result = self.run_installer("-NoModifyPath", env=env)
        self.assert_installed(result)
        self.assertIn("could not record the install", result.stderr)

    def test_busy_target_preserves_existing_install_and_marker(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        self.assert_installed(self.run_installer("-NoModifyPath", env=env))
        before_binary = self.target().read_bytes()
        before_marker = self.marker().read_bytes()
        self.stage("v2.0.0", latest=True)
        kernel32, handle = open_windows_file(self.target(), 0x80000000, 0, 3)
        try:
            result = self.run_installer("-NoModifyPath", env=env)
        finally:
            kernel32.CloseHandle(handle)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assert_update_preserved(before_binary, before_marker)

    def test_invalid_and_missing_cli_flags_fail(self):
        self.stage("v1.2.3", latest=True)
        env = self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir)
        for arguments in (("-UnknownOption",), ("-Version",)):
            with self.subTest(arguments=arguments):
                result = self.run_installer(*arguments, env=env)
                self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.install_dir.exists())

    def test_installer_observes_the_runtime_update_lock(self):
        self.install_for_updates()
        before_binary = self.target().read_bytes()
        before_marker = self.marker().read_bytes()
        self.stage("v99.0.0", latest=True)
        kernel32, handle = open_windows_file(str(self.target()) + ".update.lock", 0xC0000000, 0x7, 4)
        overlapped = lock_windows_file_range(kernel32, handle)
        try:
            result = self.run_installer("-NoModifyPath", env=self.environment(OMPIG_SLIM_INSTALL_DIR=self.install_dir))
        finally:
            unlock_windows_file_range(kernel32, handle, overlapped)
            kernel32.CloseHandle(handle)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("install or update is in progress", result.stderr)
        self.assert_update_preserved(before_binary, before_marker)

    def test_update_and_upgrade_replace_the_running_executable(self):
        target = self.install_for_updates()
        before = target.read_bytes()
        for command, version in (("update", "99.0.0"), ("upgrade", "99.0.1")):
            with self.subTest(command=command):
                self.stage(f"v{version}", latest=True)
                result = self.run_installed(command)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertIn("Updated ompig-slim", result.stdout)
                self.assertEqual(target.read_bytes(), BINARY.read_bytes())
                self.assertEqual((self.install_dir / "ompig-slim.previous.exe").read_bytes(), before)
                marker = dict(line.split("=", 1) for line in self.marker().read_text(encoding="utf-8").splitlines())
                self.assertEqual(marker["channel"], "stable")
                before = target.read_bytes()

    def test_stable_noop_force_beta_selection_and_saved_channel(self):
        target = self.install_for_updates()
        current = target.read_bytes()
        marker_before = self.marker().read_bytes()
        self.stage(f"v{CURRENT_VERSION}", latest=True)
        channel = "beta" if "-beta." in CURRENT_VERSION else "stable"
        result = self.run_installed("update", "--channel", channel)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("up to date", result.stdout)
        self.assertEqual(target.read_bytes(), current)
        if channel == "stable":
            self.assertEqual(self.marker().read_bytes(), marker_before)
        else:
            self.assertIn("channel=beta", self.marker().read_text(encoding="utf-8"))
        self.assertEqual(any("/releases?page=" in path for path in self.server.requests), channel == "beta")

        result = self.run_installed("update", "--force", "--channel", channel)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Updated ompig-slim", result.stdout)
        self.assertEqual(target.read_bytes(), BINARY.read_bytes())
        self.assertEqual((self.install_dir / "ompig-slim.previous.exe").read_bytes(), current)

        self.stage("v99.0.0-beta.1")
        result = self.run_installed("upgrade", "--channel", "beta")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("/releases/download/v99.0.0-beta.1/", "\n".join(self.server.requests))
        self.assertIn("channel=beta", self.marker().read_text(encoding="utf-8"))
        self.stage("v99.0.0-beta.2")
        result = self.run_installed("update")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("/releases/download/v99.0.0-beta.2/", "\n".join(self.server.requests))
        self.assertIn("channel=beta", self.marker().read_text(encoding="utf-8"))

    def test_stable_refuses_prerelease_and_malformed_or_bad_checksum_updates_roll_back(self):
        target = self.install_for_updates()
        before_binary = target.read_bytes()
        before_marker = self.marker().read_bytes()

        self.stage("v99.0.0-beta.1", latest=True)
        refused = self.run_installed("update", "--channel", "stable")
        self.assertNotEqual(refused.returncode, 0, refused.stdout)
        self.assertIn("stable release", refused.stderr)
        self.assert_update_preserved(before_binary, before_marker)

        malformed = build_archive(include_executable=False)
        self.stage("v99.0.0", archive=malformed, latest=True)
        result = self.run_installed("update", "--channel", "stable")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assert_update_preserved(before_binary, before_marker)

        self.stage("v99.0.1", archive=build_archive(executable=BINARY.read_bytes() + b"changed"),
                   latest=True, checksum=b"0" * 64)
        result = self.run_installed("update", "--channel", "stable")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("checksum mismatch", result.stderr.lower())
        self.assert_update_preserved(before_binary, before_marker)

    def _run_powershell(self, script, env=None):
        encoded = base64.b64encode(script.encode("utf-16le")).decode("ascii")
        return self.powershell("-EncodedCommand", encoded, env=env)

    def _get_user_path(self):
        script = "$v=[Environment]::GetEnvironmentVariable('Path','User'); if ($null -eq $v) { 'NULL' } else { 'VALUE=' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($v)) }"
        result = self._run_powershell(script)
        self.assertEqual(result.returncode, 0, result.stderr)
        line = next(line for line in result.stdout.splitlines() if line.startswith(("NULL", "VALUE=")))
        if line == "NULL":
            return None
        return base64.b64decode(line[6:]).decode("utf-8")

    def _set_user_path(self, value):
        if value is None:
            expression = "$null"
        else:
            encoded = base64.b64encode(value.encode("utf-8")).decode("ascii")
            expression = f"[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('{encoded}'))"
        result = self._run_powershell(f"[Environment]::SetEnvironmentVariable('Path',{expression},'User')")
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_path_updates_are_additive_and_idempotent(self):
        self.stage("v1.2.3", latest=True)
        original = self._get_user_path()
        existing = f"{self.root / 'existing one'};{self.root / 'existing two'}"
        try:
            self._set_user_path(existing)
            wrapper = self.root / "run-installer.ps1"
            wrapper.write_text(
                ". $env:OMPIG_SLIM_TEST_INSTALLER -Version 'v1.2.3' -Dir $env:OMPIG_SLIM_TEST_INSTALL_DIR\n"
                ". $env:OMPIG_SLIM_TEST_INSTALLER -Version 'v1.2.3' -Dir $env:OMPIG_SLIM_TEST_INSTALL_DIR\n"
                "Write-Output ('TESTPATH=' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($env:Path)))\n",
                encoding="ascii",
            )
            env = self.environment(OMPIG_SLIM_TEST_INSTALLER=INSTALLER,
                                  OMPIG_SLIM_TEST_INSTALL_DIR=self.install_dir)
            result = self.powershell("-File", wrapper, env=env)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            encoded_path = next(line.split("=", 1)[1] for line in result.stdout.splitlines() if line.startswith("TESTPATH="))
            process_path = base64.b64decode(encoded_path).decode("utf-8")
            user_path = self._get_user_path()
            self.assertEqual(user_path.split(";")[:2], existing.split(";"))
            self.assertEqual(sum(os.path.normcase(os.path.normpath(item)) == os.path.normcase(str(self.install_dir))
                                 for item in user_path.split(";")), 1)
            self.assertEqual(sum(os.path.normcase(os.path.normpath(item)) == os.path.normcase(str(self.install_dir))
                                 for item in process_path.split(";")), 1)
        finally:
            self._set_user_path(original)


def main():
    parser = argparse.ArgumentParser(description="Test scripts/install.ps1 against loopback release fixtures.")
    parser.add_argument("binary", help="the native Windows .exe built for this runner")
    args = parser.parse_args()
    if os.name != "nt":
        parser.error("native installer tests require Windows; do not substitute a shell executable")

    global BINARY, POWERSHELL, BINARY_VERSION, CURRENT_VERSION, BINARY_ARCH, NATIVE_ARCHIVE
    BINARY = pathlib.Path(args.binary).resolve()
    if not BINARY.is_file() or BINARY.suffix.lower() != ".exe":
        parser.error("the binary argument must name the built Windows .exe")
    try:
        binary_arch = pe_architecture(BINARY)
        native_arch = os.environ.get("PROCESSOR_ARCHITEW6432") or os.environ.get("PROCESSOR_ARCHITECTURE", "")
        expected_arch = {"AMD64": "x64", "X64": "x64", "X86_64": "x64", "ARM64": "arm64", "AARCH64": "arm64"}.get(native_arch.upper())
        if binary_arch is None or binary_arch != expected_arch:
            parser.error(f"--binary architecture {binary_arch!r} does not match Windows architecture {expected_arch!r}")
        BINARY_ARCH = binary_arch
        POWERSHELL = power_shell_executable()
        version = subprocess.run([str(BINARY), "--version"], text=True, capture_output=True, errors="replace", timeout=30)
        if version.returncode != 0:
            parser.error(f"--binary did not run on this Windows host: {version.stderr.strip()}")
        BINARY_VERSION = version.stdout.strip()
        if not BINARY_VERSION:
            parser.error("the executable --version returned no output")
        version_match = re.search(r"(?<![0-9])v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)", BINARY_VERSION)
        if version_match is None:
            parser.error(f"the executable version is not a release version: {BINARY_VERSION!r}")
        CURRENT_VERSION = version_match.group(1)
        NATIVE_ARCHIVE = build_archive()
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        parser.error(str(error))

    unittest.main(argv=[sys.argv[0]])


if __name__ == "__main__":
    main()
