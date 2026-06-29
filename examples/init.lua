-- ~/.config/vistty/init.lua
--
-- Vistty 启动配置与插件脚本。本文件在主渲染线程同步执行一次，
-- 同时完成「配置声明」与「钩子注册」。
--
-- 配置字段缺失时使用内置默认值；文件不存在时 Vistty 也会回退到默认配置。
-- 详细字段与 API 见 work_docs/plugins.md。

vistty.config = {
	backend   = "auto",       -- auto / wayland / drm / drm-gbm
	shell     = "/bin/bash",
	font      = "",           -- 留空使用内置 Sarasa Fixed SC
	fontsize  = 14,
	primary   = "",           -- 主屏名称（如 HDMI-A-1）或索引
	error_log = "",           -- 留空使用默认 ~/.local/share/vistty/error.log
	keybindings = {
		zoom_in     = {key="=",     mod="super"},
		zoom_out    = {key="-",     mod="super"},
		zoom_reset  = {key="0",     mod="super"},
		new_tab     = {key="t",     mod="super"},
		close_tab   = {key="w",     mod="super"},
		prev_tab    = {key="Left",  mod="super"},
		next_tab    = {key="Right", mod="super"},
		next_screen = {key="Tab",   mod="super"},
		switch_n    = {key="1-9",   mod="super"},  -- 展开为 switch_n1..switch_n9
	},
}

-- === 输入拦截示例 ===
-- Ctrl+Space → 发送 PageDown 转义序列并吞掉原事件
vistty.input.on_key(function(ev)
	-- 仅在按下时触发，忽略释放事件
	if ev.state ~= vistty.state.PRESS then return end
	-- Ctrl+Space → PageDown
	if (ev.mods % (vistty.mods.CTRL * 2)) >= vistty.mods.CTRL and ev.code == vistty.keys.SPACE then
		vistty.term.send("\x1b[6~")
		return true
	end
	-- Ctrl+C 拦截示例：按 C 键 + Ctrl 修饰
	if (ev.mods % (vistty.mods.CTRL * 2)) >= vistty.mods.CTRL and ev.code == vistty.keys.C then
		vistty.log("Ctrl+C intercepted")
		-- return true  -- 取消注释则吞掉 Ctrl+C
	end
end)

-- === 底部状态栏插件 ===
-- 启用 1 行底部面板，每帧渲染时钟与标签数
-- 颜色可用 vistty.colors 常量或 "#RRGGBB" / "#RRGGBBAA" 字符串
vistty.ui.enable("bottom", 1)
vistty.ui.on_render(function(ctx)
	local w, h = ctx:size()
	ctx:rect(0, 0, w, h, {bg=vistty.colors.DARKGRAY})
	ctx:text(2, 0, os.date("%H:%M:%S"), {fg="#64C8FF"})
	ctx:text(w - 10, 0, "tabs:" .. vistty.tab.count(), {fg=vistty.colors.GOLD})
	return true
end)

-- === 模块化加载 ===
-- 完全开放沙箱，dofile 可用，可将插件拆分到独立文件：
-- dofile(os.getenv("HOME") .. "/.config/vistty/plugins/extra.lua")
