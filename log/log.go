package log

import (
	"fmt"
	"hash/fnv"
	"os"
	"sync"
	"time"
)

const (
	reset   = "\033[0m"
	dim     = "\033[2m"
	red     = "\033[31m"
	yellow  = "\033[33m"
	boldRed = "\033[1;31m"
)

// Bold + standard ANSI colors for maximum terminal compatibility.
var tagColors = []string{
	"\033[1;36m", // bold cyan
	"\033[1;35m", // bold magenta
	"\033[1;32m", // bold green
	"\033[1;33m", // bold yellow
	"\033[1;34m", // bold blue
	"\033[1;31m", // bold red
	"\033[1;37m", // bold white
}

// colorFor returns a deterministic color for a given name.
func colorFor(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return tagColors[h.Sum32()%uint32(len(tagColors))]
}

type Logger struct {
	mu sync.Mutex
}

func New() *Logger {
	return &Logger{}
}

func (l *Logger) timestamp() string {
	return dim + time.Now().Format("15:04:05") + reset
}

func (l *Logger) write(level, msg string) {
	l.mu.Lock()
	fmt.Fprintf(os.Stderr, "%s  %s %s\n", l.timestamp(), level, msg)
	l.mu.Unlock()
}

func (l *Logger) Info(msg string) {
	l.write(dim+"INFO"+reset, msg)
}

func (l *Logger) Warn(msg string) {
	l.write(yellow+"WARN"+reset, msg)
}

func (l *Logger) Error(msg string) {
	l.write(boldRed+"ERR "+reset, msg)
}

// Tag formats a bracketed tag with a deterministic color based on name.
func Tag(s string) string {
	c := colorFor(s)
	return c + "[" + s + "]" + reset
}

// Tag2 formats two bracketed tags, each with their own deterministic color.
func Tag2(a, b string) string {
	return Tag(a) + Tag(b)
}

// Tag3 formats three bracketed tags, each with their own deterministic color.
func Tag3(a, b, c string) string {
	return Tag(a) + Tag(b) + Tag(c)
}

// Err formats an error for inline display.
func Err(err error) string {
	return red + err.Error() + reset
}
