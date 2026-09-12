#!/usr/bin/env python3
"""Check the committed Pages artifact without treating render time as content."""
import pathlib
import re
import subprocess

root = pathlib.Path(__file__).resolve().parents[1]
rendered = subprocess.check_output(["go", "run", ".", "-export"], cwd=root, text=True)
published = (root / "docs/index.html").read_text()
stamp = r"\d{4}-\d{2}-\d{2}T[0-9:]*Z"
if re.sub(stamp, "TIMESTAMP", rendered) != re.sub(stamp, "TIMESTAMP", published):
    raise SystemExit("Published page is stale. Run make page and commit docs/index.html with the source change.")
print("Published page matches the source.")
