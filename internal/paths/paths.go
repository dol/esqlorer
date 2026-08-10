package paths

import (
	"path/filepath"

	"github.com/adrg/xdg"
)

const appName = "esqlorer"

func ConfigDir() string {
	return filepath.Join(xdg.ConfigHome, appName)
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

func StateDir() string {
	base := xdg.StateHome
	if base == "" {
		base = xdg.DataHome
	}
	return filepath.Join(base, appName)
}

func QueryHistoryPath() string {
	return filepath.Join(StateDir(), "query-history.jsonl")
}
