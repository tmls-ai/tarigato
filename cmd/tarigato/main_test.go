package main

import (
	"bytes"
	"testing"
)

func TestFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0}, {[]string{"--version"}, 0},
		{nil, 2}, {[]string{"--builder", "unknown", "task"}, 2},
		{[]string{"--timeout", "0s", "task"}, 2}, {[]string{"task", "--builder", "codex"}, 2},
	} {
		var output bytes.Buffer
		if code := run(tc.args, &output, &output); code != tc.code {
			t.Fatalf("%v: exit %d, expected %d", tc.args, code, tc.code)
		}
	}
}
