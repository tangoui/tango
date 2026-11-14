// Package log sets up application logging on log/slog: records go to
// stderr and to a log file that rotates by size, and the page can log
// through ipc (Handler).
//
// The package name shadows the standard library's log; import it under
// another name if you need both, e.g. applog "github.com/tangoui/tango/service/log".
package log

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/service/paths"
)

// Defaults for Options.
const (
	DefaultFile    = "app.log"
	DefaultMaxSize = 5 << 20
	DefaultKeep    = 3
)

// Options configures New. The zero value of each field picks a default.
type Options struct {
	_ [0]func()

	// AppID picks the default Dir: paths.Logs(AppID). Required if Dir is
	// empty.
	AppID string
	// Dir is the directory of the log files; created if missing.
	Dir string
	// File is the current log file's name in Dir (DefaultFile).
	File string
	// MaxSize is the size in bytes past which the file rotates
	// (DefaultMaxSize).
	MaxSize int64
	// Keep is how many rotated files (File.1 newest … File.Keep oldest)
	// are kept (DefaultKeep).
	Keep int
	// Level is the minimum level logged (slog.LevelInfo).
	Level slog.Leveler
	// Console receives a copy of every record (os.Stderr). Use io.Discard
	// to log to the file only.
	Console io.Writer
}

// Logger is a *slog.Logger writing to the console and a rotating file.
// Shutdown closes the file, so a Logger can be registered as a service.
type Logger struct {
	*slog.Logger
	file *RotatingFile
}

// New opens the log file and returns a Logger writing text records to it
// and to the console.
func New(opts Options) (*Logger, error) {
	dir := opts.Dir
	if dir == "" {
		if opts.AppID == "" {
			return nil, errors.New("log: Options needs Dir or AppID")
		}
		d, err := paths.Logs(opts.AppID)
		if err != nil {
			return nil, fmt.Errorf("log: %w", err)
		}
		dir = d
	}
	name := opts.File
	if name == "" {
		name = DefaultFile
	}
	f, err := OpenRotating(filepath.Join(dir, name), opts.MaxSize, opts.Keep)
	if err != nil {
		return nil, err
	}
	console := opts.Console
	if console == nil {
		console = os.Stderr
	}
	h := slog.NewTextHandler(tee{file: f, console: console}, &slog.HandlerOptions{Level: opts.Level})
	return &Logger{Logger: slog.New(h), file: f}, nil
}

// Path returns the current log file's path.
func (l *Logger) Path() string { return l.file.path }

// Shutdown closes the log file. Records logged afterwards reach only the
// console.
func (l *Logger) Shutdown() error { return l.file.Close() }

// tee writes to the file and, best effort, to the console: a broken
// console (e.g. no stderr in a GUI app on Windows) must not lose records.
type tee struct {
	file    io.Writer
	console io.Writer
}

func (t tee) Write(p []byte) (int, error) {
	_, _ = t.console.Write(p)
	n, err := t.file.Write(p)
	if errors.Is(err, os.ErrClosed) {
		return len(p), nil
	}
	return n, err
}

// RotatingFile is an append-only file that, when a write would take it past
// its maximum size, is renamed to path.1 (path.1 to path.2, and so on,
// dropping the oldest) and started afresh. Safe for concurrent use.
type RotatingFile struct {
	path    string
	maxSize int64
	keep    int

	mu   sync.Mutex
	f    *os.File
	size int64
}

// OpenRotating opens (appending) or creates the file at path, creating its
// directory. maxSize <= 0 means DefaultMaxSize; keep <= 0 means
// DefaultKeep.
func OpenRotating(path string, maxSize int64, keep int) (*RotatingFile, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if keep <= 0 {
		keep = DefaultKeep
	}
	r := &RotatingFile{path: path, maxSize: maxSize, keep: keep}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	if err := r.open(); err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	return r, nil
}

func (r *RotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, fi.Size()
	return nil
}

// Write appends p, rotating first if p would not fit. A single write
// larger than the maximum goes into a file of its own.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, os.ErrClosed
	}
	if r.size > 0 && r.size+int64(len(p)) > r.maxSize {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate shifts the files and reopens path. If the shift fails, writing
// continues in the old file.
func (r *RotatingFile) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	r.f = nil
	backup := func(i int) string { return fmt.Sprintf("%s.%d", r.path, i) }
	_ = os.Remove(backup(r.keep))
	for i := r.keep - 1; i >= 1; i-- {
		_ = os.Rename(backup(i), backup(i+1)) // missing backups are fine
	}
	renameErr := os.Rename(r.path, backup(1))
	if err := r.open(); err != nil {
		return err
	}
	return renameErr
}

// Close closes the file. Later writes fail with os.ErrClosed.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Method is the ipc method Handler registers.
const Method = "Log.Write"

// Handler lets the page log through l (slog.Default() if nil). The page
// calls
//
//	tango.call("Log.Write", "warn", "upload failed", {file: "a.png"})
//
// with a level ("debug", "info", "warn" or "error"; anything else is info),
// a message and optional attributes. Records carry source=page.
func Handler(r *ipc.Router, l *slog.Logger) {
	r.Handle(Method, func(ctx context.Context, raw json.RawMessage) (any, error) {
		logger := l
		if logger == nil {
			logger = slog.Default()
		}
		var args []json.RawMessage
		if err := json.Unmarshal(raw, &args); err != nil || len(args) < 2 || len(args) > 3 {
			return nil, ipc.NewError(ipc.CodeBadArgs, "want (level, message[, attrs])")
		}
		var level, msg string
		var attrs map[string]any
		if json.Unmarshal(args[0], &level) != nil || json.Unmarshal(args[1], &msg) != nil {
			return nil, ipc.NewError(ipc.CodeBadArgs, "level and message must be strings")
		}
		if len(args) == 3 && json.Unmarshal(args[2], &attrs) != nil {
			return nil, ipc.NewError(ipc.CodeBadArgs, "attrs must be an object")
		}
		kv := make([]any, 0, 2+2*len(attrs))
		kv = append(kv, "source", "page")
		for k, v := range attrs {
			kv = append(kv, k, v)
		}
		logger.Log(ctx, parseLevel(level), msg, kv...)
		return nil, nil
	})
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug", "trace":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}
