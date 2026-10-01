package game

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestChallengeRules(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tarigato_challenge_test.go")
	valid := "package sample\nimport \"testing\"\nfunc TestBoundary(t *testing.T) { t.Error(\"boundary\") }\n"
	for _, tc := range []struct {
		name, code string
		valid      bool
	}{
		{"ordinary", valid, true},
		{"helper", valid + "func helper() int { return 1 }\n", true},
		{"aliased testing", strings.ReplaceAll(strings.ReplaceAll(valid, "\"testing\"", "tt \"testing\""), "*testing.T", "*tt.T"), true},
		{"two tests", valid + "func TestOther(t *testing.T) {}\n", false},
		{"init", valid + "func init() {}\n", false},
		{"TestMain", valid + "func TestMain(m *testing.M) {}\n", false},
		{"global state", valid + "var counter int\n", false},
		{"build constraint", "//go:build linux\n\n" + valid, false},
		{"compiler directive", "//go:linkname other thing\n" + valid, false},
		{"benchmark", valid + "func BenchmarkSpeed(b *testing.B) {}\n", false},
		{"fuzz", valid + "func FuzzBoundary(f *testing.F) {}\n", false},
		{"example", valid + "func ExampleBoundary() {}\n", false},
		{"not a test", strings.Replace(valid, "TestBoundary", "Testboundary", 1), false},
		{"wrong parameter", strings.Replace(valid, "*testing.T", "*testing.B", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.code), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := validateChallenge(root, "tarigato_challenge_test.go")
			if (err == nil) != tc.valid {
				t.Fatalf("valid = %v, error = %v", tc.valid, err)
			}
			if tc.valid && (c.TestName != "TestBoundary" || c.PackageDir != ".") {
				t.Fatalf("wrong identity: %+v", c)
			}
		})
	}
	for _, name := range []string{"../tarigato_challenge_test.go", "./tarigato_challenge_test.go", path, "another_test.go", `dir\tarigato_challenge_test.go`} {
		if _, err := validateChallenge(root, name); err == nil {
			t.Fatalf("accepted path %q", name)
		}
	}
	if err := os.Symlink(root, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := validateChallenge(root, "link/tarigato_challenge_test.go"); err == nil {
		t.Fatal("accepted symlink")
	}
}

