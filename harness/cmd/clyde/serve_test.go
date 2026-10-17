package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestBodyOwnershipAndServeBudgetValidation(t *testing.T) {
	u := fmt.Sprintf("http://localhost:7777/%s", t.Name())
	f, err := bodyLease(u)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := bodyLease(strings.Replace(u, "localhost", "127.0.0.1", 1) + "/"); err == nil {
		other.Close()
		t.Fatal("second controller acquired body")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	f, err = bodyLease(u)
	if err != nil {
		t.Fatal("ownership not released:", err)
	}
	f.Close()
	for _, args := range [][]string{{"--max-autonomous", "0"}, {"--planner-time", "0"}, {"--max-steps", "0"}, {"--max-time", "0"}, {"unexpected"}} {
		if err := serveCmd(context.Background(), args); err == nil {
			t.Fatalf("accepted invalid serve arguments %v", args)
		}
	}
}
