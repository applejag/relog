package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/typ.v4/slices"
)

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
