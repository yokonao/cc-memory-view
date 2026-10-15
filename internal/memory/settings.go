package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// managedSettingsDir is the system directory holding managed-settings.json
// and managed-settings.d/.
var managedSettingsDir = func() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode"
	case "windows":
		return `C:\Program Files\ClaudeCode`
	default:
		return "/etc/claude-code"
	}
}()

// settingsMemoryDir returns autoMemoryDirectory from a settings file, or ""
// when the file doesn't set a valid one.
func settingsMemoryDir(file, home string) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var s struct {
		AutoMemoryDirectory string `json:"autoMemoryDirectory"`
	}
	if json.Unmarshal(data, &s) != nil {
		return ""
	}
	return expandMemoryDir(s.AutoMemoryDirectory, home)
}

// expandMemoryDir resolves an autoMemoryDirectory value, which must be
// absolute or start with ~/.
func expandMemoryDir(v, home string) string {
	if rest, ok := strings.CutPrefix(v, "~/"); ok {
		if home == "" {
			return ""
		}
		return filepath.Join(home, rest)
	}
	if filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	return ""
}

// managedMemoryDir merges managed-settings.json and then
// managed-settings.d/*.json in name order, the later file winning.
func managedMemoryDir(home string) string {
	dir := settingsMemoryDir(filepath.Join(managedSettingsDir, "managed-settings.json"), home)
	dropIns, _ := filepath.Glob(filepath.Join(managedSettingsDir, "managed-settings.d", "*.json"))
	for _, f := range dropIns {
		if strings.HasPrefix(filepath.Base(f), ".") {
			continue
		}
		if d := settingsMemoryDir(f, home); d != "" {
			dir = d
		}
	}
	return dir
}

// projectMemoryDir returns autoMemoryDirectory from a project's local or
// shared settings, local first. Shared settings are read from cwd; local
// settings from the git repository root (the main checkout's for a
// worktree) unless that is the home directory or cwd is outside a repository.
func projectMemoryDir(cwd, home string) string {
	if cwd == "" {
		return ""
	}
	localDir := cwd
	if root := repoRoot(cwd); root != "" && root != home {
		localDir = root
	}
	if d := settingsMemoryDir(filepath.Join(localDir, ".claude", "settings.local.json"), home); d != "" {
		return d
	}
	return settingsMemoryDir(filepath.Join(cwd, ".claude", "settings.json"), home)
}

func repoRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		dotGit := filepath.Join(d, ".git")
		info, err := os.Stat(dotGit)
		if err == nil {
			if info.IsDir() {
				return d
			}
			if main := worktreeMain(dotGit); main != "" {
				return main
			}
			return d
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// worktreeMain returns the main checkout of the worktree whose .git file is
// dotGit, or "" when it can't tell.
func worktreeMain(dotGit string) string {
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return ""
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
	if !ok {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(filepath.Dir(dotGit), gitDir)
	}
	common, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return ""
	}
	commonDir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	commonDir = filepath.Clean(commonDir)
	if filepath.Base(commonDir) != ".git" {
		return ""
	}
	return filepath.Dir(commonDir)
}
