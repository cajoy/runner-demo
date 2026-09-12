> Update September 12, 2026: the api-linux service was later merged into api. Use `api:preflight` for current runs. The historical commands and results below are unchanged.

# Ordinary GitHub CI with signed receipt reuse

The normal demo workflow now runs a standalone Go receipt verifier followed by GitHub Actions `lint`, `unit`, and `build` steps. Each step skips only when its own signed proof passes verification. Missing or rejected proof runs the normal checks. GitHub does not install Runner for this workflow.

Both paths were verified on source commit `1c5e3cd9ac0480f4bcde11c61329dec7bfb41d43`:

| Run | Proof available to GitHub | lint | unit | build | Simulation |
| --- | --- | --- | --- | --- | --- |
| [34325387029](https://github.com/cajoy/runner-demo/actions/runs/34325387029) | Older proof; current inputs changed | Passed | Passed | Passed | Passed |
| [34325728130](https://github.com/cajoy/runner-demo/actions/runs/34325728130) | Valid current proof | Skipped | Skipped | Skipped | Passed |

The standalone verifier generated `receipt-verification/verification.json` in each run. The first artifact records `task_inputs_changed` for all three tasks. The second identifies the original local Linux proof.

Runner v0.8.28 generated that proof locally through `api-linux:preflight`, with all three checks running in parallel in the pinned Linux ARM64 Go image:

- Run: `001a0852054db-2e8da5cf579a339ef7e7920d`
- Receipt: `sha256:6354e25b54d30af6d0d98826606e7de5965c8cbcb3d7ce205ab2819fb2a58158`
- Signer: `local-alex`; actor: `agent/explicit`
- Image: `sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b`

Signing happened automatically after the successful run. An ordinary `git push` delivered the notes; a manual dispatch of the same CI workflow verified them against the unchanged commit. The pre-push hook did not execute checks or create signatures.

Validation passed: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `actionlint` on both workflows. The receiver tests cover partial proof, failures, untrusted signers, signature tampering, expiry, content/configuration changes, platform changes, conflicting receipts, multiple entries, old receipt history, and delivery through a disposable Git remote. A published v0.8.28 signature also serves as a compatibility fixture.

The initial container rehearsal exposed Git's checkout ownership check. The final workflow grants Git access only to the requested CI checkout, per invocation. The subsequent runs above exercised the corrected receiver.

The separate `Runner dogfood (manual)` workflow still tests Runner's own CLI interfaces when explicitly dispatched. The ordinary CI simulation prints a message and requires no deployment artifact. No application deployment or Runner release was performed for this change.
