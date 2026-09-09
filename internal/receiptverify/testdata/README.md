# Published proof fixture

`linux-proof.json` contains the original public Ed25519 signature from Runner v0.8.28, source `adca9b1d6b924eabb98c33fb2c3f0c282c4b87ef`, for runner-demo commit `12db1305c00cd9d3210e13683a4671ae448a9e59`.

Run: `001a08358fd6a-16079a759a5b13bfbb4f30e2`.
Receipt: `sha256:21d0e44f75e4efb77ba3535ee8be94909a823e56211980bc7b790d4ae1e022c1`.

The fixture checks the standalone receiver against proof produced by released Runner. Tests use the rehearsal time, so the fixture remains useful after its 72-hour reuse window expires. Live verification always uses the current time and independently hashes the current source.

The fixed receiver recipe corresponds to `.local-ci/api-linux.yaml`: `go vet ./...`, `go test -count=1 -cover ./...`, and `go build -o .local-ci-out/server .`. Protocol field order follows Runner's `internal/verification/identity.go`; signature verification follows its `internal/portable/dsse.go`. No private key or Runner binary is included.
