package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/tokencache"
)

var noCache bool

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Generate and print a fresh installation token",
	RunE:  runToken,
}

func init() {
	tokenCmd.Flags().BoolVar(&noCache, "no-cache", false, "skip token cache, always generate fresh")
}

func runToken(cmd *cobra.Command, args []string) error {
	// Try cache first
	if !noCache {
		if entry := tokencache.ReadCache(cfg.InstallationID); entry != nil {
			fmt.Fprintln(cmd.OutOrStdout(), entry.Token)
			return nil
		}
	}

	key, err := auth.LoadPrivateKeyFromConfig(cfg)
	if err != nil {
		return err
	}

	result, err := tokenGenerator.Generate(cfg.AppID, cfg.InstallationID, key)
	if err != nil {
		return err
	}

	// Write to cache (best-effort)
	if !noCache {
		_ = tokencache.WriteCache(&tokencache.CacheEntry{
			Token:          result.Token,
			Expiry:         result.ExpiresAt,
			InstallationID: cfg.InstallationID,
		})
	}

	fmt.Fprintln(cmd.OutOrStdout(), result.Token)
	return nil
}
