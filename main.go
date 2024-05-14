package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bytedance/sonic/ast"
	"github.com/fatih/color"
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
		scanner:      bufio.NewScanner(os.Stdin),
		mongoComp:    NewPaddedString(100),
		mongoContext: NewPaddedString(100),
		mongoID:      NewPaddedString(100),
	}
}

type Processor byte

const (
	ProcessorNone Processor = iota
	ProcessorJSON
	ProcessorLogfmt
	ProcessorZap
	ProcessorKlog
	ProcessorString
)

type Relogger struct {
	scanner *bufio.Scanner

	mongoComp    *PaddedString
	mongoContext *PaddedString
	mongoID      *PaddedString

	lastProcessor   Processor
	lastStringLevel zerolog.Level

	buf bytes.Buffer
}

func (r *Relogger) RelogAll() error {
	for r.scanner.Scan() {
		r.processLine(r.scanner.Bytes())
	}
	return r.scanner.Err()
}

var containerTimestampRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z `)

func (r *Relogger) processLine(b []byte) {
	parsedTime = time.Time{}
	if timestamp := containerTimestampRegex.Find(b); timestamp != nil {
		b = b[len(timestamp):]
		timestamp = timestamp[:len(timestamp)-1] // trim await the trailing space
		if t, err := time.Parse(time.RFC3339Nano, string(timestamp)); err == nil {
			parsedTime = t
		}
	}

	if r.processLineJson(b) {
		r.lastProcessor = ProcessorJSON
		return
	}
	if r.processLineZap(b) {
		r.lastProcessor = ProcessorZap
		return
	}
	if r.processLineKlog(b) {
		r.lastProcessor = ProcessorKlog
		return
	}
	if r.processLineLogFmt(b) {
		r.lastProcessor = ProcessorLogfmt
		return
	}
	r.processLineString(string(b))
	r.lastProcessor = ProcessorString
}

type LevelRegex struct {
	Regex *regexp.Regexp
	Level zerolog.Level
	Color *color.Color
}

var levelRegexes = []LevelRegex{
	{
		Regex: regexp.MustCompile(`(?:\[\s*)?\b(?:\d*m)?(?:ERROR|error|ERRO|erro|ERR|err|E\d+)\b\s*(?:\]\s*)?`),
		Level: zerolog.ErrorLevel,
		Color: color.New(color.FgRed, color.Bold),
	},
	{
		Regex: regexp.MustCompile(`(?:\[\s*)?\b(?:\d*m)?(?:WARNING|warning|WARN|warn|WRN|wrn|W\d+)\b\s*(?:\]\s*)?`),
		Level: zerolog.WarnLevel,
		Color: color.New(color.FgRed),
	},
	{
		Regex: regexp.MustCompile(`(?:\[\s*)?\b(?:\d*m)?(?:INFO|info|INF|inf|I\d+)\b\s*(?:\]\s*)?`),
		Level: zerolog.InfoLevel,
		Color: color.New(color.FgGreen),
	},
	{
		Regex: regexp.MustCompile(`(?:\[\s*)?\b(?:\d*m)?(?:DEBUG|debug|DBG|dbg|D\d+)\b\s*(?:\]\s*)?`),
		Level: zerolog.DebugLevel,
		Color: color.New(color.FgYellow),
	},
	{
		Regex: regexp.MustCompile(`(?:\[\s*)?\b(?:\d*m)?(?:TRACE|trace|TRC|trc|T\d+)\b\s*(?:\]\s*)?`),
		Level: zerolog.TraceLevel,
		Color: color.New(color.FgMagenta),
	},
}

func startsWithWhitespace(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return false
	}
	return unicode.IsSpace(r)
}

var ansiCutterRegex = regexp.MustCompile(`^\d*m`)

func cutANSIPart(s string) (string, string, bool) {
	ansiPart := ansiCutterRegex.FindString(s)
	if ansiPart == "" {
		return "", s, false
	}
	return ansiPart, s[len(ansiPart):], true
}

func (r *Relogger) processLineString(s string) {
	level := zerolog.NoLevel

	if t, suffix, ok := CutPrefixFuzzyTime(s); ok {
		s = suffix
		parsedTime = t
	}

	if r.lastProcessor == ProcessorString && startsWithWhitespace(s) {
		level = r.lastStringLevel
	} else {
		for _, matcher := range levelRegexes {
			var matchedAny bool
			replaced := matcher.Regex.ReplaceAllStringFunc(s, func(match string) string {
				matchedAny = true
				if strings.HasPrefix(s, match) {
					return ""
				}
				ansiPart, cleanPart, ok := cutANSIPart(match)
				if ok {
					return ansiPart + matcher.Color.Sprint(cleanPart)
				}
				return matcher.Color.Sprint(match)
			})
			if matchedAny {
				level = matcher.Level
				s = replaced
				break
			}
		}
	}

	ev := log.WithLevel(level)

	if inside, suffix, ok := cutParentheses(s, '[', ']'); ok {
		s = suffix
		ev = ev.Str("caller", inside)
	}

	ev.Msg(s)
	r.lastStringLevel = level
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
		return ParseFuzzyTime(timestampStr)
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
