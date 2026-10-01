package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tmls-ai/tarigato/internal/game"
)

const (
	violet = "\x1b[1;38;5;141m"
	teal   = "\x1b[38;5;79m"
	amber  = "\x1b[38;5;222m"
	dim    = "\x1b[38;5;245m"
)

type terminal struct {
	out, progress io.Writer
	color         bool
	mu            sync.Mutex
	stage         string
	started       time.Time
	moveStarted   time.Time
	frame         int
	stop, stopped chan struct{}
}

func newTerminal(out, progress io.Writer, color bool) *terminal {
	return &terminal{out: out, progress: progress, color: color, started: time.Now()}
}

func terminalColor(writers ...io.Writer) bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" || os.Getenv("TERM") == "" {
		return false
	}
	for _, writer := range writers {
		f, ok := writer.(*os.File)
		if !ok {
			return false
		}
		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

func (u *terminal) paint(color, text string) string {
	if u.color {
		return color + text + "\x1b[0m"
	}
	return text
}

func (u *terminal) Start(task, builder, challenger string) {
	fmt.Fprintf(u.progress, "\n  %s  %s\n  One builds. One challenges. Tests settle it.\n\n", u.paint(violet, "T A R I G A T O"), u.paint(dim, "/ TMLS.NYC"))
	fmt.Fprintf(u.progress, "  TASK  %s\n\n", shortText(task, 68))
	fmt.Fprintf(u.progress, "  %s %s  /  %s %s  /  1 repair max\n  %s\n", u.paint(teal, "BUILDER"), cleanText(builder), u.paint(amber, "CHALLENGER"), cleanText(challenger), u.paint(dim, strings.Repeat("-", 70)))
	if u.color {
		u.stop, u.stopped = make(chan struct{}), make(chan struct{})
		go func() {
			defer close(u.stopped)
			tick := time.NewTicker(100 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-u.stop:
					return
				case <-tick.C:
					u.mu.Lock()
					u.spin()
					u.mu.Unlock()
				}
			}
		}()
	}
}

func (u *terminal) Event(event game.Event) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if event.Kind == "start" {
		u.stage, u.moveStarted = event.Stage, time.Now()
		if u.color {
			u.spin()
		} else {
			fmt.Fprintf(u.progress, "  > %-12s %s\n", stageLabel(event.Stage), shortText(event.Detail, 60))
		}
		return
	}
	u.clear()
	u.stage = ""
	mark, color := "!", amber
	switch event.Outcome {
	case "pass", "completed", "test", "no_finding":
		mark, color = "+", teal
	}
	if u.color && mark == "+" {
		mark = "✓"
	}
	detail := event.Detail
	if detail == "" {
		detail = strings.ReplaceAll(event.Outcome, "_", " ")
		switch event.Stage + ":" + event.Outcome {
		case "builder:completed":
			detail = "Candidate ready for checks"
		case "challenger:test":
			detail = "One test submitted"
		case "challenger:no_finding":
			detail = "No counterexample submitted"
		case "repair:completed":
			detail = "One repair prepared"
		}
	}
	fmt.Fprintf(u.progress, "  %s %-12s %-43s %s\n", u.paint(color, mark), u.paint(stageColor(event.Stage), fmt.Sprintf("%-12s", stageLabel(event.Stage))), shortText(detail, 43), u.paint(dim, elapsed(time.Since(u.moveStarted))))
}

func (u *terminal) spin() {
	if u.stage == "" {
		return
	}
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	u.clear()
	fmt.Fprintf(u.progress, "  %s %-12s %s", u.paint(stageColor(u.stage), string(frames[u.frame%len(frames)])), u.paint(stageColor(u.stage), stageLabel(u.stage)), u.paint(dim, elapsed(time.Since(u.moveStarted))))
	u.frame++
}

func (u *terminal) clear() {
	if u.color {
		fmt.Fprint(u.progress, "\r\x1b[2K")
	}
}

func (u *terminal) Finish(result game.Result) {
	if u.stop != nil {
		close(u.stop)
		<-u.stopped
	}
	u.clear()
	color := amber
	if result.Status == "ready_for_review" {
		color = teal
	}
	fmt.Fprintf(u.out, "\n  %s  %s\n", u.paint(color, strings.ToUpper(strings.ReplaceAll(cleanText(result.Status), "_", " "))), u.paint(dim, elapsed(time.Since(u.started))))
	fmt.Fprintf(u.out, "  %s\n", cleanText(result.Reason))
	if result.Directory != "" {
		path := filepath.Join(result.Directory, "report.md")
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(os.PathSeparator)) {
			path = "~" + strings.TrimPrefix(path, home)
		}
		fmt.Fprintf(u.out, "\n  REPORT   %s\n", cleanText(path))
		if _, exists := result.Artifacts["changes.patch"]; exists {
			fmt.Fprintf(u.out, "  PATCHES  changes.patch + tests.patch\n")
		}
	}
	fmt.Fprintln(u.out)
}

func stageLabel(stage string) string {
	return strings.ToUpper(strings.ReplaceAll(cleanText(stage), "-", " "))
}

func stageColor(stage string) string {
	switch stage {
	case "builder", "repair":
		return teal
	case "challenger", "challenge-1", "challenge-2":
		return amber
	default:
		return dim
	}
}

func elapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// Agent text must never be interpreted as terminal escape sequences.
func cleanText(text string) string {
	var b strings.Builder
	for _, r := range text {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func shortText(text string, limit int) string {
	runes := []rune(cleanText(text))
	if len(runes) > limit {
		return string(runes[:limit-3]) + "..."
	}
	return string(runes)
}
