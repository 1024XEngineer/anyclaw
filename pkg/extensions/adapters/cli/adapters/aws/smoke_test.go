package aws

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestRunPreservesInheritedEnvironment(t *testing.T) {
	t.Setenv("ANYCLAW_TEST_INHERITED", "present")

	client := NewClient(Config{Region: "ap-southeast-1", Profile: "team"})
	var args []string
	if runtime.GOOS == "windows" {
		client.awsPath = "cmd"
		args = []string{"/c", "echo %ANYCLAW_TEST_INHERITED%-%AWS_DEFAULT_REGION%-%AWS_PROFILE%"}
	} else {
		client.awsPath = "sh"
		args = []string{"-c", `printf "%s-%s-%s" "$ANYCLAW_TEST_INHERITED" "$AWS_DEFAULT_REGION" "$AWS_PROFILE"`}
	}

	out, err := client.run(context.Background(), args)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "present-ap-southeast-1-team") {
		t.Fatalf("expected inherited and custom env vars, got %q", out)
	}
}

func TestRunReturnsCommandError(t *testing.T) {
	client := NewClient(Config{AWSPath: "definitely-not-a-real-aws-binary"})
	_, err := client.run(context.Background(), []string{"sts", "get-caller-identity"})
	if err == nil {
		t.Fatal("expected missing binary to fail")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "executable file not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
