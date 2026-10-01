package session

import "testing"

func TestValid(t *testing.T) {
	if !Valid(101, 100) {
		t.Fatal("future session should be valid")
	}
	if Valid(99, 100) {
		t.Fatal("past session should be invalid")
	}
}
