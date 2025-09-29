package printer

import (
	"context"
	"os"

	"github.com/hay-kot/mmdot/pkgs/styles"
)

var ConsolePrinter = New(os.Stdout)

func Ctx(ctx context.Context) *Printer {
	return ConsolePrinter.Ctx(ctx)
}

func WithBase(style styles.RenderFunc) *Printer {
	return ConsolePrinter.WithBase(style)
}

func WithLight(style styles.RenderFunc) *Printer {
	return ConsolePrinter.WithLight(style)
}

func FatalError(err error) {
	ConsolePrinter.FatalError(err)
}

func Title(title string) {
	ConsolePrinter.Title(title)
}

func StatusList(title string, items []StatusListItem) {
	ConsolePrinter.StatusList(title, items)
}

func ListTree(title string, list []Tree) {
	ConsolePrinter.ListTree(title, list)
}

func List(title string, items []string) {
	ConsolePrinter.List(title, items)
}

func LineBreak() {
	ConsolePrinter.LineBreak()
}

func KeyValueValidationError(title string, errors []KeyValueError) {
	ConsolePrinter.KeyValueValidationError(title, errors)
}
