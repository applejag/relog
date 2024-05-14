package main

import (
	"bytes"
	"fmt"
	"regexp"
	"time"

	"github.com/go-logfmt/logfmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var klogLogRegex = regexp.MustCompile(`^([EWIDT])(\d{4} \d{2}:\d{2}:\d{2}(?:\.\d+)?) +\d +([^\]]+)\] +(?:"([^"]*)")? *((?:\S+=.*)*)(.*)$`)

func (r *Relogger) processLineKlog(b []byte) bool {
	groups := klogLogRegex.FindSubmatch(b)
	if groups == nil {
		fmt.Println("doesnt match regex")
		return false
	}
	levelGroup := groups[1]
	timeGroup := groups[2]
	callerGroup := groups[3]
	messageGroup := groups[4]
	logfmtGroup := groups[5]
	plainMessage := groups[6]

	// RFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
	timeParsed, err := time.Parse("0102 15:04:05.999999999", string(timeGroup))
	if err != nil {
		fmt.Println("doesnt match time", err)
		return false
	}

	parsedTime = timeParsed

	level := parseKlogLevel(levelGroup)
	ev := log.WithLevel(level)
	ev = ev.Str("caller", string(callerGroup))

	if len(logfmtGroup) > 0 {
		dec := logfmt.NewDecoder(bytes.NewReader(logfmtGroup))
		if dec.ScanRecord() {
			for dec.ScanKeyval() {
				pair := Pair{
					Key:   string(dec.Key()),
					Value: string(dec.Value()),
				}
				ev = addLogfmtEventField(ev, pair, true)
			}
			if err := dec.Err(); err != nil {
				return false
			}
		}
	}

	if len(messageGroup) > 0 {
		ev.Msg(string(messageGroup))
	} else if len(plainMessage) > 0 {
		ev.Msg(string(plainMessage))
	}
	return true
}

func parseKlogLevel(b []byte) zerolog.Level {
	if len(b) != 1 {
		return zerolog.NoLevel
	}
	switch b[0] {
	case 'E':
		return zerolog.ErrorLevel
	case 'W':
		return zerolog.WarnLevel
	case 'I':
		return zerolog.InfoLevel
	case 'D':
		return zerolog.DebugLevel
	case 'T':
		return zerolog.TraceLevel
	default:
		return zerolog.NoLevel
	}
}
