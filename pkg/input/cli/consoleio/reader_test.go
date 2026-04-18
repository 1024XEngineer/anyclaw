package consoleio

import (
	"io"
	"os"
	"testing"
)

func TestReaderFallsBackForPipes(t *testing.T) {
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer stdinReader.Close()

	if _, err := io.WriteString(stdinWriter, "hello\n"); err != nil {
		t.Fatalf("io.WriteString: %v", err)
	}
	if err := stdinWriter.Close(); err != nil {
		t.Fatalf("stdinWriter.Close: %v", err)
	}

	reader := NewReader(stdinReader)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if line != "hello\n" {
		t.Fatalf("expected %q, got %q", "hello\n", line)
	}
}
