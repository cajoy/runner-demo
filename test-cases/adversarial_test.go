package scenarios

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReceiptAdditionalScenarios(t *testing.T) {
	if *runnerBinary == "" {
		t.Skip("set -runner to run executable receipt scenarios")
	}
	s := newSuite(t)
	t.Run("fresh-mcp-provenance", s.testMCPProvenance)
	t.Run("bad-signature", func(t *testing.T) {
		root := s.clone(t, true)
		note := s.copyNote()
		note.Entries[0].Envelope.Signatures[0].Sig = base64.StdEncoding.EncodeToString(make([]byte, 64))
		s.writeIndex(root, note)
		if _, _, code := s.plan(root, false); code == 0 {
			t.Fatal("invalid signature passed the integrity gate")
		}
	})
	for _, test := range []struct {
		name string
		when time.Time
	}{
		{"expired-proof", time.Now().Add(-30 * 24 * time.Hour)},
		{"future-proof", time.Now().Add(24 * time.Hour)},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := s.clone(t, true)
			s.mutate(root, func(value map[string]any) { value["created_at"] = test.when.UTC().Format(time.RFC3339Nano) })
			s.expect(root, false)
		})
	}
	t.Run("conflicting-signed-claims", func(t *testing.T) {
		root := s.clone(t, true)
		value := s.payload()
		value["tasks"].([]any)[0].(map[string]any)["state"] = "failed"
		note := s.copyNote()
		note.Entries = append(note.Entries, s.signed(value))
		s.writeIndex(root, note)
		if _, _, code := s.plan(root, false); code == 0 {
			t.Fatal("conflicting proof accepted")
		}
	})
	t.Run("relevant-code-edit", func(t *testing.T) {
		root := s.clone(t, true)
		name := filepath.Join(root, "main.go")
		raw, _ := os.ReadFile(name)
		write(t, name, append(raw, []byte("\n// changed relevant source\n")...))
		s.git(root, "commit", "-qam", "source change")
		s.expect(root, false)
	})
	t.Run("dirty-source-does-not-sign", func(t *testing.T) {
		root := s.clone(t, false)
		s.setup(root)
		name := filepath.Join(root, "main.go")
		raw, _ := os.ReadFile(name)
		write(t, name, append(raw, []byte("\n// uncommitted\n")...))
		s.resetCounters()
		got := s.workflow(root, "api:preflight")
		s.assertCounters(map[string]int{"vet": 1, "test": 1, "build": 1})
		if got.Proof.Signed {
			t.Fatal("dirty source received portable signature")
		}
	})
	t.Run("fresh-forces-real-execution", func(t *testing.T) {
		s.t = t
		s.resetCounters()
		got := s.workflow(s.root, "api:preflight", "--fresh")
		s.assertCounters(map[string]int{"vet": 1, "test": 1, "build": 1})
		if got.Actor.Kind != "agent" || got.Actor.Source != "explicit" {
			t.Fatalf("actor=%+v", got.Actor)
		}
	})
	t.Run("missing-artifact-rebuilds-producer", func(t *testing.T) {
		s.t = t
		got := s.workflow(s.root, "api:preflight", "--fresh")
		for _, j := range got.Jobs {
			if j.Name == "build" {
				for _, a := range j.Artifacts {
					if err := os.Remove(a.Archive); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		s.resetCounters()
		s.workflow(s.root, "api:deploy")
		s.assertCounters(map[string]int{"build": 1})
	})
	t.Run("amend-readme-keeps-original-proof", func(t *testing.T) {
		s.t = t
		before := s.workflow(s.root, "api:preflight", "--fresh")
		write(t, filepath.Join(s.root, "README.md"), []byte("amended docs\n"))
		s.git(s.root, "commit", "--amend", "-qam", "amended docs")
		s.resetCounters()
		after := s.workflow(s.root, "api:preflight")
		s.assertCounters(map[string]int{})
		for _, j := range after.Jobs {
			if j.SourceRunID != before.RunID {
				t.Fatalf("amend lost original run: %+v", j)
			}
		}
	})
	t.Run("ci-execution-and-finalization", func(t *testing.T) {
		root := s.clone(t, false)
		s.resetCounters()
		p, errs, code := s.plan(root, false)
		if code != 0 {
			t.Fatalf("plan: %s", errs)
		}
		planPath := filepath.Join(root, ".runner-ci", "plan.json")
		for _, task := range p.Tasks {
			_, errs, code := s.run(root, "ci", "execute", "--project", root, "--plan", planPath, "--task", task.ID, "--actor", "agent", "--policy", ".runner/policy.yaml", "--policy-ref", "refs/heads/policy", "--json")
			if code != 0 {
				t.Fatalf("execute %s: %d %s", task.ID, code, errs)
			}
		}
		out, finalErrors, code := s.run(root, "ci", "finalize", "--project", root, "--plan", planPath, "--evidence-dir", filepath.Join(root, ".runner-ci", "evidence"), "--policy", ".runner/policy.yaml", "--policy-ref", "refs/heads/policy", "--receipt-only", "--json")
		if code != 0 || !strings.Contains(string(out), `"receiver_verified":true`) {
			t.Fatalf("finalize: %d %s %s", code, out, finalErrors)
		}
		s.assertCounters(map[string]int{"vet": 1, "test": 1, "build": 1})
	})
	t.Run("notes-only-push-reconciles", func(t *testing.T) {
		root := s.clone(t, false)
		s.git(root, "reset", "--hard", s.commit)
		s.setup(root)
		value := s.payload()
		value["source_receipt"].(map[string]any)["run_id"] = "concurrent-proof"
		note := s.copyNote()
		note.Entries = []entry{s.signed(value)}
		s.writeIndex(root, note)
		// Publish the original note through the producer, then push a divergent
		// local note while the branch remains unchanged on the receiver.
		s.git(s.root, "push", "origin", notesRef)
		_, errs, code := command(t, root, s.env, "git", "push")
		if code == 0 || !strings.Contains(string(errs), "receipt_delivery_retry") {
			t.Fatalf("first push: %d %s", code, errs)
		}
		s.git(root, "push")
		raw := s.git(root, "notes", "--ref="+notesRef, "show", s.commit)
		var merged index
		if err := json.Unmarshal([]byte(raw), &merged); err != nil {
			t.Fatal(err)
		}
		if len(merged.Entries) < 2 {
			t.Fatal("concurrent proof was lost")
		}
	})
	t.Run("amended-proof-reaches-fresh-ci-clone", func(t *testing.T) {
		s.t = t
		original := s.git(s.root, "rev-parse", "HEAD")
		s.workflow(s.root, "api:preflight", "--fresh")
		write(t, filepath.Join(s.root, "README.md"), []byte("transported amended documentation\n"))
		s.git(s.root, "commit", "--amend", "-qam", "amended proof delivery")
		s.git(s.root, "checkout", "-b", "amended")
		// The previous concurrent writer may first require local note reconciliation.
		_, _, code := command(t, s.root, s.env, "git", "push")
		if code != 0 {
			s.git(s.root, "push")
		}
		root := filepath.Join(t.TempDir(), "receiver")
		s.git(s.root, "clone", "--no-local", "--single-branch", "--branch", "amended", s.remote, root)
		s.git(root, "remote", "set-url", "origin", fixtureOrigin)
		s.git(root, "config", "url.file://"+s.remote+".insteadOf", fixtureOrigin)
		s.git(root, "branch", "policy", "HEAD")
		if _, _, code := command(t, root, s.env, "git", "cat-file", "-e", original+"^{commit}"); code == 0 {
			t.Fatal("fixture unexpectedly received the superseded commit")
		}
		s.expect(root, true, "lint", "unit", "build")
	})
}
