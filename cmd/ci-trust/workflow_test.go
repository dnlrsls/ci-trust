package main

import (
	"os"
	"path/filepath"
	"testing"
)

type workflowUnreadInput struct{}

func (workflowUnreadInput) Read([]byte) (int, error) { panic("request read before compiled gate") }

func TestWorkflowBridgeDeniesBeforeInput(t *testing.T) {
	root := t.TempDir()
	counter := filepath.Join(root, "counter")
	if err := os.WriteFile(counter, []byte("7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, passed, err := verifyWorkflow(root, workflowUnreadInput{})
	if err == nil || err.Error() != "NODE_SECURITY_HOLD" || len(data) != 0 || passed {
		t.Fatalf("data%q passed%v err%v", data, passed, err)
	}
	b, e := os.ReadFile(counter)
	if e != nil || string(b) != "7\n" {
		t.Fatal("source changed", e)
	}
}
