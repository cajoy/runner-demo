#!/usr/bin/env python3
"""Update the reviewed receipt recipe, or check its source-file bindings."""
import argparse
import hashlib
import json
import pathlib
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]
CONTRACT = ROOT / ".runner/receipt-contract.json"
BINDINGS = ROOT / ".runner/receipt-contract-inputs.json"


def digest(path):
    return "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()


def inputs():
    paths = sorted((ROOT / ".local-ci").glob("*.yaml"))
    paths += [ROOT / ".local-ci/toolchain.lock", ROOT / ".github/workflows/demo.yml"]
    return {str(p.relative_to(ROOT)): digest(p) for p in paths}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--runner", default="runner")
    args = parser.parse_args()
    if args.check:
        expected = json.loads(BINDINGS.read_text())
        if expected != {"inputs": inputs(), "contract": digest(CONTRACT)}:
            raise SystemExit("receipt contract is stale: run make contract with the pinned Runner, review and commit both contract files")
        print("Receipt contract matches its reviewed configuration and workflow.")
        return
    before = inputs()
    validation = json.loads(subprocess.check_output(
        [args.runner, "config", "validate", "--project", str(ROOT), "--json"], text=True))
    contract = json.loads(CONTRACT.read_text())
    recipe = validation.get("verification", {}).get(contract["service"], {})
    definitions = recipe.get("task_definitions", {})
    if set(definitions) != {"lint", "unit", "build"}:
        raise SystemExit("Runner must expose exactly lint, unit and build verification definitions; inspect config warnings and use Runner v0.8.34 or newer")
    contract["config_digest"] = recipe["config_digest"]
    contract["task_definitions"] = definitions
    contract["workflow_digest"] = digest(ROOT / contract["workflow_path"])
    if before != inputs():
        raise SystemExit("configuration changed during contract generation")
    CONTRACT.write_text(json.dumps(contract, indent=2) + "\n")
    BINDINGS.write_text(json.dumps({"inputs": before, "contract": digest(CONTRACT)}, indent=2) + "\n")
    print("Updated contract and source bindings. Review runtime, environment and policy before committing.")


if __name__ == "__main__":
    main()
