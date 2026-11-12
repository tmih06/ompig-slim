"""Check beta numbering and CI's release/channel boundary without GitHub access."""
import importlib.util
import pathlib
import unittest

SCRIPTS = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("release_version", SCRIPTS / "release-version.py")
release_version = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release_version)


class ReleaseVersionTests(unittest.TestCase):
    """Pin version allocation independently of publication order."""

    def test_next_patch_and_numeric_sequence(self):
        self.assertEqual(release_version.beta_tag("v0.1.5", "9"), "v0.1.6-beta.9")
        self.assertEqual(release_version.beta_tag("v0.1.5", "10"), "v0.1.6-beta.10")
        self.assertEqual(release_version.beta_tag("v1.0.0", "101"), "v1.0.1-beta.101")

    def test_rejects_nonstable_or_invalid_numbers(self):
        for stable, number in [("v1.0.0-beta.1", "2"), ("latest", "2"),
                               ("v1.0.0", "0"), ("v1.0.0", "01"), ("v1.0.0", "x")]:
            with self.subTest(stable=stable, number=number), self.assertRaises(ValueError):
                release_version.beta_tag(stable, number)

    def test_ci_publishes_betas_without_moving_stable(self):
        # Workflow policy is text: actionlint separately validates Actions syntax.
        workflow = (SCRIPTS.parent / ".github/workflows/ci.yml").read_text()
        self.assertIn("  publish-beta:", workflow)
        self.assertIn('--prerelease --latest=false', workflow)
        self.assertIn('VERSION: ${{ needs.version.outputs.version }}', workflow)
        self.assertIn('github.run_number', workflow)
        push = workflow.split("  push:\n", 1)[1].split("  pull_request:\n", 1)[0]
        self.assertNotIn("paths-ignore", push, "documentation-only merges also need betas")


if __name__ == "__main__":
    unittest.main()
