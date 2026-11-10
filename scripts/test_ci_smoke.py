#!/usr/bin/env python3
"""Guard CI smoke sharding without launching native executables."""
import importlib.util
import pathlib
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("ci_smoke", pathlib.Path(__file__).with_name("ci-smoke.py"))
ci_smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci_smoke)


class SmokeShardTests(unittest.TestCase):
    """Keep every canonical command in exactly one shard, including new tests."""

    def test_shards_cover_recipe_once(self):
        """Round-robin slicing preserves each shard's order and full coverage."""
        recipe = "\n".join(f"python3 scripts/test-{n}.py 'binary with spaces'" for n in range(11))
        commands = ci_smoke.commands(recipe)
        shards = [commands[n::2] for n in (0, 1)]
        self.assertEqual([len(shard) for shard in shards], [6, 5])
        self.assertEqual(len({tuple(command) for shard in shards for command in shard}), 11)
        self.assertEqual(commands[0][-1], "binary with spaces")

    def test_main_runs_only_selected_shard(self):
        """The CLI must execute the shard, not just compute a disjoint split."""
        recipe = "python3 first.py\npython3 second.py\npython3 third.py"
        with mock.patch("sys.argv", ["ci-smoke.py", "1"]), \
                mock.patch.object(ci_smoke.subprocess, "check_output", return_value=recipe), \
                mock.patch.object(ci_smoke.subprocess, "run") as run:
            ci_smoke.main()
        run.assert_called_once_with(["python3", "second.py"], check=True)

    def test_main_propagates_scenario_failure(self):
        """A failed scenario must fail the job before later commands execute."""
        recipe = "python3 first.py\npython3 second.py\npython3 third.py"
        error = ci_smoke.subprocess.CalledProcessError(1, ["python3", "first.py"])
        with mock.patch("sys.argv", ["ci-smoke.py", "0"]), \
                mock.patch.object(ci_smoke.subprocess, "check_output", return_value=recipe), \
                mock.patch.object(ci_smoke.subprocess, "run", side_effect=error) as run:
            with self.assertRaises(ci_smoke.subprocess.CalledProcessError):
                ci_smoke.main()
        run.assert_called_once_with(["python3", "first.py"], check=True)

    def test_rejects_empty_or_non_python_recipe(self):
        """Fail closed if make introduces prerequisite builds or shell setup."""
        for recipe in ("", "go build .", "python3 test.py\nmake build"):
            with self.subTest(recipe=recipe), self.assertRaises(ValueError):
                ci_smoke.commands(recipe)


if __name__ == "__main__":
    unittest.main()
