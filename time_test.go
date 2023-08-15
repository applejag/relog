package main

import "testing"

func TestCutParentheses(t *testing.T) {
	inside, after, ok := cutParentheses("[lorem] ipsum", '[', ']')
	if !ok {
		t.Fatal("did not find parenthases")
	}
	if inside != "lorem" {
		t.Errorf("want inside: %q, but got: %q", "lorem", inside)
	}
	if after != "ipsum" {
		t.Errorf("want after: %q, but got: %q", "ipsum", after)
	}
}
