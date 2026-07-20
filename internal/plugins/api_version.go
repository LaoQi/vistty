package plugins

import (
	"github.com/LaoQi/vistty/internal/version"
	lua "github.com/yuin/gopher-lua"
)

// registerVersion 注册版本查询接口：
//
//	vistty.version()      → string  版本号单行（如 "v1.0.0" 或 "dev-abc12345"），便于状态栏显示
//	vistty.version_info() → table   {version, commit, build_time, go}
func registerVersion(L *lua.LState, pm *PluginManager) {
	vt := ensureVisttyTable(L)

	vt.RawSetString("version", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(version.Get().Version))
		return 1
	}))

	vt.RawSetString("version_info", L.NewFunction(func(L *lua.LState) int {
		i := version.Get()
		t := L.NewTable()
		t.RawSetString("version", lua.LString(i.Version))
		t.RawSetString("commit", lua.LString(i.Commit))
		t.RawSetString("build_time", lua.LString(i.BuildTime))
		t.RawSetString("go", lua.LString(i.GoVersion))
		L.Push(t)
		return 1
	}))
}
