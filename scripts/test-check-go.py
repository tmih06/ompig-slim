#!/usr/bin/env python3
"""Check the Go runner's command contract with the platform's actual Bash."""
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest


class CheckGoTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="ompig shell tests ")
        self.addCleanup(self.temporary.cleanup)
        self.root = pathlib.Path(self.temporary.name)
        (self.root / "scripts").mkdir()
        (self.root / "vendor" / "example.org").mkdir(parents=True)
        shutil.copyfile(pathlib.Path(__file__).with_name("check-go.sh"), self.root / "scripts" / "check-go.sh")
        self.compiler = self.root / "toolchain" / "bin" / "go"
        self.compiler.parent.mkdir(parents=True)
        self.compiler.write_text(
            "#!" + sys.executable + "\n"
            "import json, os, pathlib, sys\n"
            "if sys.argv[1:] == ['env', 'GOROOT']:\n"
            "    print(pathlib.Path(__file__).resolve().parent.parent)\n"
            "else:\n"
            "    with open(os.environ['CHECK_GO_LOG'], 'a') as log:\n"
            "        log.write(json.dumps({'args':sys.argv[1:], 'cwd':os.getcwd(), 'module':os.environ.get('GO111MODULE')})+'\\n')\n"
            "    if sys.argv[1] == 'list':\n"
            "        print(os.environ.get('CHECK_GO_PACKAGES', 'example.org/core\\nexample.org/core/tools'))\n"
        )
        self.compiler.chmod(0o700)
        self.log = self.root / "commands.jsonl"
        self.env = dict(os.environ, GO=str(self.compiler), OMPIG_CHECK_CACHE=str(self.root / "cache"), CHECK_GO_LOG=str(self.log))

    def run_mode(self, mode):
        return subprocess.run(
            [os.environ.get("BASH_FOR_TEST", "/bin/bash"), "scripts/check-go.sh", mode],
            cwd=self.root, env=self.env, text=True, capture_output=True, timeout=15,
        )

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def test_check_runs_module_and_vendored_tests_then_vet(self):
        result = self.run_mode("check")
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        self.assertEqual([call["args"] for call in calls], [
            ["test", "-mod=vendor", "./..."],
            ["list", "-mod=vendor", "./vendor/github.com/MichaelKinsy/PiG/..."],
            ["test", "example.org/core", "example.org/core/tools"],
            ["vet", "-mod=vendor", "./...", "./vendor/github.com/MichaelKinsy/PiG/..."],
        ])
        self.assertEqual(calls[2]["module"], "off")
        self.assertEqual(calls[2]["cwd"], str(self.root / "cache" / "gopath" / "src"))

    def test_race_applies_to_both_test_commands(self):
        result = self.run_mode("race")
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        self.assertEqual(calls[0]["args"][0], "test")
        self.assertCountEqual(calls[0]["args"][1:], ["-race", "-mod=vendor", "./..."])
        self.assertEqual(calls[2]["args"], ["test", "-race", "example.org/core", "example.org/core/tools"])
        self.assertEqual(len(calls), 3)

    def test_vet_does_not_run_tests(self):
        result = self.run_mode("vet")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call["args"][0] for call in self.calls()], ["vet"])

    def test_empty_package_list_fails_before_unscoped_test(self):
        self.env["CHECK_GO_PACKAGES"] = ""
        result = self.run_mode("test")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("No PiG packages found", result.stderr)
        self.assertEqual(len(self.calls()), 2)


if __name__ == "__main__":
    unittest.main()
