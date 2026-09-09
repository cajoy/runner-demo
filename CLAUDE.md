# Working on runner-demo

Preserve unrelated dirty and untracked work. The source and public distribution repositories are intentionally separate: Runner source is hap-team/runner; demo and binary distribution are cajoy/runner-demo and cajoy/runner-dist.

Implement tools and test harness logic in Go. Shell files are thin launchers.

Commit source before automatic signing. Run `runner run --actor agent --verbose --project . api:preflight`, or use Runner MCP, which records agent/mcp automatically. Keep host verification parallel with concurrency three.

The approved existing signer for human and agent runs is local-alex. Actor metadata is separate from signing identity. Do not create another key or describe the actor label as authenticated identity.

With repository-local receipt setup, successful eligible runs sign automatically and ordinary `git push` carries code and notes. Do not add manual attach or separate notes-push steps to the normal flow. Respect an actionable guard failure; a concurrent notes merge may require another ordinary push.

Host proof cannot substitute for pinned linux/arm64 proof. Use api-linux:preflight for evidence that the Linux CI verifier can reuse. Missing, expired, revoked, or otherwise ineligible evidence cannot authorize a skip. The standalone GitHub verifier falls back to normal checks for rejected, malformed, or conflicting proof. Runner's own integrity policy still applies to its separate commands.

api:deploy is a local simulation. It must reuse eligible preflight checks, restore the verified binary, and execute the simulation itself. Existing production approval requirements remain in force.

Use the Go executable harness in test-cases against the candidate Runner binary. Fixture signers and remotes must stay disposable. Never push fabricated proof to the real demo remote.

The static page describes an example state. Show real receipt status through CLI, dashboard, and GitHub summaries; do not present static numbers as live verification results.
