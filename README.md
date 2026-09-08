# Runner receipt reuse demo

Runner runs lint, unit tests, and build in parallel, then signs their proof automatically. Later runs reuse a check only when its declared inputs, runtime, and trusted policy still match. Ordinary `git push` carries the signed receipts after one-time setup.

The static preview is at https://cajoy.github.io/runner-demo/. It explains the demo; the CLI, local dashboard, and GitHub job summary show evidence from actual runs.

## Install and set up this clone

Host checks require Go 1.27 or newer on PATH. Confirm it with `GOTOOLCHAIN=local go version`; Runner fingerprints that compiler before running the checks.

Install the repository's pinned Runner version with Go:

```sh
go run ./cmd/demo-install
export PATH="$PWD/.runner-ci/bin:$PATH"
runner version
git fetch origin main:refs/remotes/origin/main
runner receipts setup --project . --signer local-alex \
  --policy .runner/receipt-policy.yaml --policy-ref refs/remotes/origin/main
```

Setup uses the existing `local-alex` signing key. It does not create a key. The demo policy trusts that key for these verification tasks. Another user needs an existing key that the protected policy explicitly trusts.

Setup configures this clone's default push to include HEAD and `refs/notes/runner-receipts`, and installs a pre-push guard. It preserves an existing default pre-push hook. Custom hook directories or conflicting push refspecs require explicit integration.

## Demonstrate local reuse

Commit the source you want to verify, then run:

```sh
runner run --verbose --project . api:preflight
runner run --verbose --project . api:deploy
```

Preflight runs lint, unit, and build on the host with concurrency three, including on battery power. A clean passing run creates and signs receipts automatically. The next deploy run reuses eligible checks and restores the verified build artifact. It still runs the local deployment simulation, which renders `.local-ci-out/preview.html` from the server binary. It changes no production service.

Logs show `task.reused`, the original run and receipt references, and receipt status. The normal local dashboard is http://127.0.0.1:7331/.

Try a README-only commit: these workflows explicitly exclude README.md, CLAUDE.md, and docs from verification inputs. Their proof remains reusable across commit changes. A source edit, embedded file edit, changed command, environment, toolchain, runtime, or missing required artifact causes affected checks to run again. Unknown coverage runs fresh.

```sh
runner run --verbose --project . --fresh api:preflight
```

`--fresh` deliberately executes checks even when reusable proof exists. Dirty or failed runs keep diagnostic receipts but do not receive automatic portable signatures.

## Deliver proof to Linux CI

Host macOS proof is valid for matching host runs. The Linux CI workflow requires its own proof from the pinned Linux image:

```sh
runner run --verbose --project . api-linux:preflight
git push
runner receipts status --project . --refresh
```

The Linux workflow uses Docker with a pinned image and linux/arm64 platform. It runs the same three verification commands. Ordinary push carries its notes; Runner makes no second network push. If concurrent notes require a local merge, the guard explains the retry and you run `git push` again.

The GitHub workflow installs the pinned Runner binary, loads policy from the protected main ref, verifies original signatures and content identity, runs any missing checks, and verifies final evidence before printing a simulated deployment. It needs no private signing key. The job summary includes every task, decision, original run, receipt digest, platform, actor, and signing key.

A delivered receipt means the remote has the code and original proof. Receiver verification is a separate step. Offline verification remains useful and reports pending delivery.

## Human and agent labels

Terminal runs default to `human/terminal_default`. Agents invoking the CLI use:

```sh
runner run --actor agent --verbose --project . api:preflight
```

MCP runs record `agent/mcp` automatically. Both use the explicitly configured existing signer. Actor labels describe invocation; they grant no authority and do not identify the person controlling a key.

## Exercise the failure cases

```sh
go test ./...
RUNNER_BIN="$PWD/.runner-ci/bin/runner" ./test-cases/run.sh
```

The Go harness invokes the real Runner binary and real Go commands in disposable repositories. It checks proof coverage, tampering, expiry, content changes, local artifacts, normal pushes, and verified CI finalization. It never publishes fabricated evidence to this repository.

The workflows preserve production authorization requirements. Portable receipts carry proof; this demo does not transport build artifacts between machines or deploy a production application.
