package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/config"
	"github.com/operator-kit/ghapp-cli/internal/tokencache"
)

func main() {
	token := resolveToken()
	realGh := resolveRealGh()
	if realGh == "" {
		fmt.Fprintln(os.Stderr, "ghapp-gh: could not find real gh on PATH")
		os.Exit(1)
	}

	cmd := exec.Command(realGh, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if token != "" {
		cmd.Env = appendEnv(os.Environ(), "GH_TOKEN="+token)
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func resolveToken() string {
	cfg, err := loadConfig()
	if err != nil {
		return ""
	}

	// Try cache
	if entry := tokencache.ReadCache(cfg.InstallationID); entry != nil {
		return entry.Token
	}

	// Generate fresh
	key, err := auth.LoadPrivateKeyFromConfig(cfg)
	if err != nil {
		return ""
	}

	gen := &auth.GitHubTokenGenerator{}
	result, err := gen.Generate(cfg.AppID, cfg.InstallationID, key)
	if err != nil {
		return ""
	}

	// Cache it
	_ = tokencache.WriteCache(&tokencache.CacheEntry{
		Token:          result.Token,
		Expiry:         result.ExpiresAt,
		InstallationID: cfg.InstallationID,
	})

	return result.Token
}

func loadConfig() (*config.Config, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	return config.Load(path)
}

// resolveRealGh finds the first `gh` on PATH that is not in the same directory
// as this binary (to avoid recursive invocation).
func resolveRealGh() string {
	selfDir := selfDirectory()
	ghName := "gh"
	if runtime.GOOS == "windows" {
		ghName = "gh.exe"
	}

	pathEnv := os.Getenv("PATH")
	for _, dir := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		candidate := filepath.Join(dir, ghName)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}

		// Resolve symlinks for both candidate and self dir
		candidateReal, err := filepath.EvalSymlinks(filepath.Dir(candidate))
		if err != nil {
			candidateReal = filepath.Dir(candidate)
		}

		if strings.EqualFold(candidateReal, selfDir) {
			continue // skip ourselves
		}

		return candidate
	}
	return ""
}

func selfDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return filepath.Dir(exe)
	}
	return filepath.Dir(real)
}

func appendEnv(env []string, kv string) []string {
	key := strings.SplitN(kv, "=", 2)[0] + "="
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), strings.ToUpper(key)) {
			env[i] = kv
			return env
		}
	}
	return append(env, kv)
}
