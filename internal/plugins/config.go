package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yuin/gopher-lua"

	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
)

type RunConfig struct {
	Backend          string
	Shell            string
	FontPath         string
	FallbackFontPath string
	FontSize         float64
	Scrollback       int
	Primary          string
	ErrorLog         string
	TermTheme        *terminal.Theme
	OSDTheme         *ui.OSDTheme
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
		Backend:          "auto",
		Shell:            "/bin/bash",
		FontPath:         "",
		FallbackFontPath: "",
		FontSize:         14,
		Scrollback:       10000,
		Primary:          "",
		ErrorLog:         "",
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
	cfg.FallbackFontPath = getString(pm.L, ct, "fallback_font", cfg.FallbackFontPath)
	cfg.FontSize = getNumber(pm.L, ct, "fontsize", cfg.FontSize)
	cfg.Scrollback = int(getNumber(pm.L, ct, "scrollback", float64(cfg.Scrollback)))
	if cfg.Scrollback < 0 {
		cfg.Scrollback = 0
	}
	cfg.Primary = getString(pm.L, ct, "primary", cfg.Primary)
	cfg.ErrorLog = getString(pm.L, ct, "error_log", cfg.ErrorLog)

	themeVal := pm.L.GetField(ct, "theme")
	if themeVal != lua.LNil {
		if tt, ok := themeVal.(*lua.LTable); ok {
			termTheme, osdTheme := parseLuaTheme(pm.L, tt)
			cfg.TermTheme = &termTheme
			cfg.OSDTheme = &osdTheme
			pm.currentTheme = &termTheme
			pm.currentOSDTheme = &osdTheme
		}
	}

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

func parseLuaTheme(L *lua.LState, t *lua.LTable) (terminal.Theme, ui.OSDTheme) {
	dt := terminal.DefaultTheme
	ot := ui.DefaultOSDTheme
	term := terminal.Theme{
		DefFg:       dt.DefFg,
		DefBg:       dt.DefBg,
		CursorColor: dt.CursorColor,
		Palette:     dt.Palette,
	}
	osd := ot

	if v := L.GetField(t, "fg"); v != lua.LNil {
		term.DefFg = luaColorToScreenColor(L, v)
	}
	if v := L.GetField(t, "bg"); v != lua.LNil {
		term.DefBg = luaColorToScreenColor(L, v)
	}
	if v := L.GetField(t, "cursor"); v != lua.LNil {
		term.CursorColor = luaColorToScreenColor(L, v)
	}

	if pv := L.GetField(t, "palette"); pv != lua.LNil {
		if pt, ok := pv.(*lua.LTable); ok {
			for i := 0; i < 16; i++ {
				if ev := pt.RawGetInt(i + 1); ev != lua.LNil {
					term.Palette[i] = luaColorToScreenColor(L, ev)
				}
			}
		}
	}

	if ov := L.GetField(t, "osd"); ov != lua.LNil {
		if ot2, ok := ov.(*lua.LTable); ok {
			if v := L.GetField(ot2, "bar_bg"); v != lua.LNil {
				osd.BarBg = luaColorToArray3(L, v)
			}
			if v := L.GetField(ot2, "active_bg"); v != lua.LNil {
				osd.ActiveBg = luaColorToArray3(L, v)
			}
			if v := L.GetField(ot2, "inactive_bg"); v != lua.LNil {
				osd.InactiveBg = luaColorToArray3(L, v)
			}
			if v := L.GetField(ot2, "active_fg"); v != lua.LNil {
				osd.ActiveFg = luaColorToArray3(L, v)
			}
			if v := L.GetField(ot2, "inactive_fg"); v != lua.LNil {
				osd.InactiveFg = luaColorToArray3(L, v)
			}
			if v := L.GetField(ot2, "csd_btn_bg"); v != lua.LNil {
				osd.CsdBtnBg = luaColorToArray3(L, v)
			}
			if v := L.GetField(ot2, "csd_close_bg"); v != lua.LNil {
				osd.CsdCloseBg = luaColorToArray3(L, v)
			}
			if v := L.GetField(ot2, "csd_btn_fg"); v != lua.LNil {
				osd.CsdBtnFg = luaColorToArray3(L, v)
			}
		}
	}

	return term, osd
}

func luaColorToScreenColor(L *lua.LState, v lua.LValue) screen.Color {
	rgba := parseColor(L, v)
	return screen.Color{R: rgba[0], G: rgba[1], B: rgba[2]}
}

func luaColorToArray3(L *lua.LState, v lua.LValue) [3]uint8 {
	rgba := parseColor(L, v)
	return [3]uint8{rgba[0], rgba[1], rgba[2]}
}
