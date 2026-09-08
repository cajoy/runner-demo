# Receipt rehearsal, 9 September 2026

Runner v0.8.28 came from source commit `adca9b1d6b924eabb98c33fb2c3f0c282c4b87ef`. The rehearsal used demo commit `12db1305c00cd9d3210e13683a4671ae448a9e59` and the existing `local-alex` key. Agent invocations recorded `agent/explicit` independently of the signing key.

| Step | Observed result |
| --- | --- |
| Host preflight | Lint, unit and build ran in parallel; all passed in 20.1 seconds. |
| Local deploy | All three checks reused their original proof; the simulation restored and executed the verified server binary. Total: 7.6 seconds. |
| Pinned Linux preflight | All three checks passed in Docker on linux/arm64 in 246.2 seconds and signed automatically. |
| Initial GitHub run, before delivery | All three checks executed and finalization passed. |

The original host proof is `sha256:0d5d4445249dca2f656653bec19f210f5a011efb228d12c1b38e3a422176835b`, from run `001a083561fbf-5db4af4ca1ce03fcdf9d2282`. Local deploy run `001a08357836d-67d86581f34ea20292c55be6` restored build artifact `sha256:75628aaa0d862a9dabaf38f5cede3574b1dfae8a7b2c923f80e79afb8b4aefff`.

The Linux proof is `sha256:21d0e44f75e4efb77ba3535ee8be94909a823e56211980bc7b790d4ae1e022c1`, from run `001a08358fd6a-16079a759a5b13bfbb4f30e2`. Its pinned image is `golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b`.

The [initial GitHub verification](https://github.com/cajoy/runner-demo/actions/runs/34288274930) records fresh execution. Ordinary push carries the original signatures with this documentation change. The [verification workflow](https://github.com/cajoy/runner-demo/actions/workflows/demo.yml) records the receiving decision and complete task/proof summary. README.md and docs are reviewed exclusions, so this push exercises reuse across commits.

The Go harness passed 30 executable scenarios across the final runs, including the original fourteen cases, amended commits, a fresh receiving clone, MCP provenance, missing artifacts and the pinned Linux workflow. The local Go prerequisite check also caught Go 1.26.3 before signing; the successful host run used Go 1.27.1.

These are controlled rehearsal results. The local deployment is a simulation, and the static page illustrates the workflow. GitHub finalization verifies evidence before its own simulation; this demo does not transfer build artifacts between machines.
