package receiptverify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("verification_read_limit")
	}
	return b.Buffer.Write(p)
}
func git(ctx context.Context, root string, args ...string) ([]byte, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	// Actions checks out as the host user, then runs this Go image as root.
	// Trust only the requested checkout, only for this Git invocation.
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-replace-objects", "-c", "safe.directory=" + absolute}, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1")
	out, errs := &limitedBuffer{limit: 16 << 20}, &limitedBuffer{limit: 4096}
	cmd.Stdout, cmd.Stderr = out, errs
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out.Bytes(), nil
}

// ReadNotes fetches only the remote's current notes. A missing or failed fetch
// never falls back to a local catalog that CI might not have received.
func ReadNotes(ctx context.Context, root string) ([]Note, error) {
	raw, err := git(ctx, root, "ls-remote", "--refs", "origin", NotesRef)
	if err != nil {
		return nil, errors.New("receipt_notes_unavailable")
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return nil, nil
	}
	if len(fields) != 2 || !validOID(fields[0]) || fields[1] != NotesRef {
		return nil, errors.New("receipt_notes_invalid")
	}
	const fetched = "refs/notes/runner-demo-received"
	if _, err := git(ctx, root, "fetch", "--no-tags", "--force", "origin", NotesRef+":"+fetched); err != nil {
		return nil, errors.New("receipt_notes_unavailable")
	}
	head, err := git(ctx, root, "rev-parse", "--verify", fetched)
	if err != nil || strings.TrimSpace(string(head)) != fields[0] {
		return nil, errors.New("receipt_notes_changed_during_fetch")
	}
	raw, err = git(ctx, root, "notes", "--ref="+fetched, "list")
	if err != nil {
		return nil, errors.New("receipt_catalog_unavailable")
	}
	result := []Note{}
	total := 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || !validOID(fields[0]) || !validOID(fields[1]) || len(result) >= 1024 {
			return nil, errors.New("receipt_catalog_invalid")
		}
		blob, err := git(ctx, root, "cat-file", "blob", fields[0])
		total += len(blob)
		if err != nil || len(blob) > maxNoteBytes || total > 16<<20 {
			return nil, errors.New("receipt_lookup_limit")
		}
		result = append(result, Note{Commit: fields[1], Raw: blob})
	}
	return result, nil
}

func excluded(name string) bool {
	for _, prefix := range Exclusions {
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			return true
		}
	}
	return false
}

