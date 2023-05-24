package main

import "testing"

func TestPaddedString(t *testing.T) {
	p := NewPaddedString(3)

	assertEqualString(t, "foo", p.Next("foo"), "1st: expect no padding")
	assertEqualString(t, "bar", p.Next("bar"), "2nd: expect no padding")
	assertEqualString(t, "lorem", p.Next("lorem"), "3rd: expect no padding")
	assertEqualString(t, "moo  ", p.Next("moo"), "4th: expect some padding")
	assertEqualString(t, "f    ", p.Next("f"), "5th: expect some padding")
	assertEqualString(t, "f  ", p.Next("f"), "6th: expect less padding")
	assertEqualString(t, "f", p.Next("f"), "7th: expect no padding")
}

func assertEqualString(t *testing.T, want, got, msg string) {
	t.Helper()
	if got != want {
		t.Errorf("got != want: %s\nwant: %q (len=%d)\ngot:  %q (len=%d)", msg, want, len(want), got, len(got))
	}
}
