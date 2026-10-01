//go:build !darwin && !linux

package game

import (
	"context"
	"errors"
)

func runCommand(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	return nil, errors.New("Tarigato process control requires macOS or Linux")
}

func runCommandInput(ctx context.Context, dir string, env []string, input, name string, args ...string) ([]byte, error) {
	return runCommand(ctx, dir, env, name, args...)
}
