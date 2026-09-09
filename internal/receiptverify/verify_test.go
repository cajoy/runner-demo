package receiptverify

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (Policy, Expected, Receipt, ed25519.PrivateKey) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := Policy{Schema: "runner-receipt-policy/v2", Keys: map[string]KeyPolicy{"fixture": {
		PublicKey: base64.StdEncoding.EncodeToString(pub), Repositories: []string{RepositoryID},
		Tasks: TaskNames, Placements: []string{"local", "ci"}, Platforms: []string{"linux/arm64"},
	}}, Tasks: map[string]TaskPolicy{}, Reuse: ReusePolicy{Effects: []string{"none"}, MaxAge: "72h"}, OnIntegrityConflict: "fail"}
	for _, name := range TaskNames {
		p.Tasks[name] = TaskPolicy{Services: []string{"api-linux"}, Platforms: []string{"linux/arm64"}, Verification: Contract{"go", Exclusions}}
	}
	e := Expected{Commit: strings.Repeat("a", 40), Tree: strings.Repeat("b", 40), FilesDigest: hashBytes([]byte("source")), Branch: "main"}
	r := Receipt{Schema: ReceiptSchema, CreatedAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)}
	r.Subject.RepositoryID, r.Subject.Commit, r.Subject.Tree = RepositoryID, e.Commit, e.Tree
	r.SourceReceipt.Schema, r.SourceReceipt.RunID, r.SourceReceipt.SHA256 = "local-ci-receipt/v5", "fixture-run", hashBytes([]byte("receipt"))
	r.Workflow.Service, r.Workflow.Entrypoint, r.Workflow.ConfigDigest = "api-linux", "preflight", ConfigDigest
	r.Producer.Placement, r.Producer.Runner = "local", "v0.8.28"
	r.Terminal.Status = "succeeded"
	for _, name := range TaskNames {
		content := identity(name, e.FilesDigest)
		r.Tasks = append(r.Tasks, Task{ID: name, State: "passed", Effect: "none", Decision: "executed", Content: &content,
			InputDigest: content.Digest, ExecutionDigest: hashBytes([]byte("execution")), Platform: "linux/arm64", RuntimeImageDigest: ImageDigest})
	}
	return p, e, r, key
}

func signedIndex(t *testing.T, r Receipt, key ed25519.PrivateKey) Note {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	envelope := Envelope{PayloadType: PayloadType, Payload: base64.StdEncoding.EncodeToString(raw), Signatures: []Signature{{KeyID: "fixture", Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(key, pae(PayloadType, raw)))}}}
	index := Index{Schema: IndexSchema, Commit: r.Subject.Commit, Entries: []Entry{{ReceiptDigest: hashBytes(raw), Envelope: envelope}}}
	encoded, _ := json.Marshal(index)
	return Note{Commit: r.Subject.Commit, Raw: encoded}
}

