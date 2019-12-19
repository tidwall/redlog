// Package redlog provides a Redis compatible logger.
//   http://build47.com/redis-log-format-levels/
package redlog

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

// Logger represents a logger
type Logger struct {
	mu     sync.RWMutex
	wr     io.Writer
	level  logLevel
	tty    bool
	pid    int
	app    byte
	filter func(line string, tty bool) (msg string, app byte, level logLevel)
	parent *Logger
	idups  bool
	last   string
	lastt  time.Time
}

type logLevel int

const (
	logLevelDebug   logLevel = 0 // '.'
	logLevelVerbose logLevel = 1 // '-'
	logLevelNotice  logLevel = 2 // '*'
	logLevelWarning logLevel = 3 // '#'
)

func (level logLevel) String() string {
	switch level {
	default:
		return string('?')
	case logLevelDebug:
		return string('.')
	case logLevelVerbose:
		return string('-')
	case logLevelNotice:
		return string('*')
	case logLevelWarning:
		return string('#')
	}
}

// New creates a new Logger
func New(wr io.Writer) *Logger {
	return &Logger{
		wr:    wr,
		level: logLevelNotice,
		pid:   os.Getpid(),
		tty:   istty(wr),
		app:   'M',
	}
}

// Sub creates a logger that inherits the properties of the caller logger.
// The app parameter will be used in the output message.
func (l *Logger) Sub(app byte) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return &Logger{
		parent: l,
		app:    app,
	}
}

