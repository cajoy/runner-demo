# Runner demo

This repository demonstrates the one-product workflow model:

- one CLI, `runner`;
- one checked-in DAG;
- local progress and receipts;
- a controlled final-step failure; and
- exact-run resume that reuses successful earlier jobs.

The workflow names its jobs `prepare`, `test`, and `finish`. Runner does not
assign special meaning to those names.

## Get the pinned bundle

Runner is currently in a private GitHub repository, so downloading the release
requires a GitHub identity with read access. Workflow execution itself does not
contact GitHub.

```bash
mkdir -p "$PWD/.runner-bundle"
gh release download v0.6.0 \
  --repo cajoy/runner \
  --dir "$PWD/.runner-bundle"
cd .runner-bundle && shasum -a 256 -c checksums.txt && cd ..
chmod 755 .runner-bundle/runner_v0.6.0_darwin_arm64
```

On Apple silicon, install the committed lock with the downloaded Runner:

```bash
RUNNER_BIN="$PWD/.runner-bundle/runner_v0.6.0_darwin_arm64"
"$RUNNER_BIN" toolchain install \
  --project "$PWD" \
  --source-dir "$PWD/.runner-bundle" \
  --json
```

## Fail, then resume

Configure the final action to fail and run the workflow:

```bash
./scripts/set-result.sh fail
"$RUNNER_BIN" run --project . --json-stream hello:demo
```

The terminal event contains the run ID and points to
`.local-ci/state/runs/<run-id>/receipt.json`. The expected exit status is `1`.

Now change only the simulated external condition and resume the exact failed
run:

```bash
./scripts/set-result.sh success
"$RUNNER_BIN" run --project . \
  --resume <failed-run-id> \
  --json-stream \
  hello:demo
```

The child receipt records:

```json
{
  "resume": {
    "parent_run_id": "<failed-run-id>",
    "reused_jobs": ["prepare", "test"],
    "executed_jobs": ["finish"]
  }
}
```

Inspect either run without executing a job or contacting a provider:

```bash
"$RUNNER_BIN" runs observe --project . --run <run-id> --json
```

The state toggle lives under `.local-ci/state/`, which Runner deliberately
excludes from source capture. It represents an external condition becoming
healthy; changing tracked source, workflow configuration, parameters, Runner,
or plugin identities would make exact resume fail closed instead of reusing
jobs.

## Remote CI

`.github/workflows/demo.yml` runs the same fail-then-resume workflow with
Docker and uploads the receipt directory. Because the Runner repository is
private, configure a read-only `RUNNER_REPOSITORY_TOKEN` Actions secret in the
demo repository. That token is used only to download immutable release assets;
it is not passed to Runner jobs, plugins, logs, locks, or receipts.
