package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testConfig writes a one cluster csv and returns the path to it. The cluster
// peaks for two hours, so the hours either side of the window are its base and
// every figure is small enough to check by hand.
func testConfig(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "clusters.csv")

	csv := "name,provisioned_vcpu,peak_vcpu,base_vcpu,peak_from,peak_to\na,10,5,1,9,11\n"
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatalf("writing the config: %v", err)
	}

	return path
}

func TestRootCommandHoldsTheSubcommands(t *testing.T) {
	var names []string
	for _, cmd := range rootCommand().Commands() {
		names = append(names, cmd.Name())
	}

	for _, want := range []string{"size", "profile", "peak", "compare"} {
		if !slices.Contains(names, want) {
			t.Errorf("expected a %q subcommand, got %v", want, names)
		}
	}
}

// Every subcommand is useless without a config, so none should get as far as
// running with one missing.
func TestSubcommandsRequireAConfig(t *testing.T) {
	for _, name := range []string{"size", "profile", "peak", "compare"} {
		t.Run(name, func(t *testing.T) {
			cmd := rootCommand()
			cmd.SetArgs([]string{name})
			cmd.SetOut(&strings.Builder{})
			cmd.SetErr(&strings.Builder{})

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), `required flag(s) "config" not set`) {
				t.Fatalf("expected the config to be required, got %v", err)
			}
		})
	}
}

// The root command on its own has nothing to report, so it prints its help
// rather than failing.
func TestRootCommandPrintsHelp(t *testing.T) {
	var b strings.Builder

	cmd := rootCommand()
	cmd.SetArgs(nil)
	cmd.SetOut(&b)
	cmd.SetErr(&b)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"size", "profile", "peak", "compare"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("expected the help to mention %q, got:\n%s", want, b.String())
		}
	}
}
