> Update September 12, 2026: the api-linux service was later merged into api. Use `api:preflight` for current runs. The historical commands and results below are unchanged.

# Released verifier rehearsal — 9 September 2026

The demo downloads `runner-receipt-verify` v0.8.29 from [cajoy/runner-dist](https://github.com/cajoy/runner-dist/releases/tag/v0.8.29). Its Go source and tests live in Runner. Normal CI keeps its own lint, unit, build, and simulated deployment steps.

The Linux ARM64 binary is pinned to SHA256 `d6b62fbaba18cadc5ac4ca20a31d9f8e8398d319a52a99da70d6ee38c5fb45ec`. GitHub checked that hash and the binary reported source commit `705c0958b00f7f920abef1b29edeeb2f56c67dd4`.

Both runs below tested demo commit `3186bf221ea6ea06d2677d4cb23cfc6708ef5c63` with the same workflow and protected receipt contract.

| Evidence available | GitHub run | lint / unit / build | Result |
| --- | --- | --- | --- |
| Older receipts, with changed task inputs | [34333918394](https://github.com/cajoy/runner-demo/actions/runs/34333918394) | All executed | Passed |
| Fresh signed proof delivered by ordinary push | [34334360167](https://github.com/cajoy/runner-demo/actions/runs/34334360167) | All skipped | Passed |

The local command was:

```sh
runner run --actor agent --verbose --project . api-linux:preflight
git push
```

All three local tasks started together and passed in the pinned Docker Linux ARM64 runtime. Runner automatically signed the successful receipt with the existing `local-alex` signer. Actor metadata records `agent/explicit`; it is separate from signer identity. The ordinary push delivered Git notes. Because that push changed only notes, the second GitHub run was dispatched on the same main commit.

The verifier's uploaded `receipt-verification` artifact records `reuse` / `verified_content_receipt` for all three tasks, with:

- Original run: `001a085776ef8-c3998866eea0408c2da559d7`.
- Receipt: `sha256:fd9279caa1222eeccd35a3f030b45e6cd05de9ce431a8f622b57a4cfe717d1f9`.
- Proof time: `2026-09-09T09:21:18.715877Z`.
- Signer: `local-alex`; actor: `agent/explicit`.

Historical unsupported or mismatching notes remain reported as warnings; they did not authorize a skip or hide the accepted proof. The final step in both runs was a deployment simulation. No application was deployed.

The release contains 18 assets. Every public asset was downloaded and matched the prepared bundle byte for byte; the published checksum list passed. Unit tests, race tests, Go vet, standalone verifier tests, and workflow lint passed before integration.
