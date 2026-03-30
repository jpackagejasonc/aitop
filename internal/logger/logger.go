package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxLogSize = 10 << 20 // 10 MB
)

// Init sets up the default slog logger to write to ~/.aitop/aitop.log.
// The log directory is created with 0700 and the log file with 0600.
// If the log file exceeds maxLogSize it is rotated to aitop.log.1 first.
// If setup fails, logging falls back to stderr.
func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".aitop")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	logPath := filepath.Join(dir, "aitop.log")
	rotateIfNeeded(logPath)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	// Default to Info. Set AITOP_LOG_LEVEL=debug to enable debug output.
	level := slog.LevelInfo
	if strings.EqualFold(os.Getenv("AITOP_LOG_LEVEL"), "debug") {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
	return nil
}

// rotateIfNeeded renames logPath to logPath+".1" when it exceeds maxLogSize.
// Errors are silently ignored — a failed rotation just means we keep appending.
func rotateIfNeeded(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil || info.Size() < maxLogSize {
		return
	}
	_ = os.Rename(logPath, logPath+".1")
}
