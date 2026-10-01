package game

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type testID struct {
	Package string `json:"package"`
	Name    string `json:"name"`
}

type challenge struct {
	Path       string `json:"path"`
	PackageDir string `json:"package_dir"`
	TestName   string `json:"test_name"`
}

type checkResult struct {
	Passed   bool     `json:"passed"`
	Expected string   `json:"expected,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Tests    []testID `json:"tests"`
	Output   []byte   `json:"-"`
}

// validateChallenge accepts one ordinary test, deriving its identity from source.
// Workspace change validation separately enforces that this is the only new file.
func validateChallenge(root, path string) (challenge, error) {
	var c challenge
	if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) || strings.Contains(path, `\`) || filepath.Base(path) != "tarigato_challenge_test.go" {
		return c, errors.New("challenge must be a clean relative path named tarigato_challenge_test.go")
	}
	current := root
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return c, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (current == filepath.Join(root, path) && !info.Mode().IsRegular()) {
			return c, errors.New("challenge path must contain only directories and a regular file, without symlinks")
		}
	}
	f, err := parser.ParseFile(token.NewFileSet(), current, nil, parser.ParseComments)
	if err != nil {
		return c, fmt.Errorf("parse challenge: %w", err)
	}
	for _, group := range f.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:") || strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(comment.Text, "//")), "+build") {
				return c, errors.New("challenge may not contain build constraints or compiler directives")
			}
		}
	}
	testingAlias := ""
	for _, imp := range f.Imports {
		name, _ := strconv.Unquote(imp.Path.Value)
		if name == "testing" {
			testingAlias = "testing"
			if imp.Name != nil {
				testingAlias = imp.Name.Name
			}
		}
	}
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.GenDecl); ok && d.Tok == token.VAR {
			return c, errors.New("challenge may not declare package variables")
		}
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == "init" || fn.Name.Name == "TestMain" || goTestName(fn.Name.Name, "Benchmark") || goTestName(fn.Name.Name, "Fuzz") || goTestName(fn.Name.Name, "Example") {
			return c, errors.New("challenge may not declare init, TestMain, benchmarks, fuzz targets, or examples")
		}
		if !goTestName(fn.Name.Name, "Test") {
			continue
		}
		if c.TestName != "" || fn.Recv != nil || fn.Type.TypeParams != nil || fn.Type.Results != nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 || len(fn.Type.Params.List[0].Names) != 1 {
			return c, errors.New("challenge must declare exactly one func TestX(t *testing.T)")
		}
		ptr, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
		if !ok {
			return c, errors.New("challenge test parameter must be *testing.T")
		}
		sel, ok := ptr.X.(*ast.SelectorExpr)
		if !ok {
			return c, errors.New("challenge test parameter must be *testing.T")
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || testingAlias == "" || testingAlias == "." || testingAlias == "_" || pkg.Name != testingAlias || sel.Sel.Name != "T" {
			return c, errors.New("challenge test parameter must be *testing.T")
		}
		c.TestName = fn.Name.Name
	}
	if c.TestName == "" {
		return c, errors.New("challenge must declare one named Go test")
	}
	c.Path, c.PackageDir = path, filepath.Dir(path)
	return c, nil
}

func goTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(strings.TrimPrefix(name, prefix))
	return !unicode.IsLower(r)
}

// Checks receive tooling settings, not the agent provider's credential environment.
// GOENV/GOFLAGS/GOWORK are fixed so inherited configuration cannot filter tests.
var checkEnvKeys = []string{"PATH", "HOME", "USER", "TMPDIR", "TMP", "TEMP", "SystemRoot", "GOROOT", "GOPATH", "GOCACHE", "GOMODCACHE", "GOPROXY", "GOSUMDB", "GOPRIVATE", "GONOPROXY", "GONOSUMDB"}
var checkEnvFixed = []string{"GOENV=off", "GOFLAGS=", "GOWORK=off", "GO111MODULE=on", "GOTOOLCHAIN=local"}

func checkEnv() []string {
	var env []string
	for _, key := range checkEnvKeys {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return append(env, checkEnvFixed...)
}

func checkSuite(ctx context.Context, dir string, want *challenge, baseline []testID) (checkResult, error) {
	var expected *testID
	if want != nil {
		out, err := runCommand(ctx, dir, checkEnv(), "go", "list", "-f", "{{.ImportPath}}", "./"+filepath.ToSlash(want.PackageDir))
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 1 {
				return checkResult{Expected: "inconclusive", Reason: "cannot resolve challenge package", Output: out}, nil
			}
			return checkResult{Output: out}, err
		}
		pkg := strings.TrimSpace(string(out))
		if pkg == "" || strings.ContainsAny(pkg, "\r\n\t ") {
			return checkResult{Expected: "inconclusive", Reason: "ambiguous challenge package identity", Output: out}, nil
		}
		expected = &testID{Package: pkg, Name: want.TestName}
	}
	out, err := runCommand(ctx, dir, checkEnv(), "go", "test", "-json", "-count=1", "./...")
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return checkResult{Output: out}, err
		}
	}
	return parseChecks(out, expected, baseline, err == nil), nil
}

type testEvent struct {
	Action     string
	Package    string
	Test       string
	Output     string
	OutputType string
}

type testState struct {
	ran       bool
	outcome   string
	assertion bool
}

func parseChecks(out []byte, expected *testID, baseline []testID, processPassed bool) checkResult {
	r := checkResult{Output: out}
	if expected != nil {
		r.Expected = "inconclusive"
	}
	tests := make(map[testID]*testState)
	packages := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 4096), 8<<20)
	for scanner.Scan() {
		var e testEvent
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			// Dependency fetch notices go to stderr, mixed into the bounded log.
			if strings.HasPrefix(scanner.Text(), "go: downloading ") {
				continue
			}
			r.Reason = "check output contains unstructured diagnostics"
			return r
		}
		if e.Action == "build-output" {
			continue
		}
		if e.Action == "build-fail" {
			r.Reason = "Go package failed to build"
			return r
		}
		if e.Package == "" {
			r.Reason = "check event has no package identity"
			return r
		}
		if e.Test == "" {
			if e.Action == "start" {
				packages[e.Package] = "start"
			}
			if e.Action == "pass" || e.Action == "fail" || e.Action == "skip" {
				packages[e.Package] = e.Action
			}
			continue
		}
		id := testID{Package: e.Package, Name: e.Test}
		s := tests[id]
		if s == nil {
			s = &testState{}
			tests[id] = s
		}
		switch e.Action {
		case "run":
			if s.ran {
				r.Reason = "named test ran more than once"
				return r
			}
			s.ran = true
		case "pass", "fail", "skip":
			if !s.ran || s.outcome != "" {
				r.Reason = "named test has invalid execution events"
				return r
			}
			s.outcome = e.Action
		case "output":
			// Go 1.27 identifies testing.T errors separately from logs and frames.
			s.assertion = s.assertion || e.OutputType == "error"
		}
	}
	if scanner.Err() != nil || len(packages) == 0 {
		r.Reason = "check output is incomplete"
		return r
	}
	for id, s := range tests {
		if !s.ran || s.outcome == "" {
			r.Reason = "named test did not finish"
			return r
		}
		if s.outcome == "pass" {
			r.Tests = append(r.Tests, id)
		}
	}
	sort.Slice(r.Tests, func(i, j int) bool {
		if r.Tests[i].Package != r.Tests[j].Package {
			return r.Tests[i].Package < r.Tests[j].Package
		}
		return r.Tests[i].Name < r.Tests[j].Name
	})
	for _, id := range baseline {
		if s := tests[id]; s == nil || !s.ran || s.outcome != "pass" {
			r.Reason = "original test did not pass: " + id.Package + "/" + id.Name
			return r
		}
	}
	allPassed := processPassed
	for _, status := range packages {
		allPassed = allPassed && (status == "pass" || status == "skip")
	}
	for _, s := range tests {
		allPassed = allPassed && s.outcome != "fail"
	}
	if expected == nil {
		r.Passed = allPassed
		if !allPassed {
			r.Reason = "Go checks failed or did not finish"
		}
		return r
	}
	s := tests[*expected]
	if s == nil || !s.ran {
		r.Expected, r.Reason = "absent", "expected challenge test did not run"
		return r
	}
	for id, state := range tests {
		if id.Package == expected.Package && strings.HasPrefix(id.Name, expected.Name+"/") && state.outcome == "skip" {
			r.Reason = "challenge contains a skipped subtest"
			return r
		}
	}
	if s.outcome == "pass" && allPassed {
		r.Expected, r.Passed = "pass", true
		return r
	}
	if s.outcome != "fail" || processPassed || packages[expected.Package] != "fail" {
		r.Reason = "challenge was skipped, incomplete, or contradicted the suite outcome"
		return r
	}
	for pkg, status := range packages {
		if pkg != expected.Package && status != "pass" && status != "skip" {
			r.Reason = "another package failed or did not finish"
			return r
		}
	}
	assertion := false
	for id, state := range tests {
		isExpected := id.Package == expected.Package && (id.Name == expected.Name || strings.HasPrefix(id.Name, expected.Name+"/"))
		if state.outcome == "fail" {
			if !isExpected {
				r.Reason = "a test outside the challenge failed"
				return r
			}
			assertion = assertion || state.assertion
		}
	}
	for _, diagnostic := range []string{"panic:", "fatal error:", "WARNING: DATA RACE", "race detected during execution of test", "[build failed]", "[setup failed]", "test timed out", "testing: warning: no tests to run"} {
		if bytes.Contains(out, []byte(diagnostic)) {
			r.Reason = "challenge failure contains unsupported runtime or build diagnostics"
			return r
		}
	}
	if !assertion {
		r.Reason = "challenge failure lacks a supported Go assertion diagnostic"
		return r
	}
	r.Expected = "fail"
	return r
}
