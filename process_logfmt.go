package main

import (
	"bytes"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/go-logfmt/logfmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var crudeLogfmtRegex = regexp.MustCompile(`^\w+=[^ ]+`)

func (r Relogger) processLineLogFmt(b []byte) bool {
	if !crudeLogfmtRegex.Match(b) {
		return false
	}
	d := logfmt.NewDecoder(bytes.NewReader(b))
	if !d.ScanRecord() {
		return false
	}
	var (
		timestamp    time.Time
		hasTimestamp bool
		level        = zerolog.NoLevel
		hasLevel     bool
		message      string
		hasMessage   bool
		hasCaller    bool
	)
	var fields []Pair
	for d.ScanKeyval() {
		pair := Pair{string(d.Key()), string(d.Value())}
		if !hasTimestamp && (pair.Key == "time" || pair.Key == "timestamp" || pair.Key == "@timestamp" || pair.Key == "ts" || pair.Key == "t" || pair.Key == "datetime") {
			if t, ok := ParseFuzzyTime(pair.Value); ok {
				timestamp = t
				hasTimestamp = true
				continue
			}
		} else if !hasLevel && (pair.Key == "level" || pair.Key == "lvl" || pair.Key == "severity") {
			level = parseLevel(pair.Value)
			hasLevel = true
			continue
		} else if !hasMessage && (pair.Key == "message" || pair.Key == "msg") {
			message = pair.Value
			hasMessage = true
			continue
		} else if !hasCaller && (pair.Key == "caller") {
			hasCaller = true
		}
		fields = append(fields, pair)
	}
	if hasTimestamp {
		parsedTime = timestamp
	}
	ev := log.WithLevel(level)
	for _, pair := range fields {
		ev = addLogfmtEventField(ev, pair, hasCaller)
	}
	if len(fields) == 0 && !hasMessage && !hasLevel && !hasTimestamp {
		return false
	}
	ev.Msg(message)
	return true
}

type Pair struct {
	Key   string
	Value string
}

func addLogfmtEventField(ev *zerolog.Event, pair Pair, hasCaller bool) *zerolog.Event {
	if i, err := strconv.ParseInt(pair.Value, 10, 64); err == nil {
		return ev.Int64(pair.Key, i)
	} else if f, err := strconv.ParseFloat(pair.Value, 64); err == nil {
		return ev.Float64(pair.Key, f)
	} else if pair.Value == "true" {
		return ev.Bool(pair.Key, true)
	} else if pair.Value == "false" {
		return ev.Bool(pair.Key, false)
	} else if pair.Key == "err" {
		return ev.Err(errors.New(pair.Value))
	} else if (pair.Key == "logger" || pair.Key == "source") && !hasCaller {
		return ev.Str("caller", pair.Value)
	} else {
		return ev.Str(pair.Key, pair.Value)
	}
}
