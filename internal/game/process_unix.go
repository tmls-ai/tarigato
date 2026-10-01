//go:build darwin || linux

package game

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func runCommand(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	return runCommandInput(ctx, dir, env, "", name, args...)
}

func runCommandInput(ctx context.Context, dir string, env []string, input, name string, args ...string) ([]byte, error) {
	limited, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(limited, name, args...)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdin = strings.NewReader(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	// A descendant holding a pipe open must not keep a finished command alive.
	cmd.WaitDelay = time.Second
	output := &commandOutput{cancel: cancel}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	cmd.Stdout, cmd.Stderr = writer, writer
	drained := make(chan error, 1)
	go func() {
		_, err := io.Copy(output, reader)
		drained <- err
	}()
	err = cmd.Run()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	_ = writer.Close()
	_ = reader.SetReadDeadline(time.Now().Add(time.Second))
	if drainErr := <-drained; err == nil {
		err = drainErr
	}
	if output.exceeded {
		err = ErrOutputLimit
	} else if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		err = fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return output.buffer.Bytes(), err
}

type commandOutput struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	exceeded bool
}

func (b *commandOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := maxCommandOutput - b.buffer.Len()
	if n > remaining {
		p = p[:remaining]
		b.exceeded = true
		b.cancel()
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}
