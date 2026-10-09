package ahm

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeContractTask writes a task in the given status into the bucket that
// status implies and returns its path.
func writeContractTask(t *testing.T, root string, id string, status string) string {
	t.Helper()
	path := filepath.Join(root, ".ahm", "tasks", bucketForStatus(status), id+".md")
	writeTaskFile(t, path, id, "Contract Task", status, "")
	return path
}

// TestTaskStatusContracts pins the ADR 030 table: every lifecycle verb declares
// a target and an accepted-status set, the target is never an accepted start
// state, and every contract has a registered subcommand.
func TestTaskStatusContracts(t *testing.T) {
	want := map[string]string{
		"accept":   "Pending",
		"start":    "In Progress",
		"complete": "Completed",
		"cancel":   "Cancelled",
		"reopen":   "Open",
		"block":    "Blocked",
		"unblock":  "Pending",
	}
	if len(taskStatusContracts) != len(want) {
		t.Fatalf("contract count = %d, want %d", len(taskStatusContracts), len(want))
	}
	for verb, target := range want {
		contract, ok := taskStatusContracts[verb]
		if !ok {
			t.Errorf("missing contract for %q", verb)
			continue
		}
		if contract.target != target {
			t.Errorf("%s target = %q, want %q", verb, contract.target, target)
		}
		if len(contract.from) == 0 {
			t.Errorf("%s has an empty accepted-status set", verb)
		}
		if slices.Contains(contract.from, contract.target) {
			t.Errorf("%s accepted set must not contain its target %q", verb, contract.target)
		}
		for _, from := range contract.from {
			if !validTaskStatus(from) {
				t.Errorf("%s accepts unknown status %q", verb, from)
			}
		}
	}

	root := (&app{}).command()
	task := mustChildCommand(t, root, "task")
	registered := map[string]bool{}
	for _, cmd := range task.Commands() {
		registered[cmd.Name()] = true
	}
	for verb := range taskStatusContracts {
		if !registered[verb] {
			t.Errorf("contract for %q has no registered subcommand", verb)
		}
	}
}

// TestTaskStatusContractRejectsInapplicableState covers each verb invoked from a
// status its contract does not accept: a usage error (exit 2) that names the
// current status and the accepted ones, leaving the record unchanged.
func TestTaskStatusContractRejectsInapplicableState(t *testing.T) {
	tests := []struct {
		verb     string
		status   string
		wantFrom string
	}{
		{verb: "accept", status: "Blocked", wantFrom: "applies only to Open tasks"},
		{verb: "start", status: "Open", wantFrom: "applies only to Pending tasks"},
		{verb: "complete", status: "Cancelled", wantFrom: "applies only to Open, Pending, In Progress, or Blocked tasks"},
		{verb: "cancel", status: "Completed", wantFrom: "applies only to Open, Pending, In Progress, or Blocked tasks"},
		{verb: "reopen", status: "In Progress", wantFrom: "applies only to Completed, Cancelled, or Pending tasks"},
		{verb: "block", status: "Completed", wantFrom: "applies only to Open or Pending tasks"},
		{verb: "unblock", status: "Open", wantFrom: "applies only to Blocked tasks"},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			root := projectRoot(t)
			writeContractTask(t, root, "001", tt.status)

			args := []string{"--root", root, "task", tt.verb, "001"}
			if tt.verb == "cancel" || tt.verb == "block" {
				args = append(args, "--reason", "because")
			}
			stdout, stderr, code := runCLI(t, args...)
			if code != 2 {
				t.Fatalf("%s from %s: exit = %d, want 2, stdout = %s, stderr = %s",
					tt.verb, tt.status, code, stdout, stderr)
			}
			assertContainsAll(t, stderr,
				fmt.Sprintf("cannot %s task 001", tt.verb),
				"status is "+tt.status,
				tt.wantFrom,
			)
			assertFileContainsAll(t, writeContractTaskPath(root, tt.status), "status: "+tt.status)
		})
	}
}

// TestTaskStatusContractNoOpAtTarget covers each verb invoked when the task
// already holds the verb's target status: a no-op (exit 0) that reports the
// status and writes nothing.
func TestTaskStatusContractNoOpAtTarget(t *testing.T) {
	for _, verb := range []string{"accept", "start", "complete", "cancel", "reopen", "block", "unblock"} {
		t.Run(verb, func(t *testing.T) {
			target := taskStatusContracts[verb].target
			root := projectRoot(t)
			path := writeContractTask(t, root, "001", target)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			args := []string{"--root", root, "task", verb, "001"}
			if verb == "cancel" || verb == "block" {
				args = append(args, "--reason", "because")
			}
			stdout, stderr, code := runCLI(t, args...)
			if code != 0 {
				t.Fatalf("%s on %s: exit = %d, stderr = %s", verb, target, code, stderr)
			}
			assertContainsAll(t, stdout, "001 already "+target)
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Errorf("%s on %s rewrote the record", verb, target)
			}
		})
	}
}

func writeContractTaskPath(root string, status string) string {
	return filepath.Join(root, ".ahm", "tasks", bucketForStatus(status), "001.md")
}
