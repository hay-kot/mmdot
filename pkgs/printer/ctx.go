package printer

import (
	"context"
	"io"
)

type ctxkey string

const writerKey = ctxkey("writerKey")

// WithWriter sets the writer to be used within the context of the printer
// function.
func WithWriter(ctx context.Context, writer io.Writer) context.Context {
	return context.WithValue(ctx, writerKey, writer)
}

// GetWriter returns the writer from the context, or the fallback if provided
func GetWriter(ctx context.Context) (io.Writer, bool) {
	w, ok := ctx.Value(writerKey).(io.Writer)
	return w, ok
}
