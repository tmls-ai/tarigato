package game

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type fileState struct {
	Hash string
	Mode fs.FileMode
}
type manifest map[string]fileState

// Resolve existing ancestors as well as the destination: /var and /private/var
// are the same place on macOS, and a symlink must not bypass the output guard.
func resolveDestination(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := resolveDestination(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// A manifest includes generated files too: checks may not rewrite their inputs.
func snapshot(dir string) (manifest, error) {
	files := manifest{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported non-regular file: %s", rel)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		files[filepath.ToSlash(rel)] = fileState{hex.EncodeToString(h.Sum(nil)), info.Mode().Perm() & 0111}
		return nil
	})
	return files, err
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	env := append(checkEnv(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	return runCommand(ctx, dir, env, "git", append([]string{"-c", "core.hooksPath=/dev/null"}, args...)...)
}

func cloneAt(ctx context.Context, source, base, dest string, patch []byte) error {
	if _, err := git(ctx, "", "clone", "--quiet", "--no-hardlinks", "--no-checkout", "--", source, dest); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if _, err := git(ctx, dest, "checkout", "--quiet", "--detach", base); err != nil {
		return err
	}
	if len(patch) != 0 {
		env := append(checkEnv(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if _, err := runCommandInput(ctx, dest, env, string(patch), "git", "-c", "core.hooksPath=/dev/null", "apply", "--index", "--binary", "-"); err != nil {
			return fmt.Errorf("reconstruct patch: %w", err)
		}
	}
	return nil
}

func capturePatch(ctx context.Context, dir, base string, paths ...string) ([]byte, error) {
	if _, err := git(ctx, dir, "add", "--all", "--", "."); err != nil {
		return nil, err
	}
	args := []string{"diff", "--cached", "--binary", "--no-ext-diff", "--no-textconv", base, "--"}
	return git(ctx, dir, append(args, paths...)...)
}

func changedFiles(before, after manifest) []string {
	var changed []string
	for p, v := range before {
		if after[p] != v {
			changed = append(changed, p)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			changed = append(changed, p)
		}
	}
	return changed
}

func productionOnly(before, after manifest) error {
	for _, p := range changedFiles(before, after) {
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.Contains("/"+p+"/", "/testdata/") || strings.Contains("/"+p+"/", "/vendor/") || strings.HasPrefix(p, ".github/") {
			return fmt.Errorf("protected file changed: %s", p)
		}
		a, aok := before[p]
		b, bok := after[p]
		if (aok && bok && a.Mode != b.Mode) || (!aok && b.Mode != 0) {
			return fmt.Errorf("file mode changed: %s", p)
		}
	}
	return nil
}

func checkWorkspace(ctx context.Context, dir string, want *challenge, baseline []testID) (checkResult, error) {
	before, err := snapshot(dir)
	if err != nil {
		return checkResult{}, err
	}
	result, checkErr := checkSuite(ctx, dir, want, baseline)
	after, err := snapshot(dir)
	if err != nil {
		return result, err
	}
	if !reflect.DeepEqual(before, after) {
		result.Passed, result.Expected, result.Reason = false, "inconclusive", "checks modified workspace files"
	}
	return result, checkErr
}

func repository(ctx context.Context, dir string) (string, string, error) {
	root, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", errors.New("run inside a committed Git repository")
	}
	source := strings.TrimSpace(string(root))
	status, err := git(ctx, source, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", "", err
	}
	if len(status) != 0 {
		return "", "", errors.New("repository must be clean, including nonignored untracked files")
	}
	head, err := git(ctx, source, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", "", errors.New("repository needs a base commit")
	}
	files, err := git(ctx, source, "ls-files", "--stage")
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(files)), "\n") {
		if !strings.HasPrefix(line, "100644 ") && !strings.HasPrefix(line, "100755 ") {
			return "", "", errors.New("symlinks and submodules are unsupported")
		}
	}
	if _, err := os.Stat(filepath.Join(source, "go.mod")); err != nil {
		return "", "", errors.New("a Go module at the repository root is required")
	}
	for _, p := range []string{".gitattributes", ".lfsconfig", "go.work"} {
		if _, err := os.Stat(filepath.Join(source, p)); err == nil {
			return "", "", fmt.Errorf("%s is unsupported in this version", p)
		}
	}
	return source, strings.TrimSpace(string(head)), nil
}
