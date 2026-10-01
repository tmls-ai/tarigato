package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tmls-ai/tarigato/internal/game"
)

func TestTerminal(t *testing.T) {
	for _, color := range []bool{false, true} {
		var output bytes.Buffer
		u := newTerminal(&output, &output, color)
		u.Start("Reject expired tokens\x1b[2J\n", "codex", "claude")
		u.Event(game.Event{Stage: "builder", Kind: "start"})
		if color {
			// Let the animated writer run, then prove Finish joins it.
			time.Sleep(120 * time.Millisecond)
		}
		u.Event(game.Event{Stage: "builder", Kind: "done", Outcome: "completed"})
		u.Event(game.Event{Stage: "challenger", Kind: "start"})
		u.Event(game.Event{Stage: "challenger", Kind: "done", Outcome: "test"})
		u.Event(game.Event{Stage: "challenge-1", Kind: "start"})
		u.Event(game.Event{Stage: "challenge-1", Kind: "done", Outcome: "fail", Detail: "assertion failed"})
		u.Event(game.Event{Stage: "repair", Kind: "start"})
		u.Event(game.Event{Stage: "repair", Kind: "done", Outcome: "completed"})
		u.Finish(game.Result{Status: "needs_review", Reason: "test skipped\r\x1b]52;c;unsafe\a", Directory: "/tmp/run", Artifacts: map[string]string{"changes.patch": "hash"}})
		text := output.String()
		for _, want := range []string{"T A R I G A T O", "TMLS.NYC", "BUILDER", "CHALLENGER", "Candidate ready for checks", "One test submitted", "One repair prepared", "CHALLENGE 1", "assertion failed", "NEEDS REVIEW", "report.md", "changes.patch + tests.patch", `\u001b[2J\u000a`, `\u000d\u001b]52;c;unsafe\u0007`} {
			if !strings.Contains(text, want) {
				t.Errorf("color=%t: missing %q in %q", color, want, text)
			}
		}
		if strings.Contains(text, "\x1b[2J") || strings.Contains(text, "\x1b]52") {
			t.Fatal("untrusted terminal controls reached the output")
		}
		if !color && strings.ContainsAny(text, "\x1b\r") {
			t.Fatal("plain output contains terminal controls")
		}
		if color && !strings.Contains(text, violet) {
			t.Fatal("color output missing title styling")
		}
	}
}

func TestTerminalColorDisabled(t *testing.T) {
	var output bytes.Buffer
	t.Setenv("TERM", "xterm-256color")
	if terminalColor(&output) {
		t.Fatal("piped output must be plain")
	}
	t.Setenv("TERM", "dumb")
	if terminalColor() {
		t.Fatal("TERM=dumb must disable styling")
	}
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	if terminalColor() {
		t.Fatal("even an empty NO_COLOR must disable styling")
	}
}

func TestTerminalDoesNotMarkMissingEvidenceAsSuccess(t *testing.T) {
	for _, outcome := range []string{"absent", "inconclusive", "unknown"} {
		var output bytes.Buffer
		u := newTerminal(&output, &output, false)
		u.Event(game.Event{Stage: "final", Kind: "start"})
		u.Event(game.Event{Stage: "final", Kind: "done", Outcome: outcome})
		if !strings.Contains(output.String(), "  ! FINAL") {
			t.Fatalf("%s must display a warning: %s", outcome, output.String())
		}
	}
}
