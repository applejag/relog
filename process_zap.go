package main

import (
	"bytes"
	"regexp"
	"time"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
)

var kubernetesLogRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\t([A-Z]+)\t(?:([a-z0-9\.\-]+)\t)?([^\{]+)(?:\t(\{.*))?$`)

func (r *Relogger) processLineZap(b []byte) bool {
	groups := kubernetesLogRegex.FindSubmatch(b)
	if groups == nil {
		return false
	}
	timeGroup := groups[1]
	levelGroup := groups[2]
	callerGroup := groups[3]
	messageGroup := bytes.TrimSpace(groups[4])
	jsonGroup := groups[5]

	timeParsed, err := time.Parse(time.RFC3339, string(timeGroup))
	if err != nil {
		return false
	}

	var data map[string]any
	if len(jsonGroup) > 0 {
		if err := sonic.Unmarshal(jsonGroup, &data); err != nil {
			return false
		}
	}

	parsedTime = timeParsed

	level := parseLevel(string(levelGroup))
	ev := log.WithLevel(level)

	if len(callerGroup) > 0 {
		ev = ev.Str("caller", string(callerGroup))
	}

	for k, v := range data {
		ev = ev.Interface(k, v)
	}

	ev.Msg(string(messageGroup))
	return true
}
