//go:build windows

package consoleio

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

type windowsConsoleLineReader struct {
	handle  windows.Handle
	pending string
}

func newConsoleLineReader(file *os.File) lineReader {
	if file == nil {
		return nil
	}
	handle := windows.Handle(file.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return nil
	}
	return &windowsConsoleLineReader{handle: handle}
}

func (r *windowsConsoleLineReader) ReadString(delim byte) (string, error) {
	if delim != '\n' {
		return "", fmt.Errorf("console reader only supports newline delimiter")
	}

	text := r.pending
	r.pending = ""
	for {
		if idx := strings.IndexByte(text, delim); idx >= 0 {
			r.pending = text[idx+1:]
			return text[:idx+1], nil
		}

		chunk, err := r.readChunk()
		if chunk != "" {
			text += chunk
		}
		if err != nil {
			if text != "" {
				return text, err
			}
			return "", err
		}
	}
}

func (r *windowsConsoleLineReader) readChunk() (string, error) {
	buf := make([]uint16, 256)
	var read uint32
	if err := windows.ReadConsole(r.handle, &buf[0], uint32(len(buf)), &read, nil); err != nil {
		return "", err
	}
	if read == 0 {
		return "", io.EOF
	}
	return string(utf16.Decode(buf[:read])), nil
}
