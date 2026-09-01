# Runner demo

One repository, one CLI, one checked-in DAG, no scripts. This demo proves the
behavior Runner exists for: **work verified on a laptop is not repeated in CI.**
A local run signs a portable receipt bound to an exact commit; GitHub Actions
verifies that receipt against a protected policy, reuses the tasks it covers,
and executes only what is left.

```
                    lint ─┐
local laptop        unit ─┼─→ deploy-github        GitHub Actions
(placement: any)   build ─┘   (placement: ci)
```

Everything the demo does is a `runner` command. The whole repository is a Go
module, four task definitions, and a trust policy.

| Path | Purpose |
| --- | --- |
| `.local-ci/api.yaml` | the four tasks and their entrypoints |
| `.local-ci/runner.yaml` | driver, plugins, concurrency |
| `.local-ci/toolchain.lock` | exact Runner, plugin, and runtime image versions |
| `.runner/receipt-policy.yaml` | which signers CI trusts, for which tasks and platforms |
| `.github/workflows/demo.yml` | plan → execute → finalize |

## Run it locally

```bash
runner run --project . api:preflight
```

`lint`, `unit`, and `build` run in parallel in the pinned container.

Runner delegates to the release named in `.local-ci/toolchain.lock`, so this
repository always runs the exact version it pins. Live rendering
(`runner run --verbose`, which shows progress events and task output as they
happen instead of a summary afterwards) arrives here once the lock names a
release that carries it.

Inspect what Runner resolved and where each value came from:

```bash
runner config explain api --project .
```

## Sign the result and bind it to this commit

```bash
runner receipts attach --project . --run <run-id> --commit HEAD \
  --signer local-alex --json
```

On first use Runner creates the `local-alex` Ed25519 key in the login Keychain
and prints its public key. The private terminal receipt stays in
`.local-ci/state/runs/<run-id>/receipt.json`; the redacted, signed projection
goes into `refs/notes/runner-receipts`. The portable copy carries repository,
commit, tree, workflow, task digests, platform, producer, and outcome — never
paths, commands, environment values, logs, or credentials.

A dirty working tree cannot produce a portable receipt. Reuse is bound to an
exact commit.

## Watch it

```bash
runner dashboard
```

Open <http://127.0.0.1:7331/>. Run detail shows each task's producer,
placement, scheduling decision, platform, and digests.

## Push, and let CI skip the verified work

```bash
git push --atomic origin HEAD refs/notes/runner-receipts
```

`.github/workflows/demo.yml` then runs three stages:

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
- **Rehearse CI locally.** `runner ci plan api:github --project . --policy .runner/receipt-policy.yaml --branch main --fetch=false --output .runner-ci/plan.json --json`
- **Read the remote evidence only.** `runner receipts fetch --project . --remote origin --commit HEAD --json`
- **Inspect without executing.** `runner runs observe --project . --run <run-id> --json`

## Setting this up from scratch

Already done in this checkout; these are the steps if the repository or its
signers ever move.

1. Install the pinned toolchain:

   ```bash
   gh release download v0.8.2 --repo cajoy/runner --dir .runner-bundle
   (cd .runner-bundle && shasum -a 256 -c checksums.txt)
   runner toolchain install --project "$PWD" --source-dir "$PWD/.runner-bundle" --json
   ```

2. Mint the two signers. `receipts attach --signer local-alex` creates the
   local key; the CI key is created the same way and its private half becomes
   the `RUNNER_RECEIPT_SIGNING_KEY` Actions secret:

   ```bash
   security find-generic-password -a github -s cajoy.runner.receipt-signing -w |
     gh secret set RUNNER_RECEIPT_SIGNING_KEY --repo cajoy/runner-demo
   ```

3. Put each signer's printed `public_key` into `.runner/receipt-policy.yaml`,
   along with this repository's portable identity, which any plan reports:

   ```bash
   runner ci plan api:github --project . --policy .runner/receipt-policy.yaml \
     --branch main --fetch=false --output .runner-ci/plan.json --json |
     jq -r '.subject.repository_id'
   ```

   The identity is derived from the `origin` URL, so renaming the GitHub
   repository invalidates the policy until it is updated.

4. Commit the policy first, then run and attach. Receipts bind to the commit
   that contains the policy that verifies them.

CI also needs a read-only `RUNNER_REPOSITORY_TOKEN` secret to download release
assets from the private `cajoy/runner` repository.
