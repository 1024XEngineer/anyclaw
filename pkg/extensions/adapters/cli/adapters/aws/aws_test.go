package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("ANYCLAW_AWS_ADAPTER_HELPER") == "1" || strings.HasPrefix(strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"), "aws-helper") {
		payload := map[string]string{
			"aws_access_key_id":  os.Getenv("AWS_ACCESS_KEY_ID"),
			"aws_default_region": os.Getenv("AWS_DEFAULT_REGION"),
			"aws_profile":        os.Getenv("AWS_PROFILE"),
			"ordinary_inherited": os.Getenv("ANYCLAW_AWS_INHERITED"),
		}
		data, _ := json.Marshal(payload)
		fmt.Print(string(data))
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestRunInheritsEnvironmentWhenOverridingAWSSettings(t *testing.T) {
	t.Setenv("ANYCLAW_AWS_INHERITED", "kept")
	t.Setenv("AWS_ACCESS_KEY_ID", "access-key")
	client := NewClient(Config{
		AWSPath: fakeAWSPath(t),
		Region:  "us-west-2",
		Profile: "dev",
	})

	out, err := client.Run(context.Background(), []string{"sts", "get-caller-identity"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode helper output: %v", err)
	}
	if got["ordinary_inherited"] != "kept" {
		t.Fatalf("expected ordinary environment to be inherited, got %#v", got)
	}
	if got["aws_access_key_id"] != "access-key" {
		t.Fatalf("expected AWS_ACCESS_KEY_ID to be inherited, got %#v", got)
	}
	if got["aws_default_region"] != "us-west-2" {
		t.Fatalf("expected region override, got %#v", got)
	}
	if got["aws_profile"] != "dev" {
		t.Fatalf("expected profile override, got %#v", got)
	}
}

func fakeAWSPath(t *testing.T) string {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	path := filepath.Join(t.TempDir(), "aws-helper.exe")
	copyExecutable(t, exe, path)
	return path
}

func copyExecutable(t *testing.T, src, dst string) {
	t.Helper()

	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open source executable: %v", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("create helper executable: %v", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy helper executable: %v", err)
	}
}
