package cmd

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/authstate"
	"github.com/operator-kit/ghapp-cli/internal/config"
	"github.com/operator-kit/ghapp-cli/internal/shellinit"
	"github.com/operator-kit/ghapp-cli/internal/tokencache"
)

var (
	removeKey  bool
	ghAuthFlag string
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Configure git and gh authentication",
}

var authConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Set up git credential helper and gh auth",
	RunE:  runAuthConfigure,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE:  runAuthStatus,
}

var authResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove git credential helper and gh auth config",
	RunE:  runAuthReset,
}

func init() {
	authResetCmd.Flags().BoolVar(&removeKey, "remove-key", false, "also delete private key from OS keyring")
	authConfigureCmd.Flags().StringVar(&ghAuthFlag, "gh-auth", "", "gh auth mode: shell-function, path-shim, none")
	authCmd.AddCommand(authConfigureCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authResetCmd)
	authCmd.AddCommand(shellInitCmd)
}

func runAuthConfigure(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	// Resolve helper command
	helperCmd, err := resolveHelperCommand()
	if err != nil {
		return err
	}

	// Resolve bot identity early so we can show it in the banner
	identityLine := "app bot identity"
	slug, botUserID, identityErr := resolveAppIdentity()
	if identityErr == nil {
		botName := fmt.Sprintf("%s[bot]", slug)
		botEmail := fmt.Sprintf("%d+%s[bot]@users.noreply.github.com", botUserID, slug)
		identityLine = fmt.Sprintf("%s <%s>", botName, botEmail)
	}

	// Banner
	fmt.Fprintln(out, "Auth will automatically configure:")
	fmt.Fprintf(out, "  1. Set git credential helper: %s\n", helperCmd)
	fmt.Fprintln(out, "  2. Write github.com entry to gh hosts.yml")
	fmt.Fprintln(out, "  3. Rewrite git@github.com SSH URLs to HTTPS (insteadOf)")
	fmt.Fprintf(out, "  4. Set git identity to %s\n", identityLine)
	fmt.Fprintln(out)

	// Resolve gh-auth mode — this is the single gate for interactive use
	mode := ghAuthFlag
	if mode == "" {
		if !isInteractive(cmd) {
			mode = "none"
		} else {
			fmt.Fprintln(out, "How would you like to authenticate gh CLI commands?")
			fmt.Fprintln(out, "  1. Shell function (recommended) — auto-refreshes token per invocation")
			fmt.Fprintln(out, "  2. PATH binary — wrapper binary that injects token")
			fmt.Fprintln(out, "  3. None — keep hosts.yml only (token expires in ~1hr)")
			fmt.Fprintln(out)

			choice := promptChoice(cmd, "Choice [1/2/3]:", "")
			switch choice {
			case "1":
				mode = "shell-function"
			case "2":
				mode = "path-shim"
			case "3":
				mode = "none"
			default:
				fmt.Fprintln(out, "Skipped. Run 'ghapp auth configure' to set up later.")
				return nil
			}
		}
	}

	// --- Execute all steps ---

	// 1. Git credential helper
	gitHelper := fmt.Sprintf("!%s credential-helper", helperCmd)
	gitCmd := exec.Command("git", "config", "--global", "credential.https://github.com.helper", gitHelper)
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config: %s: %w", strings.TrimSpace(string(output)), err)
	}
	fmt.Fprintln(out, "Git credential helper configured.")

	// 2. gh CLI (hosts.yml as baseline)
	if err := configureGhHosts(); err != nil {
		fmt.Fprintf(out, "Warning: could not configure gh CLI: %v\n", err)
		fmt.Fprintln(out, "Use: export GH_TOKEN=$(ghapp token)")
	} else {
		fmt.Fprintln(out, "gh CLI configured.")
	}

	// 3. Git identity
	if identityErr != nil {
		fmt.Fprintf(out, "Warning: could not configure git identity: %v\n", identityErr)
	} else {
		if err := configureGitIdentity(cmd, slug, botUserID); err != nil {
			fmt.Fprintf(out, "Warning: could not configure git identity: %v\n", err)
		}
	}

	// 4. SSH-to-HTTPS URL rewrite
	configureInsteadOf(cmd)

	// 5. gh CLI dynamic auth
	switch mode {
	case "shell-function":
		if err := configureShellFunction(cmd, helperCmd); err != nil {
			fmt.Fprintf(out, "Warning: gh dynamic auth: %v\n", err)
		}
	case "path-shim":
		if err := configurePathShim(cmd, helperCmd); err != nil {
			fmt.Fprintf(out, "Warning: gh dynamic auth: %v\n", err)
		}
	case "none":
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Note: gh hosts.yml token expires in ~1hr.")
		fmt.Fprintln(out, "For long sessions, use: export GH_TOKEN=$(ghapp token)")
	}

	return nil
}


