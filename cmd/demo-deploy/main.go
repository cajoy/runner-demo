// demo-deploy checks the build artifact restored by Runner and exports its page.
// It performs a local deployment simulation without starting a server.
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func main() {
	if err := simulate(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}

func simulate() error {
	binary, err := filepath.Abs(".local-ci-out/server")
	if err != nil { return err }
	raw, err := os.ReadFile(binary)
	if err != nil { return fmt.Errorf("verified build artifact is unavailable: %w", err) }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	page, err := exec.CommandContext(ctx, binary, "-export").Output()
	if err != nil { return fmt.Errorf("verified build artifact could not render its page: %w", err) }
	if err := os.WriteFile(".local-ci-out/preview.html", page, 0600); err != nil { return err }
	fmt.Printf("Simulated deploy used build artifact sha256:%x\nRendered %d bytes from that binary; no production deployment.\n", sha256.Sum256(raw), len(page))
	return nil
}
