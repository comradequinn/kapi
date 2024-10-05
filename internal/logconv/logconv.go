// logconv provides a logr.Logger implementation based on a simple context-aware, structured logFunc
//
// This is required as the ctrl-runtime lib has specified the logr.Logger as its log interface
package logconv

import (
	"context"

	"github.com/go-logr/logr"
)

var _ logr.LogSink = &LogrWrapper{} // implementation guard

type LogrWrapper struct {
	ctx   context.Context
	lf    func(ctx context.Context, level int, msg string, attributes ...any)
	attrs []any
}

func NewLogrWrapper(ctx context.Context, logFunc func(ctx context.Context, level int, msg string, attributes ...any)) logr.Logger {
	lf := func(ctx context.Context, level int, msg string, attributes ...any) {
		logFunc(ctx, level, msg, append(attributes, "dep", "ctrl-runtime")...)
	}

	return logr.New(&LogrWrapper{
		ctx: ctx,
		lf:  lf,
	})
}

func (*LogrWrapper) Init(logr.RuntimeInfo) {}

func (*LogrWrapper) Enabled(level int) bool {
	// return the same logger; level-specific behavior is implemented by the underlying logFunc
	return true
}

func (l *LogrWrapper) Info(level int, msg string, attr ...any) {
	l.lf(l.ctx, 2, msg, attr...)
}

func (l *LogrWrapper) Error(err error, msg string, attr ...any) {
	l.lf(l.ctx, 0, msg, append(attr, "error", err)...)
}

func (l *LogrWrapper) V(level int) logr.LogSink {
	// return the same logger; level-specific behavior is implemented by the underlying logFunc
	return l
}

func (l *LogrWrapper) WithValues(attr ...any) logr.LogSink {
	lw := &LogrWrapper{
		ctx: l.ctx,
		lf:  l.lf,
	}
	lw.attrs = append(l.attrs, attr...)
	return lw
}

func (l *LogrWrapper) WithName(name string) logr.LogSink {
	return l.WithValues("name", name)
}