func configureShellFunction(cmd *cobra.Command, ghappBin string) error {
	out := cmd.OutOrStdout()

	sh := shellinit.DetectShell()
	if sh == nil {
		fmt.Fprintln(out, "Could not detect shell. Use 'ghapp auth shell-init --shell <name>' manually.")
		return nil
	}

	fmt.Fprintf(out, "\nDetected shell: %s\n", sh.Name())

	path, err := shellinit.InstallHook(sh, ghappBin)
	if err != nil {
		return fmt.Errorf("install hook for %s: %w", sh.Name(), err)
	}
	fmt.Fprintf(out, "Shell hook installed in %s\n", path)

	// Save auth state
	state, _ := authstate.LoadDefault()
	state.GhAuthMode = "shell-function"
	state.ShellHooks = append(state.ShellHooks, authstate.ShellHookInfo{
		ShellName:   sh.Name(),
		FilePath:    path,
		InstalledAt: time.Now().UTC(),
	})
	if err := authstate.SaveDefault(state); err != nil {
		fmt.Fprintf(out, "Warning: could not save auth state: %v\n", err)
	}

	fmt.Fprintln(out, "Restart your shell or run:")
	fmt.Fprintf(out, "  source %s\n", path)
	return nil
}

func configurePathShim(cmd *cobra.Command, ghappBin string) error {
	out := cmd.OutOrStdout()

	installDir := resolveInstallDir(ghappBin)
	ghName := "gh"
	if runtime.GOOS == "windows" {
		ghName = "gh.exe"
	}
	destPath := filepath.Join(installDir, ghName)

	// Find the ghapp-gh wrapper binary
	wrapperSrc, err := findGhWrapper()
	if err != nil {
		return fmt.Errorf("find ghapp-gh binary: %w", err)
	}

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	// Copy wrapper to destination
	checksum, err := copyFile(wrapperSrc, destPath)
	if err != nil {
		return fmt.Errorf("install wrapper: %w", err)
	}

	fmt.Fprintf(out, "\nghapp-gh wrapper installed to %s\n", destPath)
	fmt.Fprintln(out, "Ensure this directory is earlier in PATH than the real gh.")

	// Save auth state
	state, _ := authstate.LoadDefault()
	state.GhAuthMode = "path-shim"
	state.GhappGhPath = destPath
	state.WrapperChecksum = checksum
	if err := authstate.SaveDefault(state); err != nil {
		fmt.Fprintf(out, "Warning: could not save auth state: %v\n", err)
	}

	return nil
}

