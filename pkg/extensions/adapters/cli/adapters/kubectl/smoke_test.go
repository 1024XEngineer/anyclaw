package kubectl

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestRunPreservesInheritedEnvironment(t *testing.T) {
	t.Setenv("ANYCLAW_TEST_INHERITED", "present")

	client := NewClient(Config{Kubeconfig: "/tmp/config"})
	var args []string
	if runtime.GOOS == "windows" {
		client.kubectlPath = "cmd"
		args = []string{"/c", "echo %ANYCLAW_TEST_INHERITED%-%KUBECONFIG%"}
	} else {
		client.kubectlPath = "sh"
		args = []string{"-c", `printf "%s-%s" "$ANYCLAW_TEST_INHERITED" "$KUBECONFIG"`}
	}

	out, err := client.run(context.Background(), args)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "present-/tmp/config") {
		t.Fatalf("expected inherited and kubeconfig env vars, got %q", out)
	}
}

func TestRunReturnsCommandError(t *testing.T) {
	client := NewClient(Config{KubectlPath: "definitely-not-a-real-kubectl-binary"})
	_, err := client.run(context.Background(), []string{"version", "--client"})
	if err == nil {
		t.Fatal("expected missing binary to fail")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "executable file not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
