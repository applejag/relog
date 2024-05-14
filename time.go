package main

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var knownTimestampLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05.999999999",
	"Jan-01 15:04",
}

func ParseFuzzyTime(str string) (time.Time, bool) {
	for _, layout := range knownTimestampLayouts {
		if t, err := time.Parse(layout, str); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func ParsePrefixFuzzyTime(str string) (time.Time, string, bool) {
	for _, layout := range knownTimestampLayouts {
		t, err := time.Parse(layout, str)
		if err == nil {
			return t, "", true
		}
		var parseErr *time.ParseError
		if !errors.As(err, &parseErr) {
			continue
		}
		if !strings.HasPrefix(": extra text:", parseErr.Message) {
			continue
		}
		index := strings.Index(str, parseErr.ValueElem)
		if index < 0 {
			continue
		}
		t, err = time.Parse(layout, str[:index])
		if err == nil {
			return t, str[index+1:], true
		}
	}
	return time.Time{}, "", false
}

func CutPrefixFuzzyTime(s string) (time.Time, string, bool) {
	if inside, after, ok := cutParentheses(s, '[', ']'); ok {
		if t, ok := ParseFuzzyTime(inside); ok {
			return t, after, true
		}
		return time.Time{}, s, false
	}

	return ParsePrefixFuzzyTime(s)
}

func cutParentheses(s string, start, end rune) (string, string, bool) {
	r, size := utf8.DecodeRuneInString(s)
	if r != start {
		return "", s, false
	}
	for i, r := range s[size:] {
		if r == end {
			return s[size : size+i], strings.TrimPrefix(s[size+i+utf8.RuneLen(r):], " "), true
		}
	}
	return "", s, false
}
