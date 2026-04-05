package clihub

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type ExecOptions struct {
	JSON              bool
	AutoInstall       bool
	PreferLocalSrc    bool
	RetryAfterInstall bool
	RequestedCwd      string
}

type ResolvedCommand struct {
	Args  []string
	Cwd   string
	Shell string
}

func ResolveCommand(status EntryStatus, args []string, opts ExecOptions) (ResolvedCommand, error) {
	baseArgs, cwd, err := ResolveInvocation(status, opts.RequestedCwd, opts)
	if err != nil {
		return ResolvedCommand{}, err
	}

	if opts.JSON && ShouldInjectJSON(args) {
		args = append([]string{"--json"}, args...)
	}

	return ResolvedCommand{
		Args:  append(baseArgs, args...),
		Cwd:   cwd,
		Shell: DefaultShell(),
	}, nil
}

func (cmd ResolvedCommand) ShellCommand() string {
	return JoinArgsForShell(cmd.Args, cmd.Shell)
}

func ResolveInvocation(status EntryStatus, requestedCwd string, opts ExecOptions) ([]string, string, error) {
	if status.Installed && strings.TrimSpace(status.ExecutablePath) != "" {
		return []string{status.ExecutablePath}, strings.TrimSpace(requestedCwd), nil
	}

	hasLocalSrc := strings.TrimSpace(status.DevModule) != "" && strings.TrimSpace(status.SourcePath) != ""
	if hasLocalSrc && (opts.PreferLocalSrc || !status.Installed) {
		cmdArgs, err := pythonModuleArgs(status.DevModule)
		if err != nil {
			return nil, "", err
		}
		return cmdArgs, status.SourcePath, nil
	}

	if !status.Installed && opts.AutoInstall && strings.TrimSpace(status.InstallCmd) != "" {
		if err := RunInstall(status); err != nil {
			if opts.RetryAfterInstall && hasLocalSrc {
				cmdArgs, resolveErr := pythonModuleArgs(status.DevModule)
				if resolveErr != nil {
					return nil, "", resolveErr
				}
				return cmdArgs, status.SourcePath, nil
			}
			return nil, "", fmt.Errorf("auto-install failed for %s: %w", status.Name, err)
		}

		if path, err := exec.LookPath(strings.TrimSpace(status.EntryPoint)); err == nil {
			return []string{path}, strings.TrimSpace(requestedCwd), nil
		}

		if hasLocalSrc {
			cmdArgs, err := pythonModuleArgs(status.DevModule)
			if err != nil {
				return nil, "", err
			}
			return cmdArgs, status.SourcePath, nil
		}

		return nil, "", fmt.Errorf("auto-install succeeded but %s not found in PATH", status.EntryPoint)
	}

	if hasLocalSrc {
		cmdArgs, err := pythonModuleArgs(status.DevModule)
		if err != nil {
			return nil, "", err
		}
		return cmdArgs, status.SourcePath, nil
	}

	if strings.TrimSpace(status.InstallCmd) != "" {
		return nil, "", fmt.Errorf("CLI Hub entry %s is not runnable yet; install it first: %s", status.Name, status.InstallCmd)
	}

	return nil, "", fmt.Errorf("CLI Hub entry %s is not runnable yet", status.Name)
}

func RunInstall(status EntryStatus) error {
	installCmd := strings.TrimSpace(status.InstallCmd)
	if installCmd == "" {
		return fmt.Errorf("no install command available for %s", status.Name)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/d", "/s", "/c", installCmd)
	} else {
		cmd = exec.Command("sh", "-c", installCmd)
	}

	if dir := installWorkDir(status); dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ShouldInjectJSON(args []string) bool {
	for _, arg := range args {
		trimmed := strings.TrimSpace(strings.ToLower(arg))
		if trimmed == "--json" || trimmed == "--help" || trimmed == "-h" || trimmed == "--version" {
			return false
		}
	}
	return true
}

func DefaultShell() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "sh"
}

func JoinArgsForShell(args []string, shellName string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, QuoteArgForShell(arg, shellName))
	}
	return strings.Join(quoted, " ")
}

func QuoteArgForShell(value string, shellName string) string {
	switch strings.ToLower(strings.TrimSpace(shellName)) {
	case "powershell", "pwsh":
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	default:
		return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
	}
}

func installWorkDir(status EntryStatus) string {
	source := strings.TrimSpace(status.SourcePath)
	if source == "" {
		return ""
	}

	root := filepath.Dir(filepath.Dir(source))
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		return root
	}
	return ""
}

func pythonModuleArgs(module string) ([]string, error) {
	module = strings.TrimSpace(module)
	if module == "" {
		return nil, fmt.Errorf("python module is required")
	}

	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath("py"); err == nil {
			return []string{path, "-3", "-m", module}, nil
		}
		for _, candidate := range []string{"python", "python3"} {
			if path, err := exec.LookPath(candidate); err == nil {
				return []string{path, "-m", module}, nil
			}
		}
	} else {
		for _, candidate := range []string{"python3", "python"} {
			if path, err := exec.LookPath(candidate); err == nil {
				return []string{path, "-m", module}, nil
			}
		}
	}

	return nil, fmt.Errorf("python launcher not found in PATH")
}
