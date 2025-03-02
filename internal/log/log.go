package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

////////////////////////////////////////////////////////////////////////////////

var (
	FileName  string
	Debugging bool

	logger *slog.Logger = slog.Default()
)

////////////////////////////////////////////////////////////////////////////////

func Init() {
	var (
		err error
		w   io.Writer
		lv  = &slog.LevelVar{}
		h   slog.Handler
	)

	if FileName != "" {
		if w, err = os.OpenFile(
			FileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666,
		); err != nil {
			Fatal("error initializing log", err)
		}
	} else {
		w = os.Stderr
	}
	if Debugging {
		lv.Set(slog.LevelDebug)
	} else {
		lv.Set(slog.LevelInfo)
	}

	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     lv,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				source := a.Value.Any().(*slog.Source)
				source.File = filepath.Base(source.File)
			}
			return a
		},
	}

	if filepath.Ext(FileName) == ".json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}

	logger = slog.New(h)
	slog.SetDefault(logger)
}

////////////////////////////////////////////////////////////////////////////////

func logAttrs(level slog.Level, msg string, attrs ...slog.Attr) {
	ctx := context.Background()
	l := slog.Default()
	if !l.Enabled(ctx, level) {
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, sourcePC())
	if len(attrs) != 0 {
		r.AddAttrs(attrs...)
	}
	logger.Handler().Handle(ctx, r)
}

func sourcePC() uintptr {
	var pcs [1]uintptr
	// skip: Callers, sourcePC, logAttrs, Debug|Info|Error|Fatal
	runtime.Callers(4, pcs[:])
	return pcs[0]
}

////////////////////////////////////////////////////////////////////////////////

func Debug(msg string, attrs ...slog.Attr) {
	logAttrs(slog.LevelDebug, msg, attrs...)
}

func Info(msg string, attrs ...slog.Attr) {
	logAttrs(slog.LevelInfo, msg, attrs...)
}

func Warn(msg string, attrs ...slog.Attr) {
	logAttrs(slog.LevelWarn, msg, attrs...)
}

func Error(msg string, err error, attrs ...slog.Attr) {
	var attrsE []slog.Attr

	if err != nil {
		attrsE = make([]slog.Attr, 1, len(attrs)+1)
		attrsE[0] = slog.Any("error", err)
		attrsE = append(attrsE, attrs...)
	} else {
		attrsE = attrs
	}

	logAttrs(slog.LevelError, msg, attrsE...)
}

func Fatal(msg string, err error, attrs ...slog.Attr) {
	var attrsE []slog.Attr

	if err != nil {
		attrsE = make([]slog.Attr, 1, len(attrs)+1)
		attrsE[0] = slog.Any("error", err)
		attrsE = append(attrsE, attrs...)
	} else {
		attrsE = attrs
	}

	logAttrs(slog.LevelError, msg, attrsE...)
	os.Exit(1)
}

////////////////////////////////////////////////////////////////////////////////