func resolveInstallDir(ghappBin string) string {
	// If ghapp is in a user-writable bin, use same dir
	dir := filepath.Dir(ghappBin)
	if isWritable(dir) {
		return dir
	}
	// Fallback
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			return filepath.Join(localAppData, "ghapp", "bin")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func findGhWrapper() (string, error) {
	// Look for ghapp-gh next to ghapp
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	name := "ghapp-gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	// Try PATH
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("%s not found next to ghapp or on PATH", name)
}

func copyFile(src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	defer out.Close()

	h := sha256.New()
	w := io.MultiWriter(out, h)
	if _, err := io.Copy(w, in); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func isWritable(dir string) bool {
	tmp := filepath.Join(dir, ".ghapp-write-test")
	f, err := os.Create(tmp)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(tmp)
	return true
}

func isInteractive(cmd *cobra.Command) bool {
	// If stdin is a pipe/file, not interactive
	if f, ok := cmd.InOrStdin().(*os.File); ok {
		info, err := f.Stat()
		if err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
			return true
		}
	}
	return false
}

func promptChoice(cmd *cobra.Command, prompt, defaultVal string) string {
	fmt.Fprintf(cmd.OutOrStdout(), "%s ", prompt)
	reader := bufio.NewReader(cmd.InOrStdin())
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return defaultVal
	}
	return answer
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "App ID:          %d\n", cfg.AppID)
	fmt.Fprintf(out, "Installation ID: %d\n", cfg.InstallationID)

	if cfg.KeyInKeyring {
		fmt.Fprintln(out, "Private key:     keyring")
	} else {
		fmt.Fprintf(out, "Private key:     %s\n", cfg.PrivateKeyPath)
	}

	// Check git credential helper
	gitCmd := exec.Command("git", "config", "--global", "credential.https://github.com.helper")
	if output, err := gitCmd.Output(); err == nil {
		fmt.Fprintf(out, "Git helper:      %s\n", strings.TrimSpace(string(output)))
	} else {
		fmt.Fprintln(out, "Git helper:      not configured")
	}

	// Check SSH-to-HTTPS URL rewrite
	if v := gitConfigGet(`url.https://github.com/.insteadof`); v != "" {
		fmt.Fprintf(out, "URL rewrite:     %s → https://github.com/\n", v)
	} else {
		fmt.Fprintln(out, "URL rewrite:     not configured (SSH URLs won't use credential helper)")
	}

	// Check git identity
	gitName := gitConfigGet("user.name")
	gitEmail := gitConfigGet("user.email")
	if gitName != "" || gitEmail != "" {
		fmt.Fprintf(out, "Git identity:    %s <%s>\n", gitName, gitEmail)
	} else {
		fmt.Fprintln(out, "Git identity:    not configured")
	}

	// Check gh hosts.yml
	hostsPath, err := ghHostsPath()
	if err == nil {
		if _, err := os.Stat(hostsPath); err == nil {
			fmt.Fprintf(out, "gh hosts.yml:    %s\n", hostsPath)
		} else {
			fmt.Fprintln(out, "gh hosts.yml:    not found")
		}
	}

	// gh auth mode
	state, _ := authstate.LoadDefault()
	switch state.GhAuthMode {
	case "shell-function":
		fmt.Fprintln(out, "gh auth mode:    shell function")
		for _, hook := range state.ShellHooks {
			present := shellinit.HasHook(shellinit.ShellByName(hook.ShellName))
			status := "present"
			if !present {
				status = "MISSING"
			}
			fmt.Fprintf(out, "  %s hook:      %s (%s)\n", hook.ShellName, hook.FilePath, status)
		}
	case "path-shim":
		fmt.Fprintln(out, "gh auth mode:    PATH binary")
		if state.GhappGhPath != "" {
			if _, err := os.Stat(state.GhappGhPath); err == nil {
				fmt.Fprintf(out, "  wrapper:       %s (exists)\n", state.GhappGhPath)
			} else {
				fmt.Fprintf(out, "  wrapper:       %s (MISSING)\n", state.GhappGhPath)
			}
		}
	default:
		fmt.Fprintln(out, "gh auth mode:    not configured")
	}

	// Token cache status
	if entry := tokencache.ReadCache(cfg.InstallationID); entry != nil {
		fmt.Fprintf(out, "Token cache:     valid (expires %s)\n", entry.Expiry.Format("15:04:05"))
	} else {
		fmt.Fprintln(out, "Token cache:     empty/expired")
	}

	// Test token generation
	fmt.Fprint(out, "Token test:      ")
	key, err := auth.LoadPrivateKeyFromConfig(cfg)
	if err != nil {
		fmt.Fprintf(out, "error loading key: %v\n", err)
		return nil
	}
	result, err := tokenGenerator.Generate(cfg.AppID, cfg.InstallationID, key)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
	} else {
		fmt.Fprintf(out, "OK (expires %s)\n", result.ExpiresAt.Format("15:04:05"))
	}
	return nil
}

func runAuthReset(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	// Remove git credential helper
	gitCmd := exec.Command("git", "config", "--global", "--unset", "credential.https://github.com.helper")
	if output, err := gitCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(out, "Warning: git config unset: %s\n", strings.TrimSpace(string(output)))
	} else {
		fmt.Fprintln(out, "Git credential helper removed.")
	}

	// Remove github.com from gh hosts.yml
	if err := removeGhHost(); err != nil {
		fmt.Fprintf(out, "Warning: could not update gh hosts.yml: %v\n", err)
	} else {
		fmt.Fprintln(out, "github.com removed from gh hosts.yml.")
	}

	// Remove SSH-to-HTTPS URL rewrite
	resetInsteadOf(cmd)

	// Undo gh auth mode
	resetGhAuth(cmd)

	// Restore git identity
	resetGitIdentity(cmd)

	// Remove token cache
	tokencache.RemoveCache()
	fmt.Fprintln(out, "Token cache cleared.")

	if removeKey {
		if err := auth.DeletePrivateKey(); err != nil {
			fmt.Fprintf(out, "Warning: could not delete keyring entry: %v\n", err)
		} else {
			fmt.Fprintln(out, "Private key removed from keyring.")
		}
	}

	return nil
}

