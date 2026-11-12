#!/usr/bin/env python3
"""Check dependency restoration without downloads or a real Go toolchain."""
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest


class BootstrapTests(unittest.TestCase):
    """Exercise the scratch-module and maintained-source boundaries."""

    def setUp(self):
        """Create a minimal checkout and a fake checksum-verifying Go command."""
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        (self.root / "scripts").mkdir()
        shutil.copyfile(pathlib.Path(__file__).with_name("bootstrap-vendor.sh"),
                        self.root / "scripts/bootstrap-vendor.sh")
        self.pig = self.root / "vendor/github.com/MichaelKinsy/PiG/patched.go"
        self.pig.parent.mkdir(parents=True)
        self.pig.write_text("personal patches\n")
        self.manifest = self.root / "vendor/modules.txt"
        self.manifest.write_text(
            "# github.com/MichaelKinsy/PiG v0.7.0\n"
            "github.com/MichaelKinsy/PiG/coding\n"
            "# example.org/lib v1.0.0\nexample.org/lib/internal/thing\n"
            "# github.com/example/lib v1.0.0\ngithub.com/example/lib\n")
        (self.root / "go.mod").write_text("module fixture\n")
        (self.root / "go.sum").write_text("locked checksums\n")
        self.go = self.root / "fake-go"
        self.go.write_text('''#!/usr/bin/env bash
set -eu
[[ "$PWD" != "$CHECKOUT" ]]
[[ "$*" == 'mod vendor' ]]
[[ -z "$GOFLAGS" ]]
cmp go.sum "$CHECKOUT/go.sum"
! grep -q MichaelKinsy main.go
grep -q 'example.org/lib/internal/thing' main.go
if [[ "${FAIL_DOWNLOAD:-}" == 1 ]]; then exit 1; fi
mkdir -p vendor/example.org/lib vendor/github.com/example/lib
printf restored > vendor/example.org/lib/lib.go
printf restored > vendor/github.com/example/lib/lib.go
printf notices > vendor/example.org/lib/LICENSE
printf scratch > vendor/modules.txt
''')
        self.go.chmod(0o755)

    def run_bootstrap(self, **env):
        """Run with controlled download behavior, capturing failure diagnostics."""
        return subprocess.run(
            ["bash", "scripts/bootstrap-vendor.sh"], cwd=self.root,
            env={**os.environ, "GO": str(self.go), "CHECKOUT": str(self.root),
                 "GOFLAGS": "-mod=vendor", **env}, capture_output=True, text=True)

    def test_restore_preserves_patches_manifest_and_notices(self):
        """Generated namespaces are replaced; maintained sources remain exact."""
        manifest = self.manifest.read_bytes()
        stale = self.root / "vendor/example.org/lib/obsolete.go"
        stale.parent.mkdir(parents=True)
        stale.write_text("obsolete")
        result = self.run_bootstrap()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(stale.exists())
        self.assertEqual(self.pig.read_text(), "personal patches\n")
        self.assertEqual(self.manifest.read_bytes(), manifest)
        self.assertEqual((stale.parent / "LICENSE").read_text(), "notices")
        self.assertTrue((self.root / "vendor/github.com/example/lib/lib.go").exists())
        # A complete restore needs no working downloader on subsequent calls.
        self.go.unlink()
        self.assertEqual(self.run_bootstrap().returncode, 0)

    def test_failure_is_retryable_and_manifest_change_restores_again(self):
        """No completion marker or destructive copy precedes a successful fetch."""
        marker = self.root / "vendor/.bootstrap-modules.txt"
        self.assertNotEqual(self.run_bootstrap(FAIL_DOWNLOAD="1").returncode, 0)
        self.assertFalse(marker.exists())
        self.assertEqual(self.pig.read_text(), "personal patches\n")
        self.assertEqual(self.run_bootstrap().returncode, 0)
        self.manifest.write_text(self.manifest.read_text() + "# new lock\n")
        self.assertNotEqual(self.run_bootstrap(FAIL_DOWNLOAD="1").returncode, 0)
        self.assertNotEqual(marker.read_bytes(), self.manifest.read_bytes())
        self.assertEqual(self.run_bootstrap().returncode, 0)
        self.assertEqual(marker.read_bytes(), self.manifest.read_bytes())


if __name__ == "__main__":
    unittest.main()
