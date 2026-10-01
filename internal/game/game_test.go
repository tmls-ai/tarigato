package game

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const originalSource = "package fixture\nfunc Valid(expiry, now int) bool { return expiry > now }\n"
const buggySource = "package fixture\nfunc Valid(expiry, now int) bool { return expiry >= now }\n"
const challengeSource = `package fixture
import "testing"
func TestExpiryBoundary(t *testing.T) {
 if Valid(10,10) { t.Fatal("expiry equal to now must be rejected") }
}
`

func writeFixture(t *testing.T, dir, path, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for p, s := range map[string]string{"go.mod": "module fixture\n\ngo 1.27.1\n", "session.go": originalSource, "session_test.go": "package fixture\nimport \"testing\"\nfunc TestFuture(t *testing.T) { if !Valid(20,10) {t.Fatal(\"future session must be valid\")} }\n"} {
		writeFixture(t, dir, p, s)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "."}, {"-c", "user.name=Tarigato Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "test fixture"}} {
		if out, err := git(context.Background(), dir, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func TestGame(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		repair       bool
	}{
		{"repair", "ready_for_review", true},
		{"no_finding", "ready_for_review", false},
		{"passing_challenge", "ready_for_review", false},
		{"failed_repair", "needs_review", true},
		{"frozen_test", "needs_review", true},
		{"protected_file", "needs_review", false},
		{"candidate_failure", "needs_review", false},
		{"invalid_challenge", "needs_review", false},
		{"unfinished_challenger", "error", false},
		{"permission_block", "blocked", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := fixtureRepo(t)
			before, err := snapshot(repo)
			if err != nil {
				t.Fatal(err)
			}
			builderCalls, challengerCalls := 0, 0
			builder := func(ctx context.Context, req AgentRequest) (AgentResponse, error) {
				builderCalls++
				if tc.name == "permission_block" {
					return AgentResponse{Status: "blocked"}, nil
				}
				if req.Role == "repair" {
					if !strings.Contains(req.Evidence, "TestExpiryBoundary") {
						t.Fatal("repair missing frozen evidence")
					}
					if tc.name == "frozen_test" {
						writeFixture(t, req.Dir, "tarigato_challenge_test.go", strings.ReplaceAll(challengeSource, "t.Fatal", "t.Log"))
					} else if tc.name != "failed_repair" {
						writeFixture(t, req.Dir, "session.go", originalSource)
					}
				} else {
					switch tc.name {
					case "protected_file":
						writeFixture(t, req.Dir, "go.mod", "module changed\n\ngo 1.27.1\n")
					case "candidate_failure":
						writeFixture(t, req.Dir, "session.go", "package fixture\nfunc Valid(expiry, now int) bool{return false}\n")
					case "no_finding", "passing_challenge":
						writeFixture(t, req.Dir, "session.go", originalSource+"// Clarified behavior.\n")
					default:
						writeFixture(t, req.Dir, "session.go", buggySource)
					}
				}
				return AgentResponse{Status: "completed"}, nil
			}
			challenger := func(ctx context.Context, req AgentRequest) (AgentResponse, error) {
				challengerCalls++
				if req.Evidence != "" {
					t.Fatal("challenger received private builder evidence")
				}
				if tc.name == "no_finding" {
					return AgentResponse{Status: "no_finding"}, nil
				}
				if tc.name == "unfinished_challenger" {
					return AgentResponse{}, nil
				}
				if tc.name == "invalid_challenge" {
					writeFixture(t, req.Dir, "session.go", originalSource)
				}
				writeFixture(t, req.Dir, "tarigato_challenge_test.go", challengeSource)
				return AgentResponse{Status: "test", File: "tarigato_challenge_test.go", Test: "TestExpiryBoundary"}, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			r, runErr := Run(ctx, Options{Repo: repo, Task: "Reject expired sessions", OutputDir: t.TempDir(), BuilderName: "fake", ChallengerName: "fake", Builder: builder, Challenger: challenger})
			if r.Status != tc.status || r.RepairAttempted != tc.repair {
				t.Fatalf("status=%s repair=%t reason=%s err=%v", r.Status, r.RepairAttempted, r.Reason, runErr)
			}
			if tc.status != "error" && runErr != nil {
				t.Fatal(runErr)
			}
			expectedCalls := 1
			if tc.repair {
				expectedCalls = 2
			}
			if builderCalls != expectedCalls || challengerCalls > 1 {
				t.Fatalf("unbounded moves: builder=%d challenger=%d", builderCalls, challengerCalls)
			}
			after, err := snapshot(repo)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("input repository changed")
			}
			data, err := os.ReadFile(filepath.Join(r.Directory, "result.json"))
			if err != nil {
				t.Fatal(err)
			}
			var saved Result
			if err = json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Status != r.Status {
				t.Fatal("stored result differs")
			}
			if _, err = os.Stat(filepath.Join(r.Directory, "workspaces")); r.Status == "ready_for_review" && !os.IsNotExist(err) {
				t.Fatal("temporary workspaces retained")
			}
			for name, hash := range r.Artifacts {
				data, err := os.ReadFile(filepath.Join(r.Directory, name))
				if err != nil || digest(data) != hash {
					t.Fatalf("artifact %s invalid: %v", name, err)
				}
			}
			if r.Status == "ready_for_review" {
				var combined []byte
				for _, name := range []string{"changes.patch", "tests.patch"} {
					data, err := os.ReadFile(filepath.Join(r.Directory, name))
					if err != nil {
						t.Fatal(err)
					}
					combined = append(combined, data...)
				}
				replay := filepath.Join(t.TempDir(), "replay")
				if err := cloneAt(ctx, repo, r.Base, replay, combined); err != nil {
					t.Fatal(err)
				}
				check, err := checkWorkspace(ctx, replay, r.Challenge, nil)
				if err != nil || !check.Passed {
					t.Fatalf("replay failed: %+v %v", check, err)
				}
			}
		})
	}
}

func TestRunGuardsAndCancellation(t *testing.T) {
	repo := fixtureRepo(t)
	forbidden := filepath.Join(repo, "runs")
	r, err := Run(context.Background(), Options{Repo: repo, Task: "task", OutputDir: forbidden})
	if err != nil || r.Status != "blocked" {
		t.Fatalf("inside output: %+v %v", r, err)
	}
	if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
		t.Fatal("created output inside source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	r, err = Run(ctx, Options{Repo: repo, Task: "task", OutputDir: t.TempDir(), Builder: func(ctx context.Context, req AgentRequest) (AgentResponse, error) {
		writeFixture(t, req.Dir, "session.go", buggySource)
		cancel()
		return AgentResponse{}, ctx.Err()
	}, Challenger: func(context.Context, AgentRequest) (AgentResponse, error) {
		t.Fatal("challenger after cancellation")
		return AgentResponse{}, nil
	}})
	if r.Status != "interrupted" || err == nil {
		t.Fatalf("cancel: %+v %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(r.Directory, "report.md")); err != nil {
		t.Fatal("missing interrupted report", err)
	}
	partial, err := os.ReadFile(filepath.Join(r.Directory, "workspaces", "builder", "session.go"))
	if err != nil || string(partial) != buggySource {
		t.Fatal("lost interrupted work", err)
	}
}
