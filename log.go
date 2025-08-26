package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fatih/color"
)

// PrettyHandler is a custom slog.Handler for colorful log output.
type PrettyHandler struct {
	Out   *os.File
	Level slog.Level
}

// Enabled enables all log levels.
func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.Level
}

// Handle prints the record with colorized level and key-value attributes.
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	// Colorize level
	level := r.Level.String()
	var coloredLevel string
	switch r.Level {
	case slog.LevelDebug:
		coloredLevel = color.MagentaString(level)
	case slog.LevelInfo:
		coloredLevel = color.BlueString(level)
	case slog.LevelWarn:
		coloredLevel = color.YellowString(level)
	case slog.LevelError:
		coloredLevel = color.RedString(level)
	default:
		coloredLevel = level
	}

	// Time
	ts := r.Time.Format("2006-01-02 15:04:05")

	// Message
	msg := r.Message

	// Collect key-values into a slice
	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		keyCol := color.CyanString(a.Key)          // colorize key
		valCol := color.GreenString("%v", a.Value) // colorize value
		attrs = append(attrs, fmt.Sprintf("%s=%s", keyCol, valCol))
		return true
	})

	// Join and print all
	out := fmt.Sprintf("[%s] %s %-5s %s %s", ts, coloredLevel, r.Level, msg, strings.Join(attrs, " "))
	fmt.Fprintln(h.Out, out)
	return nil
}

// WithAttrs returns self (for slog.Handler interface).
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *PrettyHandler) WithGroup(name string) slog.Handler       { return h }
