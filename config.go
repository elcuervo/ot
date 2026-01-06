package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	DefaultProfile string             `toml:"default_profile"`
	Profiles       map[string]Profile `toml:"profiles"`
	Tabs           bool               `toml:"tabs"`
	Theme          string             `toml:"theme"`
}

type Profile struct {
	Vault  string `toml:"vault"`
	Query  string `toml:"query"`
	Editor string `toml:"editor"`
}

type ResolvedProfile struct {
	Name        string
	VaultPath   string
	Query       string
	QueryIsFile bool
	EditorMode  string
}

type ProfileError struct {
	Profile string
	Field   string
	Err     error
}

func (e *ProfileError) Error() string {
	if e.Profile == "" {
		return fmt.Sprintf("config: %s: %v", e.Field, e.Err)
	}

	if e.Field == "" {
		return fmt.Sprintf("profile %q: %v", e.Profile, e.Err)
	}

	return fmt.Sprintf("profile %q: %s: %v", e.Profile, e.Field, e.Err)
}

func (e *ProfileError) Unwrap() error {
	return e.Err
}

var (
	ErrEmptyPath    = errors.New("path is empty")
	ErrPathNotExist = errors.New("path does not exist")
	ErrNotDirectory = errors.New("path is not a directory")
)

func validateProfile(name string, p Profile) error {
	if strings.TrimSpace(p.Vault) == "" {
		return &ProfileError{Profile: name, Field: "vault", Err: ErrEmptyPath}
	}

	// Query is optional - if empty, all tasks will be shown
	return nil
}

func validateVaultExists(name, vaultPath string) error {
	info, err := os.Stat(vaultPath)

	if err != nil {
		if os.IsNotExist(err) {
			return &ProfileError{Profile: name, Field: "vault", Err: fmt.Errorf("%w: %s", ErrPathNotExist, vaultPath)}
		}

		return &ProfileError{Profile: name, Field: "vault", Err: err}
	}

	if !info.IsDir() {
		return &ProfileError{Profile: name, Field: "vault", Err: fmt.Errorf("%w: %s", ErrNotDirectory, vaultPath)}
	}

	return nil
}

func validateConfig(cfg Config) error {
	if cfg.DefaultProfile != "" && cfg.Profiles != nil {
		if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
			return &ProfileError{Field: "default_profile", Err: fmt.Errorf("profile %q not found", cfg.DefaultProfile)}
		}
	}

	return nil
}

func selectProfile(profileFlag string, cfg Config) (string, *Profile, error) {
	if profileFlag != "" {
		if cfg.Profiles == nil {
			return "", nil, &ProfileError{Profile: profileFlag, Err: errors.New("no profiles defined in config")}
		}

		p, ok := cfg.Profiles[profileFlag]

		if !ok {
			return "", nil, &ProfileError{Profile: profileFlag, Err: errors.New("profile not found")}
		}

		return profileFlag, &p, nil
	}

	if cfg.DefaultProfile != "" {
		if cfg.Profiles == nil {
			return "", nil, &ProfileError{Field: "default_profile", Err: fmt.Errorf("profile %q not found", cfg.DefaultProfile)}
		}

		p, ok := cfg.Profiles[cfg.DefaultProfile]

		if !ok {
			return "", nil, &ProfileError{Field: "default_profile", Err: fmt.Errorf("profile %q not found", cfg.DefaultProfile)}
		}

		return cfg.DefaultProfile, &p, nil
	}

	return "", nil, nil
}

func resolveProfilePaths(name string, p Profile) (*ResolvedProfile, error) {
	if err := validateProfile(name, p); err != nil {
		return nil, err
	}

	vaultPath, err := resolveVaultPath(p.Vault)

	if err != nil {
		return nil, &ProfileError{Profile: name, Field: "vault", Err: err}
	}

	vaultPath = filepath.Clean(vaultPath)
	resolved, err := filepath.EvalSymlinks(vaultPath)
	if err == nil {
		vaultPath = resolved
	}

	if err := validateVaultExists(name, vaultPath); err != nil {
		return nil, err
	}

	// Query is optional - if empty, all tasks will be shown
	query := strings.TrimSpace(p.Query)
	queryIsFile := false

	if query != "" {
		// Check if it's a file path (markdown file that exists)
		queryPath, err := resolveQueryPath(query, vaultPath)
		if err == nil {
			queryPath = filepath.Clean(queryPath)
			if info, statErr := os.Stat(queryPath); statErr == nil && !info.IsDir() {
				// It's an existing file
				query = queryPath
				queryIsFile = true
			}
		}
		// If not a file, query remains as inline query string
	}

	return &ResolvedProfile{Name: name, VaultPath: vaultPath, Query: query, QueryIsFile: queryIsFile, EditorMode: p.Editor}, nil
}

func configPath() (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(configDir, "ot", "config.toml"), nil
}

func loadConfig() (Config, string, error) {
	path, err := configPath()

	if err != nil {
		return Config{}, "", err
	}

	data, err := os.ReadFile(path)

	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, path, nil
		}

		return Config{}, path, err
	}

	var cfg Config

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, path, err
	}

	return cfg, path, nil
}

func expandPath(value string) (string, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return value, nil
	}

	expanded := os.ExpandEnv(value)

	if !strings.HasPrefix(expanded, "~") {
		return expanded, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if expanded == "~" {
		return homeDir, nil
	}

	if strings.HasPrefix(expanded, "~/") {
		return filepath.Join(homeDir, expanded[2:]), nil
	}

	if strings.HasPrefix(expanded, "~\\") {
		return filepath.Join(homeDir, expanded[2:]), nil
	}

	return expanded, nil
}

func resolveVaultPath(value string) (string, error) {
	expanded, err := expandPath(value)

	if err != nil {
		return "", err
	}

	if expanded == "" || filepath.IsAbs(expanded) {
		return expanded, nil
	}

	homeDir, err := os.UserHomeDir()

	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, expanded), nil
}

func resolveQueryPath(value, vault string) (string, error) {
	expanded, err := expandPath(value)

	if err != nil {
		return "", err
	}

	if expanded == "" || filepath.IsAbs(expanded) || vault == "" {
		return expanded, nil
	}

	return filepath.Join(vault, expanded), nil
}