// FilesDigest implements Runner's runner.task-files.v2 contract on immutable
// Git blobs, including mode bits. Unsupported source forms cause fresh checks.
func FilesDigest(ctx context.Context, root, commit string) (string, error) {
	if !validOID(commit) {
		return "", errors.New("source_commit_invalid")
	}
	raw, err := git(ctx, root, "ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return "", err
	}
	type fileEntry struct {
		Name   string
		Mode   int64
		Digest string
		Link   string
	}
	entries := []fileEntry{}
	excludedNames := []string{}
	goSources := map[string][]byte{}
	total := 0
	for _, record := range strings.Split(string(raw), "\x00") {
		if record == "" {
			continue
		}
		metadata, name, ok := strings.Cut(record, "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 3 || fields[1] != "blob" || !validOID(fields[2]) || len(entries) > 10000 {
			return "", errors.New("source_tree_unsupported")
		}
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\x00\r\n\\") {
			return "", errors.New("source_path_unsupported")
		}
		// Runner omits sensitive names from snapshots. Refuse reuse if any are
		// tracked here rather than letting ordinary CI read unhashed content.
		lower := strings.ToLower(name)
		base := path.Base(lower)
		if lower == ".git" || strings.HasPrefix(lower, ".git/") || strings.HasPrefix(lower, ".local-ci/state/") || strings.HasPrefix(base, ".env") || base == "credentials" || strings.Contains(base, "credentials.") || base == "kubeconfig" || base == "id_rsa" || base == "id_ed25519" || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") {
			return "", errors.New("source_path_unsupported")
		}
		mode := int64(0644)
		switch fields[0] {
		case "100644":
		case "100755":
			mode = 0755
		default:
			return "", errors.New("source_mode_unsupported")
		}
		contents, err := git(ctx, root, "cat-file", "blob", fields[2])
		total += len(contents)
		if err != nil || total > 32<<20 {
			return "", errors.New("source_read_limit")
		}
		if strings.HasSuffix(name, ".go") {
			goSources[name] = contents
		}
		if excluded(name) {
			excludedNames = append(excludedNames, name)
			continue
		}
		entries = append(entries, fileEntry{Name: name, Mode: mode, Digest: hashBytes(contents)})
	}
	for name, source := range goSources {
		parsed, err := parser.ParseFile(token.NewFileSet(), name, source, parser.ParseComments)
		if err != nil {
			return "", errors.New("source_go_parse_failed")
		}
		for _, group := range parsed.Comments {
			for _, comment := range group.List {
				text := strings.TrimSpace(comment.Text)
				if !strings.HasPrefix(text, "//go:embed ") {
					continue
				}
				for _, pattern := range strings.Fields(strings.TrimPrefix(text, "//go:embed ")) {
					if strings.HasPrefix(pattern, "\"") || strings.HasPrefix(pattern, "`") {
						pattern, err = strconv.Unquote(pattern)
						if err != nil {
							return "", errors.New("verification_embed_unsupported")
						}
					}
					pattern = strings.TrimPrefix(pattern, "all:")
					for _, candidate := range excludedNames {
						rel := candidate
						if dir := path.Dir(name); dir != "." {
							if !strings.HasPrefix(candidate, dir+"/") {
								continue
							}
							rel = strings.TrimPrefix(candidate, dir+"/")
						}
						for part := rel; part != "."; part = path.Dir(part) {
							matched, err := path.Match(pattern, part)
							if err != nil || matched {
								return "", errors.New("verification_inputs_excluded")
							}
						}
					}
				}
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return digest("runner.task-files.v2", entries), nil
}

type Options struct{ Project, PolicyRef, Branch, Image string }

// Inspect validates the receiving environment and current policy before it
// considers any receipt. Its errors become run decisions, never skips.
func Inspect(ctx context.Context, o Options) Result {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" || runtime.Version() != "go1.27.1" || o.Image != Image {
		return Fallback("runtime_unavailable_or_changed")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return Fallback("pinned_container_required")
	}
	for k, v := range Environment {
		if os.Getenv(k) != v {
			return Fallback("environment_changed")
		}
	}
	raw, err := git(ctx, o.Project, "config", "--get", "remote.origin.url")
	if err != nil || !demoOrigin(strings.TrimSpace(string(raw))) {
		return Fallback("repository_mismatch")
	}
	raw, err = git(ctx, o.Project, "status", "--porcelain", "--untracked-files=normal")
	if err != nil || len(raw) != 0 {
		return Fallback("working_tree_not_clean")
	}
	commit, err := git(ctx, o.Project, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return Fallback("source_commit_unavailable")
	}
	tree, err := git(ctx, o.Project, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil {
		return Fallback("source_tree_unavailable")
	}
	if !strings.HasPrefix(o.PolicyRef, "refs/remotes/origin/") {
		return Fallback("protected_policy_ref_required")
	}
	policyCommit, err := git(ctx, o.Project, "rev-parse", "--verify", o.PolicyRef+"^{commit}")
	if err != nil {
		return Fallback("protected_policy_unavailable")
	}
	raw, err = git(ctx, o.Project, "show", strings.TrimSpace(string(policyCommit))+":.runner/receipt-policy.yaml")
	if err != nil {
		return Fallback("protected_policy_unavailable")
	}
	policy, err := LoadPolicy(raw)
	if err != nil {
		return Fallback(err.Error())
	}
	expected := Expected{Commit: strings.TrimSpace(string(commit)), Tree: strings.TrimSpace(string(tree)), Branch: o.Branch}
	workflow, err := git(ctx, o.Project, "show", expected.Commit+":.github/workflows/demo.yml")
	if err != nil || hashBytes(workflow) != WorkflowDigest {
		return Fallback("workflow_contract_changed")
	}
	expected.FilesDigest, err = FilesDigest(ctx, o.Project, expected.Commit)
	if err != nil {
		return Fallback("source_identity_unavailable")
	}
	notes, err := ReadNotes(ctx, o.Project)
	if err != nil {
		return Fallback(err.Error())
	}
	return Verify(notes, policy, expected, time.Now().UTC())
}

func demoOrigin(remote string) bool {
	if strings.HasPrefix(remote, "git@github.com:") {
		remote = "https://github.com/" + strings.TrimPrefix(remote, "git@github.com:")
	}
	u, err := url.Parse(remote)
	return err == nil && (u.Scheme == "https" || u.Scheme == "ssh") && strings.EqualFold(u.Hostname(), "github.com") && strings.EqualFold(strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), "cajoy/runner-demo")
}
