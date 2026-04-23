package consoleio

import (
	"bufio"
	"io"
	"os"
)

type Reader struct {
	fallback *bufio.Reader
	console  lineReader
}

type lineReader interface {
	ReadString(delim byte) (string, error)
}

func NewReader(input io.Reader) *Reader {
	reader := &Reader{}
	if file, ok := input.(*os.File); ok {
		reader.console = newConsoleLineReader(file)
	}
	if reader.console == nil {
		reader.fallback = bufio.NewReader(input)
	}
	return reader
}

func (r *Reader) ReadString(delim byte) (string, error) {
	if r == nil {
		return "", io.EOF
	}
	if r.console != nil {
		return r.console.ReadString(delim)
	}
	if r.fallback == nil {
		return "", io.EOF
	}
	return r.fallback.ReadString(delim)
}
