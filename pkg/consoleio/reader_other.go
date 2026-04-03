//go:build !windows

package consoleio

import "os"

func newConsoleLineReader(*os.File) lineReader {
	return nil
}
