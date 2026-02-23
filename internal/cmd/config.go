package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/config"
)

var (
	setAppID          int64
	setInstallationID int64
	setPrivateKeyPath string
	setImportKey      string
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and modify configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set config values",
	RunE:  runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Print config values",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigGet,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print config file location",
	RunE:  runConfigPath,
}

func init() {
	configSetCmd.Flags().Int64Var(&setAppID, "app-id", 0, "GitHub App ID")
	configSetCmd.Flags().Int64Var(&setInstallationID, "installation-id", 0, "GitHub App installation ID")
	configSetCmd.Flags().StringVar(&setPrivateKeyPath, "private-key-path", "", "path to private key PEM file")
	configSetCmd.Flags().StringVar(&setImportKey, "import-key", "", "import private key into OS keyring from PEM file")

	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configPathCmd)
}

func configFilePath() (string, error) {
	if cfgPath != "" {
		return cfgPath, nil
	}
	return config.DefaultPath()
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	if cmd.Flags().Changed("private-key-path") && cmd.Flags().Changed("import-key") {
		return errors.New("--private-key-path and --import-key are mutually exclusive")
	}

	path, err := configFilePath()
	if err != nil {
		return err
	}

	// Load existing or start fresh
	existing, err := config.Load(path)
	if err != nil {
		existing = &config.Config{}
	}

	if cmd.Flags().Changed("app-id") {
		existing.AppID = setAppID
	}
	if cmd.Flags().Changed("installation-id") {
		existing.InstallationID = setInstallationID
	}
	if cmd.Flags().Changed("private-key-path") {
		existing.PrivateKeyPath = setPrivateKeyPath
		existing.KeyInKeyring = false
	}
	if cmd.Flags().Changed("import-key") {
		pemData, err := os.ReadFile(setImportKey)
		if err != nil {
			return fmt.Errorf("read key file: %w", err)
		}
		if err := auth.StorePrivateKey(pemData); err != nil {
			return fmt.Errorf("store key in keyring: %w", err)
		}
		existing.KeyInKeyring = true
		existing.PrivateKeyPath = ""
	}

	if err := config.Save(path, existing); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Config saved to %s\n", path)
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}

	c, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	out := cmd.OutOrStdout()

	if len(args) == 0 {
		fmt.Fprintf(out, "app-id: %d\n", c.AppID)
		fmt.Fprintf(out, "installation-id: %d\n", c.InstallationID)
		fmt.Fprintf(out, "private-key-path: %s\n", c.PrivateKeyPath)
		fmt.Fprintf(out, "key-in-keyring: %v\n", c.KeyInKeyring)
		return nil
	}

	switch args[0] {
	case "app-id":
		fmt.Fprintf(out, "%d\n", c.AppID)
	case "installation-id":
		fmt.Fprintf(out, "%d\n", c.InstallationID)
	case "private-key-path":
		fmt.Fprintf(out, "%s\n", c.PrivateKeyPath)
	case "key-in-keyring":
		fmt.Fprintf(out, "%v\n", c.KeyInKeyring)
	default:
		return fmt.Errorf("unknown config key: %s", args[0])
	}
	return nil
}

func runConfigPath(cmd *cobra.Command, args []string) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), path)
	return nil
}
