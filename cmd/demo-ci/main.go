// demo-ci verifies receipts with Runner before simulating a deployment.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
)

type options struct {
	Runner, Selector, Policy, PolicyRef, Branch, Directory, Summary string
	Parallel                                                        int
}

type ciTask struct {
	ID       string `json:"id"`
	Decision string `json:"decision"`
	Level    int    `json:"level"`
}

func main() {
	o := options{}
	flag.StringVar(&o.Runner, "runner", "runner", "Runner executable")
	flag.StringVar(&o.Selector, "selector", "api-linux:github", "verification workflow")
	flag.StringVar(&o.Policy, "policy", ".runner/receipt-policy.yaml", "receipt policy path")
	flag.StringVar(&o.PolicyRef, "policy-ref", "refs/remotes/origin/main", "protected policy Git ref")
	flag.StringVar(&o.Branch, "branch", "main", "receiving branch")
	flag.StringVar(&o.Directory, "directory", ".runner-ci", "evidence directory")
	flag.StringVar(&o.Summary, "summary", os.Getenv("GITHUB_STEP_SUMMARY"), "GitHub summary file")
	flag.IntVar(&o.Parallel, "parallel", 3, "maximum simultaneous checks")
	flag.Parse()
	invoke := func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, o.Runner, args...)
		cmd.Stderr = os.Stderr
		return cmd.Output()
	}
	if err := runCI(context.Background(), o, invoke, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCI(ctx context.Context, o options, invoke func(context.Context, ...string) ([]byte, error), output io.Writer) error {
	if o.Parallel < 1 {
		return errors.New("parallel must be positive")
	}
	if o.Directory == "" {
		o.Directory = ".runner-ci"
	}
	if o.Selector == "" {
		o.Selector = "api-linux:github"
	}
	if o.Policy == "" {
		o.Policy = ".runner/receipt-policy.yaml"
	}
	if o.PolicyRef == "" {
		o.PolicyRef = "refs/remotes/origin/main"
	}
	if o.Branch == "" {
		o.Branch = "main"
	}
	planPath := filepath.Join(o.Directory, "plan.json")
	evidenceDir := filepath.Join(o.Directory, "evidence")
	policyArgs := []string{"--policy", o.Policy, "--policy-ref", o.PolicyRef}
	args := append([]string{"ci", "plan", o.Selector, "--branch", o.Branch, "--output", planPath, "--json"}, policyArgs...)
	raw, err := invoke(ctx, args...)
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	var plan struct {
		Tasks []ciTask `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		return fmt.Errorf("decode plan: %w", err)
	}
	levels := map[int][]ciTask{}
	for _, task := range plan.Tasks {
		if task.ID == "" || filepath.Base(task.ID) != task.ID || task.ID == "." || task.ID == ".." {
			return errors.New("invalid task ID")
		}
		switch task.Decision {
		case "executed", "spot_checked":
			levels[task.Level] = append(levels[task.Level], task)
		case "reused":
			fmt.Fprintf(output, "REUSED %s\n", task.ID)
		default:
			return fmt.Errorf("task %s has unsupported decision %q", task.ID, task.Decision)
		}
	}
	order := make([]int, 0, len(levels))
	for level := range levels {
		order = append(order, level)
	}
	sort.Ints(order)
	var executionErr error
	for _, level := range order {
		tasks := levels[level]
		results := make([]error, len(tasks))
		logs := make([][]byte, len(tasks))
		var wg sync.WaitGroup
		limit := make(chan struct{}, o.Parallel)
		for i, task := range tasks {
			wg.Add(1)
			go func(i int, task ciTask) {
				defer wg.Done()
				select {
				case limit <- struct{}{}:
				case <-ctx.Done():
					results[i] = ctx.Err()
					return
				}
				defer func() { <-limit }()
				args := append([]string{"ci", "execute", "--plan", planPath, "--task", task.ID, "--actor", "agent", "--evidence", filepath.Join(evidenceDir, task.ID+".receipt.json"), "--json"}, policyArgs...)
				logs[i], results[i] = invoke(ctx, args...)
			}(i, task)
		}
		wg.Wait()
		for i, err := range results {
			if len(logs[i]) > 0 {
				fmt.Fprintf(output, "%s: %s\n", tasks[i].ID, logs[i])
			}
			if err != nil {
				executionErr = errors.Join(executionErr, fmt.Errorf("%s: %w", tasks[i].ID, err))
			}
		}
		if executionErr != nil {
			break
		}
	}
	// Finalization validates every reused claim and executed task, including
	// failure evidence, before the simulated deployment becomes reachable.
	args = append([]string{"ci", "finalize", "--plan", planPath, "--evidence-dir", evidenceDir, "--receipt", filepath.Join(o.Directory, "final-receipt.json"), "--receipt-only", "--json"}, policyArgs...)
	if o.Summary != "" {
		args = append(args, "--github-summary", o.Summary)
	}
	raw, err = invoke(ctx, args...)
	if err != nil {
		return errors.Join(executionErr, fmt.Errorf("finalize: %w", err))
	}
	if executionErr != nil {
		return executionErr
	}
	var final struct {
		State            string `json:"state"`
		ReceiverVerified bool   `json:"receiver_verified"`
	}
	if err := json.Unmarshal(raw, &final); err != nil {
		return fmt.Errorf("decode finalization: %w", err)
	}
	if final.State != "passed" || !final.ReceiverVerified {
		return errors.New("finalization did not verify passing evidence")
	}
	fmt.Fprintln(output, "Simulated deployment: Runner verified all required checks. No production service was changed.")
	return nil
}
