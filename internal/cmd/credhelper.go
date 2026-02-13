package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/gitcred"
	"github.com/operator-kit/ghapp-cli/internal/tokencache"
)

var credentialHelperCmd = &cobra.Command{
	Use:    "credential-helper",
	Short:  "Git credential helper (called by git, not user)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE:   runCredentialHelper,
}

func runCredentialHelper(cmd *cobra.Command, args []string) error {
	action := args[0]

	// Only handle "get" — store/erase are no-ops
	if action != "get" {
		return nil
	}

	req, err := gitcred.Parse(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("parse credential request: %w", err)
	}

	// Only respond for github.com over https
	if req.Host != "github.com" || req.Protocol != "https" {
		return nil
	}

	// Try cache first
	if entry := tokencache.ReadCache(cfg.InstallationID); entry != nil {
		return gitcred.WriteResponse(cmd.OutOrStdout(), "x-access-token", entry.Token, entry.Expiry.Unix())
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
	_ = tokencache.WriteCache(&tokencache.CacheEntry{
		Token:          result.Token,
		Expiry:         result.ExpiresAt,
		InstallationID: cfg.InstallationID,
	})

	return gitcred.WriteResponse(cmd.OutOrStdout(), "x-access-token", result.Token, result.ExpiresAt.Unix())
}
