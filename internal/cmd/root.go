package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/config"
	"github.com/operator-kit/ghapp-cli/internal/selfupdate"
)

var (
	cfgPath string

	cfg            *config.Config
	tokenGenerator auth.TokenGenerator = &auth.GitHubTokenGenerator{}

	versionStr string
	commitStr  string
	dateStr    string

	updateResult chan string
)

func SetVersionInfo(version, commit, date string) {
	versionStr = version
	commitStr = commit
	dateStr = date
}

var rootCmd = &cobra.Command{
	Use:   "ghapp",
	Short: "GitHub App authentication for git and gh",
	Long:  "Authenticate as a GitHub App, generate installation tokens, and configure git/gh to use them transparently.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		startUpdateCheck(cmd)

		// Commands that don't need config
		name := cmd.Name()
		if name == "ghapp" || name == "version" || name == "setup" || name == "shell-init" || name == "update" {
			return nil
		}
		for c := cmd; c != nil; c = c.Parent() {
			if c == configCmd {
				return nil
			}
		}

		path := cfgPath
		if path == "" {
			var err error
			path, err = config.DefaultPath()
			if err != nil {
				return err
			}
		}

		var err error
		cfg, err = config.Load(path)
		if err != nil {
			return fmt.Errorf("load config: %w\nRun 'ghapp setup' to configure", err)
		}
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if updateResult == nil {
			return nil
		}
		select {
		case latest := <-updateResult:
			if latest != "" {
				fmt.Fprintf(os.Stderr, "\nA new version of ghapp is available: v%s (current: v%s)\nRun 'ghapp update' to upgrade.\n", latest, versionStr)
			}
		default:
			// goroutine hasn't finished, skip silently
		}
		return nil
	},
}

func startUpdateCheck(cmd *cobra.Command) {
	if os.Getenv("GHAPP_NO_UPDATE_CHECK") == "1" {
		return
	}
	if versionStr == "dev" {
		return
	}
	name := cmd.Name()
	if name == "credential-helper" || name == "shell-init" || name == "token" || name == "update" {
		return
	}
	if !selfupdate.ShouldCheck(versionStr) {
		return
	}
	updateResult = make(chan string, 1)
	go func() {
		updateResult <- selfupdate.CheckForUpdate(versionStr)
	}()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "", "config file path")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(tokenCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(credentialHelperCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(configCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