func resetGhAuth(cmd *cobra.Command) {
	out := cmd.OutOrStdout()

	state, err := authstate.LoadDefault()
	if err != nil {
		// No state file — try fallback: remove hooks from all shells
		for _, sh := range shellinit.AllShells() {
			_ = shellinit.UninstallHook(sh)
		}
		return
	}

	switch state.GhAuthMode {
	case "shell-function":
		for _, hook := range state.ShellHooks {
			sh := shellinit.ShellByName(hook.ShellName)
			if sh == nil {
				continue
			}
			if err := shellinit.UninstallHook(sh); err != nil {
				fmt.Fprintf(out, "Warning: remove %s hook: %v\n", hook.ShellName, err)
			} else {
				fmt.Fprintf(out, "Shell hook removed from %s\n", hook.FilePath)
			}
		}
	case "path-shim":
		if state.GhappGhPath != "" {
			// Verify checksum before deleting
			if state.WrapperChecksum != "" {
				if checksum := fileChecksum(state.GhappGhPath); checksum != state.WrapperChecksum {
					fmt.Fprintf(out, "Warning: wrapper checksum mismatch at %s — skipping removal\n", state.GhappGhPath)
				} else if err := os.Remove(state.GhappGhPath); err != nil && !os.IsNotExist(err) {
					fmt.Fprintf(out, "Warning: remove wrapper: %v\n", err)
				} else {
					fmt.Fprintf(out, "Wrapper binary removed from %s\n", state.GhappGhPath)
				}
			} else {
				if err := os.Remove(state.GhappGhPath); err != nil && !os.IsNotExist(err) {
					fmt.Fprintf(out, "Warning: remove wrapper: %v\n", err)
				} else {
					fmt.Fprintf(out, "Wrapper binary removed from %s\n", state.GhappGhPath)
				}
			}
		}
	}

	// Clear auth state
	state.GhAuthMode = ""
	state.ShellHooks = nil
	state.GhappGhPath = ""
	state.WrapperChecksum = ""
	_ = authstate.SaveDefault(state)
}

