package log

import (
	"io"
	"log/slog"
	"os"
)

var L *slog.Logger

func Init(level string, file string) {
	var w io.Writer = os.Stdout
	if file != "" {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("open log file failed, fallback to stdout", "err", err)
		} else {
			w = io.MultiWriter(os.Stdout, f)
		}
	}

	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	case "silent":
		lvl = slog.Level(100)
	default:
		lvl = slog.LevelInfo
	}

	L = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl}))
}
