"""Inert archive fixtures only: no build, runtime or network."""
import io
import json
import tarfile
import tempfile
import unittest
from pathlib import Path

import runtime_inventory as inv


class InventoryTest(unittest.TestCase):
    def test_deterministic_safe_sorted_identity(self):
        files = {"bin/z": (b"signed", 0o755), "LICENSE": (b"text", 0o644)}
        data = inv.encode("1.2.3", "darwin-arm64", files)
        self.assertEqual(data, inv.encode("1.2.3", "darwin-arm64", dict(reversed(list(files.items())))))
        doc = json.loads(data)
        self.assertEqual([m["path"] for m in doc["members"]], ["LICENSE", "bin/z"])
        self.assertEqual([m["executable"] for m in doc["members"]], [False, True])
        for path in ("/a", "../a", "a/../b", "a//b", "./a", "a\\b", "C:a", "a\n"):
            with self.subTest(path=path), self.assertRaises(ValueError):
                inv.encode("1.2.3", "darwin-arm64", {path: (b"", 0o644)})

    def test_archive_refusals(self):
        for platform in ("darwin-arm64", "linux-x64", "windows-x64"):
            files = {"bin/agent": (b"final signed bytes", 0o755)}
            inventory = inv.encode("1.2.3", platform, files)
            cases = {
                "valid": [("bin/agent", b"final signed bytes", 0o755), (inv.NAME, inventory, 0o644)],
                "changed": [("bin/agent", b"pre-sign bytes", 0o755), (inv.NAME, inventory, 0o644)],
                "mode": [("bin/agent", b"final signed bytes", 0o644), (inv.NAME, inventory, 0o644)],
                "missing": [(inv.NAME, inventory, 0o644)],
                "no inventory": [("bin/agent", b"final signed bytes", 0o755)],
                "identity": [("bin/agent", b"final signed bytes", 0o755), (inv.NAME, inventory.replace(b'1.2.3', b'1.2.4'), 0o644)],
            }
            cases["duplicate"] = cases["valid"] + cases["valid"][:1]
            cases["extra"] = cases["valid"] + [("extra", b"extra", 0o644)]
            cases["unsafe"] = cases["valid"] + [("../escape", b"bad", 0o644)]
            for label, entries in cases.items():
                with self.subTest(platform=platform, case=label), tempfile.TemporaryDirectory() as tmp:
                    archive = Path(tmp) / "fixture.tar.gz"
                    with tarfile.open(archive, "w:gz") as tf:
                        for name, data, mode in entries:
                            item = tarfile.TarInfo("top/" + name)
                            item.mode, item.size = mode, len(data)
                            tf.addfile(item, io.BytesIO(data))
                    if label == "valid":
                        inv.validate_archive(archive, "top", "1.2.3", platform)
                    else:
                        with self.assertRaises(ValueError):
                            inv.validate_archive(archive, "top", "1.2.3", platform)

    def test_producer_rejects_symlinks_and_overwrite(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "data").write_bytes(b"payload")
            inv.produce(root, "1.2.3", "linux-x64", {"data": 0o644})
            with self.assertRaises(FileExistsError):
                inv.produce(root, "1.2.3", "linux-x64", {"data": 0o644})
            (root / "alias").symlink_to("data")
            with self.assertRaises(ValueError):
                inv.produce(root, "1.2.3", "linux-x64", {"alias": 0o644})


if __name__ == "__main__":
    unittest.main()
