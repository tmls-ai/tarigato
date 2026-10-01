package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeResult(r Result) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Tarigato result\n\n**%s** — %s\n\nTask: %s\n\nBase: `%s`\n\nBuilder: `%s`; challenger: `%s`; repair attempted: `%t`.\n\nToolchain: `%s`.\n\n", r.Status, r.Reason, r.Task, r.Base, r.Builder, r.Challenger, r.RepairAttempted, r.GoVersion)
	b.WriteString("## Checks\n\n| Stage | Suite passed | Challenge | Detail |\n|---|---|---|---|\n")
	for _, o := range r.Observations {
		fmt.Fprintf(&b, "| %s | %t | %s | %s |\n", o.Stage, o.Passed, o.Expected, strings.ReplaceAll(o.Reason, "|", "/"))
	}
	b.WriteString("\nLogs and result.json contain the observations and patch hashes. No finding does not prove correctness. Review the requirement, test expectation, and source diff before applying anything.\n")
	if _, ok := r.Artifacts["changes.patch"]; ok {
		fmt.Fprintf(&b, "\n## Replay\n\nUse a separate clean checkout at base `%s`. Candidate and final patches are alternatives. Use the same Go toolchain and tooling environment as the original run. The command below filters the environment like the controller; see result.json and logs for observed test identities.\n\nFinal source:\n\n```sh\ngit checkout --detach %s\n", r.Base, quoteShell(r.Base))
		// Empty patches are valid artifacts for no-op source changes.
		for _, name := range []string{"changes.patch", "tests.patch"} {
			path := filepath.Join(r.Directory, name)
			fmt.Fprintf(&b, "test ! -s %s || git apply %s\n", quoteShell(path), quoteShell(path))
		}
		b.WriteString(replayCommand())
		b.WriteString("\n```\n\nTo replay the candidate, start from another clean checkout at the same base and substitute `candidate.patch` for `changes.patch`. Apply `tests.patch` there too. Never apply both source patches in sequence.\n")
	}
	if err = os.WriteFile(filepath.Join(r.Directory, "report.md"), []byte(b.String()), 0600); err != nil {
		return err
	}
	// result.json is the completion marker, published after all other artifacts.
	temp := filepath.Join(r.Directory, ".result.json.tmp")
	if err = os.WriteFile(temp, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(r.Directory, "result.json"))
}

func quoteShell(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// Shell parameter expansion preserves only the same tooling variables as checks.
func replayCommand() string {
	var b strings.Builder
	b.WriteString("env -i")
	for _, key := range checkEnvKeys {
		fmt.Fprintf(&b, " ${%s+\"%s=$%s\"}", key, key, key)
	}
	for _, setting := range checkEnvFixed {
		b.WriteString(" " + quoteShell(setting))
	}
	b.WriteString(" go test -json -count=1 ./...")
	return b.String()
}
