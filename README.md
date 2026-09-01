# Runner demo

Work verified on a laptop is not repeated in CI.

A local run signs a receipt bound to the exact commit. CI reads that receipt,
skips every step it already covers, and goes straight to deploy.

## The workflow

```
lint ─┐
unit ─┼─→ deploy-local     not implemented, deploys run in CI
build ┘   deploy-github    "deploy to production"
```

Five tasks in [`.local-ci/api.yaml`](.local-ci/api.yaml). `lint`, `unit`, and
`build` are independent and run in parallel. The two deploy tasks are separate
identities, so a laptop can never produce evidence that stands in for a
production deploy.

## Locally

```bash
runner run --project . api:preflight            # lint, unit, build
runner run --project . --verbose api:preflight  # watch it happen
runner run --project . api:deploy               # stops: not implemented locally
runner dashboard                                # http://127.0.0.1:7331/
```

Sign the result and bind it to this commit:

```bash
runner receipts attach --project . --run <run-id> --commit HEAD \
  --signer local-alex --json
```

The first attach creates the `local-alex` Ed25519 key in your login Keychain.
The full receipt stays in `.local-ci/state/runs/<run-id>/`; a redacted, signed
copy goes into `refs/notes/runner-receipts`, carrying the commit, the workflow,
task digests, platform, and outcome — never paths, commands, environment
values, or logs. A dirty tree cannot be signed.

## In CI

Receipts live in a notes ref, and an ordinary `git push` does not carry one.
Either name it every time:

```bash
git push --atomic origin HEAD refs/notes/runner-receipts
```

or teach this clone to send and fetch it with every push, once:

```bash
git config --add remote.origin.push HEAD
git config --add remote.origin.push refs/notes/runner-receipts
git config --add remote.origin.fetch '+refs/notes/runner-receipts:refs/notes/runner-receipts'
```

A run is not signed by running it. `runner run` writes the receipt into
`.local-ci/state/`, and `runner receipts attach` is what binds a signed copy to
the commit. Push a commit whose receipt never left your laptop and CI correctly
reports that it covers nothing.

[`.github/workflows/demo.yml`](.github/workflows/demo.yml) always shows the
same four steps. It reads the receipt for the pushed commit and skips what is
covered:

| | with a receipt | without one |
| --- | --- | --- |
| receipt | `covers: ["build","lint","unit"]` | `covers: []` |
| lint | skipped | `go vet ./...` |
| unit | skipped | `go test ./...` |
| build | skipped | `go build` |
| deploy | deploy to production | deploy to production |

Push an ordinary commit to see the right-hand column, then attach a receipt to
that same commit and re-run to see the left.

GitHub's web UI does not render Git notes anywhere, so the workflow prints the
receipt into the run's job summary: digest, signer, producer, commit, workflow,
what it covers, and the platform it ran on.

The workflow needs no secrets and installs nothing: it reads the signed
evidence with `git` and `jq`. It trusts the receipt's contents rather than
checking the signature, which is the part a real deployment would not simulate.

## Files

| Path | |
| --- | --- |
| `.local-ci/api.yaml` | the five tasks |
| `.local-ci/runner.yaml` | driver, plugins, concurrency |
| `.local-ci/toolchain.lock` | the exact Runner and runtime image this repository pins |
| `.github/workflows/demo.yml` | 48 lines, no Runner |

`.local-ci/state/` holds runs, logs, and receipts, and is not tracked.
