#!/usr/bin/env python3
"""Guard CI smoke sharding without launching native executables."""
import importlib.util
import pathlib
import time
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
        """A failed scenario fails the job; other shard commands still launch."""
        recipe = "python3 first.py\npython3 second.py\npython3 third.py"
        error = ci_smoke.subprocess.CalledProcessError(1, ["python3", "first.py"])
        with mock.patch("sys.argv", ["ci-smoke.py", "0"]), \
                mock.patch.object(ci_smoke.subprocess, "check_output", return_value=recipe), \
                mock.patch.object(ci_smoke.subprocess, "run", side_effect=error) as run:
            with self.assertRaises(ci_smoke.subprocess.CalledProcessError) as caught:
                ci_smoke.main()
        self.assertEqual(caught.exception.cmd, ["python3", "first.py"])
        # The shard's commands are submitted together; both ran (and failed).
        self.assertEqual(run.call_count, 2)

    def test_main_runs_shard_in_parallel(self):
        """Independent scenarios overlap: two slow commands beat their serial sum."""
        recipe = "python3 one.py\npython3 two.py\npython3 three.py\npython3 four.py"
        def nap(command, check):
            time.sleep(0.4)
            return None
        with mock.patch("sys.argv", ["ci-smoke.py", "0"]), \
                mock.patch.object(ci_smoke.subprocess, "check_output", return_value=recipe), \
                mock.patch.object(ci_smoke.subprocess, "run", side_effect=nap) as run:
            start = time.monotonic()
            ci_smoke.main()
            elapsed = time.monotonic() - start
        # Serial would take 0.8s for shard 0's two commands; parallel ~0.4s.
        self.assertLess(elapsed, 0.7)
        self.assertEqual(run.call_count, 2)

    def test_rejects_empty_or_non_python_recipe(self):
        """Fail closed if make introduces prerequisite builds or shell setup."""
        for recipe in ("", "go build .", "python3 test.py\nmake build"):
            with self.subTest(recipe=recipe), self.assertRaises(ValueError):
                ci_smoke.commands(recipe)


if __name__ == "__main__":
    unittest.main()
