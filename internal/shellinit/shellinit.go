package shellinit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	ps "github.com/mitchellh/go-ps"
)

// Shell describes a supported shell for hook installation.
type Shell interface {
	Name() string
	HookCode(ghappBin string) string
	EvalLine(ghappBin string) string
	RCFilePath() (string, error)
	UsesConfD() bool
	ConfDFilePath() (string, error)
}

// AllShells returns all supported shell implementations.
func AllShells() []Shell {
	return []Shell{
		&bashShell{},
		&zshShell{},
		&fishShell{},
		&pwshShell{},
	}
}

// ShellByName returns a Shell by name, or nil if not recognized.
func ShellByName(name string) Shell {
	name = strings.ToLower(name)
	for _, sh := range AllShells() {
		if sh.Name() == name {
			return sh
		}
	}
	return nil
}

// ShellOverride allows tests to bypass parent process detection.
var ShellOverride Shell

// DetectShell returns the shell of the parent process.
func DetectShell() Shell {
	if ShellOverride != nil {
		return ShellOverride
	}
	name := detectParentShellName()
	if name == "" {
		return nil
	}
	return ShellByName(name)
}

func detectParentShellName() string {
	ppid := os.Getppid()
	p, err := ps.FindProcess(ppid)
	if err != nil || p == nil {
		return ""
	}

	exe := strings.ToLower(filepath.Base(p.Executable()))
	// Strip .exe on Windows
	exe = strings.TrimSuffix(exe, ".exe")

	switch {
	case exe == "bash" || exe == "sh":
		return "bash"
	case exe == "zsh":
		return "zsh"
	case exe == "fish":
		return "fish"
	case exe == "pwsh" || exe == "powershell":
		return "powershell"
	default:
		return ""
	}
}

// GhappBinPath returns the best path to use for the ghapp binary in hook code.
// Prefers the short name if on PATH, otherwise uses the absolute path.
func GhappBinPath() string {
	if p, err := findOnPath("ghapp"); err == nil {
		return p
	}
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "ghapp"
}

func findOnPath(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	sep := string(os.PathListSeparator)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, dir := range strings.Split(pathEnv, sep) {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found on PATH", name)
}
