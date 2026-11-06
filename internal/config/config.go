package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ConfigDir       string
	ConfigFile      string
	WorkspaceRoot   string
	ProjectsFile    string
	GitCacheRoot    string
	Agent           string
	ClaudeTheme     string
	ClaudeModel     string
	ClaudeEffort    string
	SandboxMemory   string
	HostShell       string
	Multiplexer     string
	Editor          string
	TmuxSocket      string
	TmuxConfig      string
	DockerSBXBinary string
	Mounts          []Mount
}

// Mount is an extra host directory passed to every new sandbox, such as a
// notes vault. Description tells the agent what the directory holds.
type Mount struct {
	Path        string
	Writable    bool
	Description string
}

type Project struct {
	Name            string
	Repo            string
	Branch          string
	BeforeBootstrap string
}

var projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var sandboxMemoryPattern = regexp.MustCompile(`^[1-9][0-9]*[mMgG]$`)

func Load(_ string) (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("cannot determine home directory: %w", err)
	}

	configDir := os.Getenv("SBX_CONFIG_DIR")
	if configDir == "" {
		configDir = filepath.Join(home, ".config", "sbx")
	}
	configDir = expandHome(configDir, home)

	configFile := os.Getenv("SBX_CONFIG_FILE")
	if configFile == "" {
		configFile = filepath.Join(configDir, "config.yaml")
	}
	if info, err := os.Stat(configFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("configuration not found: %s (copy config.yaml.example and projects.yaml.example into %s)", configFile, configDir)
		}
		return Config{}, fmt.Errorf("cannot read configuration %s: %w", configFile, err)
	} else if !info.Mode().IsRegular() {
		return Config{}, fmt.Errorf("configuration not found: %s (copy config.yaml.example and projects.yaml.example into %s)", configFile, configDir)
	}

	values, err := loadStringMap(configFile, "configuration")
	if err != nil {
		return Config{}, err
	}

	get := func(key, fallback string) (string, error) {
		node, ok := values[key]
		if !ok {
			return fallback, nil
		}
		if node.Kind != yaml.ScalarNode || (node.Tag != "!!str" && node.Tag != "!!int" && node.Tag != "!!float") {
			return "", fmt.Errorf("configuration value %q must be a string or number", key)
		}
		if node.Tag == "!!str" {
			return node.Value, nil
		}
		// Match Ruby's puts for ordinary YAML numeric scalars.
		if node.Tag == "!!int" {
			value, parseErr := strconv.ParseInt(node.Value, 0, 64)
			if parseErr == nil {
				return strconv.FormatInt(value, 10), nil
			}
		}
		if node.Tag == "!!float" {
			value, parseErr := strconv.ParseFloat(node.Value, 64)
			if parseErr == nil {
				return strconv.FormatFloat(value, 'g', -1, 64), nil
			}
		}
		return node.Value, nil
	}

	workspaceRoot, err := get("workspace_root", filepath.Join(home, "sandboxes"))
	if err != nil {
		return Config{}, err
	}
	projectsFile, err := get("projects_file", filepath.Join(configDir, "projects.yaml"))
	if err != nil {
		return Config{}, err
	}
	gitCacheRoot, err := get("git_cache_root", filepath.Join(home, ".cache", "sbx", "git"))
	if err != nil {
		return Config{}, err
	}
	agent, err := get("agent", "claude")
	if err != nil {
		return Config{}, err
	}
	claudeTheme, err := get("claude_theme", "dark")
	if err != nil {
		return Config{}, err
	}
	switch claudeTheme {
	case "", "light", "dark", "light-daltonized", "dark-daltonized", "light-ansi", "dark-ansi":
	default:
		return Config{}, fmt.Errorf("invalid claude_theme: %s", claudeTheme)
	}
	claudeModel, err := get("claude_model", "claude-opus-5-5")
	if err != nil {
		return Config{}, err
	}
	claudeEffort, err := get("claude_effort", "high")
	if err != nil {
		return Config{}, err
	}
	switch claudeEffort {
	case "", "low", "medium", "high", "xhigh", "max":
	default:
		return Config{}, fmt.Errorf("invalid claude_effort: %s", claudeEffort)
	}
	sandboxMemory, err := get("sandbox_memory", "8g")
	if err != nil {
		return Config{}, err
	}
	if sandboxMemory != "" && !sandboxMemoryPattern.MatchString(sandboxMemory) {
		return Config{}, fmt.Errorf("invalid sandbox_memory: %s (expected e.g. 8g or 4096m)", sandboxMemory)
	}
	hostShell, err := get("host_shell", "fish")
	if err != nil {
		return Config{}, err
	}
	multiplexer, err := get("multiplexer", "tmux")
	if err != nil {
		return Config{}, err
	}
	if multiplexer != "tmux" && multiplexer != "rex" {
		return Config{}, fmt.Errorf("multiplexer must be 'tmux' or 'rex', got: %s", multiplexer)
	}
	editor, err := get("editor", "")
	if err != nil {
		return Config{}, err
	}
	tmuxSocket, err := get("tmux_socket", "sbx-v1")
	if err != nil {
		return Config{}, err
	}
	tmuxConfig, err := get("tmux_config", "")
	if err != nil {
		return Config{}, err
	}
	dockerBinary, err := get("docker_sbx_bin", "")
	if err != nil {
		return Config{}, err
	}

	mounts, err := loadMounts(values["mounts"], home)
	if err != nil {
		return Config{}, err
	}

	workspaceRoot = expandHome(workspaceRoot, home)
	projectsFile = expandHome(projectsFile, home)
	gitCacheRoot = expandHome(gitCacheRoot, home)
	tmuxConfig = expandHome(tmuxConfig, home)
	dockerBinary = expandHome(dockerBinary, home)
	hostShell = expandHome(hostShell, home)

	if strings.ContainsRune(hostShell, filepath.Separator) {
		info, statErr := os.Stat(hostShell)
		if statErr != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			return Config{}, fmt.Errorf("host_shell is not executable: %s", hostShell)
		}
	} else {
		resolved, lookupErr := exec.LookPath(hostShell)
		if lookupErr != nil {
			return Config{}, errors.New("host_shell command not found")
		}
		hostShell = resolved
	}

	if agent != "claude" && agent != "codex" {
		return Config{}, fmt.Errorf("agent must be 'claude' or 'codex', got: %s", agent)
	}
	if !validTmuxSocket(tmuxSocket) {
		return Config{}, fmt.Errorf("invalid tmux_socket: %s", tmuxSocket)
	}
	if tmuxConfig != "" {
		if info, statErr := os.Stat(tmuxConfig); statErr != nil || !info.Mode().IsRegular() {
			return Config{}, fmt.Errorf("tmux configuration not found: %s", tmuxConfig)
		}
	}

	return Config{
		ConfigDir:       configDir,
		ConfigFile:      configFile,
		WorkspaceRoot:   workspaceRoot,
		ProjectsFile:    projectsFile,
		GitCacheRoot:    gitCacheRoot,
		Agent:           agent,
		ClaudeTheme:     claudeTheme,
		ClaudeModel:     claudeModel,
		ClaudeEffort:    claudeEffort,
		SandboxMemory:   sandboxMemory,
		HostShell:       hostShell,
		Multiplexer:     multiplexer,
		Editor:          editor,
		TmuxSocket:      tmuxSocket,
		TmuxConfig:      tmuxConfig,
		DockerSBXBinary: dockerBinary,
		Mounts:          mounts,
	}, nil
}

