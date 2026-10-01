package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tmls-ai/tarigato/internal/game"
)

const version = "0.1.0-dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("tarigato", flag.ContinueOnError)
	flags.SetOutput(stderr)
	builder := flags.String("builder", "codex", "builder CLI: codex or claude")
	challenger := flags.String("challenger", "codex", "challenger CLI: codex or claude")
	timeout := flags.Duration("timeout", 30*time.Minute, "total execution deadline")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprint(stderr, "TARIGATO\n\nOne builds. One challenges. Tests settle the challenge.\n\nUsage: tarigato [options] \"task\"\n\nExample:\n  tarigato \"Reject tokens at their exact expiry time\"\n\nRun inside a clean, committed Go repository.\nOptions must come before the task.\n\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(out, "tarigato", version)
		return 0
	}
	if flags.NArg() != 1 || *timeout <= 0 {
		flags.Usage()
		return 2
	}
	for _, name := range []string{*builder, *challenger} {
		if name != "codex" && name != "claude" {
			fmt.Fprintln(stderr, "agents must be codex or claude")
			return 2
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signals, *timeout)
	defer cancel()
	ui := newTerminal(out, stderr, terminalColor(out, stderr))
	ui.Start(flags.Arg(0), *builder, *challenger)
	result, err := game.Run(ctx, game.Options{Repo: cwd, Task: flags.Arg(0), BuilderName: *builder, ChallengerName: *challenger, Progress: ui.Event})
	if err != nil && result.Reason == "" {
		result.Reason = err.Error()
	}
	ui.Finish(result)
	return result.ExitCode()
}
