# Runner demo

Work verified on a laptop is not repeated in CI.

A local run signs a receipt bound to the exact commit. CI reads that receipt,
skips every step it already covers, and goes straight to deploy.

This repository is a small Go web server so there is something real to change,
build, and look at in a browser.

## Install

Runner is a single binary. This installs the version this repository pins,
verified against the release's published checksums:

```bash
curl -fsSL https://github.com/cajoy/runner-dist/releases/latest/download/install.sh | sh
runner version
```

Add `-s -- --with-mcp` to also register Runner with Claude Code and Codex:

```bash
curl -fsSL https://github.com/cajoy/runner-dist/releases/latest/download/install.sh | sh -s -- --with-mcp
```

`~/.local/bin` needs to be on your `PATH`. You do not have to install anything
else: the first `runner run` in this repository downloads the exact Runner that
[`.local-ci/toolchain.lock`](.local-ci/toolchain.lock) pins, verifies every byte
against that lock, and caches it.

```
$ runner run --project . api:preflight
installing Runner v0.8.12 from cajoy/runner-dist ...
installed Runner v0.8.12 (7.8 MB), verified against .local-ci/toolchain.lock
```

That happens once per version, per machine. The lock declares no plugins, so
nothing but Runner itself is downloaded.

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

`lint` and `unit` declare `runtime: host`: they run as host processes using your
own Go toolchain, in about 0.3 s each instead of ten seconds in a container.
`build` keeps the pinned container. Their receipts record that difference, and
it matters — `runner ci plan` scopes reuse by platform, so a host result from a
laptop is deliberately not eligible to satisfy a Linux CI task. The simulated
workflow here is more permissive than real verification would be.

## Run it

```bash
runner run --project . api:preflight            # lint, unit, build
runner run --project . --verbose api:preflight  # watch it happen
runner run --project . api:deploy               # stops: not implemented locally
```

Then look at the thing you just built:

```bash
go run .                    # http://127.0.0.1:8080/
ADDR=127.0.0.1:9000 go run .
```

`/` renders [`index.html`](index.html) through `html/template`; `/healthz`
returns `ok`. The template is embedded in the binary, so a broken page fails
`unit` rather than reaching a browser.

## The dashboard

```bash
runner dashboard            # http://127.0.0.1:7331/
```

Every run, its tasks, its receipt, and who produced it. The dashboard binds to
loopback only — a non-loopback `--listen` is refused, not warned about.

## Sign the run

A run is not signed by running it. `runner run` writes the receipt into the
project's state; `runner receipts attach` is what binds a signed copy to the
commit.

```bash
runner receipts attach --project . --run <run-id> --commit HEAD \
  --signer local-alex --json
```

The first attach creates the `local-alex` Ed25519 key in your login Keychain.
The full receipt stays in the run's state directory; a redacted, signed copy
goes into `refs/notes/runner-receipts`, carrying the commit, the workflow, task
digests, platform, and outcome — never paths, commands, environment values, or
logs. A dirty tree cannot be signed.

## Push it so CI can see it

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

Push a commit whose receipt never left your laptop and CI correctly reports
that it covers nothing.

## In CI

[`.github/workflows/demo.yml`](.github/workflows/demo.yml) always shows the same
four steps. It reads the receipt for the pushed commit and skips what is
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
receipt into the run's job summary: digest, signer, producer, who invoked it,
commit, workflow, what it covers, and the platform it ran on.

The workflow needs no secrets and installs nothing — not even Runner. It reads
the signed evidence with `git` and `jq`. It trusts the receipt's contents rather
than checking the signature, which is the part a real deployment would not
simulate.

## Who ran it

A receipt records the tool that asked for the run, and Runner stamps that itself
rather than accepting a claim:

| | |
| --- | --- |
| you, at a terminal | `invoked by: terminal` |
| an agent over MCP | `invoked by: runner-mcp · v0.8.12` |

So "an agent changed this and shipped it" is visible in CI and in the dashboard,
not just in a commit message. See [`CLAUDE.md`](CLAUDE.md) for the agent loop.

## Files

| Path | |
| --- | --- |
| `main.go`, `index.html` | the server and the page it renders |
| `.local-ci/api.yaml` | the five tasks |
| `.local-ci/runner.yaml` | driver, concurrency, no plugins |
| `.local-ci/toolchain.lock` | the exact Runner and runtime image this repository pins |
| `.github/workflows/demo.yml` | reads the receipt, skips what it covers |
| `CLAUDE.md` | how an agent drives all of the above |

Runs, logs, and receipts are not tracked.
