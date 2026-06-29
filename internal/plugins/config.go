package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/ui"
	"github.com/yuin/gopher-lua"
)

// RunConfig 是从 init.lua 的 vistty.config 表解析出的运行期配置。
// main.go 拿到此结构后构造后端与 Master。字段名大写以便外部构造。
//
// 为避免 plugins → session 的循环依赖（session 未来会通过
// PluginContext 接口反向依赖 plugins），Keybindings 使用 plugins
// 包本地定义的 KeybindTable 类型，而非 session.KeybindTable。
// main.go 负责将其转换为 session.KeybindTable（两者结构相同）。
type RunConfig struct {
	Backend     string
	Shell       string
	FontPath    string
	FontSize    float64
	Primary     string
	ModKey      string
	ErrorLog    string
	Record      string
	OSD         ui.Config
	Keybindings KeybindTable
}

// ResolvedKeybind 与 session.ResolvedKeybind 结构等价，
// 在 plugins 包本地定义以切断对 session 包的依赖。
type ResolvedKeybind struct {
	Mod  platform.Modifiers
	Rune rune
	Code uint16
}

// KeybindTable 与 session.KeybindTable 结构等价，
// main.go 负责两者之间的转换。
type KeybindTable map[string]ResolvedKeybind

// luaKeybind 是从 Lua 表 {key=, mod=} 读取的中间结构。
type luaKeybind struct {
	Key string
	Mod string
}

// luaKeybindings 收集 9 个动作的键绑定，未指定的字段为零值。
type luaKeybindings struct {
	ZoomIn, ZoomOut, ZoomReset      luaKeybind
	NewTab, CloseTab                luaKeybind
	PrevTab, NextTab                luaKeybind
	NextScreen, SwitchN            luaKeybind
	hasZoomIn, hasZoomOut           bool
	hasZoomReset                    bool
	hasNewTab, hasCloseTab          bool
	hasPrevTab, hasNextTab          bool
	hasNextScreen, hasSwitchN       bool
}

// DefaultInitPath 返回 init.lua 的默认路径。
// 遵循 XDG 规范：XDG_CONFIG_HOME 优先，回退 ~/.config，
// 最终拼接 vistty/init.lua。错误时返回空字符串而非 panic。
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

// DefaultRunConfig 返回与原 internal/config.Default() 等价的默认配置，
// 用于 init.lua 不存在时静默回退。
func DefaultRunConfig() *RunConfig {
	return &RunConfig{
		Backend:  "auto",
		Shell:    "/bin/bash",
		FontPath: "",
		FontSize: 14,
		Primary:  "",
		ModKey:   "super",
		ErrorLog: "",
		Record:   "",
		OSD: ui.Config{
			Top: ui.SideConfig{Enabled: true},
		},
		Keybindings: defaultKeybindTable(),
	}
}

// defaultLuaKeybindings 与 internal/config.DefaultKeybindings 等价。
func defaultLuaKeybindings() *luaKeybindings {
	return &luaKeybindings{
		ZoomIn:     luaKeybind{Key: "equal"},
		ZoomOut:    luaKeybind{Key: "minus"},
		ZoomReset:  luaKeybind{Key: "0"},
		NewTab:     luaKeybind{Key: "t"},
		CloseTab:   luaKeybind{Key: "w"},
		PrevTab:    luaKeybind{Key: "Left"},
		NextTab:    luaKeybind{Key: "Right"},
		NextScreen: luaKeybind{Key: "Tab"},
		SwitchN:    luaKeybind{Key: "1-9"},
		hasZoomIn: true, hasZoomOut: true, hasZoomReset: true,
		hasNewTab: true, hasCloseTab: true,
		hasPrevTab: true, hasNextTab: true,
		hasNextScreen: true, hasSwitchN: true,
	}
}

func defaultKeybindTable() KeybindTable {
	return toKeybindTable(defaultLuaKeybindings(), "super")
}

