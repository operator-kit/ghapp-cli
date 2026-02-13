package shellinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	markerStart = "# >>> ghapp initialize >>>"
	markerEnd   = "# <<< ghapp initialize <<<"
	managedNote = "# !! Contents within this block are managed by 'ghapp auth configure' !!\n# !! Do not edit manually - run 'ghapp auth reset' to remove !!"
)

// ManagedBlock returns the full managed block with markers.
func ManagedBlock(evalLine string) string {
	return fmt.Sprintf("%s\n%s\n%s\n%s", markerStart, managedNote, evalLine, markerEnd)
}

// InstallHook adds the managed block to the appropriate file for the given shell.
// For fish with conf.d, it writes a standalone file instead.
func InstallHook(sh Shell, ghappBin string) (string, error) {
	evalLine := sh.EvalLine(ghappBin)

	if sh.UsesConfD() {
		return installConfD(sh, evalLine)
	}
	return installRCFile(sh, evalLine)
}

func installConfD(sh Shell, evalLine string) (string, error) {
	path, err := sh.ConfDFilePath()
	if err != nil {
		return "", fmt.Errorf("conf.d path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create conf.d dir: %w", err)
	}

	content := ManagedBlock(evalLine) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write conf.d file: %w", err)
	}
	return path, nil
}

func installRCFile(sh Shell, evalLine string) (string, error) {
	path, err := sh.RCFilePath()
	if err != nil {
		return "", fmt.Errorf("rc file path: %w", err)
	}

	// Check for symlinks (NixOS etc)
	if target, err := os.Readlink(path); err == nil {
		return "", fmt.Errorf("rc file %s is a symlink to %s — cannot modify (NixOS or similar)", path, target)
	}

	// Check read-only
	if info, err := os.Stat(path); err == nil {
		if info.Mode().Perm()&0o200 == 0 {
			return "", fmt.Errorf("rc file %s is read-only", path)
		}
	}

	// Read existing content
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}

	// Check for orphaned markers
	hasStart := strings.Contains(existing, markerStart)
	hasEnd := strings.Contains(existing, markerEnd)
	if hasStart != hasEnd {
		return "", fmt.Errorf(
			"orphaned ghapp marker in %s — please manually remove the line containing %q or %q",
			path, markerStart, markerEnd,
		)
	}

	block := ManagedBlock(evalLine)

	if hasStart && hasEnd {
		// Replace existing block
		updated, err := replaceBlock(existing, block)
		if err != nil {
			return "", fmt.Errorf("replace block in %s: %w", path, err)
		}
		if err := atomicWriteRC(path, updated); err != nil {
			return "", err
		}
		return path, nil
	}

	// Append new block
	content := existing
	if !strings.HasSuffix(content, "\n") && content != "" {
		content += "\n"
	}
	content += "\n" + block + "\n"

	if err := atomicWriteRC(path, content); err != nil {
		return "", err
	}
	return path, nil
}

// UninstallHook removes the managed block from the appropriate file.
func UninstallHook(sh Shell) error {
	if sh.UsesConfD() {
		path, err := sh.ConfDFilePath()
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	path, err := sh.RCFilePath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	content := string(data)
	if !strings.Contains(content, markerStart) {
		return nil // nothing to remove
	}

	updated, err := removeBlock(content)
	if err != nil {
		return fmt.Errorf("remove block from %s: %w", path, err)
	}

	return atomicWriteRC(path, updated)
}

// HasHook checks whether the managed block is present in the shell's rc file.
func HasHook(sh Shell) bool {
	var path string
	var err error
	if sh.UsesConfD() {
		path, err = sh.ConfDFilePath()
	} else {
		path, err = sh.RCFilePath()
	}
	if err != nil {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), markerStart) && strings.Contains(string(data), markerEnd)
}

func replaceBlock(content, newBlock string) (string, error) {
	startIdx := strings.Index(content, markerStart)
	endIdx := strings.Index(content, markerEnd)
	if startIdx == -1 || endIdx == -1 {
		return "", fmt.Errorf("markers not found")
	}
	endIdx += len(markerEnd)
	// Include trailing newline if present
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}
	return content[:startIdx] + newBlock + "\n" + content[endIdx:], nil
}

func removeBlock(content string) (string, error) {
	startIdx := strings.Index(content, markerStart)
	endIdx := strings.Index(content, markerEnd)
	if startIdx == -1 || endIdx == -1 {
		return content, nil
	}
	endIdx += len(markerEnd)
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}
	// Remove leading blank line if the block was preceded by one
	if startIdx > 0 && content[startIdx-1] == '\n' {
		startIdx--
	}
	return content[:startIdx] + content[endIdx:], nil
}

func atomicWriteRC(path, content string) error {
	// Backup existing file
	if _, err := os.Stat(path); err == nil {
		backup := path + ".ghapp-backup"
		data, err := os.ReadFile(path)
		if err == nil {
			_ = os.WriteFile(backup, data, 0o600)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}

	tmp := path + ".ghapp-tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write temp rc: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename rc: %w", err)
	}
	return nil
}
