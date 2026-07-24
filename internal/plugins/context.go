package plugins

import (
	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
)

// ScreenInfo 描述单个屏幕的信息，供 Lua 层查询。
type ScreenInfo struct {
	ID      uint32
	Name    string
	Width   int
	Height  int
	Focused bool
}

// TabInfo 描述一个终端标签的信息，供插件层查询当前标签列表。
type TabInfo struct {
	Title  string
	Active bool
}

// PluginContext 由 session 层实现并注入到 PluginManager。
// 它向插件暴露终端会话的核心操作能力，使 Lua 脚本能够驱动
// 焦点路由、标签管理、屏幕切换、缩放、面板开关与插件热重载。
//
// 在 PluginManager.Activate 被调用前，ctx 为 nil，此时任何
// 依赖 ctx 的 vistty.term.* API 都不应被调用（Lua 脚本通常
// 在 init.lua 阶段只做配置声明，不调用 term API）。
type PluginContext interface {
	FocusTerm() *terminal.Terminal
	Terms() []*terminal.Terminal
	NewTab() error
	CloseCurrentTab()
	NextTab()
	PrevTab()
	SwitchTab(idx int)
	TabList() []TabInfo
	NextScreen()
	PrevScreen()
	SwitchScreen(idx int)
	ScreenCount() int
	FocusScreenIdx() int
	FocusOutputID() uint32
	ScreenInfos() []ScreenInfo
	ZoomIn()
	ZoomOut()
	ZoomReset()
	EnablePanel(side string, lines int)
	DisablePanel(side string)
	ApplyTheme(term terminal.Theme, osd ui.OSDTheme)
	ReloadPlugins() error
	RequestRender()
	Exit()
}
