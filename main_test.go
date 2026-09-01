package main

import "testing"

func TestMessage(t *testing.T) {
	if got := message(); got != "hello from Runner" {
		t.Fatalf("message = %q", got)
	}
}
