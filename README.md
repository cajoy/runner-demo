# Runner receipt reuse demo

Runner runs lint, unit tests, and build in parallel, then signs their proof automatically. Later runs reuse a check only when its declared inputs, runtime, and trusted policy still match. Ordinary `git push` carries the signed receipts after one-time setup.

The static preview is at https://cajoy.github.io/runner-demo/. It explains the demo; the CLI, local dashboard, and GitHub job summary show evidence from actual runs.

## Install and set up this clone

The verification checks bring their own Go: they run in the pinned image, so nothing about your local toolchain can change their result. You still need Docker running, and Go 1.27 or newer on PATH for the installer below and for `go test ./...` on this repository itself. Confirm it with `GOTOOLCHAIN=local go version`.

Install the repository's pinned Runner version with Go:

```sh
go run ./cmd/demo-install
export PATH="$PWD/.runner-ci/bin:$PATH"
runner version
git fetch origin main:refs/remotes/origin/main
runner receipts setup --project . --signer local-alex \
  --policy .runner/receipt-policy.json --policy-ref refs/remotes/origin/main
```

Setup uses the existing `local-alex` signing key. It does not create a key. The demo policy trusts that key for these verification tasks. Another user needs an existing key that the protected policy explicitly trusts.

Setup configures this clone's default push to include HEAD and `refs/notes/runner-receipts`, and installs a pre-push guard. It preserves an existing default pre-push hook. Custom hook directories or conflicting push refspecs require explicit integration.

## Demonstrate local reuse

Commit the source you want to verify, then run:

```sh
runner run --verbose --project . api:preflight
runner run --verbose --project . api:deploy
```

Preflight runs lint, unit, and build in the pinned image with concurrency three, including on battery power. A clean passing run creates and signs receipts automatically. The next deploy run reuses eligible checks and restores the verified build artifact. It still runs the local deployment simulation, which renders `.local-ci-out/preview.html` from the server binary. It changes no production service.

Logs show `task.reused`, the original run and receipt references, and receipt status. The normal local dashboard is http://127.0.0.1:7331/.

Try a README-only commit: these workflows explicitly exclude README.md, CLAUDE.md, and docs from verification inputs. Their proof remains reusable across commit changes. A source edit, embedded file edit, changed command, environment, toolchain, runtime, or missing required artifact causes affected checks to run again. Unknown coverage runs fresh.

```sh
runner run --verbose --project . --fresh api:preflight
```

`--fresh` deliberately executes checks even when reusable proof exists. Dirty or failed runs keep diagnostic receipts but do not receive automatic portable signatures.

## Deliver proof to CI

There is one workflow, and it runs where CI runs. Proof is scoped to the environment that produced it, so the checks execute in the same pinned `linux/arm64` image the GitHub job uses, and what you signed locally is eligible there:

```sh
git push
runner receipts status --project . --refresh
```

That is the whole delivery step: the `api:preflight` you already ran produced the proof. Ordinary push carries its notes; Runner makes no second network push. If concurrent notes require a local merge, the guard explains the retry and you run `git push` again.

`git push origin main` is not the same command. An explicit refspec replaces the two this clone was configured with, leaving the notes ref behind, and the guard refuses the push rather than delivering code without its proof.

GitHub downloads the standalone `runner-receipt-verify` binary from `cajoy/runner-dist`, checks its pinned SHA256, then runs ordinary `lint`, `unit`, and `build` steps. The verifier reads policy and the receipt contract from the protected main ref and fetches signed Git notes. It verifies signatures, signer scope, proof age, task inputs, and the pinned Linux runtime. Each normal step skips only when its own receipt passes verification. Missing, rejected, malformed, or conflicting proof makes checks run normally. A download, verifier, or notes-fetch failure also runs the checks.

The verifier source and tests live in the Runner repository. This demo contains the binary version/checksum pin and `.runner/receipt-contract.json`; it does not compile the verifier or install Runner in normal CI. The job runs in the same pinned Go image as `api:preflight`, which is why proof produced on a laptop is eligible here at all. Its summary shows a decision for every task and the original run, receipt digest, and signer for reused checks. The uploaded `verification.json` also includes the actor and original proof time. The final `deploy (simulation)` step prints a message after successful checks; it requires no deployment artifact and changes no application.

The receiver supports this demo's three fixed verification commands and Runner's v3 receipts with v2 content identities. Its reviewed workflow, configuration, and task-definition digests live in `.runner/receipt-contract.json`. Changing the workflow or verification recipe makes CI run fresh until that contract is updated on protected main. Source content is recomputed on each checkout, so README-only commits can still reuse proof. Unknown receipt versions run fresh.

Signing happens after a successful eligible `runner run`. The push hook checks delivery of the existing signed notes; it does not run checks or create signatures.

A delivered receipt means the remote has the code and original proof. Receiver verification is a separate step. Offline verification remains useful and reports pending delivery.

## Maintain the demo

A task with `verification: {kind: go}` must use bare `go test`, `go vet`, or `go build` commands. Shell operators, quoting, redirection, command substitution, external executables, `-exec`, `-toolexec`, `-overlay`, absolute arguments and parent-directory arguments make command coverage unknown. The task still runs, but Runner cannot reuse its proof. `runner config validate` now warns about this before execution. Keep export and formatting commands in separate tasks without `verification`.

After changing `.local-ci` or `.github/workflows/demo.yml`, run `make contract` with the pinned Runner, review the generated digests, and commit both `.runner/receipt-contract*.json` files. Runner computes the task definitions from the current configuration without executing a check. `make check-contract` and CI detect a changed configuration or workflow that was not incorporated into the reviewed contract. Changes to the runtime, environment or policy still require reviewing those fields in the contract.

The Pages deployment contract remains `main:/docs`. Run `make page` after changing `main.go` or `index.html`, and commit `docs/index.html` in the same change. `make check-page` is the freshness gate. CI also runs `make check-format`; use `make format` to correct formatting. These gates run separately from the three reusable verification tasks.

`go run .` serves a local preview on port 8080, including `/healthz`. This is the supported way to inspect the page locally; `go run . -export` renders the same template for Pages. The rounded timings on the page are illustrative figures from the September 9 rehearsal, not a current benchmark.

A run that reuses all checks cites the original signed receipt and its original commit. It does not sign those checks again or attach a new note to the current commit. The push must carry both the code and the original notes. `receipts status` labels its counters as pending observations and confirmations made during that refresh; a zero refresh count does not mean previously delivered proof disappeared.

Git notes use the repository-local Git identity. If Runner reports `receipt_identity_missing`, set `git config --local user.name` and `git config --local user.email` to your own identity. Runner intentionally ignores the global Git configuration for note writes.

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

`go test ./...` tests this demo. The standalone-verifier tests now live with its source in Runner. The separate Go dogfood harness invokes the real Runner binary and real Go commands in disposable repositories. Its GitHub workflow, `Runner dogfood (manual)`, runs only when explicitly dispatched; it is not part of normal pushes or pull requests. It checks local signing, artifacts, normal pushes, and Runner's own distributed-CI interfaces without publishing fabricated evidence to this repository.

The workflows preserve production authorization requirements. Portable receipts carry proof; this demo does not transport build artifacts between machines or deploy a production application.
