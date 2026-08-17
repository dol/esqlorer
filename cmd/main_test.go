package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/dominicluechinger/esqlorer/internal/config"
)

// Regression test for a bug where `--config` was parsed into the cfgFile
// variable but never bound to viper, so `esqlorer auth` subcommands (which
// resolve the config path via viper.GetString("config")) silently ignored
// the flag and wrote to the default config path instead.

func TestConfigFlagIsBoundToViper(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfgFile = ""

	rootCmd.SetArgs([]string{"--config", cfgPath, "auth", "list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got := viper.GetString("config"); got != cfgPath {
		t.Fatalf("viper config key not bound to --config flag: want %q, got %q", cfgPath, got)
	}
}

func TestAuthAddHonorsConfigFlag(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfgFile = ""

	rootCmd.SetArgs([]string{
		"--config", cfgPath,
		"auth", "add",
		"--name", "local",
		"--url", "http://localhost:9200",
		"--auth-method", "basic",
		"--username", "elastic",
		"--password", "changeme",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("expected config file at --config path %s: %v", cfgPath, err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config file is not valid yaml: %v", err)
	}

	if len(cfg.Servers) != 1 || cfg.Servers[0].Name != "local" {
		t.Fatalf("expected server 'local' written to --config path, got %#v", cfg.Servers)
	}
	if cfg.Servers[0].Username != "elastic" || cfg.Servers[0].Password != "changeme" {
		t.Fatalf("expected basic auth credentials to be persisted, got %#v", cfg.Servers[0])
	}
}

func TestAuthAddDoesNotTouchDefaultConfigPath(t *testing.T) {
	defaultPath := config.DefaultConfigPath()
	before, beforeErr := os.ReadFile(defaultPath)

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfgFile = ""

	rootCmd.SetArgs([]string{
		"--config", cfgPath,
		"auth", "add",
		"--name", "isolated",
		"--url", "http://localhost:9200",
		"--auth-method", "basic",
		"--username", "u",
		"--password", "p",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	after, afterErr := os.ReadFile(defaultPath)
	if beforeErr != nil {
		if afterErr == nil {
			t.Fatalf("default config path %s was created by a command that passed --config", defaultPath)
		}
		return
	}
	if afterErr != nil {
		t.Fatalf("default config path %s disappeared unexpectedly", defaultPath)
	}
	if string(before) != string(after) {
		t.Fatalf("default config path %s was modified by a command that passed --config", defaultPath)
	}
}
