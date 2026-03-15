package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	DefaultLevel = "info"
)

type Logger struct {
	component string
	minLevel  level
	out       io.Writer
	mu        sync.Mutex
}

type level int

const (
	levelDebug level = iota
	levelInfo
	levelWarn
	levelError
)

type Field struct {
	Key   string
	Value any
}

func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Any(key string, value any) Field {
	return Field{Key: key, Value: value}
}

func Error(err error) Field {
	if err == nil {
		return Field{}
	}
	return Field{Key: "error", Value: err.Error()}
}

func New(component, levelName string) (*Logger, error) {
	lvl, err := parseLevel(levelName)
	if err != nil {
		return nil, err
	}

	return &Logger{
		component: component,
		minLevel:  lvl,
		out:       os.Stdout,
	}, nil
}

func NewWithWriter(component, levelName string, out io.Writer) (*Logger, error) {
	l, err := New(component, levelName)
	if err != nil {
		return nil, err
	}
	l.out = out
	return l, nil
}

func (l *Logger) Debug(msg string, fields ...Field) { l.log(levelDebug, msg, fields...) }
func (l *Logger) Info(msg string, fields ...Field)  { l.log(levelInfo, msg, fields...) }
func (l *Logger) Warn(msg string, fields ...Field)  { l.log(levelWarn, msg, fields...) }
func (l *Logger) Error(msg string, fields ...Field) { l.log(levelError, msg, fields...) }

func (l *Logger) log(lvl level, msg string, fields ...Field) {
	if l == nil || lvl < l.minLevel {
		return
	}

	entry := map[string]any{
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"level":     lvl.String(),
		"component": l.component,
		"msg":       msg,
	}

	for _, field := range fields {
		if field.Key == "" {
			continue
		}
		entry[field.Key] = field.Value
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		payload = []byte(fmt.Sprintf(`{"ts":"%s","level":"error","component":"%s","msg":"marshal log entry","error":%q}`,
			time.Now().UTC().Format(time.RFC3339Nano), l.component, err.Error()))
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(payload, '\n'))
}

func parseLevel(name string) (level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", DefaultLevel:
		return levelInfo, nil
	case "debug":
		return levelDebug, nil
	case "warn", "warning":
		return levelWarn, nil
	case "error":
		return levelError, nil
	default:
		return levelInfo, fmt.Errorf("unsupported log level: %s", name)
	}
}

func (l level) String() string {
	switch l {
	case levelDebug:
		return "debug"
	case levelInfo:
		return "info"
	case levelWarn:
		return "warn"
	case levelError:
		return "error"
	default:
		return "info"
	}
}
