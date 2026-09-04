# Working in this repository

This is a Go web server whose verification runs through Runner. A local run
signs a receipt bound to a commit; CI trusts that receipt instead of repeating
the work.

When asked to change the page and ship it — "update index.html and release the
changes" — do the whole loop, not just the edit.

## The loop

1. **Edit.** The page is [`index.html`](index.html); its content comes from
   `content()` in [`main.go`](main.go). Changing user-visible copy usually means
   touching both, plus `TestIndexRendersThepage` in `main_test.go`.

2. **Commit.** A dirty tree cannot be signed, so commit before running.

   ```bash
   git add index.html main.go main_test.go
   git commit -m "..."
   ```

3. **Run the workflow** through the Runner MCP server, not by shelling out to
   `runner run`. Use `runner_run` with `selector: "api:preflight"`. Find the
   `project_id` with `runner_projects_list`; if this project is not listed, it
   has not been registered yet — say so rather than guessing an id.

   Running through MCP is what records the run as agent-initiated. Runner stamps
   the invoker itself, from the process holding the MCP connection, so the
   receipt says `runner-mcp` and not `terminal`. Shelling out to `runner run`
   would produce a receipt indistinguishable from a person's.

4. **Sign it.** There is no MCP tool for this; use the CLI:

   ```bash
   runner receipts attach --project . --run <run-id> --commit HEAD \
     --signer agent-claude --json
   ```

   Use `--signer agent-claude`. This is a convention, not a guarantee — the
   signer is whatever key you name. The unspoofable part is the invoker from
   step 3. Do not describe the signer as proof that an agent did the work.

5. **Push both the commit and the receipt.** An ordinary push does not carry a
   notes ref:

   ```bash
   git push --atomic origin HEAD refs/notes/runner-receipts
   ```

## What to check afterwards

The CI job summary should show the tasks the receipt covers, and
`invoked by: runner-mcp`. If it shows `terminal`, the run did not go through
MCP and step 3 was skipped.

## Rules

- Never attach a receipt to a commit you did not just verify. The receipt binds
  to an exact commit and tree; attaching a stale run is a false claim.
- Never use `--signer local-alex`. That identity is Alex's.
- `api:deploy` stops on purpose — deploying from a laptop is not supported. Do
  not try to route around it.
- If `lint` or `unit` fails, fix the code. Do not attach a receipt for a failed
  run, and do not skip tasks to make a run pass.
