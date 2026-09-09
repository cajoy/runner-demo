// receipt-verify emits skip decisions for ordinary GitHub Actions steps.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"example.com/runner-portable-ci-demo/internal/receiptverify"
)

func main() {
	o := receiptverify.Options{}
	flag.StringVar(&o.Project, "project", ".", "repository checkout")
	flag.StringVar(&o.PolicyRef, "policy-ref", "refs/remotes/origin/main", "protected policy ref fetched by CI")
	flag.StringVar(&o.Branch, "branch", "main", "receiving branch")
	flag.StringVar(&o.Image, "image", "", "pinned image running this verifier")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result := receiptverify.Inspect(ctx, o)
	if err := emit(result, os.Getenv("GITHUB_OUTPUT"), os.Getenv("GITHUB_STEP_SUMMARY"), filepath.Join(o.Project, ".runner-ci", "verification.json")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, task := range result.Tasks {
		fmt.Printf("%s %s: %s\n", task.Decision, task.Task, task.Reason)
	}
}

func emit(result receiptverify.Result, output, summary, evidence string) error {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(evidence), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(evidence, append(raw, '\n'), 0644); err != nil {
		return err
	}
	if summary != "" {
		if err := appendFile(summary, result.Markdown()); err != nil {
			return err
		}
	}
	// Write outputs last, after all validation and evidence writes succeeded.
	if output != "" {
		verified, err := json.Marshal(result.Verified())
		if err != nil {
			return err
		}
		return appendFile(output, "verified="+string(verified)+"\n")
	}
	return nil
}
func appendFile(name, contents string) error {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(contents)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