func fileChecksum(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func resolveHelperCommand() (string, error) {
	// Check if ghapp is on PATH
	if _, err := exec.LookPath("ghapp"); err == nil {
		return "ghapp", nil
	}

	// Use absolute path of current executable
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return exePath, nil
}

func confirm(cmd *cobra.Command, prompt string) bool {
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N] ", prompt)
	reader := bufio.NewReader(cmd.InOrStdin())
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

// git identity management

func configureGitIdentity(cmd *cobra.Command, slug string, botUserID int64) error {
	out := cmd.OutOrStdout()

	botName := fmt.Sprintf("%s[bot]", slug)
	botEmail := fmt.Sprintf("%d+%s[bot]@users.noreply.github.com", botUserID, slug)

	// Backup existing identity if present
	existingName := gitConfigGet("user.name")
	existingEmail := gitConfigGet("user.email")
	if existingName != "" || existingEmail != "" {
		cfg.PrevGitUserName = existingName
		cfg.PrevGitUserEmail = existingEmail
	}

	// Set bot identity
	if err := gitConfigSet("user.name", botName); err != nil {
		return fmt.Errorf("set user.name: %w", err)
	}
	if err := gitConfigSet("user.email", botEmail); err != nil {
		return fmt.Errorf("set user.email: %w", err)
	}
	fmt.Fprintf(out, "Git identity set to %s <%s>\n", botName, botEmail)

	// Save cached values to config
	cfg.AppSlug = slug
	cfg.BotUserID = botUserID
	return saveConfig()
}

func resolveAppIdentity() (string, int64, error) {
	// Use cached values if available
	if cfg.AppSlug != "" && cfg.BotUserID != 0 {
		return cfg.AppSlug, cfg.BotUserID, nil
	}

	key, err := auth.LoadPrivateKeyFromConfig(cfg)
	if err != nil {
		return "", 0, err
	}

	slug, err := auth.FetchAppSlug(cfg.AppID, key)
	if err != nil {
		return "", 0, fmt.Errorf("fetch app slug: %w", err)
	}

	botUserID, err := auth.FetchBotUserID(slug)
	if err != nil {
		return "", 0, fmt.Errorf("fetch bot user ID: %w", err)
	}

	return slug, botUserID, nil
}

func resetGitIdentity(cmd *cobra.Command) {
	out := cmd.OutOrStdout()

	if cfg.PrevGitUserName != "" || cfg.PrevGitUserEmail != "" {
		// Restore previous identity
		if cfg.PrevGitUserName != "" {
			_ = gitConfigSet("user.name", cfg.PrevGitUserName)
		}
		if cfg.PrevGitUserEmail != "" {
			_ = gitConfigSet("user.email", cfg.PrevGitUserEmail)
		}
		fmt.Fprintf(out, "Git identity restored to %q <%s>\n", cfg.PrevGitUserName, cfg.PrevGitUserEmail)
		cfg.PrevGitUserName = ""
		cfg.PrevGitUserEmail = ""
	} else if cfg.AppSlug != "" {
		// We set the identity but there was no previous one — unset
		_ = gitConfigUnset("user.name")
		_ = gitConfigUnset("user.email")
		fmt.Fprintln(out, "Git bot identity removed.")
	}

	cfg.AppSlug = ""
	cfg.BotUserID = 0
	_ = saveConfig()
}

// SSH-to-HTTPS URL rewrite

const insteadOfKey = `url.https://github.com/.insteadOf`
const insteadOfValue = `git@github.com:`

func configureInsteadOf(cmd *cobra.Command) {
	out := cmd.OutOrStdout()

	if err := gitConfigSet(insteadOfKey, insteadOfValue); err != nil {
		fmt.Fprintf(out, "Warning: could not set URL rewrite: %v\n", err)
		return
	}
	fmt.Fprintln(out, "SSH URL rewrite configured (git@github.com: → https://github.com/).")

	// Record in auth state
	state, _ := authstate.LoadDefault()
	state.URLRewrite = true
	_ = authstate.SaveDefault(state)
}

func resetInsteadOf(cmd *cobra.Command) {
	out := cmd.OutOrStdout()

	state, _ := authstate.LoadDefault()
	if !state.URLRewrite {
		return // we didn't set it, don't touch it
	}

	if err := gitConfigUnset(insteadOfKey); err != nil {
		fmt.Fprintf(out, "Warning: could not remove URL rewrite: %v\n", err)
	} else {
		fmt.Fprintln(out, "SSH URL rewrite removed.")
	}

	state.URLRewrite = false
	_ = authstate.SaveDefault(state)
}

func gitConfigGet(key string) string {
	out, err := exec.Command("git", "config", "--global", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitConfigSet(key, value string) error {
	return exec.Command("git", "config", "--global", key, value).Run()
}

func gitConfigUnset(key string) error {
	return exec.Command("git", "config", "--global", "--unset", key).Run()
}

func saveConfig() error {
	savePath := cfgPath
	if savePath == "" {
		var err error
		savePath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}
	return config.Save(savePath, cfg)
}

// gh hosts.yml management

type ghHostEntry struct {
	OauthToken  string `yaml:"oauth_token"`
	User        string `yaml:"user"`
	GitProtocol string `yaml:"git_protocol"`
}

func ghHostsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "gh", "hosts.yml"), nil
}

func configureGhHosts() error {
	hostsPath, err := ghHostsPath()
	if err != nil {
		return err
	}

	// Load private key and generate token for gh
	key, err := auth.LoadPrivateKeyFromConfig(cfg)
	if err != nil {
		return err
	}
	result, err := tokenGenerator.Generate(cfg.AppID, cfg.InstallationID, key)
	if err != nil {
		return err
	}

	// Read existing hosts.yml if present
	hosts := make(map[string]ghHostEntry)
	if data, err := os.ReadFile(hostsPath); err == nil {
		_ = yaml.Unmarshal(data, &hosts)
	}

	hosts["github.com"] = ghHostEntry{
		OauthToken:  result.Token,
		User:        "x-access-token",
		GitProtocol: "https",
	}

	data, err := yaml.Marshal(hosts)
	if err != nil {
		return fmt.Errorf("marshal hosts: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(hostsPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(hostsPath, data, 0o600)
}

func removeGhHost() error {
	hostsPath, err := ghHostsPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return err
	}

	hosts := make(map[string]ghHostEntry)
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return err
	}

	delete(hosts, "github.com")

	newData, err := yaml.Marshal(hosts)
	if err != nil {
		return err
	}
	return os.WriteFile(hostsPath, newData, 0o600)
}
