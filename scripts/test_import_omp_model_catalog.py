"""Test OMP compact-catalog decoding without npm, network, or a model provider."""

import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location(
    "import_omp_catalog", Path(__file__).with_name("import-omp-model-catalog.py")
)
importer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(importer)


class CatalogImportTests(unittest.TestCase):
    """Keep compact references, false flags, and compat removal semantics intact."""

    def test_shared_constants_and_compat_delta(self):
        """A provider row inherits constants but deletes only the authored compat key."""
        source = {
            "version": 1,
            "keys": ["provider", "api", "reasoning", "old", "enabled", "name"],
            "booleanShapes": [[], [2], [4]],
            "nodes": [
                [0, [0, "fixture", 1, "openai-completions"], 0, []],
                [0, [3, "remove"], 2, [1]],
                [0, [5, "Fixture"], 1, [0]],
                [0, [], 2, [0]],
            ],
            "compat": [["openai-completions", [1]]],
            "providers": [["fixture", [0], [["model", [2], [[3], [3]]]]]],
        }
        row = importer.decode_catalog(source)["fixture"][0]
        self.assertEqual(row["provider"], "fixture")
        self.assertFalse(row["reasoning"])
        self.assertEqual(row["compat"], {"enabled": False})
        self.assertEqual(row["name"], "Fixture")

    def test_unknown_version_is_refused(self):
        """A changed upstream format must fail, never generate an empty roster."""
        with self.assertRaises(ValueError):
            importer.decode_catalog({"version": 2})


if __name__ == "__main__":
    unittest.main()