// toKeybindTable 将 luaKeybindings 转换为 KeybindTable。
// 逻辑与 session.ResolveKeybindings 等价：modKey 作为默认 mod，
// 各键绑定非空 mod 则覆盖；SwitchN 的 "1-9" 展开为 9 个条目。
func toKeybindTable(lkb *luaKeybindings, modKey string) KeybindTable {
	defaultMod := platform.ParseModKey(modKey)
	table := make(KeybindTable)

	resolve := func(name string, kb luaKeybind) {
		mod := defaultMod
		if kb.Mod != "" {
			mod = platform.ParseModKey(kb.Mod)
		}
		if kb.Key == "1-9" {
			for i := 1; i <= 9; i++ {
				digit := string(rune('0' + i))
				r, code := platform.ParseKey(digit)
				table[name+digit] = ResolvedKeybind{
					Mod:  mod,
					Rune: r,
					Code: code,
				}
			}
			return
		}
		r, code := platform.ParseKey(kb.Key)
		table[name] = ResolvedKeybind{Mod: mod, Rune: r, Code: code}
	}

	defaults := defaultLuaKeybindings()
	type field struct {
		name   string
		kb     luaKeybind
		has    bool
		defKb  luaKeybind
	}
	fields := []field{
		{"zoom_in", lkb.ZoomIn, lkb.hasZoomIn, defaults.ZoomIn},
		{"zoom_out", lkb.ZoomOut, lkb.hasZoomOut, defaults.ZoomOut},
		{"zoom_reset", lkb.ZoomReset, lkb.hasZoomReset, defaults.ZoomReset},
		{"new_tab", lkb.NewTab, lkb.hasNewTab, defaults.NewTab},
		{"close_tab", lkb.CloseTab, lkb.hasCloseTab, defaults.CloseTab},
		{"prev_tab", lkb.PrevTab, lkb.hasPrevTab, defaults.PrevTab},
		{"next_tab", lkb.NextTab, lkb.hasNextTab, defaults.NextTab},
		{"next_screen", lkb.NextScreen, lkb.hasNextScreen, defaults.NextScreen},
		{"switch_n", lkb.SwitchN, lkb.hasSwitchN, defaults.SwitchN},
	}
	for _, f := range fields {
		if !f.has {
			resolve(f.name, f.defKb)
			continue
		}
		resolve(f.name, f.kb)
	}
	return table
}

// readConfig 从 pm.L 的 vistty.config 表解析 RunConfig。
// 表缺失或字段缺失时使用默认值，不报错。
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
	cfg.ModKey = getString(pm.L, ct, "mod_key", cfg.ModKey)
	cfg.ErrorLog = getString(pm.L, ct, "error_log", cfg.ErrorLog)
	cfg.Record = getString(pm.L, ct, "record", cfg.Record)

	if osdVal := pm.L.GetField(ct, "osd"); osdVal != lua.LNil {
		if osdT, ok := osdVal.(*lua.LTable); ok {
			cfg.OSD = ui.Config{
				Top:    ui.SideConfig{Enabled: getBool(pm.L, osdT, "top", cfg.OSD.Top.Enabled)},
				Bottom: ui.SideConfig{Enabled: getBool(pm.L, osdT, "bottom", cfg.OSD.Bottom.Enabled)},
				Left:   ui.SideConfig{Enabled: getBool(pm.L, osdT, "left", cfg.OSD.Left.Enabled)},
				Right:  ui.SideConfig{Enabled: getBool(pm.L, osdT, "right", cfg.OSD.Right.Enabled)},
			}
		}
	}

	if kbVal := pm.L.GetField(ct, "keybindings"); kbVal != lua.LNil {
		if kbT, ok := kbVal.(*lua.LTable); ok {
			lkb := readLuaKeybindings(pm.L, kbT)
			cfg.Keybindings = toKeybindTable(lkb, cfg.ModKey)
		}
	}

	return cfg, nil
}

func readLuaKeybindings(L *lua.LState, t *lua.LTable) *luaKeybindings {
	lkb := &luaKeybindings{}
	if v := L.GetField(t, "zoom_in"); v != lua.LNil {
		lkb.ZoomIn, lkb.hasZoomIn = readKeybind(L, v)
	}
	if v := L.GetField(t, "zoom_out"); v != lua.LNil {
		lkb.ZoomOut, lkb.hasZoomOut = readKeybind(L, v)
	}
	if v := L.GetField(t, "zoom_reset"); v != lua.LNil {
		lkb.ZoomReset, lkb.hasZoomReset = readKeybind(L, v)
	}
	if v := L.GetField(t, "new_tab"); v != lua.LNil {
		lkb.NewTab, lkb.hasNewTab = readKeybind(L, v)
	}
	if v := L.GetField(t, "close_tab"); v != lua.LNil {
		lkb.CloseTab, lkb.hasCloseTab = readKeybind(L, v)
	}
	if v := L.GetField(t, "prev_tab"); v != lua.LNil {
		lkb.PrevTab, lkb.hasPrevTab = readKeybind(L, v)
	}
	if v := L.GetField(t, "next_tab"); v != lua.LNil {
		lkb.NextTab, lkb.hasNextTab = readKeybind(L, v)
	}
	if v := L.GetField(t, "next_screen"); v != lua.LNil {
		lkb.NextScreen, lkb.hasNextScreen = readKeybind(L, v)
	}
	if v := L.GetField(t, "switch_n"); v != lua.LNil {
		lkb.SwitchN, lkb.hasSwitchN = readKeybind(L, v)
	}
	return lkb
}

func readKeybind(L *lua.LState, v lua.LValue) (luaKeybind, bool) {
	t, ok := v.(*lua.LTable)
	if !ok {
		return luaKeybind{}, false
	}
	return luaKeybind{
		Key: getString(L, t, "key", ""),
		Mod: getString(L, t, "mod", ""),
	}, true
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
