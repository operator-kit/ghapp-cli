package cmd

import (
	"bufio"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/config"
)

var importKey bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure GitHub App credentials",
	RunE:  runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&importKey, "import-key", false, "import private key into OS keyring")
}

func runSetup(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	appID, err := promptInt64(reader, out, "App ID")
	if err != nil {
		return err
	}

	installationID, err := promptInt64(reader, out, "Installation ID")
	if err != nil {
		return err
	}

	fmt.Fprint(out, "Path to private key (.pem): ")
	keyPath, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	keyPath = strings.TrimSpace(keyPath)

	// Read and validate PEM
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read key file: %w", err)
	}

	if err := validatePEM(keyData); err != nil {
		return err
	}

	newCfg := &config.Config{
		AppID:          appID,
		InstallationID: installationID,
	}

	if importKey {
		if err := auth.StorePrivateKey(keyData); err != nil {
			return fmt.Errorf("store key in keyring: %w", err)
		}
		newCfg.KeyInKeyring = true
		fmt.Fprintln(out, "Private key stored in OS keyring.")
	} else {
		newCfg.PrivateKeyPath = keyPath
	}

	// Validate by generating a token
	fmt.Fprint(out, "Validating credentials... ")
	_, err = tokenGenerator.Generate(appID, installationID, keyData)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	fmt.Fprintln(out, "OK")

	// Save config
	savePath := cfgPath
	if savePath == "" {
		savePath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}

	if err := config.Save(savePath, newCfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(out, "\nConfig saved to %s\n", savePath)

	cfg = newCfg

	fmt.Fprintln(out)
	if confirm(cmd, "Configure git + gh auth now?") {
		fmt.Fprintln(out)
		return runAuthConfigure(cmd, nil)
	}

	fmt.Fprintln(out, "\nTo configure later:")
	fmt.Fprintln(out, "  ghapp auth configure     # set up git + gh auth")
	return nil
}

func validatePEM(data []byte) error {
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("invalid PEM file: no PEM block found")
	}
	if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		if _, err2 := x509.ParsePKCS8PrivateKey(block.Bytes); err2 != nil {
			return fmt.Errorf("invalid private key: not PKCS1 (%v) or PKCS8 (%v)", err, err2)
		}
	}
	return nil
}

func promptInt64(reader *bufio.Reader, out io.Writer, label string) (int64, error) {
	fmt.Fprintf(out, "%s: ", label)
	text, err := reader.ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("read input: %w", err)
	}
	val, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", label, err)
	}
	return val, nil
}
