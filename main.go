package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/fatih/color"
	"github.com/go-logfmt/logfmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/typ.v4/slices"
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
		scanner:      bufio.NewScanner(os.Stdin),
		mongoComp:    NewPaddedString(100),
		mongoContext: NewPaddedString(100),
		mongoID:      NewPaddedString(100),
	}
}

type Relogger struct {
	scanner *bufio.Scanner

	mongoComp    *PaddedString
	mongoContext *PaddedString
	mongoID      *PaddedString

	buf bytes.Buffer
}

func (r *Relogger) RelogAll() error {
	for r.scanner.Scan() {
		r.processLine(r.scanner.Bytes())
	}
	return r.scanner.Err()
}

func (r *Relogger) processLine(b []byte) {
	if r.processLineJson(b) {
		return
	}
	if r.processLineLogFmt(b) {
		return
	}
	r.processLineString(string(b))
}

type LevelRegex struct {
	Regex *regexp.Regexp
	Level zerolog.Level
	Color *color.Color
}

var levelRegexes = []LevelRegex{
	{
		Regex: regexp.MustCompile(`\b(?:TRACE|trace|TRC|trc|T\d+)\b`),
		Level: zerolog.TraceLevel,
		Color: color.New(color.FgMagenta),
	},
	{
		Regex: regexp.MustCompile(`\b(?:DEBUG|debug|DBG|dbg|D\d+)\b`),
		Level: zerolog.DebugLevel,
		Color: color.New(color.FgYellow),
	},
	{
		Regex: regexp.MustCompile(`\b(?:INFO|info|INF|inf|I\d+)\b`),
		Level: zerolog.InfoLevel,
		Color: color.New(color.FgGreen),
	},
	{
		Regex: regexp.MustCompile(`\b(?:WARNING|warning|WARN|warn|WRN|wrn|W\d+)\b`),
		Level: zerolog.WarnLevel,
		Color: color.New(color.FgRed),
	},
	{
		Regex: regexp.MustCompile(`\b(?:ERROR|error|ERRO|erro|ERR|err|E\d+)\b`),
		Level: zerolog.ErrorLevel,
		Color: color.New(color.FgRed, color.Bold),
	},
}

func (r *Relogger) processLineString(s string) {
	level := zerolog.NoLevel
	for _, matcher := range levelRegexes {
		replaced := matcher.Regex.ReplaceAllStringFunc(s, func(match string) string {
			return matcher.Color.Sprint(match)
		})
		if len(replaced) != len(s) {
			level = matcher.Level
			s = replaced
			break
		}
	}
	log.WithLevel(level).Msg(s)
}