func TestReceiptDecisions(t *testing.T) {
	for _, name := range []string{"no-receipt", "full-receipt", "partial", "failed-task", "empty-index", "unknown-signer", "invalid-signature", "wrong-platform", "wrong-image", "host-proof", "config-drift", "source-drift", "other-workflow", "expired", "future-proof", "revoked", "key-expired", "signer-scope", "task-scope", "effect", "reused-claim", "identity-tampering", "wrong-tree", "stale-commit", "corrupt-note", "multi-entry", "conflicting-outcomes", "duplicate-task", "branch-rerun", "branch-spot-check", "docs-only-commit", "unknown-schema", "unreviewed-exclusions"} {
		t.Run(name, func(t *testing.T) {
			p, expected, receipt, key := fixture(t)
			now := receipt.CreatedAt.Add(time.Hour)
			want := []string{}
			switch name {
			case "full-receipt", "multi-entry":
				want = []string{"lint", "unit", "build"}
			case "partial":
				receipt.Tasks = receipt.Tasks[:2]
				want = []string{"lint", "unit"}
			case "failed-task":
				receipt.Tasks[1].State = "failed"
				receipt.Terminal.Status = "failed"
				want = []string{"lint", "build"}
			case "unknown-signer", "revoked":
				delete(p.Keys, "fixture")
			case "wrong-platform":
				for i := range receipt.Tasks {
					receipt.Tasks[i].Platform = "linux/amd64"
				}
			case "wrong-image":
				for i := range receipt.Tasks {
					receipt.Tasks[i].RuntimeImageDigest = hashBytes([]byte("other image"))
				}
			case "host-proof":
				for i := range receipt.Tasks {
					receipt.Tasks[i].Content.Runtime.Driver = "host"
				}
			case "config-drift":
				receipt.Workflow.ConfigDigest = hashBytes([]byte("new config"))
			case "source-drift":
				expected.FilesDigest = hashBytes([]byte("edited source"))
			case "other-workflow":
				receipt.Workflow.Service = "other"
			case "expired":
				now = now.Add(72 * time.Hour)
			case "future-proof":
				now = receipt.CreatedAt.Add(-10 * time.Minute)
			case "key-expired":
				k := p.Keys["fixture"]
				k.NotAfter = receipt.CreatedAt.Add(time.Minute)
				p.Keys["fixture"] = k
			case "signer-scope":
				k := p.Keys["fixture"]
				k.Repositories = []string{"other"}
				p.Keys["fixture"] = k
			case "task-scope":
				k := p.Keys["fixture"]
				k.Tasks = []string{"other"}
				p.Keys["fixture"] = k
			case "effect":
				for i := range receipt.Tasks {
					receipt.Tasks[i].Effect = "deploy"
				}
			case "reused-claim":
				for i := range receipt.Tasks {
					receipt.Tasks[i].Decision = "reused"
				}
			case "identity-tampering":
				for i := range receipt.Tasks {
					receipt.Tasks[i].Content.FilesDigest = hashBytes([]byte("lie"))
				}
			case "wrong-tree":
				receipt.Subject.Tree = strings.Repeat("c", 40)
			case "duplicate-task":
				receipt.Tasks = append(receipt.Tasks, receipt.Tasks[0])
			case "branch-rerun":
				p.Branches = map[string]BranchPolicy{"*": {OnValid: "rerun_all"}}
			case "branch-spot-check":
				p.Branches = map[string]BranchPolicy{"*": {OnValid: "spot_check", Rate: 10}}
			case "docs-only-commit":
				expected.Commit = strings.Repeat("c", 40)
				expected.Tree = strings.Repeat("d", 40)
				want = []string{"lint", "unit", "build"}
			case "unknown-schema":
				receipt.Schema = "runner-portable-receipt/v99"
			case "unreviewed-exclusions":
				task := p.Tasks["lint"]
				task.Verification.Exclude = []string{"main.go"}
				p.Tasks["lint"] = task
				want = []string{"unit", "build"}
			}
			notes := []Note{signedIndex(t, receipt, key)}
			switch name {
			case "no-receipt":
				notes = nil
			case "empty-index":
				notes[0].Raw = []byte(`{"schema":"` + IndexSchema + `","commit":"` + expected.Commit + `","entries":[]}`)
			case "corrupt-note":
				notes[0].Raw = []byte("not json")
			case "invalid-signature":
				var index Index
				_ = json.Unmarshal(notes[0].Raw, &index)
				index.Entries[0].Envelope.Signatures[0].Sig = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
				notes[0].Raw, _ = json.Marshal(index)
			case "stale-commit":
				notes[0].Commit = strings.Repeat("c", 40)
			case "multi-entry":
				a, b := receipt, receipt
				a.Tasks = receipt.Tasks[:1]
				b.Tasks = receipt.Tasks[1:]
				b.SourceReceipt.RunID = "second-run"
				b.SourceReceipt.SHA256 = hashBytes([]byte("second"))
				notes = []Note{signedIndex(t, a, key), signedIndex(t, b, key)}
			case "conflicting-outcomes":
				notes[0] = signedIndex(t, receipt, key)
				receipt.Tasks[0].State = "failed"
				receipt.SourceReceipt.RunID = "failed-run"
				receipt.SourceReceipt.SHA256 = hashBytes([]byte("failed"))
				notes = append(notes, signedIndex(t, receipt, key))
			}
			got := Verify(notes, p, expected, now)
			if !slices.Equal(got.Verified(), want) {
				t.Fatalf("verified=%v want=%v result=%+v", got.Verified(), want, got)
			}
			if len(got.Tasks) != 3 {
				t.Fatalf("missing task decisions: %+v", got)
			}
			if name == "multi-entry" {
				for _, task := range TaskNames {
					if !strings.Contains(got.Markdown(), "| "+task+" | reuse |") {
						t.Fatalf("summary omits %s: %s", task, got.Markdown())
					}
				}
			}
		})
	}
}

func TestLegacyHistoryDoesNotHideCurrentProof(t *testing.T) {
	p, expected, receipt, key := fixture(t)
	current := signedIndex(t, receipt, key)
	legacy := signedIndex(t, receipt, key)
	var index Index
	if err := json.Unmarshal(legacy.Raw, &index); err != nil {
		t.Fatal(err)
	}
	index.Entries[0].Envelope.PayloadType = "application/vnd.cajoy.runner.portable-receipt+json;version=2"
	legacy.Raw, _ = json.Marshal(index)
	result := Verify([]Note{legacy, current}, p, expected, receipt.CreatedAt.Add(time.Hour))
	if !slices.Equal(result.Verified(), TaskNames) {
		t.Fatalf("legacy history hid current valid proof: %+v", result)
	}
}
