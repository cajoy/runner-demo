package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInvalidProofStopsBeforeExecutionAndSimulation(t *testing.T) {
	var output bytes.Buffer
	calls := 0
	err := runCI(context.Background(), options{Parallel: 3}, func(context.Context, ...string) ([]byte, error) {
		calls++
		return nil, errors.New("receipt_integrity_conflict")
	}, &output)
	if err == nil || calls != 1 || strings.Contains(output.String(), "Simulated deployment") {
		t.Fatalf("calls=%d err=%v output=%s", calls, err, &output)
	}
}

func TestVerifiedReuseRunsNoChecksAndStillFinalizes(t *testing.T) {
	var output bytes.Buffer
	calls := []string{}
	err := runCI(context.Background(), options{Parallel: 3}, func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args[1])
		if args[1] == "plan" {
			return []byte(`{"tasks":[{"id":"lint","decision":"reused"},{"id":"unit","decision":"reused"},{"id":"build","decision":"reused"}]}`), nil
		}
		return []byte(`{"state":"passed","receiver_verified":true}`), nil
	}, &output)
	if err != nil || strings.Join(calls, ",") != "plan,finalize" || !strings.Contains(output.String(), "Simulated deployment") {
		t.Fatalf("calls=%v err=%v output=%s", calls, err, &output)
	}
}

func TestFailedFinalizationDoesNotSimulateDeployment(t *testing.T) {
	var output bytes.Buffer
	err := runCI(context.Background(), options{Parallel: 3}, func(_ context.Context, args ...string) ([]byte, error) {
		if args[1] == "plan" {
			return []byte(`{"tasks":[]}`), nil
		}
		return nil, errors.New("ci_task_failed")
	}, &output)
	if err == nil || strings.Contains(output.String(), "Simulated deployment") {
		t.Fatalf("err=%v output=%s", err, &output)
	}
}
