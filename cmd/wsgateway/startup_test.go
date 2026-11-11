package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestOptionalStartup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{"default", nil, "browser gateway is disabled; use -enable to opt in"},
		{"explicitly_disabled", []string{"-enable=false"}, "browser gateway is disabled; use -enable to opt in"},
		{"explicit_opt_in", []string{"-enable", "-target=invalid"}, "invalid default target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOptionalStartupProcess$", "--")
			cmd.Args = append(cmd.Args, tc.args...)
			cmd.Env = append(os.Environ(), "WLO_OPTIONAL_STARTUP_TEST=1", "WLO_AGENT_TOKEN="+strings.Repeat("x", 32))
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("startup did not reject configuration promptly: %s", output)
			}
			if err == nil {
				t.Fatalf("expected startup rejection: %s", output)
			}
			if !strings.Contains(string(output), tc.message) {
				t.Fatalf("expected %q, got %s", tc.message, output)
			}
		})
	}
}

// Exercise real command startup without opening listeners or launching Chrome.
func TestOptionalStartupProcess(t *testing.T) {
	if os.Getenv("WLO_OPTIONAL_STARTUP_TEST") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"wsgateway"}, os.Args[i+1:]...)
			main()
			return
		}
	}
	t.Fatal("missing command arguments")
}
