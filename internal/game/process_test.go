//go:build darwin || linux

package game

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCommand(t *testing.T) {
	dir := t.TempDir()
	output, err := runCommand(context.Background(), dir, []string{"CHECK_VALUE=present"}, "/bin/sh", "-c", `printf '%s' "$CHECK_VALUE"; printf ':stderr' >&2`)
	if err != nil || string(output) != "present:stderr" {
		t.Fatalf("combined output: %q %v", output, err)
	}
	_, err = runCommand(context.Background(), dir, nil, "/bin/sh", "-c", "exit 7")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("lost exit status: %v", err)
	}
	_, err = runCommand(context.Background(), dir, nil, "tarigato-missing-command")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("lost missing-executable error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	marker := filepath.Join(dir, "survived")
	start := time.Now()
	_, err = runCommand(ctx, dir, nil, "/bin/sh", "-c", `(sleep 0.2; printf survived > "$1") & wait`, "sh", marker)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatalf("deadline did not stop process tree promptly: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descendant survived cancellation: %v", err)
	}
	_, err = runCommand(context.Background(), dir, nil, "/bin/sh", "-c", `(sleep 0.2; printf survived > "$1") &`, "sh", marker)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descendant survived parent completion: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err = runCommand(ctx, dir, nil, "/bin/sh", "-c", `yes output`)
	if !errors.Is(err, ErrOutputLimit) || len(output) != maxCommandOutput || !strings.HasPrefix(string(output[:7]), "output") {
		t.Fatalf("output limit: bytes=%d error=%v", len(output), err)
	}
}
