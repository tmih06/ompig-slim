#!/usr/bin/env python3
"""Exercise the native PowerShell dependency and Go-check tooling."""
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parent.parent
INSTALLER = ROOT / "scripts/install.ps1"
LOAD_FUNCTIONS = r'''
$ErrorActionPreference = 'Stop'
$tokens = $null
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($env:FUNCTIONS_SCRIPT, [ref]$tokens, [ref]$errors)
if ($errors.Count) { throw ($errors | Out-String) }
$functions = $ast.FindAll({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] }, $false)
foreach ($function in $functions) { . ([scriptblock]::Create($function.Extent.Text)) }
'''

FAKE_GO = r'''package main

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

func fail(message string) {
    fmt.Fprintln(os.Stderr, message)
    os.Exit(71)
}

func record(args []string) {
    path := os.Getenv("CHECK_GO_LOG")
    if path == "" {
        return
    }
    file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
    if err != nil {
        fail(err.Error())
    }
    defer file.Close()
    call := map[string]interface{}{
        "args": args,
        "cwd": mustGetwd(),
        "gopath": os.Getenv("GOPATH"),
        "goflags": os.Getenv("GOFLAGS"),
        "gomodule": os.Getenv("GO111MODULE"),
        "gotoolchain": os.Getenv("GOTOOLCHAIN"),
        "home": os.Getenv("HOME"),
        "userprofile": os.Getenv("USERPROFILE"),
    }
    if err := json.NewEncoder(file).Encode(call); err != nil {
        fail(err.Error())
    }
}

func mustGetwd() string {
    path, err := os.Getwd()
    if err != nil {
        fail(err.Error())
    }
    return path
}

func write(path, contents string) {
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        fail(err.Error())
    }
    if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
        fail(err.Error())
    }
}

func main() {
    args := os.Args[1:]
    if len(args) == 2 && args[0] == "env" && args[1] == "GOROOT" {
        fmt.Println(os.Getenv("FAKE_GOROOT"))
        return
    }
    record(args)
    if len(args) == 2 && args[0] == "mod" && args[1] == "vendor" {
        if filepath.Clean(mustGetwd()) == filepath.Clean(os.Getenv("CHECKOUT")) {
            fail("go mod vendor was not run in a scratch module")
        }
        if os.Getenv("GOFLAGS") != "" {
            fail("GOFLAGS was not cleared")
        }
        root := os.Getenv("CHECKOUT")
        original, err := os.ReadFile(filepath.Join(root, "go.sum"))
        if err != nil {
            fail(err.Error())
        }
        scratch, err := os.ReadFile("go.sum")
        if err != nil || string(original) != string(scratch) {
            fail("scratch module did not preserve go.sum")
        }
        source, err := os.ReadFile("main.go")
        if err != nil {
            fail(err.Error())
        }
        if strings.Contains(string(source), "github.com/MichaelKinsy/PiG") ||
            !strings.Contains(string(source), "example.org/lib/internal/thing") {
            fail("scratch imports did not exclude PiG and include vendored internals")
        }
    }
    if len(args) > 0 && os.Getenv("FAIL_GO_COMMAND") == args[0] {
        fmt.Fprintln(os.Stderr, "injected Go command failure")
        os.Exit(23)
    }
    if len(args) > 0 && args[0] == "list" {
        packages, exists := os.LookupEnv("CHECK_GO_PACKAGES")
        if !exists {
            packages = "example.org/core\nexample.org/core/tools"
        }
        fmt.Print(packages)
        if packages != "" && !strings.HasSuffix(packages, "\n") {
            fmt.Println()
        }
    }
    if len(args) == 2 && args[0] == "mod" && args[1] == "vendor" {
        write("vendor/example.org/lib/lib.go", "package lib\n")
        write("vendor/example.org/lib/LICENSE", "notices\n")
        write("vendor/github.com/example/lib/lib.go", "package lib\n")
        write("vendor/modules.txt", "scratch manifest\n")
    }
}
'''


class PowerShellToolingTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        candidates = [
            os.environ.get("PWSH"),
            shutil.which("pwsh"),
            shutil.which("powershell"),
            "/tmp/ompig-pwsh/bin/pwsh",
        ]
        cls.powershell = next((candidate for candidate in candidates if candidate and pathlib.Path(candidate).is_file()), None)
        go = shutil.which("go")
        if cls.powershell is None or go is None:
            raise unittest.SkipTest("PowerShell and Go are required for native tooling tests")
        cls.toolchain_temp = tempfile.TemporaryDirectory(prefix="ompig fake go ")
        cls.addClassCleanup(cls.toolchain_temp.cleanup)
        temp = pathlib.Path(cls.toolchain_temp.name)
        source = temp / "fake-go.go"
        source.write_text(FAKE_GO, encoding="utf-8")
        cls.fake_go = temp / ("go.exe" if os.name == "nt" else "go")
        env = dict(os.environ, GO111MODULE="off", GOTOOLCHAIN="local", GOFLAGS="")
        built = subprocess.run(
            [go, "build", "-o", str(cls.fake_go), str(source)],
            cwd=temp, env=env, text=True, capture_output=True, timeout=120,
        )
        if built.returncode:
            raise RuntimeError(f"could not build the fake native Go command: {built.stderr}")

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="ompig powershell tooling ")
        self.addCleanup(self.temporary.cleanup)
        self.base = pathlib.Path(self.temporary.name).resolve()
        self.root = self.base / "checkout with spaces"
        self.root.mkdir()
        self.scripts = self.root / "scripts"
        self.scripts.mkdir()
        self.bootstrap = self.scripts / "bootstrap-vendor.ps1"
        self.check_go = self.scripts / "check-go.ps1"
        shutil.copyfile(ROOT / "scripts/bootstrap-vendor.ps1", self.bootstrap)
        shutil.copyfile(ROOT / "scripts/check-go.ps1", self.check_go)

        self.goroot = self.root / "fake toolchain with spaces"
        self.bin = self.goroot / "bin"
        self.bin.mkdir(parents=True)
        self.compiler = self.bin / ("go.exe" if os.name == "nt" else "go")
        shutil.copy2(self.fake_go, self.compiler)
        self.cache = self.root / "check cache with spaces"
        self.log = self.root / "commands.jsonl"
        self.env = dict(os.environ)
        for key in list(self.env):
            if key.casefold() == "psmodulepath":
                del self.env[key]
        self.env.update({
            "CHECK_GO_LOG": str(self.log),
            "CHECKOUT": str(self.root),
            "FAKE_GOROOT": str(self.goroot),
            "GO": str(self.compiler),
            "OMPIG_CHECK_CACHE": str(self.cache),
        })
        self.env.pop("FAIL_GO_COMMAND", None)
        self.env.pop("CHECK_GO_PACKAGES", None)

    def run_ps_file(self, script, *arguments, env=None, cwd=None):
        return subprocess.run(
            [self.powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
             "-File", str(script), *arguments],
            cwd=cwd or self.base, env=env or self.env, text=True, capture_output=True, timeout=60,
        )

    def run_ps_command(self, command, env=None, cwd=None):
        return subprocess.run(
            [self.powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command],
            cwd=cwd or self.base, env=env or self.env, text=True, capture_output=True, timeout=60,
        )

    def run_functions(self, script, body, **env):
        return self.run_ps_command(LOAD_FUNCTIONS + body, env=dict(self.env, FUNCTIONS_SCRIPT=str(script), **env))

    def calls(self):
        return [json.loads(line) for line in self.log.read_text(encoding="utf-8").splitlines()]

    def prepare_bootstrap(self):
        (self.root / "go.mod").write_text("module fixture\n\ngo 1.27.0\n", encoding="utf-8")
        (self.root / "go.sum").write_text("locked checksums\n", encoding="utf-8")
        pig = self.root / "vendor/github.com/MichaelKinsy/PiG/patched.go"
        pig.parent.mkdir(parents=True)
        pig.write_text("personal patches\n", encoding="utf-8")
        self.manifest = self.root / "vendor/modules.txt"
        self.manifest.write_text(
            "# github.com/MichaelKinsy/PiG v0.7.0\n"
            "github.com/MichaelKinsy/PiG/coding\n"
            "# example.org/lib v1.0.0\nexample.org/lib/internal/thing\n"
            "# github.com/example/lib v1.0.0\ngithub.com/example/lib\n",
            encoding="utf-8",
        )
        return pig

    def test_installer_validates_windows_names_and_deduplicates_path(self):
        command = r'''
foreach ($name in @('../bad', 'ompig-slim/trailing.', 'ompig-slim/trailing ', 'ompig-slim/NUL.txt', 'ompig-slim/stream:alias')) {
    if (Test-SafeArchiveName $name) { throw "Unsafe archive name accepted $name" }
}
if (Test-SafeArchiveName ('ompig-slim/COM' + [char]0x00B9 + '.txt')) { throw 'Reserved device accepted' }
if (-not (Test-SafeArchiveName 'ompig-slim/ompig-slim.exe')) { throw 'Executable rejected' }
$path = 'C:\one;C:\two'
if ((Add-PathEntry $path 'c:\ONE') -cne $path) { throw 'Case alias duplicated in PATH' }
if ((Add-PathEntry $path 'C:\three') -cne 'C:\one;C:\two;C:\three') { throw 'PATH entries were overwritten' }
$root = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($root)
try {
    $target = Join-Path $root 'target'
    $staged = Join-Path $root 'staged'
    [IO.File]::WriteAllText($target, 'old')
    [IO.File]::WriteAllText($staged, 'new')
    [IO.File]::Replace($staged, $target, [NullString]::Value)
    if ([IO.File]::ReadAllText($target) -ne 'new') { throw 'Replacement failed' }
    Write-InstallMarker $root 'first marker'
    Write-InstallMarker $root 'second marker'
    if ([IO.File]::ReadAllText((Join-Path $root 'ompig-slim-install')) -ne 'second marker') { throw 'Marker replacement failed' }
} finally {
    [IO.Directory]::Delete($root, $true)
}
'''
        result = self.run_functions(INSTALLER, command)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_loaded_functions_stop_on_errors_like_their_scripts(self):
        for body in ("Write-Error 'injected'\n'continued'\n", "Get-UndefinedFixtureFunction\n'continued'\n"):
            with self.subTest(body=body):
                result = self.run_functions(INSTALLER, body)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertNotIn("continued", result.stdout)

    def test_installer_retries_transient_sharing_errors_and_fails_fast_other_io(self):
        command = r'''
$root = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($root)
try {
    foreach ($hresult in @(0x80070020, 0x80070021)) {
        $source = Join-Path $root ("staged-$hresult.exe")
        $target = Join-Path $root ("installed-$hresult.exe")
        [IO.File]::WriteAllText($source, 'executable')
        $script:attempts = 0
        Invoke-InstallFileMutation {
            $script:attempts++
            if ($script:attempts -eq 1) {
                throw [IO.IOException]::new('transient sharing violation', $hresult)
            }
            [IO.File]::Move($source, $target)
        }
        if ($script:attempts -ne 2) { throw "Sharing violation attempts: $script:attempts" }
        if (-not [IO.File]::Exists($target) -or [IO.File]::ReadAllText($target) -ne 'executable') {
            throw 'The file move did not complete after the transient sharing violation.'
        }
    }

    $script:attempts = 0
    $failure = $null
    try {
        Invoke-InstallFileMutation {
            $script:attempts++
            throw [IO.IOException]::new('permanent I/O failure', 0x80070005)
        }
    } catch {
        $failure = $_.Exception
    }
    if ($null -eq $failure -or $script:attempts -ne 1) {
        throw "A non-sharing I/O error was retried or hidden: $script:attempts attempts"
    }

    $script:attempts = 0
    $failure = $null
    try {
        Invoke-InstallFileMutation {
            $script:attempts++
            throw [InvalidOperationException]::new('non-I/O failure')
        }
    } catch {
        $failure = $_.Exception
    }
    if ($null -eq $failure -or $script:attempts -ne 1) {
        throw "A non-I/O error was retried or hidden: $script:attempts attempts"
    }
} finally {
    [IO.Directory]::Delete($root, $true)
}
'''
        result = self.run_functions(INSTALLER, command)
        self.assertEqual(result.returncode, 0, result.stderr)

        if os.name == "nt":
            import ctypes
            import threading
            import time
            from ctypes import wintypes

            native_root = self.base / "native sharing lock"
            native_root.mkdir()
            source = native_root / "staged.exe"
            target = native_root / "installed.exe"
            source.write_text("executable", encoding="utf-8")
            kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
            kernel32.CreateFileW.argtypes = [
                wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD, wintypes.LPVOID,
                wintypes.DWORD, wintypes.DWORD, wintypes.HANDLE,
            ]
            kernel32.CreateFileW.restype = wintypes.HANDLE
            kernel32.CloseHandle.argtypes = [wintypes.HANDLE]
            kernel32.CloseHandle.restype = wintypes.BOOL
            handle = kernel32.CreateFileW(
                str(source), 0x80000000, 0, None, 3, 0x80, None,
            )
            self.assertNotEqual(handle, wintypes.HANDLE(-1).value, ctypes.FormatError(ctypes.get_last_error()))
            failure_marker = native_root / "sharing-failure.txt"

            def release_after_failure():
                deadline = time.monotonic() + 15
                while time.monotonic() < deadline and not failure_marker.exists():
                    time.sleep(0.01)
                kernel32.CloseHandle(handle)

            release = threading.Thread(target=release_after_failure)
            release.start()
            try:
                command = r'''
Invoke-InstallFileMutation {
    try { [IO.File]::Move($env:MOVE_SOURCE, $env:MOVE_TARGET) }
    catch [IO.IOException] {
        [IO.File]::WriteAllText($env:MOVE_FAILURE, ($_.Exception.HResult -band 0xFFFF))
        throw
    }
}
'''
                result = self.run_functions(INSTALLER, command, MOVE_SOURCE=str(source),
                                            MOVE_TARGET=str(target), MOVE_FAILURE=str(failure_marker))
            finally:
                release.join()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(target.read_text(encoding="utf-8"), "executable")
            self.assertIn(int(failure_marker.read_text(encoding="utf-8-sig")), (32, 33))

    def test_bootstrap_preserves_pig_and_manifest_and_skips_cached_restore(self):
        pig = self.prepare_bootstrap()
        manifest = self.manifest.read_bytes()
        stale = self.root / "vendor/example.org/lib/obsolete.go"
        stale.parent.mkdir(parents=True)
        stale.write_text("obsolete\n", encoding="utf-8")
        env = dict(self.env, GOFLAGS="-mod=vendor")
        wrapper = (
            "$env:GOFLAGS = 'restore this'; $before = (Get-Location).Path; "
            "& $env:BOOTSTRAP_SCRIPT; "
            "if ($env:GOFLAGS -ne 'restore this' -or (Get-Location).Path -ne $before) { exit 19 }"
        )
        env["BOOTSTRAP_SCRIPT"] = str(self.bootstrap)
        result = self.run_ps_command(wrapper, env=env)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(stale.exists())
        self.assertEqual(pig.read_text(encoding="utf-8"), "personal patches\n")
        self.assertEqual(self.manifest.read_bytes(), manifest)
        self.assertEqual((self.root / "vendor/example.org/lib/LICENSE").read_text(encoding="utf-8"), "notices\n")
        self.assertTrue((self.root / "vendor/github.com/example/lib/lib.go").exists())
        marker = self.root / "vendor/.bootstrap-modules.txt"
        self.assertEqual(marker.read_bytes(), manifest)
        self.compiler.unlink()
        result = self.run_ps_file(self.bootstrap)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_bootstrap_failure_is_retryable_and_does_not_touch_patched_sources(self):
        pig = self.prepare_bootstrap()
        manifest = self.manifest.read_bytes()
        env = dict(self.env, FAIL_GO_COMMAND="mod")
        result = self.run_ps_file(self.bootstrap, env=env)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exit code 23", result.stderr)
        self.assertFalse((self.root / "vendor/.bootstrap-modules.txt").exists())
        self.assertEqual(pig.read_text(encoding="utf-8"), "personal patches\n")
        self.assertEqual(self.manifest.read_bytes(), manifest)

    def test_bootstrap_refuses_to_recurse_through_vendor_reparse_points(self):
        self.prepare_bootstrap()
        external = self.base / "outside vendor data"
        external.mkdir()
        (external / "keep.txt").write_text("user data\n", encoding="utf-8")
        link = self.root / "vendor/example.org/lib"
        link.parent.mkdir(parents=True)
        if os.name == "nt":
            env = dict(self.env, LINK_PATH=str(link), LINK_TARGET=str(external))
            result = self.run_ps_command(
                "New-Item -ItemType Junction -Path $env:LINK_PATH -Target $env:LINK_TARGET | Out-Null",
                env=env,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            link.symlink_to(external, target_is_directory=True)
        result = self.run_ps_file(self.bootstrap)
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(link.exists())
        self.assertEqual((external / "keep.txt").read_text(encoding="utf-8"), "user data\n")
        self.assertFalse((self.root / "vendor/.bootstrap-modules.txt").exists())

    def prepare_check_fixture(self):
        (self.root / "vendor/github.com/MichaelKinsy/PiG/core").mkdir(parents=True)
        (self.root / "vendor/example.org/core").mkdir(parents=True)
        (self.root / "vendor/github.com/MichaelKinsy/PiG/core/core.go").write_text("package core\n", encoding="utf-8")
        (self.root / "vendor/example.org/core/core.go").write_text("package core\n", encoding="utf-8")

    def test_modes_collect_canonical_packages_and_isolate_home(self):
        self.prepare_check_fixture()
        expected = {
            "check": [
                ["test", "-mod=vendor", "./..."],
                ["list", "-mod=vendor", "./vendor/github.com/MichaelKinsy/PiG/..."],
                ["test", "example.org/core", "example.org/core/tools"],
                ["vet", "-mod=vendor", "./...", "./vendor/github.com/MichaelKinsy/PiG/..."],
            ],
            "test": [
                ["test", "-mod=vendor", "./..."],
                ["list", "-mod=vendor", "./vendor/github.com/MichaelKinsy/PiG/..."],
                ["test", "example.org/core", "example.org/core/tools"],
            ],
            "race": [
                ["test", "-race", "-mod=vendor", "./..."],
                ["list", "-mod=vendor", "./vendor/github.com/MichaelKinsy/PiG/..."],
                ["test", "-race", "example.org/core", "example.org/core/tools"],
            ],
            "vet": [["vet", "-mod=vendor", "./...", "./vendor/github.com/MichaelKinsy/PiG/..."]],
        }
        for mode, arguments in expected.items():
            with self.subTest(mode=mode):
                self.log.unlink(missing_ok=True)
                result = self.run_ps_file(self.check_go, "-Mode", mode)
                self.assertEqual(result.returncode, 0, result.stderr)
                calls = self.calls()
                self.assertEqual([call["args"] for call in calls], arguments)
                self.assertEqual(calls[0]["gomodule"], "on")
                self.assertEqual(calls[0]["goflags"], "")
                self.assertEqual(calls[0]["gotoolchain"], "local")
                self.assertEqual(calls[0]["home"], str(self.cache / "profile/home"))
                self.assertEqual(calls[0]["userprofile"], str(self.cache / "profile/home"))
                if mode in ("check", "test", "race"):
                    gopath_call = calls[2]
                    self.assertEqual(gopath_call["gomodule"], "off")
                    self.assertEqual(gopath_call["gopath"], str(self.cache / "gopath"))
                    self.assertEqual(gopath_call["cwd"], str(self.cache / "gopath/src"))
                for call in calls:
                    self.assertEqual(call["home"], str(self.cache / "profile/home"))
                    self.assertEqual(call["userprofile"], str(self.cache / "profile/home"))

    def test_default_mode_is_check_and_caller_environment_is_restored(self):
        self.prepare_check_fixture()
        env = dict(self.env, HOME="original home", USERPROFILE="original profile", GOFLAGS="original flags")
        env["CHECK_SCRIPT"] = str(self.check_go)
        wrapper = (
            "$before = (Get-Location).Path; & $env:CHECK_SCRIPT; "
            "if ($env:HOME -ne 'original home' -or $env:USERPROFILE -ne 'original profile' "
            "-or $env:GOFLAGS -ne 'original flags' -or (Get-Location).Path -ne $before) { exit 19 }"
        )
        result = self.run_ps_command(wrapper, env=env)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call["args"][0] for call in self.calls()], ["test", "list", "test", "vet"])

    def test_default_cache_uses_an_isolated_home_and_restores_environment(self):
        self.prepare_check_fixture()
        home = self.base / "fallback home with spaces"
        env = dict(self.env, HOME=str(home), USERPROFILE=str(home))
        env.pop("OMPIG_CHECK_CACHE", None)
        env.pop("XDG_CACHE_HOME", None)
        env["CHECK_SCRIPT"] = str(self.check_go)
        wrapper = (
            "$before = (Get-Location).Path; & $env:CHECK_SCRIPT -Mode vet; "
            "if ($env:HOME -ne $env:EXPECTED_HOME -or $env:USERPROFILE -ne $env:EXPECTED_HOME "
            "-or (Get-Location).Path -ne $before) { exit 19 }"
        )
        env["EXPECTED_HOME"] = str(home)
        result = self.run_ps_command(wrapper, env=env)
        self.assertEqual(result.returncode, 0, result.stderr)
        cache = home / ".cache/ompig-check"
        call = self.calls()[0]
        self.assertEqual(call["args"][0], "vet")
        self.assertEqual(call["home"], str(cache / "profile/home"))
        self.assertEqual(call["userprofile"], str(cache / "profile/home"))

    def test_gopath_links_replace_junctions_without_deleting_their_targets(self):
        self.prepare_check_fixture()
        external = self.base / "outside GOPATH data"
        external.mkdir()
        (external / "keep.txt").write_text("user data\n", encoding="utf-8")
        link = self.cache / "gopath/src/github.com"
        link.parent.mkdir(parents=True)
        if os.name == "nt":
            env = dict(self.env, LINK_PATH=str(link), LINK_TARGET=str(external))
            result = self.run_ps_command(
                "New-Item -ItemType Junction -Path $env:LINK_PATH -Target $env:LINK_TARGET | Out-Null",
                env=env,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            link.symlink_to(external, target_is_directory=True)
        result = self.run_ps_file(self.check_go, "-Mode", "vet")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((external / "keep.txt").read_text(encoding="utf-8"), "user data\n")
        self.assertEqual(link.resolve(), (self.root / "vendor/github.com").resolve())

    def test_empty_package_list_and_native_command_failure_are_errors(self):
        self.prepare_check_fixture()
        env = dict(self.env, CHECK_GO_PACKAGES="")
        result = self.run_ps_file(self.check_go, "-Mode", "test", env=env)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("No PiG packages found", result.stderr)
        self.assertEqual([call["args"][0] for call in self.calls()], ["test", "list"])

        self.log.unlink()
        env = dict(self.env, FAIL_GO_COMMAND="test")
        result = self.run_ps_file(self.check_go, "-Mode", "check", env=env)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exit code 23", result.stderr)
        self.assertEqual([call["args"][0] for call in self.calls()], ["test"])


if __name__ == "__main__":
    unittest.main()
