// Package redlog provides a Redis compatible logger.
//   http://build47.com/redis-log-format-levels/
package redlog

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

const (
	levelDebug   = 0 // '.'
	levelVerbose = 1 // '-'
	levelNotice  = 2 // '*'
	levelWarning = 3 // '#'
	levelFatal   = 4 // '#' special condition
)

var levelChars = []byte{'.', '-', '*', '#', '#'}
var levelColors = []string{"35", "", "1", "33", "31"}

// Options ...
type Options struct {
	Level  int
	Filter func(line string, tty bool) (msg string, app byte, level int)
	App    byte
}

// DefaultOptions ...
var DefaultOptions = &Options{
	Level:  2,
	Filter: nil,
	App:    'M',
}

// Logger ...
type Logger struct {
	app    byte
	level  int
	pid    int
	filter func(line string, tty bool) (msg string, app byte, level int)
	tty    bool

	mu sync.Mutex
	wr io.Writer
}

// New sets the level of the logger.
//   0 - Debug
//   1 - Verbose
//   2 - Notice
//   3 - Warning
func New(wr io.Writer, opts *Options) *Logger {
	if wr == nil {
		wr = ioutil.Discard
	}
	if opts == nil {
		opts = DefaultOptions
	}
	if opts.Level < levelDebug || opts.Level > levelWarning {
		panic("invalid level")
	}
	l := new(Logger)
	l.wr = wr
	l.filter = opts.Filter
	l.app = opts.App
	l.level = opts.Level
	l.pid = os.Getpid()
	if f, ok := wr.(*os.File); ok && terminal.IsTerminal(int(f.Fd())) {
		l.tty = true
	}
	return l
}

// Debugf ...
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.writef(levelDebug, format, args)
}

// Debug ...
func (l *Logger) Debug(args ...interface{}) {
	l.write(levelDebug, args)
}

// Debugln ...
func (l *Logger) Debugln(args ...interface{}) {
	l.write(levelDebug, args)
}

// Verbf ...
func (l *Logger) Verbf(format string, args ...interface{}) {
	l.writef(levelVerbose, format, args)
}

// Verb ...
func (l *Logger) Verb(args ...interface{}) {
	l.write(levelVerbose, args)
}

// Verbln ...
func (l *Logger) Verbln(args ...interface{}) {
	l.write(levelVerbose, args)
}

// Noticef ...
func (l *Logger) Noticef(format string, args ...interface{}) {
	l.writef(levelNotice, format, args)
}

// Notice ...
func (l *Logger) Notice(args ...interface{}) {
	l.write(levelNotice, args)
}

// Noticeln ...
func (l *Logger) Noticeln(args ...interface{}) {
	l.write(levelNotice, args)
}

// Printf ...
func (l *Logger) Printf(format string, args ...interface{}) {
	l.writef(levelNotice, format, args)
}

// Print ...
func (l *Logger) Print(args ...interface{}) {
	l.write(levelNotice, args)
}

// Println ...
func (l *Logger) Println(args ...interface{}) {
	l.write(levelNotice, args)
}

// Warningf ...
func (l *Logger) Warningf(format string, args ...interface{}) {
	l.writef(levelWarning, format, args)
}

// Warning ...
func (l *Logger) Warning(args ...interface{}) {
	l.write(levelWarning, args)
}

// Warningln ...
func (l *Logger) Warningln(args ...interface{}) {
	l.write(levelWarning, args)
}

// Fatalf ...
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.writef(levelFatal, format, args)
	os.Exit(1)
}

// Fatal ...
func (l *Logger) Fatal(args ...interface{}) {
	l.write(levelFatal, args)
	os.Exit(1)
}

// Fatalln ...
func (l *Logger) Fatalln(args ...interface{}) {
	l.write(levelFatal, args)
	os.Exit(1)
}

// Panicf ...
func (l *Logger) Panicf(format string, args ...interface{}) {
	l.writef(levelFatal, format, args)
	panic("")
}

// Panic ...
func (l *Logger) Panic(args ...interface{}) {
	l.write(levelFatal, args)
	panic("")
}

// Panicln ...
func (l *Logger) Panicln(args ...interface{}) {
	l.write(levelFatal, args)
	panic("")
}

// Write writes to the log
func (l *Logger) Write(p []byte) (int, error) {
	var app byte
	var level int
	line := string(p)
	if l.filter != nil {
		line, app, level = l.filter(line, l.tty)
	}
	if level >= l.level {
		write(false, l, app, level, "", []interface{}{line})
	}
	return len(p), nil
}

