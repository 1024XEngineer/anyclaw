package main

import (
	"fmt"
	"io"
	"os"
)

var consoleUTF8WarningWriter io.Writer = os.Stderr

func configureConsoleUTF8() {
	configureConsoleUTF8Platform()
}

func printConsoleUTF8Warning(format string, args ...any) {
	if consoleUTF8WarningWriter == nil {
		return
	}
	fmt.Fprintf(consoleUTF8WarningWriter, "Warning: "+format+"\n", args...)
}
