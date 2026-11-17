#!/usr/bin/env python3
"""Build inert fixture packages, never execute their synthetic ELF payloads."""
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from test_render_packages import write_archive, VERSION

root = Path(sys.argv[1]).resolve()
root.mkdir()  # fresh output only
archive, digest = write_archive(root)
subprocess.run(["bash", str(Path(__file__).resolve().parents[1] / "package-verified.sh"),
                VERSION, str(archive), digest, str(root / "packages")], check=True)