func (l *Logger) writef(level int, format string, args []interface{}) {
	if level >= l.level {
		write(true, l, l.app, level, format, args)
	}
}

func (l *Logger) write(level int, args []interface{}) {
	if level >= l.level {
		write(false, l, l.app, level, "", args)
	}
}

//go:noinline
func write(useFormat bool, l *Logger, app byte, level int, format string,
	args []interface{}) {
	if l.wr == ioutil.Discard {
		return
	}
	var prefix []byte
	now := time.Now()
	prefix = strconv.AppendInt(prefix, int64(l.pid), 10)
	prefix = append(prefix, ':', app, ' ')
	prefix = now.AppendFormat(prefix, "02 Jan 15:04:05.000")
	prefix = append(prefix, ' ')
	if l.tty && levelColors[level] != "" {
		prefix = append(prefix, "\x1b["+levelColors[level]+"m"...)
		prefix = append(prefix, levelChars[level])
		prefix = append(prefix, "\x1b[0m"...)
	} else {
		prefix = append(prefix, levelChars[level])
	}
	var msg string
	if useFormat {
		msg = fmt.Sprintf(format, args...)
	} else {
		msg = fmt.Sprint(args...)
	}
	for len(msg) > 0 {
		switch msg[len(msg)-1] {
		case '\t', ' ', '\r', '\n':
			msg = msg[:len(msg)-1]
			continue
		}
		break
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.wr, "%s %s\n", prefix, msg)
}

// HashicorpRaftFilter is used as a filter to convert a log message
// from the hashicorp/raft package into redlog structured message.
var HashicorpRaftFilter func(line string, tty bool) (msg string, app byte,
	level int)

func init() {
	HashicorpRaftFilter = func(line string, tty bool) (msg string, app byte,
		level int) {
		msg = string(line)
		idx := strings.IndexByte(msg, ' ')
		if idx != -1 {
			msg = msg[idx+1:]
		}
		idx = strings.IndexByte(msg, ']')
		if idx != -1 && msg[0] == '[' {
			switch msg[1] {
			default: // -> verbose
				level = levelVerbose
			case 'W': // warning -> warning
				level = levelWarning
			case 'E': // error -> warning
				level = levelWarning
			case 'D': // debug -> debug
				level = levelDebug
			case 'V': // verbose -> verbose
				level = levelVerbose
			case 'I': // info -> notice
				level = levelNotice
			}
			msg = msg[idx+1:]
			for len(msg) > 0 && msg[0] == ' ' {
				msg = msg[1:]
			}
		}
		if tty {
			msg = strings.Replace(msg, "[Leader]",
				"\x1b[32m[Leader]\x1b[0m", 1)
			msg = strings.Replace(msg, "[Follower]",
				"\x1b[33m[Follower]\x1b[0m", 1)
			msg = strings.Replace(msg, "[Candidate]",
				"\x1b[36m[Candidate]\x1b[0m", 1)
		}
		return msg, app, level
	}
}

// RedisLogColorizer filters the Redis log output and colorizes it.
func RedisLogColorizer(wr io.Writer) io.Writer {
	if f, ok := wr.(*os.File); !ok || !terminal.IsTerminal(int(f.Fd())) {
		return wr
	}
	pr, pw := io.Pipe()
	go func() {
		rd := bufio.NewReader(pr)
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			parts := strings.Split(line, " ")
			if len(parts) > 5 {
				var color string
				switch parts[4] {
				case ".":
					color = "\x1b[35m"
				case "-":
					color = ""
				case "*":
					color = "\x1b[1m"
				case "#":
					color = "\x1b[33m"
				}
				if color != "" {
					parts[4] = color + parts[4] + "\x1b[0m"
					line = strings.Join(parts, " ")
				}
			}
			os.Stdout.Write([]byte(line))
			continue
		}
	}()
	return pw
}

// GoLogger returns a standard Go log.Logger which when used, will print
// in the Redlog format.
func (l *Logger) GoLogger() *log.Logger {
	rd, wr := io.Pipe()
	gl := log.New(wr, "", 0)
	go func() {
		brd := bufio.NewReader(rd)
		for {
			line, err := brd.ReadBytes('\n')
			if err != nil {
				continue
			}
			l.Printf("%s", line[:len(line)-1])
		}
	}()
	return gl
}
