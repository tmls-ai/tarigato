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

func TestAgentResponses(t *testing.T) {
	for _, tc := range []struct {
		role, data string
		valid      bool
	}{
		{"builder", `{"status":"completed","file":"","test":"","summary":"done"}`, true},
		{"challenger", `{"status":"no_finding","file":"","test":"","summary":"none"}`, true},
		{"challenger", `{"status":"test","file":"pkg/tarigato_challenge_test.go","test":"TestBoundary","summary":"boundary"}`, true},
		{"repair", `{"status":"blocked","file":"","test":"","summary":"permission"}`, true},
		{"challenger", `{"status":"completed","file":"","test":"","summary":"done"}`, false},
		{"challenger", `{"status":"test","file":"../tarigato_challenge_test.go","test":"TestBoundary","summary":"bad"}`, false},
		{"builder", `{"status":"completed","file":"","test":"","summary":"done","extra":true}`, false},
		{"builder", `{"status":"completed"} {}`, false},
		{"builder", `{"status":"completed"}`, false},
		{"builder", `{"status":"completed","file":null,"test":"","summary":"done"}`, false},
		{"builder", `I finished the work.`, false},
	} {
		_, err := decodeAgentResponse(tc.role, []byte(tc.data))
		if (err == nil) != tc.valid {
			t.Errorf("%s: valid=%v, error=%v", tc.data, tc.valid, err)
		}
	}
	good := `{"type":"result","subtype":"success","is_error":false,"structured_output":{"status":"completed","file":"","test":"","summary":"done"}}`
	for _, output := range []string{good, "provider warning\n" + good} {
		if _, err := claudeResponse([]byte(output)); err != nil {
			t.Fatal(err)
		}
	}
	for _, output := range []string{good + "\n" + good, `{"type":"result","subtype":"error","is_error":true}`, `{"type":"result","subtype":"success","result":"completed"}`} {
		if _, err := claudeResponse([]byte(output)); err == nil {
			t.Errorf("accepted invalid provider envelope %s", output)
		}
	}
	data, err := claudeResponse([]byte(`{"type":"result","permission_denials":[{}]}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := decodeAgentResponse("builder", data)
	if err != nil || response.Status != "blocked" {
		t.Fatalf("denied tool: %+v %v", response, err)
	}
}

func TestCLIActorWithFakeExecutables(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"codex", "claude"} {
		body := `#!/bin/sh
set -eu
test -n "$GOCACHE"
mkdir -p "$GOCACHE"
printf '%s\n' "$@" > args
cat > prompt
response='{"status":"completed","file":"","test":"","summary":"done"}'
`
		if name == "codex" {
			body += `while [ "$#" -gt 0 ]; do
  if [ "$1" = --output-last-message ]; then shift; printf '%s' "$response" > "$1"; fi
  shift
done
`
		} else {
			body += `printf 'provider warning\n' >&2
printf '{"type":"result","subtype":"success","is_error":false,"structured_output":%s}\n' "$response"
`
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"codex", "claude"} {
		actor, err := CLIActor(name)
		if err != nil {
			t.Fatal(err)
		}
		response, err := actor(context.Background(), AgentRequest{Role: "builder", Dir: dir, Task: "Handle an expiry boundary."})
		if err != nil || response.Status != "completed" {
			t.Fatalf("%s: %+v %v", name, response, err)
		}
		args, _ := os.ReadFile(filepath.Join(dir, "args"))
		prompt, _ := os.ReadFile(filepath.Join(dir, "prompt"))
		if strings.Contains(string(args), "expiry") || strings.Contains(string(args), "dangerously") || !strings.Contains(string(prompt), "expiry") {
			t.Fatalf("%s: prompt must travel on stdin with no permission bypass", name)
		}
		if name == "codex" && !strings.Contains(string(args), "--sandbox\nworkspace-write\n--ephemeral") {
			t.Fatalf("codex protection flags absent: %s", args)
		}
		if name == "claude" && !strings.Contains(string(args), "--permission-mode\ndefault\n--no-session-persistence") {
			t.Fatalf("claude protection flags absent: %s", args)
		}
	}
	if _, err := CLIActor("unknown"); err == nil {
		t.Fatal("accepted unknown agent")
	}
	var schema any
	if err := json.Unmarshal([]byte(agentSchema), &schema); err != nil {
		t.Fatal(err)
	}
}
