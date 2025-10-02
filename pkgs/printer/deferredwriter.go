package printer

import (
	"bytes"
	"io"
)

type DeferredWriter struct {
	buff   bytes.Buffer
	writer io.Writer
}

func NewDeferedWriter(w io.Writer) *DeferredWriter {
	return &DeferredWriter{
		writer: w,
	}
}

func (dw *DeferredWriter) Write(bytes []byte) (int, error) {
	return dw.buff.Write(bytes)
}

func (dw *DeferredWriter) Flush() error {
	_, err := dw.buff.WriteTo(dw.writer)
	return err
}