func TestCheckEvents(t *testing.T) {
	want := testID{Package: "example.test/sample", Name: "TestBoundary"}
	old := testID{Package: want.Package, Name: "TestExisting"}
	base := []testEvent{
		{Action: "start", Package: want.Package},
		{Action: "run", Package: old.Package, Test: old.Name},
		{Action: "pass", Package: old.Package, Test: old.Name},
	}
	for _, tc := range []struct {
		name, action, outputType, output, packageOutcome, expected string
		processPassed, passed                                      bool
	}{
		{name: "assertion", action: "fail", outputType: "error", output: "boundary mismatch", packageOutcome: "fail", expected: "fail"},
		{name: "pass", action: "pass", packageOutcome: "pass", processPassed: true, expected: "pass", passed: true},
		{name: "skip", action: "skip", packageOutcome: "pass", processPassed: true, expected: "inconclusive"},
		{name: "panic", action: "fail", outputType: "error", output: "panic: crash", packageOutcome: "fail", expected: "inconclusive"},
		{name: "plain failure", action: "fail", outputType: "frame", output: "--- FAIL", packageOutcome: "fail", expected: "inconclusive"},
		{name: "log plus fail", action: "fail", outputType: "log", output: "    tarigato_challenge_test.go:4: looks like an assertion", packageOutcome: "fail", expected: "inconclusive"},
		{name: "wrong exit", action: "fail", outputType: "error", packageOutcome: "fail", processPassed: true, expected: "inconclusive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := append([]testEvent(nil), base...)
			events = append(events,
				testEvent{Action: "run", Package: want.Package, Test: want.Name},
				testEvent{Action: "output", Package: want.Package, Test: want.Name, Output: tc.output, OutputType: tc.outputType},
				testEvent{Action: tc.action, Package: want.Package, Test: want.Name},
				testEvent{Action: tc.packageOutcome, Package: want.Package},
			)
			r := parseChecks(checkEvents(t, events), &want, []testID{old}, tc.processPassed)
			if r.Expected != tc.expected || r.Passed != tc.passed {
				t.Fatalf("got %+v", r)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		events []testEvent
	}{
		{"missing", append(append([]testEvent(nil), base...), testEvent{Action: "pass", Package: want.Package})},
		{"compile error", []testEvent{{Action: "build-fail"}}},
		{"original skipped", []testEvent{{Action: "start", Package: old.Package}, {Action: "run", Package: old.Package, Test: old.Name}, {Action: "skip", Package: old.Package, Test: old.Name}, {Action: "pass", Package: old.Package}}},
		{"identity mismatch", []testEvent{{Action: "start", Package: "elsewhere"}, {Action: "run", Package: "elsewhere", Test: want.Name}, {Action: "pass", Package: "elsewhere", Test: want.Name}, {Action: "pass", Package: "elsewhere"}}},
		{"incomplete", []testEvent{{Action: "start", Package: want.Package}, {Action: "run", Package: want.Package, Test: want.Name}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := parseChecks(checkEvents(t, tc.events), &want, []testID{old}, true)
			if r.Passed || r.Expected == "fail" || r.Expected == "pass" || r.Reason == "" {
				t.Fatalf("accepted incomplete evidence: %+v", r)
			}
		})
	}
}

func checkEvents(t *testing.T, events []testEvent) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, event := range events {
		if err := json.NewEncoder(&out).Encode(event); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}

func TestCheckSuiteRealGo(t *testing.T) {
	t.Setenv("PATH", filepath.Join(runtime.GOROOT(), "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENAI_API_KEY", "must-not-reach-tests")
	t.Setenv("GOFLAGS", "-run=Nothing")
	root := t.TempDir()
	write := func(name, code string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/checks\n\ngo 1.27.1\n")
	write("expiry.go", "package checks\nfunc valid(expiry, now int) bool { return expiry >= now }\n")
	write("expiry_test.go", "package checks\nimport (\"testing\"; \"os\")\nfunc TestExisting(t *testing.T) { if !valid(2,1) || os.Getenv(\"OPENAI_API_KEY\") != \"\" { t.Fatal(\"unexpected state\") } }\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base, err := checkSuite(ctx, root, nil, nil)
	if err != nil || !base.Passed || len(base.Tests) != 1 {
		t.Fatalf("baseline: %+v %v\n%s", base, err, base.Output)
	}
	write("tarigato_challenge_test.go", "package checks\nimport \"testing\"\nfunc TestBoundary(t *testing.T) { if valid(2,2) { t.Error(\"expired session accepted\") } }\n")
	want, err := validateChallenge(root, "tarigato_challenge_test.go")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := checkSuite(ctx, root, &want, base.Tests)
	if err != nil || failed.Expected != "fail" || failed.Passed {
		t.Fatalf("failure: %+v %v\n%s", failed, err, failed.Output)
	}
	write("expiry.go", "package checks\nfunc valid(expiry, now int) bool { return expiry > now }\n")
	passed, err := checkSuite(ctx, root, &want, base.Tests)
	if err != nil || !passed.Passed || passed.Expected != "pass" {
		t.Fatalf("repair: %+v %v\n%s", passed, err, passed.Output)
	}
	// The report's copyable command must strip credentials just like the controller.
	replay, err := runCommand(ctx, root, nil, "sh", "-c", replayCommand())
	if err != nil {
		t.Fatalf("report replay failed: %v\n%s", err, replay)
	}
	write("tarigato_challenge_test.go", "package checks\nimport \"testing\"\nfunc TestBoundary(t *testing.T) { t.Skip(\"unsupported\") }\n")
	skipped, err := checkSuite(ctx, root, &want, base.Tests)
	if err != nil || skipped.Expected != "inconclusive" || skipped.Passed {
		t.Fatalf("skip: %+v %v\n%s", skipped, err, skipped.Output)
	}
	write("tarigato_challenge_test.go", "package checks\nimport \"testing\"\nfunc TestBoundary(t *testing.T) { t.Run(\"boundary\", func(t *testing.T) { t.Skip(\"unsupported\") }) }\n")
	subSkipped, err := checkSuite(ctx, root, &want, base.Tests)
	if err != nil || subSkipped.Expected != "inconclusive" || subSkipped.Passed {
		t.Fatalf("subtest skip: %+v %v\n%s", subSkipped, err, subSkipped.Output)
	}
}
