package ahm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnboardOutputModes(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		want []string
		not  []string
	}{
		{"text", nil, []string{"Paste this snippet", "## Managed Work (ahm)", "ALWAYS run `ahm prime`"}, nil},
		{"plain", []string{"--plain"}, []string{"## Managed Work (ahm)", "ALWAYS run `ahm prime`"}, []string{"Paste this snippet"}},
		{"json", []string{"--json"}, []string{"\"snippet\"", "Managed Work (ahm)", "ahm prime"}, []string{"Paste this snippet"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--root", root}, tc.args...)
			args = append(args, "onboard")
			stdout, stderr, code := runCLI(t, args...)
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			assertContainsAll(t, stdout, tc.want...)
			for _, s := range tc.not {
				assertNotContains(t, stdout, s)
			}
		})
	}
}

func TestDoctorOnboardFinding(t *testing.T) {
	for _, tc := range []struct {
		name, agents string
		want         bool
	}{{"absent", "", false}, {"missing prime", "# Instructions\n", true}, {"compliant", "Run `ahm prime`.\n", false}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.agents != "" {
				if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(tc.agents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			stdout, _, _ := runCLI(t, "--root", root, "--json", "doctor")
			if tc.want {
				assertContainsAll(t, stdout, "agents_prime_missing", "ahm onboard")
			} else {
				assertNotContains(t, stdout, "agents_prime_missing")
			}
		})
	}
}

func TestAgentsCommandRemoved(t *testing.T) {
	_, stderr, code := runCLI(t, "agents", "suggestions")
	if code != 2 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	assertContainsAll(t, stderr, "unknown command \"agents\"")
}
