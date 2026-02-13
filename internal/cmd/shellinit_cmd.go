package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/operator-kit/ghapp-cli/internal/shellinit"
)

var shellFlag string

var shellInitCmd = &cobra.Command{
	Use:    "shell-init",
	Short:  "Print shell hook code for gh CLI authentication",
	Hidden: true,
	RunE:   runShellInit,
}

func init() {
	shellInitCmd.Flags().StringVar(&shellFlag, "shell", "", "override shell detection (bash, zsh, fish, powershell)")
}

func runShellInit(cmd *cobra.Command, args []string) error {
	var sh shellinit.Shell

	if shellFlag != "" {
		sh = shellinit.ShellByName(shellFlag)
		if sh == nil {
			return fmt.Errorf("unsupported shell: %s", shellFlag)
		}
	} else {
		sh = shellinit.DetectShell()
		if sh == nil {
			return fmt.Errorf("could not detect shell — use --shell flag")
		}
	}

	ghappBin := shellinit.GhappBinPath()
	fmt.Fprintln(cmd.OutOrStdout(), sh.HookCode(ghappBin))
	return nil
}
