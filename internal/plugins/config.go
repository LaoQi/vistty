package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yuin/gopher-lua"
)

type RunConfig struct {
	Backend  string
	Shell    string
	FontPath string
	FontSize float64
	Primary  string
	ErrorLog string
}

func DefaultInitPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "vistty", "init.lua")
}

func DefaultRunConfig() *RunConfig {
	return &RunConfig{
		Backend:  "auto",
		Shell:    "/bin/bash",
		FontPath: "",
		FontSize: 14,
		Primary:  "",
		ErrorLog: "",
	}
}

func (pm *PluginManager) readConfig() (*RunConfig, error) {
	cfg := DefaultRunConfig()

	vistty := pm.L.GetGlobal("vistty")
	if vistty == lua.LNil {
		return cfg, nil
	}
	vt, ok := vistty.(*lua.LTable)
	if !ok {
		return cfg, fmt.Errorf("vistty must be a table")
	}
	configVal := pm.L.GetField(vt, "config")
	if configVal == lua.LNil {
		return cfg, nil
	}
	ct, ok := configVal.(*lua.LTable)
	if !ok {
		return cfg, fmt.Errorf("vistty.config must be a table")
	}

	cfg.Backend = getString(pm.L, ct, "backend", cfg.Backend)
	cfg.Shell = getString(pm.L, ct, "shell", cfg.Shell)
	cfg.FontPath = getString(pm.L, ct, "font", cfg.FontPath)
	cfg.FontSize = getNumber(pm.L, ct, "fontsize", cfg.FontSize)
	cfg.Primary = getString(pm.L, ct, "primary", cfg.Primary)
	cfg.ErrorLog = getString(pm.L, ct, "error_log", cfg.ErrorLog)

	return cfg, nil
}

func getString(L *lua.LState, t *lua.LTable, key, def string) string {
	v := L.GetField(t, key)
	if v == lua.LNil {
		return def
	}
	if s, ok := v.(lua.LString); ok {
		return string(s)
	}
	if n, ok := v.(lua.LNumber); ok {
		return fmt.Sprintf("%g", n)
	}
	return def
}

func getNumber(L *lua.LState, t *lua.LTable, key string, def float64) float64 {
	v := L.GetField(t, key)
	if v == lua.LNil {
		return def
	}
	if n, ok := v.(lua.LNumber); ok {
		return float64(n)
	}
	return def
}

func getBool(L *lua.LState, t *lua.LTable, key string, def bool) bool {
	v := L.GetField(t, key)
	if v == lua.LNil {
		return def
	}
	if b, ok := v.(lua.LBool); ok {
		return bool(b)
	}
	return def
}