func loadMounts(node *yaml.Node, home string) ([]Mount, error) {
	if node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("mounts must be a sequence")
	}
	var mounts []Mount
	seen := map[string]bool{}
	for _, entry := range node.Content {
		fields, err := stringMap(entry)
		if err != nil {
			return nil, fmt.Errorf("mounts: %w", err)
		}
		var mount Mount
		for key, value := range fields {
			switch key {
			case "path", "description":
				if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
					return nil, fmt.Errorf("mounts: %s must be a string", key)
				}
				if key == "path" {
					mount.Path = value.Value
				} else {
					mount.Description = strings.TrimSpace(value.Value)
				}
			case "writable":
				if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
					return nil, fmt.Errorf("mounts: writable must be true or false")
				}
				mount.Writable = value.Value == "true"
			default:
				return nil, fmt.Errorf("mounts: unknown key %q", key)
			}
		}
		if mount.Path == "" {
			return nil, fmt.Errorf("mounts: every entry needs a path")
		}
		mount.Path = filepath.Clean(expandHome(mount.Path, home))
		if !filepath.IsAbs(mount.Path) {
			return nil, fmt.Errorf("mounts: path must be absolute or start with ~/: %s", mount.Path)
		}
		// sbx create separates a path from its :ro suffix with a colon.
		if strings.ContainsAny(mount.Path, ":,") {
			return nil, fmt.Errorf("mounts: path cannot contain ':' or ',': %s", mount.Path)
		}
		if seen[mount.Path] {
			return nil, fmt.Errorf("mounts: duplicate path %s", mount.Path)
		}
		seen[mount.Path] = true
		mounts = append(mounts, mount)
	}
	return mounts, nil
}
