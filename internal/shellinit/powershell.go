package shellinit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type pwshShell struct{}

func (p *pwshShell) Name() string { return "powershell" }

func (p *pwshShell) HookCode(ghappBin string) string {
	return fmt.Sprintf(`function gh {
    $token = & %q token 2>$null
    if ($token) {
        $env:GH_TOKEN = $token
        & (Get-Command gh -CommandType Application | Select-Object -First 1).Source @args
        Remove-Item Env:\GH_TOKEN -ErrorAction SilentlyContinue
    } else {
        & (Get-Command gh -CommandType Application | Select-Object -First 1).Source @args
    }
}`, ghappBin)
}

func (p *pwshShell) EvalLine(ghappBin string) string {
	return fmt.Sprintf(`& %q auth shell-init | Invoke-Expression`, ghappBin)
}

func (p *pwshShell) RCFilePath() (string, error) {
	return pwshProfilePath()
}

func (p *pwshShell) UsesConfD() bool          { return false }
func (p *pwshShell) ConfDFilePath() (string, error) { return "", nil }

func pwshProfilePath() (string, error) {
	// Try pwsh first, then powershell
	for _, bin := range []string{"pwsh", "powershell"} {
		out, err := exec.Command(bin, "-NoProfile", "-Command", "$PROFILE").Output()
		if err == nil {
			path := strings.TrimSpace(string(out))
			if path != "" {
				return path, nil
			}
		}
	}
	// Fallback to default paths
	if runtime.GOOS == "windows" {
		docs := filepath.Join(os.Getenv("USERPROFILE"), "Documents")
		return filepath.Join(docs, "PowerShell", "Microsoft.PowerShell_profile.ps1"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), nil
}
