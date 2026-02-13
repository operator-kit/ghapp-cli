package shellinit

import (
	"fmt"
	"os"
	"path/filepath"
)

type fishShell struct{}

func (f *fishShell) Name() string { return "fish" }

func (f *fishShell) HookCode(ghappBin string) string {
	return fmt.Sprintf(`function gh --wraps gh
    set -l _ghapp_token (%q token 2>/dev/null)
    if test -n "$_ghapp_token"
        GH_TOKEN="$_ghapp_token" command gh $argv
    else
        command gh $argv
    end
end`, ghappBin)
}

func (f *fishShell) EvalLine(ghappBin string) string {
	return fmt.Sprintf(`%q auth shell-init | source`, ghappBin)
}

func (f *fishShell) RCFilePath() (string, error) {
	configDir, err := fishConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.fish"), nil
}

func (f *fishShell) UsesConfD() bool { return true }

func (f *fishShell) ConfDFilePath() (string, error) {
	configDir, err := fishConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "conf.d", "ghapp.fish"), nil
}

func fishConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "fish"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "fish"), nil
}
