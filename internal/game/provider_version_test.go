//go:build darwin || linux

package game

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderVersionFailureIsBlocked(t *testing.T) {
	for _, name := range []string{"codex", "claude"} {
		t.Run(name, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			for _, role := range []string{"builder", "challenger"} {
				t.Run(role, func(t *testing.T) {
					unexpectedMove := func(context.Context, AgentRequest) (AgentResponse, error) {
						t.Error("agent started after a failed provider version check")
						return AgentResponse{Status: "blocked"}, nil
					}
					opts := Options{Repo: fixtureRepo(t), Task: "Synthetic provider check", OutputDir: t.TempDir(), BuilderName: "fake", ChallengerName: "fake", Builder: unexpectedMove, Challenger: unexpectedMove}
					if role == "builder" {
						opts.BuilderName, opts.Builder = name, nil
					} else {
						opts.ChallengerName, opts.Challenger = name, nil
					}
					result, err := Run(context.Background(), opts)
					t.Logf("provider=%s role=%s status=%s exit=%d reason=%q", name, role, result.Status, result.ExitCode(), result.Reason)
					if err != nil || result.Status != "blocked" || result.ExitCode() != 2 {
						t.Fatalf("status=%s exit=%d err=%v; want blocked, exit 2, and no controller error", result.Status, result.ExitCode(), err)
					}
					if !strings.Contains(result.Reason, name) || !strings.Contains(result.Reason, "exit status 7") {
						t.Fatalf("missing provider failure detail: %q", result.Reason)
					}
					if len(result.Observations) != 0 {
						t.Fatal("checks started after a failed provider version check")
					}
					data, err := os.ReadFile(filepath.Join(result.Directory, "result.json"))
					if err != nil {
						t.Fatal(err)
					}
					var saved Result
					if err := json.Unmarshal(data, &saved); err != nil {
						t.Fatal(err)
					}
					if saved.Status != result.Status || saved.Reason != result.Reason {
						t.Fatal("saved provider failure differs from the returned result")
					}
				})
			}
		})
	}
}
