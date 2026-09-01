# Runner demo

One repository, one CLI, one checked-in DAG. This demo proves the behavior
Runner exists for: **work verified on a laptop is not repeated in CI.** A local
run signs a portable receipt bound to an exact commit; GitHub Actions verifies
that receipt against a protected policy, reuses the tasks it covers, and
executes only what is left.

```
                    lint ─┐
local laptop        unit ─┼─→ deploy-github        GitHub Actions
(placement: any)   build ─┘   (placement: ci)
```

`api:preflight` runs `lint`, `unit`, and `build` in parallel on this machine.
`api:github` selects the dependent `deploy-github` task, which only GitHub may
execute. The workflow lives in [`.local-ci/api.yaml`](.local-ci/api.yaml).

## Setup

Requires Docker or Apple Container, `jq`, `openssl`, and `gh` authenticated
against the private `cajoy/runner` repository.

```bash
./scripts/bootstrap.sh
```

Bootstrap downloads the Runner release pinned by
[`.local-ci/toolchain.lock`](.local-ci/toolchain.lock), installs it into this
project, creates the `local-alex` and `github` Ed25519 signers in your login
Keychain, and renders [`.runner/receipt-policy.yaml`](.runner/receipt-policy.yaml)
from their public keys and this repository's identity.

Publish the CI signer's private half once, from a trusted shell:

```bash
security find-generic-password -a github -s cajoy.runner.receipt-signing -w |
  gh secret set RUNNER_RECEIPT_SIGNING_KEY --repo cajoy/runner-demo
```

Inspect every resolved value and where it came from before running anything:

```bash
runner config explain api --project "$PWD"
```

## 1. Run locally and sign the result

```bash
./scripts/local-preflight.sh
```

That runs `api:preflight` and attaches its signed portable projection to
`HEAD`. The private terminal receipt stays in
`.local-ci/state/runs/<run-id>/receipt.json`; the redacted, signed copy goes
into `refs/notes/runner-receipts`. The portable copy carries repository,
commit, tree, workflow, task digests, platform, producer, and outcome — never
paths, commands, environment values, logs, or credentials.

A dirty working tree cannot produce a portable receipt. Reuse is bound to an
exact commit.

## 2. Watch it in the dashboard

```bash
runner dashboard
```

Open <http://127.0.0.1:7331/>. This project registers itself on its first run;
the run detail shows each task's producer, placement, scheduling decision,
platform, and digests.

## 3. Push and let CI skip the verified work

```bash
git push --atomic origin HEAD refs/notes/runner-receipts
```

[`.github/workflows/demo.yml`](.github/workflows/demo.yml) then runs three
stages:

| Stage | Command | What it proves |
| --- | --- | --- |
| plan | `runner ci plan api:github` | Verifies signatures and policy for the exact commit, marks `lint`, `unit`, `build` as `reused`, and emits a matrix of only what must run. |
| execute | `runner ci execute --task <id>` | Runs each remaining task in isolation. No signing key is present. |
| finalize | `runner ci finalize` | Combines reused and executed claims into one signed CI receipt and attaches it to the same commit. |

The job summary prints the decision and outcome per task. With a valid local
receipt, only `deploy-github` executes.

## What to try next

- **Nothing left to do.** Push again without changing anything. The CI receipt
  from the previous run covers all four tasks, so `execute_count` is `0` and
  zero task commands run — Runner still writes an evaluation receipt.
- **Invalidate one task.** Change `main_test.go` and push. `unit` and its
  dependent `deploy-github` execute; `lint` and `build` stay reused.
- **Tamper with a receipt.** Edit the signed bytes in the notes ref. Planning
  reports `receipt_integrity_conflict`, quarantines the candidate, and executes
  the affected graph in CI instead of trusting it.
- **Inspect without executing.** `runner runs observe --project "$PWD" --run <run-id> --json`
- **Read the remote evidence only.** `runner receipts fetch --project "$PWD" --remote origin --commit HEAD --json`

## Second workflow: fail, then resume

[`.local-ci/hello.yaml`](.local-ci/hello.yaml) is a separate v1 workflow that
demonstrates exact resume. `prepare → test → finish`, where `finish` reads a
simulated external condition from `.local-ci/state/`, which Runner deliberately
excludes from source capture.

```bash
./scripts/set-result.sh fail
runner run --project "$PWD" --json-stream hello:demo   # exits 1

./scripts/set-result.sh success
runner run --project "$PWD" --resume <failed-run-id> --json-stream hello:demo
```

The child receipt records `reused_jobs: ["prepare", "test"]` and
`executed_jobs: ["finish"]`. Changing tracked source, workflow configuration,
parameters, or Runner and plugin identities makes exact resume fail closed
instead of reusing jobs.

## Layout

| Path | Purpose |
| --- | --- |
| `.local-ci/api.yaml` | v2 workflow: the four demo tasks. |
| `.local-ci/hello.yaml` | v1 workflow: fail-then-resume. |
| `.local-ci/runner.yaml` | Project configuration: driver, plugins, concurrency. |
| `.local-ci/toolchain.lock` | The exact Runner, plugin, and runtime image versions. |
| `.runner/receipt-policy.yaml` | Which signers CI trusts, for which tasks and platforms. |
| `.github/workflows/demo.yml` | plan → execute → finalize. |
| `tasks/deploy-hello.sh` | The CI-only, `effect:none` stage. |
| `scripts/` | Bootstrap and demo drivers. |

`.local-ci/state/` holds run records, logs, and receipts for this project and
is not tracked.