func (r *Relogger) processLineJson(b []byte) bool {
	if r.buf.Len() > 0 {
		r.buf.Write(b)
		b = r.buf.Bytes()
	}
	root, err := sonic.Get(b)
	if err != nil {
		if isSonicEOFErr(err) {
			if r.buf.Len() == 0 {
				r.buf.Write(b)
			}
			return true
		}
		r.buf.Reset()
		return false
	}
	r.buf.Reset()
	if root.Type() != ast.V_OBJECT {
		return false
	}
	var (
		level            = zerolog.NoLevel
		message          = ""
		caller           = ""
		isMongoDBLogging = false
		ignoreNodes      []string
	)
	levelNodeName, levelNode := findWithAnyName(root, "level", "lvl", "severity", "log.level")
	if levelNode != nil {
		if levelStr, err := levelNode.String(); err == nil {
			level = parseLevel(levelStr)
			ignoreNodes = append(ignoreNodes, levelNodeName)
		}
	} else {
		// MongoDB styled logging
		// https://www.mongodb.com/docs/manual/reference/log-messages/#std-label-log-severity-levels
		levelNodeName, levelNode = findWithAnyName(root, "s")
		if levelNode != nil {
			if levelStr, err := levelNode.String(); err == nil {
				if lvl, ok := parseMongoDBLevel(levelStr); ok {
					level = lvl
					isMongoDBLogging = true
					ignoreNodes = append(ignoreNodes, levelNodeName)
				}
			}
		}
	}

	messageNodeName, messageNode := findWithAnyName(root, "message", "msg")
	if messageNode != nil {
		if messageStr, err := messageNode.String(); err == nil {
			message = messageStr
			ignoreNodes = append(ignoreNodes, messageNodeName)

			if isMongoDBLogging {
				_, componentNode := findWithAnyName(root, "c")
				_, contextNode := findWithAnyName(root, "ctx")
				_, idNode := findWithAnyName(root, "id")
				if componentNode != nil && contextNode != nil && idNode != nil {
					componentStr, _ := componentNode.String()
					contextStr, _ := contextNode.String()
					idStr, _ := idNode.String()

					componentStr = r.mongoComp.Next(componentStr)
					contextStr = r.mongoContext.Next(contextStr)
					idStr = r.mongoID.Next(idStr)

					caller = fmt.Sprintf("[%s|%s|%s]", componentStr, contextStr, idStr)
				} else {
					isMongoDBLogging = false
				}
			}
		}
	}

	timestampNodeName, timestampNode := findWithAnyName(root, "time", "timestamp", "@timestamp", "ts", "datetime")
	if t, ok := parseTimestampNode(timestampNode); ok {
		parsedTime = t
		ignoreNodes = append(ignoreNodes, timestampNodeName)
	} else if isMongoDBLogging {
		timestampNode = root.GetByPath("t", "$date")
		if t, ok := parseTimestampNode(timestampNode); ok {
			parsedTime = t
		}
	}

	if isMongoDBLogging {
		attrNode := root.Get("attr")
		if attrNode != nil {
			root = *attrNode
		}
	}

	stacktraceNodeName, stacktraceNode := findWithAnyName(root, "stacktrace", "stack_trace", "stack")
	if stacktraceNode != nil {
		ignoreNodes = append(ignoreNodes, stacktraceNodeName)
		if stacktraceNode.Type() == ast.V_ARRAY {
			children, _ := stacktraceNode.ArrayUseNode()
			var sb strings.Builder
			for _, child := range children {
				childStr, _ := child.String()
				sb.WriteString(childStr)
				sb.WriteString("\n\t")
			}
			message = fmt.Sprintf("%s\n\tSTACKTRACE\n\t==========\n\t%s", message, sb.String())
		} else {
			stacktraceStr, _ := stacktraceNode.String()
			stacktraceStr = strings.ReplaceAll(stacktraceStr, "\n", "\n\t")
			message = fmt.Sprintf("%s\n\tSTACKTRACE\n\t==========\n\t%s", message, stacktraceStr)
		}
	}

	//level_value=20000 logger_name=akka.kafka.internal.CommittableSubSourceStageLogic sourceActorSystem=Main sourceThread=Main-akka.actor.default-dispatcher-6 thread_name=Main-akka.actor.default-dispatcher-5
	callerNodes := findManyWithAllNames(root, "caller_file_name", "caller_line_number", "caller_class_name", "caller_method_name")
	if callerNodes != nil {
		callerFileName, _ := callerNodes[0].String()
		callerLineNumber, _ := callerNodes[1].String()
		callerClassName, _ := callerNodes[2].String()
		callerMethodName, _ := callerNodes[3].String()
		caller = fmt.Sprintf("%s:%s (%s:%s)", callerFileName, callerLineNumber, callerClassName, callerMethodName)
		ignoreNodes = append(ignoreNodes, "caller_file_name", "caller_line_number", "caller_class_name", "caller_method_name")
	}

	ev := log.WithLevel(level)
	if caller != "" {
		ev = ev.Str("caller", caller)
	}

	root.ForEach(func(path ast.Sequence, node *ast.Node) bool {
		if path.Key == nil {
			return true
		}
		key := *path.Key
		if slices.Contains(ignoreNodes, key) {
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
	return true
}

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
	type Pair struct {
		Key   string
		Value string
	}
	var fields []Pair
	for d.ScanKeyval() {
		pair := Pair{string(d.Key()), string(d.Value())}
		if !hasTimestamp && (pair.Key == "time" || pair.Key == "timestamp" || pair.Key == "@timestamp" || pair.Key == "ts" || pair.Key == "t" || pair.Key == "datetime") {
			if t, ok := parseTime(pair.Value); ok {
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
	} else {
		parsedTime = time.Time{}
	}
	ev := log.WithLevel(level)
	for _, pair := range fields {
		if i, err := strconv.ParseInt(pair.Value, 10, 64); err == nil {
			ev = ev.Int64(pair.Key, i)
		} else if f, err := strconv.ParseFloat(pair.Value, 64); err == nil {
			ev = ev.Float64(pair.Key, f)
		} else if pair.Value == "true" {
			ev = ev.Bool(pair.Key, true)
		} else if pair.Value == "false" {
			ev = ev.Bool(pair.Key, false)
		} else if pair.Key == "err" {
			ev = ev.Err(errors.New(pair.Value))
		} else if pair.Key == "logger" && !hasCaller {
			ev = ev.Str("caller", pair.Value)
		} else {
			ev = ev.Str(pair.Key, pair.Value)
		}
	}
	if len(fields) == 0 && !hasMessage && !hasLevel && !hasTimestamp {
		return false
	}
	ev.Msg(message)
	return true
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

func findManyWithAllNames(node ast.Node, names ...string) []*ast.Node {
	nodes := make([]*ast.Node, len(names))
	node.ForEach(func(path ast.Sequence, node *ast.Node) bool {
		if path.Key == nil {
			return false
		}
		key := *path.Key
		for i, n := range names {
			if key == n {
				nodes[i] = node
				return true
			}
		}
		return true
	})
	for _, node := range nodes {
		if node == nil {
			return nil
		}
	}
	return nodes
}

func parseLevel(levelStr string) zerolog.Level {
	level, err := zerolog.ParseLevel(strings.ToLower(levelStr))
	if err != nil {
		return zerolog.NoLevel
	}
	return level
}

func parseMongoDBLevel(levelStr string) (zerolog.Level, bool) {
	switch levelStr {
	case "F":
		return zerolog.FatalLevel, true
	case "E":
		return zerolog.ErrorLevel, true
	case "W":
		return zerolog.WarnLevel, true
	case "I":
		return zerolog.InfoLevel, true
	case "D1":
		return zerolog.DebugLevel, true
	case "D2", "D3", "D4", "D5":
		return zerolog.TraceLevel, true
	default:
		return zerolog.NoLevel, false
	}
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

var eofErrRegex = regexp.MustCompile(`^"Syntax error at index \d+: eof`)

func isSonicEOFErr(err error) bool {
	return eofErrRegex.MatchString(err.Error())
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
