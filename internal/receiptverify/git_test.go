package receiptverify

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func testGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Receipt tests", "GIT_AUTHOR_EMAIL=tests@example.invalid", "GIT_COMMITTER_NAME=Receipt tests", "GIT_COMMITTER_EMAIL=tests@example.invalid")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, raw)
	}
	return strings.TrimSpace(string(raw))
}
func writeTestFile(t *testing.T, root, name, value string) {
	t.Helper()
	file := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(value), 0644); err != nil {
		t.Fatal(err)
	}
}
func sourceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testGit(t, root, "init", "-b", "main")
	writeTestFile(t, root, "go.mod", "module example.com/source\n\ngo 1.27\n")
	writeTestFile(t, root, "main.go", "package main\nfunc main() {}\n")
	writeTestFile(t, root, "README.md", "first\n")
	writeTestFile(t, root, "docs/example.txt", "excluded documentation\n")
	writeTestFile(t, root, "index.html", "embedded page\n")
	testGit(t, root, "add", ".")
	testGit(t, root, "commit", "-qm", "fixture")
	return root
}
func snapshot(t *testing.T, root string) string {
	t.Helper()
	commit := testGit(t, root, "rev-parse", "HEAD")
	digest, err := FilesDigest(context.Background(), root, commit)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestSourceContentChanges(t *testing.T) {
	for _, name := range []string{"docs", "source", "embedded-page", "configuration", "mode", "embed-exclusion", "symlink"} {
		t.Run(name, func(t *testing.T) {
			root := sourceFixture(t)
			before := snapshot(t, root)
			switch name {
			case "docs":
				writeTestFile(t, root, "README.md", "changed docs\n")
			case "source":
				writeTestFile(t, root, "main.go", "package main\nfunc main() { println(1) }\n")
			case "embedded-page":
				writeTestFile(t, root, "index.html", "changed page\n")
			case "configuration":
				writeTestFile(t, root, ".local-ci/api-linux.yaml", "changed recipe\n")
			case "mode":
				testGit(t, root, "update-index", "--chmod=+x", "main.go")
			case "embed-exclusion":
				writeTestFile(t, root, "main.go", "package main\nimport _ \"embed\"\n//go:"+"embed docs\nvar text string\nfunc main() {}\n")
			case "symlink":
				if err := os.Symlink("docs/example.txt", filepath.Join(root, "linked.txt")); err != nil {
					t.Fatal(err)
				}
			}
			if name != "mode" {
				testGit(t, root, "add", ".")
			}
			testGit(t, root, "commit", "-qm", name)
			after, err := FilesDigest(context.Background(), root, testGit(t, root, "rev-parse", "HEAD"))
			if name == "embed-exclusion" || name == "symlink" {
				if err == nil {
					t.Fatal("unsafe source unexpectedly received an identity")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (before == after) != (name == "docs") {
				t.Fatalf("before=%s after=%s", before, after)
			}
		})
	}
}

func TestDeliveryRequiresRemoteNotesAndNeverUsesStaleFetch(t *testing.T) {
	root := sourceFixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	testGit(t, root, "init", "--bare", "-b", "main", remote)
	testGit(t, root, "remote", "add", "origin", remote)
	testGit(t, root, "push", "origin", "main")
	p, expected, receipt, key := fixture(t)
	receipt.Subject.Commit = testGit(t, root, "rev-parse", "HEAD")
	receipt.Subject.Tree = testGit(t, root, "rev-parse", "HEAD^{tree}")
	expected.Commit, expected.Tree = receipt.Subject.Commit, receipt.Subject.Tree
	note := signedIndex(t, receipt, key)
	testGit(t, root, "notes", "--ref="+NotesRef, "add", "-m", string(note.Raw), note.Commit)
	notes, err := ReadNotes(context.Background(), root)
	if err != nil || len(notes) != 0 {
		t.Fatalf("unpushed notes reached receiver: %v %v", notes, err)
	}
	testGit(t, root, "push", "origin", NotesRef)
	notes, err = ReadNotes(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	result := Verify(notes, p, expected, receipt.CreatedAt.Add(time.Hour))
	if !slices.Equal(result.Verified(), TaskNames) {
		t.Fatalf("delivered proof rejected: %+v", result)
	}
	// A later absence cannot reuse the previously fetched local ref.
	testGit(t, root, "push", "origin", ":"+NotesRef)
	notes, err = ReadNotes(context.Background(), root)
	if err != nil || len(notes) != 0 {
		t.Fatalf("stale proof reused: %v %v", notes, err)
	}
	testGit(t, root, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	if _, err := ReadNotes(context.Background(), root); err == nil {
		t.Fatal("unreachable remote accepted")
	}
}

func TestPublishedRunnerProofCompatibility(t *testing.T) {
	// An actual public Runner v0.8.28 signature, not one produced by this
	// verifier's fixture code. Verification time is fixed to its rehearsal.
	raw, err := os.ReadFile("testdata/linux-proof.json")
	if err != nil {
		t.Fatal(err)
	}
	policyRaw, err := os.ReadFile("testdata/linux-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicy(policyRaw)
	if err != nil {
		t.Fatal(err)
	}
	expected := Expected{Commit: "12db1305c00cd9d3210e13683a4671ae448a9e59", Tree: "7e59b9762b72c5758665009c3c339632623e581d", FilesDigest: "sha256:a3e181ac2de2324decc77e60ff3c0870a49ff35902b1be907c29b14db1a6deee", Branch: "main"}
	result := Verify([]Note{{Commit: expected.Commit, Raw: raw}}, policy, expected, time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC))
	if !slices.Equal(result.Verified(), TaskNames) {
		encoded, _ := json.Marshal(result)
		t.Fatalf("published proof rejected: %s", encoded)
	}
}

func TestMalformedPolicyAndAmbiguousJSON(t *testing.T) {
	for _, raw := range []string{`{}`, `{"schema":"unknown"}`, `{"schema":"a","schema":"b"}`, `{} {}`, `null`} {
		if _, err := LoadPolicy([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	p, expected, receipt, key := fixture(t)
	for _, tail := range []string{" {}", "\nnull"} {
		note := signedIndex(t, receipt, key)
		note.Raw = append(note.Raw, tail...)
		if got := Verify([]Note{note}, p, expected, receipt.CreatedAt); len(got.Verified()) != 0 {
			t.Fatalf("ambiguous JSON allowed reuse: %+v", got)
		}
	}
}

func TestOrdinaryWorkflowMatchesReviewedReceiptRecipe(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/demo.yml")
	if err != nil {
		t.Fatal(err)
	}
	if hashBytes(raw) != WorkflowDigest {
		t.Fatal("CI workflow changed: review its commands, environment and receipt contract together")
	}
	for _, name := range TaskNames {
		condition := "!contains(fromJSON(steps.receipt.outputs.verified || '[]'), '" + name + "')"
		if !strings.Contains(string(raw), condition) {
			t.Fatalf("%s does not run when proof is absent", name)
		}
	}
	if strings.Contains(string(raw), "demo-install") || strings.Contains(string(raw), "demo-ci") || strings.Contains(string(raw), "runner ci") {
		t.Fatal("ordinary CI must use standalone verification and normal steps")
	}
}
