package game

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Repo, Task, OutputDir       string
	BuilderName, ChallengerName string
	Builder, Challenger         AgentFunc
	Progress                    io.Writer
}

type Observation struct {
	Stage string `json:"stage"`
	checkResult
}

type Result struct {
	Status           string            `json:"status"`
	Reason           string            `json:"reason"`
	Task             string            `json:"task"`
	Directory        string            `json:"directory"`
	Base             string            `json:"base,omitempty"`
	Builder          string            `json:"builder"`
	Challenger       string            `json:"challenger"`
	GoVersion        string            `json:"go_version,omitempty"`
	ProviderVersions map[string]string `json:"provider_versions,omitempty"`
	Started          time.Time         `json:"started"`
	Finished         time.Time         `json:"finished"`
	RepairAttempted  bool              `json:"repair_attempted"`
	Challenge        *challenge        `json:"challenge,omitempty"`
	Observations     []Observation     `json:"observations"`
	Artifacts        map[string]string `json:"artifact_sha256"`
}

func (r Result) ExitCode() int {
	switch r.Status {
	case "ready_for_review":
		return 0
	case "needs_review":
		return 1
	case "blocked":
		return 2
	case "interrupted":
		return 130
	default:
		return 3
	}
}

// Run keeps every move in disposable clones and leaves only review artifacts.
func Run(ctx context.Context, opts Options) (r Result, err error) {
	r = Result{Status: "error", Task: opts.Task, Builder: opts.BuilderName, Challenger: opts.ChallengerName, Started: time.Now().UTC(), Artifacts: map[string]string{}}
	if opts.OutputDir == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return r, e
		}
		opts.OutputDir = filepath.Join(home, ".tarigato", "runs")
	}
	opts.OutputDir, err = resolveDestination(opts.OutputDir)
	if err != nil {
		return r, err
	}
	if root, e := git(ctx, opts.Repo, "rev-parse", "--show-toplevel"); e == nil {
		if rel, e := filepath.Rel(strings.TrimSpace(string(root)), opts.OutputDir); e == nil && filepath.IsLocal(rel) {
			r.Status, r.Reason = "blocked", "output directory must be outside the source repository"
			return r, nil
		}
	}
	if err = os.MkdirAll(opts.OutputDir, 0700); err != nil {
		return r, err
	}
	r.Directory, err = os.MkdirTemp(opts.OutputDir, time.Now().UTC().Format("20060102T150405Z-"))
	if err != nil {
		return r, err
	}
	defer func() {
		if err != nil {
			r.Reason = err.Error()
			switch {
			case errors.Is(err, context.Canceled):
				r.Status = "interrupted"
			case errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrOutputLimit):
				r.Status = "blocked"
			default:
				r.Status = "error"
			}
		}
		r.Finished = time.Now().UTC()
		if e := writeResult(r); e != nil {
			err = errors.Join(err, e)
			r.Status = "error"
			r.Reason = err.Error()
		}
	}()
	stop := func(status, reason string) (Result, error) { r.Status, r.Reason = status, reason; return r, nil }
	if strings.TrimSpace(opts.Task) == "" {
		return stop("blocked", "task must not be empty")
	}
	source, base, e := repository(ctx, opts.Repo)
	if e != nil {
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		return stop("blocked", e.Error())
	}
	r.Base = base
	// Storing run artifacts inside the input repository would alter the user's checkout.
	if rel, e := filepath.Rel(source, r.Directory); e == nil && filepath.IsLocal(rel) {
		return stop("blocked", "output directory must be outside the source repository")
	}
	version, e := runCommand(ctx, source, checkEnv(), "go", "version")
	if e != nil {
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		return stop("blocked", "Go 1.27.1 or newer must be on PATH")
	}
	r.GoVersion = strings.TrimSpace(string(version))
	match := regexp.MustCompile(`go1\.([0-9]+)(?:\.([0-9]+))?`).FindStringSubmatch(r.GoVersion)
	minor, patch := 0, 0
	if len(match) > 1 {
		minor, _ = strconv.Atoi(match[1])
		patch, _ = strconv.Atoi(match[2])
	}
	if minor < 27 || (minor == 27 && patch < 1) {
		return stop("blocked", "Go 1.27.1 or newer is required for assertion evidence")
	}
	r.ProviderVersions = map[string]string{}
	for _, provider := range []struct {
		name  string
		actor *AgentFunc
	}{{opts.BuilderName, &opts.Builder}, {opts.ChallengerName, &opts.Challenger}} {
		if *provider.actor != nil {
			continue
		}
		*provider.actor, e = CLIActor(provider.name)
		if e != nil {
			return stop("blocked", e.Error())
		}
		if _, seen := r.ProviderVersions[provider.name]; seen {
			continue
		}
		out, e := runCommand(ctx, source, nil, provider.name, "--version")
		if e != nil {
			return r, e
		}
		r.ProviderVersions[provider.name] = strings.TrimSpace(string(out))
	}
	work := filepath.Join(r.Directory, "workspaces")
	if e = os.Mkdir(work, 0700); e != nil {
		return r, e
	}
	defer func() {
		if r.Status == "ready_for_review" {
			err = errors.Join(err, os.RemoveAll(work))
		}
	}()
	if e = os.Mkdir(filepath.Join(r.Directory, "logs"), 0700); e != nil {
		return r, e
	}
	savePatch := func(name string, p []byte) error {
		if e := os.WriteFile(filepath.Join(r.Directory, name), p, 0600); e != nil {
			return e
		}
		r.Artifacts[name] = digest(p)
		return nil
	}
	check := func(stage, dir string, want *challenge, original []testID) (checkResult, error) {
		if opts.Progress != nil {
			fmt.Fprintln(opts.Progress, stage)
		}
		result, e := checkWorkspace(ctx, dir, want, original)
		r.Observations = append(r.Observations, Observation{stage, result})
		path := "logs/" + stage + ".jsonl"
		if writeErr := savePatch(path, result.Output); writeErr != nil {
			return result, writeErr
		}
		return result, e
	}
	move := func(actor AgentFunc, role, dir, evidence string) (AgentResponse, error) {
		if opts.Progress != nil {
			fmt.Fprintln(opts.Progress, role)
		}
		response, e := actor(ctx, AgentRequest{role, dir, opts.Task, evidence})
		if e != nil {
			var exit *exec.ExitError
			var invocation *exec.Error
			if errors.As(e, &exit) || errors.As(e, &invocation) {
				return AgentResponse{Status: "blocked", Summary: e.Error()}, nil
			}
			return response, e
		}
		// Fake/test actors and provider actors obey the same protocol boundary.
		if response.Status == "blocked" {
			return response, nil
		}
		if role == "challenger" {
			if response.Status != "test" && response.Status != "no_finding" {
				return response, errors.New("challenger did not complete its move")
			}
		} else if response.Status != "completed" {
			return response, errors.New("builder did not complete its move")
		}
		return response, nil
	}
	builderDir := filepath.Join(work, "builder")
	if e = cloneAt(ctx, source, base, builderDir, nil); e != nil {
		return r, e
	}
	baselineFiles, e := snapshot(builderDir)
	if e != nil {
		return stop("blocked", e.Error())
	}
	original, e := check("baseline", builderDir, nil, nil)
	if e != nil {
		return r, e
	}
	if !original.Passed || len(original.Tests) == 0 {
		return stop("blocked", "baseline must pass and execute at least one named test: "+original.Reason)
	}
	response, e := move(opts.Builder, "builder", builderDir, "")
	if e != nil {
		return r, e
	}
	if response.Status == "blocked" {
		return stop("blocked", "builder blocked: "+response.Summary)
	}
	candidateFiles, e := snapshot(builderDir)
	if e != nil {
		return stop("needs_review", e.Error())
	}
	if e = productionOnly(baselineFiles, candidateFiles); e != nil {
		return stop("needs_review", e.Error())
	}
	candidate, e := capturePatch(ctx, builderDir, base)
	if e != nil {
		return r, e
	}
	if e = savePatch("candidate.patch", candidate); e != nil {
		return r, e
	}
	if e = savePatch("changes.patch", candidate); e != nil {
		return r, e
	}
	if e = savePatch("tests.patch", nil); e != nil {
		return r, e
	}
	candidateDir := filepath.Join(work, "candidate")
	if e = cloneAt(ctx, source, base, candidateDir, candidate); e != nil {
		return r, e
	}
	reconstructed, e := snapshot(candidateDir)
	if e != nil {
		return r, e
	}
	if !reflect.DeepEqual(candidateFiles, reconstructed) {
		return stop("needs_review", "candidate patch does not reproduce the builder files (possibly ignored changes)")
	}
	candidateCheck, e := check("candidate", candidateDir, nil, original.Tests)
	if e != nil {
		return r, e
	}
	if !candidateCheck.Passed {
		return stop("needs_review", "candidate failed original checks: "+candidateCheck.Reason)
	}
	challengerDir := filepath.Join(work, "challenger")
	if e = cloneAt(ctx, source, base, challengerDir, candidate); e != nil {
		return r, e
	}
	response, e = move(opts.Challenger, "challenger", challengerDir, "")
	if e != nil {
		return r, e
	}
	if response.Status == "blocked" {
		return stop("blocked", "challenger blocked: "+response.Summary)
	}
	challengedFiles, e := snapshot(challengerDir)
	if e != nil {
		return stop("needs_review", e.Error())
	}
	changed := changedFiles(candidateFiles, challengedFiles)
	var testPatch []byte
	finalPatch := candidate
	if response.Status == "no_finding" {
		if len(changed) != 0 || response.File != "" || response.Test != "" {
			return stop("needs_review", "challenger reported no finding but changed files or supplied a test")
		}
	} else {
		if len(changed) != 1 || changed[0] != response.File {
			return stop("needs_review", "challenger must add exactly its one reported test file")
		}
		if _, exists := candidateFiles[response.File]; exists {
			return stop("needs_review", "challenger changed an existing file")
		}
		if challengedFiles[response.File].Mode != 0 {
			return stop("needs_review", "challenge test must not be executable")
		}
		c, e := validateChallenge(challengerDir, response.File)
		if e != nil {
			return stop("needs_review", e.Error())
		}
		if c.TestName != response.Test {
			return stop("needs_review", "reported test name differs from the source")
		}
		r.Challenge = &c
		testPatch, e = capturePatch(ctx, challengerDir, base, c.Path)
		if e != nil {
			return r, e
		}
		if e = savePatch("tests.patch", testPatch); e != nil {
			return r, e
		}
		outcomes := make([]checkResult, 2)
		for i := range outcomes {
			dir := filepath.Join(work, fmt.Sprintf("reproduce-%d", i+1))
			if e = cloneAt(ctx, source, base, dir, append(append([]byte{}, candidate...), testPatch...)); e != nil {
				return r, e
			}
			outcomes[i], e = check(fmt.Sprintf("challenge-%d", i+1), dir, &c, original.Tests)
			if e != nil {
				return r, e
			}
		}
		if outcomes[0].Expected != outcomes[1].Expected || (outcomes[0].Expected != "pass" && outcomes[0].Expected != "fail") {
			return stop("needs_review", "challenge outcomes are invalid or inconsistent; inspect test logs")
		}
		if outcomes[0].Expected == "fail" {
			repairDir := filepath.Join(work, "repair")
			if e = cloneAt(ctx, source, base, repairDir, append(append([]byte{}, candidate...), testPatch...)); e != nil {
				return r, e
			}
			frozen, e := snapshot(repairDir)
			if e != nil {
				return r, e
			}
			r.RepairAttempted = true
			evidence := fmt.Sprintf("Frozen challenge: %s, test %s. It failed twice while original tests passed.\n%s", c.Path, c.TestName, string(outcomes[1].Output))
			response, e = move(opts.Builder, "repair", repairDir, evidence)
			if e != nil {
				return r, e
			}
			if response.Status == "blocked" {
				return stop("blocked", "repair blocked: "+response.Summary)
			}
			repaired, e := snapshot(repairDir)
			if e != nil {
				return stop("needs_review", e.Error())
			}
			if e = productionOnly(frozen, repaired); e != nil {
				return stop("needs_review", e.Error())
			}
			// Capture the production diff separately from the frozen test diff.
			if e = os.Remove(filepath.Join(repairDir, c.Path)); e != nil {
				return r, e
			}
			finalPatch, e = capturePatch(ctx, repairDir, base)
			if e != nil {
				return r, e
			}
			if e = savePatch("changes.patch", finalPatch); e != nil {
				return r, e
			}
		}
	}
	finalDir := filepath.Join(work, "final")
	if e = cloneAt(ctx, source, base, finalDir, append(append([]byte{}, finalPatch...), testPatch...)); e != nil {
		return r, e
	}
	finalCheck, e := check("final", finalDir, r.Challenge, original.Tests)
	if e != nil {
		return r, e
	}
	if !finalCheck.Passed {
		return stop("needs_review", "final checks failed: "+finalCheck.Reason)
	}
	// Verify the original checkout did not change while external processes ran.
	current, e := git(ctx, source, "rev-parse", "HEAD")
	if e != nil {
		return r, e
	}
	if strings.TrimSpace(string(current)) != base {
		return stop("needs_review", "source checkout moved during the run")
	}
	status, e := git(ctx, source, "status", "--porcelain=v1", "--untracked-files=all")
	if e != nil {
		return r, e
	}
	if len(status) != 0 {
		return stop("needs_review", "source checkout changed during the run")
	}
	return stop("ready_for_review", "challenge completed and final checks passed; review the test and patches")
}
