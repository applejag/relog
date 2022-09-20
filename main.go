package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var parsedTime time.Time

func main() {
	loggerSetup()
	relogger := NewRelogger(os.Stdin)

	if err := relogger.RelogAll(); err != nil {
		log.Err(err).Msg("Failed to scan.")
	}
}

func NewRelogger(r io.Reader) Relogger {
	return Relogger{
		scanner: bufio.NewScanner(os.Stdin),
	}
}

type Relogger struct {
	scanner *bufio.Scanner
}

func (r Relogger) RelogAll() error {
	for r.scanner.Scan() {
		r.processLine(r.scanner.Bytes())
	}
	return r.scanner.Err()
}

func (r Relogger) processLine(b []byte) {
	root, err := sonic.Get(b)
	if err != nil || root.Type() == ast.V_NULL {
		log.WithLevel(zerolog.NoLevel).Err(errors.New("line was not JSON formatted")).Msg(string(b))
		return
	}
	if root.Type() != ast.V_ARRAY && root.Type() != ast.V_OBJECT {
		s, err := root.String()
		if err != nil {
			log.WithLevel(zerolog.NoLevel).Err(err).Msg(string(b))
		} else {
			log.WithLevel(zerolog.NoLevel).Msg(s)
		}
		return
	}
	var (
		level   = zerolog.NoLevel
		message = ""
	)
	levelNodeName, levelNode := findWithAnyName(root, "level", "lvl")
	if levelNode != nil {
		if levelStr, err := levelNode.String(); err != nil {
			level = parseLevel(levelStr)
		} else {
			levelNode = nil
			levelNodeName = ""
		}
	}

	messageNodeName, messageNode := findWithAnyName(root, "message", "msg")
	if messageNode != nil {
		if messageStr, err := messageNode.String(); err != nil {
			message = messageStr
		} else {
			messageNode = nil
			messageNodeName = ""
		}
	}

	timestampNodeName, timestampNode := findWithAnyName(root, "time", "timestamp", "datetime")
	if t, ok := parseTimestampNode(timestampNode); ok {
		parsedTime = t
	} else {
		timestampNode = nil
		timestampNodeName = ""
		parsedTime = time.Now()
	}

	ev := log.WithLevel(level)

	root.ForEach(func(path ast.Sequence, node *ast.Node) bool {
		if path.Key == nil {
			return true
		}
		key := *path.Key
		if levelNode != nil && key == levelNodeName {
			return true // skip, already processed
		}
		if messageNode != nil && key == messageNodeName {
			return true // skip, already processed
		}
		if timestampNode != nil && key == timestampNodeName {
			return true // skip, already processed
		}
		switch node.Type() {
		case ast.V_NULL:
			ev = ev.Interface(key, nil)
		case ast.V_TRUE:
			ev = ev.Bool(key, true)
		case ast.V_FALSE:
			ev = ev.Bool(key, false)
		case ast.V_ARRAY:
			arr, _ := node.Array()
			ev = ev.Interface(key, arr)
		case ast.V_OBJECT:
			m, _ := node.Map()
			ev = ev.Interface(key, m)
		case ast.V_STRING:
			str, _ := node.String()
			ev = ev.Str(key, str)
		case ast.V_NUMBER:
			num, _ := node.Number()
			if i, err := strconv.ParseInt(num.String(), 10, 64); err == nil {
				ev = ev.Int64(key, i)
			} else if f, err := strconv.ParseFloat(num.String(), 64); err == nil {
				ev = ev.Float64(key, f)
			} else {
				ev = ev.Str(key, num.String())
			}
		}
		return true
	})
	ev.Msg(message)
}

func findWithAnyName(node ast.Node, names ...string) (string, *ast.Node) {
	var name string
	var child *ast.Node
	node.ForEach(func(path ast.Sequence, node *ast.Node) bool {
		if path.Key == nil {
			return false
		}
		key := *path.Key
		for _, n := range names {
			if key == n {
				name = n
				child = node
				return false
			}
		}
		return true
	})
	return name, child
}

func parseLevel(levelStr string) zerolog.Level {
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		return zerolog.NoLevel
	}
	return level
}

func parseTimestampNode(node *ast.Node) (time.Time, bool) {
	if node == nil {
		return time.Time{}, false
	}
	if node.Type() == ast.V_NUMBER {
		if i, err := node.Int64(); err != nil {
			return time.Unix(i, 0), true
		}
	} else if timestampStr, err := node.String(); err == nil {
		return parseTime(timestampStr)
	}
	return time.Time{}, false
}

var knownTimestampLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
}

func parseTime(str string) (time.Time, bool) {
	for _, layout := range knownTimestampLayouts {
		if t, err := time.Parse(layout, str); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func loggerSetup() error {
	zerolog.TimestampFunc = func() time.Time {
		return parsedTime
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "Jan-02 15:04",
	}).Level(zerolog.TraceLevel)
	return nil
}
