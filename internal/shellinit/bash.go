package shellinit

import (
	"fmt"
	"os"
	"path/filepath"
)

type bashShell struct{}

func (b *bashShell) Name() string { return "bash" }

func (b *bashShell) HookCode(ghappBin string) string {
	return fmt.Sprintf(`gh() {
    local _ghapp_token
    _ghapp_token=$(%q token 2>/dev/null)
    if [ -n "$_ghapp_token" ]; then
        GH_TOKEN="$_ghapp_token" command gh "$@"
    else
        command gh "$@"
    fi
}`, ghappBin)
}

func (b *bashShell) EvalLine(ghappBin string) string {
	return fmt.Sprintf(`eval "$(%q auth shell-init)"`, ghappBin)
}

func (b *bashShell) RCFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bashrc"), nil
}

func (b *bashShell) UsesConfD() bool          { return false }
func (b *bashShell) ConfDFilePath() (string, error) { return "", nil }
