# Working on runner-demo

Preserve unrelated dirty and untracked work. The source and public distribution repositories are intentionally separate: Runner source is hap-team/runner; demo and binary distribution are cajoy/runner-demo and cajoy/runner-dist.

Implement tools and test harness logic in Go. Shell files are thin launchers.

Normal GitHub CI downloads a checksum-pinned standalone verifier from cajoy/runner-dist. Keep verifier source/tests in Runner; keep only the public receipt contract and binary pin here. The contract comes from protected main and binds the normal workflow, task definitions, inputs, and runtime.

Commit source before automatic signing. Keep host verification parallel with concurrency three.

When working through Claude Code, or any client with the Runner MCP server available, drive runs with the MCP tools rather than the `runner` binary: `runner_projects_list` for the durable `project_id`, then `runner_run` with the selector. Runner stamps the invoker from the process holding the MCP connection, so an MCP run records `agent/mcp` and a shelled-out one records the terminal default no matter what `--actor` claims. Reach for `runner run --actor agent --verbose --project . api:preflight` only where MCP is genuinely unavailable, and say which you used. Commands with no MCP tool — `receipts setup`, `receipts status`, `cleanup` — stay on the binary.

Signing keys are per machine: the private half never leaves the Keychain that generated it, so each machine that signs needs its own entry in `.runner/receipt-policy.yaml`. The approved signers are exactly the ones that policy names, currently local-alex and local-alex-mbp. Do not mint a key to get past an untrusted or unavailable signer — a key the protected policy does not list signs nothing the verifier will accept, and adding one is the repository owner's decision. Actor metadata is separate from signing identity; never describe the actor label as authenticated identity.

With repository-local receipt setup, successful eligible runs sign automatically and ordinary `git push` carries code and notes. Do not add manual attach or separate notes-push steps to the normal flow. Respect an actionable guard failure; a concurrent notes merge may require another ordinary push.

There is one local workflow, `api`, and every task in it runs in the pinned linux/arm64 image — the same one the GitHub job uses. That is deliberate: proof is scoped to the environment that produced it, so `api:preflight` is the only run needed and what it signs is eligible in CI. Do not reintroduce a host variant of these tasks; darwin proof cannot authorize a linux skip, and having both is what made the delivery step ambiguous. Missing, expired, revoked, or otherwise ineligible evidence cannot authorize a skip. The standalone GitHub verifier falls back to normal checks for rejected, malformed, or conflicting proof. Runner's own integrity policy still applies to its separate commands.

The verifier rebuilds each task input from `.runner/receipt-contract.json`, and the service name and config digest are part of that identity. Renaming the service or editing `.local-ci/api.yaml` changes it, so the contract has to move in the same commit or every check reports subject_or_workflow_mismatch and runs fresh.

Push with plain `git push`. An explicit refspec such as `git push origin main` overrides the configured ones and omits the notes ref, and the guard rejects it.

api:deploy is a local simulation. It must reuse eligible preflight checks, restore the verified binary, and execute the simulation itself. Existing production approval requirements remain in force.

Use the Go executable harness in test-cases against the candidate Runner binary. Fixture signers and remotes must stay disposable. Never push fabricated proof to the real demo remote.

The static page describes an example state. Show real receipt status through CLI, dashboard, and GitHub summaries; do not present static numbers as live verification results.