// SetIgnoreDups ignores further duplicate messages
func (l *Logger) SetIgnoreDups(t bool) {
	if l.parent != nil {
		l.parent.SetIgnoreDups(t)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.idups = t
}

// SetLevel sets the level of the logger.
//   0 - Debug
//   1 - Verbose
//   2 - Notice
//   3 - Warning
func (l *Logger) SetLevel(level int) {
	if l.parent != nil {
		l.parent.SetLevel(level)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if level < int(logLevelDebug) {
		level = int(logLevelDebug)
	} else if level > int(logLevelWarning) {
		level = int(logLevelWarning)
	}
	l.level = logLevel(level)
}

func (l *Logger) doesAccept(level logLevel) bool {
	if l.parent != nil {
		return l.parent.doesAccept(level)
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

// SetFilter set the logger filter.
// A filter can be used to process standard writes into
// structured redlog format.
func (l *Logger) SetFilter(filter func(line string, tty bool) (msg string, app byte, level logLevel)) {
	if l.parent != nil {
		l.parent.SetFilter(filter)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.filter = filter
}

// Write writes to the log
func (l *Logger) Write(p []byte) (int, error) {
	return l.iwrite(p, l.app)
}

// Write writes to the log
func (l *Logger) iwrite(p []byte, app byte) (int, error) {
	if l.parent != nil {
		return l.parent.iwrite(p, app)
	}
	l.mu.RLock()
	filter := l.filter
	l.mu.RUnlock()
	if filter == nil {
		return l.wr.Write(p)
	}
	msg, _, level := filter(strings.TrimSpace(string(p)), l.tty)
	l.logf(app, level, "%s", false, msg)
	return len(p), nil
}

func (l *Logger) logf(app byte, level logLevel, format string, noformat bool, args ...interface{}) {
	if l.parent != nil {
		l.parent.logf(l.app, level, format, noformat, args...)
		return
	}
	if !l.doesAccept(level) {
		return
	}
	now := time.Now()
	tm := now.Format("02 Jan 15:04:05.000")
	var msg string
	if noformat {
		msg = fmt.Sprint(args...)
	} else {
		msg = fmt.Sprintf(format, args...)
	}
	dup := false
	l.mu.Lock()
	if l.idups {
		dup = l.last == msg && !l.lastt.IsZero() &&
			now.Sub(l.lastt) < time.Millisecond
		l.last = msg
		l.lastt = now
	}
	l.mu.Unlock()
	if !dup {
		l.write(fmt.Sprintf("%d:%c %s %s %s\n", l.pid, app, tm, level, msg))
	}
}

// Debugf writes a debug message.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.logf(l.app, logLevelDebug, format, false, args...)
}

// Verbosef writes a verbose message.
func (l *Logger) Verbosef(format string, args ...interface{}) {
	l.logf(l.app, logLevelVerbose, format, false, args...)
}

// Noticef writes a notice message.
func (l *Logger) Noticef(format string, args ...interface{}) {
	l.logf(l.app, logLevelNotice, format, false, args...)
}

// Warningf writes a warning message.
func (l *Logger) Warningf(format string, args ...interface{}) {
	l.logf(l.app, logLevelWarning, format, false, args...)
}

// Fatalf writes a warning message and exit process with exit code 1.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.logf(l.app, logLevelWarning, format, false, args...)
	os.Exit(1)
}

// Printf writes a default message.
func (l *Logger) Printf(format string, args ...interface{}) {
	l.logf(l.app, logLevelNotice, format, false, args...)
}

// Debug writes a debug message.
func (l *Logger) Debug(args ...interface{}) {
	l.logf(l.app, logLevelDebug, "", true, args...)
}

// Verbose writes a verbose message.
func (l *Logger) Verbose(args ...interface{}) {
	l.logf(l.app, logLevelVerbose, "", true, args...)
}

// Notice writes a notice message.
func (l *Logger) Notice(args ...interface{}) {
	l.logf(l.app, logLevelNotice, "", true, args...)
}

// Warning writes a warning message.
func (l *Logger) Warning(args ...interface{}) {
	l.logf(l.app, logLevelWarning, "", true, args...)
}

// Fatal writes a warning message and exit process with exit code 1.
func (l *Logger) Fatal(args ...interface{}) {
	l.logf(l.app, logLevelWarning, "", true, args...)
	os.Exit(1)
}

// Print writes a default message.
func (l *Logger) Print(args ...interface{}) {
	l.logf(l.app, logLevelNotice, "", true, args...)
}

// Debugln writes a debug message.
func (l *Logger) Debugln(args ...interface{}) {
	l.logf(l.app, logLevelDebug, "", true, args...)
}

// Verboseln writes a verbose message.
func (l *Logger) Verboseln(args ...interface{}) {
	l.logf(l.app, logLevelVerbose, "", true, args...)
}

// Noticeln writes a notice message.
func (l *Logger) Noticeln(args ...interface{}) {
	l.logf(l.app, logLevelNotice, "", true, args...)
}

// Warningln writes a warning message.
func (l *Logger) Warningln(args ...interface{}) {
	l.logf(l.app, logLevelWarning, "", true, args...)
}

// Fatalln writes a warning message and exit process with exit code 1.
func (l *Logger) Fatalln(args ...interface{}) {
	l.logf(l.app, logLevelWarning, "", true, args...)
	os.Exit(1)
}

// Println writes a default message.
func (l *Logger) Println(args ...interface{}) {
	l.logf(l.app, logLevelNotice, "", true, args...)
}

func (l *Logger) write(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tty {
		parts := strings.SplitN(msg, " ", 6)
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
			msg = strings.Join(parts, " ")
		}
	}
	io.WriteString(l.wr, strings.TrimSpace(msg)+"\n")
}

func istty(wr io.Writer) bool {
	if f, ok := wr.(*os.File); ok {
		return terminal.IsTerminal(int(f.Fd()))
	}
	return false
}

// HashicorpRaftFilter is used as a filter to convert a log message
// from the hashicorp/raft package into redlog structured message.
var HashicorpRaftFilter func(line string, tty bool) (msg string, app byte, level logLevel)

func init() {
	HashicorpRaftFilter = func(line string, tty bool) (msg string, app byte, level logLevel) {
		msg = line
		idx := strings.IndexByte(msg, ' ')
		if idx != -1 {
			msg = msg[idx+1:]
		}
		idx = strings.IndexByte(msg, ']')
		if idx != -1 && msg[0] == '[' {
			switch msg[1] {
			default: // -> verbose
				level = logLevelVerbose
			case 'W': // warning -> warning
				level = logLevelWarning
			case 'E': // error -> warning
				level = logLevelWarning
			case 'D': // debug -> debug
				level = logLevelDebug
			case 'V': // verbose -> verbose
				level = logLevelVerbose
			case 'I': // info -> notice
				level = logLevelNotice
			}
			msg = msg[idx+1:]
			for len(msg) > 0 && msg[0] == ' ' {
				msg = msg[1:]
			}
		}
		if tty {
			msg = strings.Replace(msg, "[Leader]", "\x1b[32m[Leader]\x1b[0m", 1)
			msg = strings.Replace(msg, "[Follower]", "\x1b[33m[Follower]\x1b[0m", 1)
			msg = strings.Replace(msg, "[Candidate]", "\x1b[36m[Candidate]\x1b[0m", 1)
		}
		return msg, app, level
	}
}

// RedisLogColorizer filters the Redis log output and colorizes it.
func RedisLogColorizer(wr io.Writer) io.Writer {
	if !istty(wr) {
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
