# Executable receipt scenarios

Normal GitHub CI downloads the checksum-pinned standalone Go verifier from `cajoy/runner-dist` and uses ordinary conditional check steps. The verifier source and its signature, coverage, content, and delivery tests live in Runner. Invalid proof runs normal checks.

The opt-in scenarios below test Runner itself. Their GitHub workflow is `Runner dogfood (manual)` and runs only through an explicit dispatch.

Run `RUNNER_BIN=/absolute/path/to/runner ./test-cases/run.sh`. The harness is Go; the shell scripts only launch it. Run `go test ./cmd/...` for the CI command and installer tests.

The default run covers 29 host and delivery scenarios. Add `RUNNER_DEMO_LINUX=1` to include the thirtieth scenario: execution, signing, reuse, and finalization with Docker's pinned Linux ARM image. The launcher allows 30 minutes for the suite and each command has a ten-minute bound. A cold Linux Go build can take several minutes.

Every executable scenario uses a disposable repository, bare remote, and Ed25519 fixture key. A Go proxy counts actual vet, test, and build invocations while forwarding them to the installed Go toolchain. Fabricated evidence never reaches the real demo repository.

The original cases now exercise Runner's verifier:

| Case | Expected behavior |
| --- | --- |
| no-receipt | Execute all checks |
| full-receipt | Reuse lint, unit, build |
| partial | Reuse only covered tasks |
| failed-task | Rerun the failed task |
| empty-index | Execute all checks |
| not-pushed | CI has no reusable proof |
| pushed-later | CI reuses after ordinary push delivers proof |
| stale-commit | Reject the forged subject/index association |
| unknown-signer | Do not reuse the untrusted claims |
| wrong-platform | Do not reuse incompatible runtime proof |
| config-drift | Rerun changed verification |
| other-workflow | Do not reuse another service's proof |
| corrupt-note | Stop at the integrity gate |
| multi-entry | Combine eligible original proof and summarize every task |

Additional scenarios cover invalid signatures, expired and future-dated proof, conflicting signed outcomes, source edits, dirty-source signing refusal, forced fresh execution, missing artifacts, README-only commits and amendments, local preflight-to-deploy reuse, actual CI execution/finalization, and concurrent notes-only pushes. MCP tests check agent provenance and proof status. A fresh clone verifies amended proof without receiving the superseded commit object.

The standalone receiver tests also verify fallback outputs, conflicting claims, and compatibility with a published Runner signature. The ordinary workflow preserves GitHub's failure handling: a failed lint, unit, or build step prevents the deployment simulation. Installation tests cover the optional local/dogfood installer and its checksum check.
