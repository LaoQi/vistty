-- 主题配置：require 加载预设主题（themes/ 目录需与 init.lua 同目录）。
-- 可用预设：dracula / solarized_dark / solarized_light / gruvbox / monokai / nord / one_dark
-- 也可自定义：theme = { fg="#rrggbb", bg="#rrggbb", cursor="#rrggbb",
--                      palette={"#...×16"}, osd={bar_bg="#...", ...} }
-- 运行时动态切换：vistty.theme.apply(require("themes.gruvbox"))
-- require 失败时 theme 为 nil，Go 层用 DefaultTheme 兜底。
local theme = require("themes.xterm")
-- pcall(function() theme = require("themes.dracula") end)

vistty.config = {
	backend    = "auto",
	shell      = "/bin/bash",
	-- 指定 font 即完整接管字体查找链：默认的 Sarasa 校准补丁与内嵌 Nerd 图标
	-- fallback 均不再装配（规避基线对齐/风格混搭问题）。此时缺字形渲染为空白，
	-- 需要 Nerd 图标或补充字形时用 fallback_font 指向磁盘上的字体文件（如系统
	-- 安装的 Nerd Font），该 fallback 恒生效。
	font       = "",
	fontsize   = 24,
	scrollback = 10000,
	primary    = "",
	error_log  = "",
	theme      = theme,
}

-- 当前运行后端的默认 mod 键：wayland 用 ALT，drm/drm-gbm 用 SUPER。
-- backend_name 在 Load 阶段尚未注入（auto 模式需探测后才知道），
-- 故此处用 backend_name()=="" 的降级：未确定时按 wayland 风格用 ALT。
local function mod_key()
	if vistty.backend_name() == "" or vistty.backend.is_wayland() then
		return vistty.keys.LEFT_ALT
	end
	return vistty.keys.LEFT_SUPER
end

local function super()
	return vistty.input.pressed(mod_key())
end

vistty.input.bind(vistty.keys.EQUAL, function()
	if super() then vistty.zoom.increase(); return true end
end)
vistty.input.bind(vistty.keys.MINUS, function()
	if super() then vistty.zoom.decrease(); return true end
end)
vistty.input.bind(vistty.keys.NUM0, function()
	if super() then vistty.zoom.reset(); return true end
end)
vistty.input.bind(vistty.keys.T, function()
	if super() then vistty.tab.new(); return true end
end)
vistty.input.bind(vistty.keys.W, function()
	if super() then vistty.tab.close(); return true end
end)
vistty.input.bind(vistty.keys.LEFT, function()
	if super() then vistty.screen.prev(); return true end
end)
vistty.input.bind(vistty.keys.RIGHT, function()
	if super() then vistty.screen.next(); return true end
end)
vistty.input.bind(vistty.keys.TAB, function()
	if super() then vistty.tab.next(); return true end
end)
vistty.input.bind_keys({
	vistty.keys.NUM1, vistty.keys.NUM2, vistty.keys.NUM3,
	vistty.keys.NUM4, vistty.keys.NUM5, vistty.keys.NUM6,
	vistty.keys.NUM7, vistty.keys.NUM8, vistty.keys.NUM9,
}, function(n)
	if super() then vistty.tab.switch(n); return true end
end)

vistty.input.bind(vistty.keys.J, function()
	if super() then vistty.term.scroll_by(-1); return true end
end)
vistty.input.bind(vistty.keys.K, function()
	if super() then vistty.term.scroll_by(1); return true end
end)
vistty.input.bind(vistty.keys.R, function()
	if super() then vistty.reload(); return true end
end)
vistty.input.bind(vistty.keys.Q, function()
	if super() then
		vistty.ui.dialog("退出确认", "确定要退出 Vistty 吗？\n\n←/→  切换    Enter  确认    Esc  取消", "", {"确认", "取消"},
			function(result)
				if result == 1 then
					vistty.exit()
				end
			end)
		return true
	end
end)

-- Print Screen：截取焦点屏幕并保存为 PNG（路径：$XDG_PICTURES_DIR → ~/Pictures → $HOME → /tmp）
vistty.input.bind(vistty.keys.PRINT, function()
	vistty.screenshot()
	return true
end)

-- Mod+Shift+/ 显示快捷键帮助
vistty.input.bind(vistty.keys.SLASH, function()
	if super() and vistty.input.pressed(vistty.keys.LEFT_SHIFT) then
		local mod_name = "ALT"
		if not (vistty.backend_name() == "" or vistty.backend.is_wayland()) then
			mod_name = "SUPER"
		end
		local content = table.concat({
			mod_name .. "+T       新建标签",
			mod_name .. "+W       关闭标签",
			mod_name .. "+Tab     切换标签",
			mod_name .. "+1~9     跳转标签",
			mod_name .. "+J/K     滚动终端",
			mod_name .. "+←/→     切换屏幕",
			mod_name .. "+=/-/0   缩放字体",
			mod_name .. "+R       重载插件",
			mod_name .. "+Q       退出",
			"PrtSc         截图",
			mod_name .. "+Shift+/  帮助",
			"",
			"Esc / Enter   关闭此窗口",
		}, "\n")
		vistty.ui.dialog("快捷键帮助", content)
		return true
	end
end)

local statusbar = require("statusbar")
statusbar.init()

-- === 生命周期钩子示例（默认注释，按需启用） ===
--
-- -- on_activate：后端确定后触发（backend_name 已注入），可注册后端专属快捷键。
-- vistty.on_activate(function(name)
-- 	vistty.log.debug("activated with backend: " .. name)
-- end)
--
-- -- on_exit：程序退出前触发，可保存状态或清理资源。
-- vistty.on_exit(function()
-- 	vistty.log.debug("vistty exiting, bye")
-- end)
--
-- -- on_tab_new / on_tab_close / on_tab_switch：标签生命周期。
-- vistty.on_tab_new(function(idx, title)
-- 	vistty.log.debug("tab #" .. idx .. " created: " .. title)
-- end)
-- vistty.on_tab_close(function(idx, title)
-- 	vistty.log.debug("tab #" .. idx .. " closed: " .. title)
-- end)
-- vistty.on_tab_switch(function(newIdx, oldIdx)
-- 	vistty.log.debug("tab switch " .. oldIdx .. " -> " .. newIdx)
-- end)
--
-- -- on_screen_switch：屏幕焦点切换后触发。
-- vistty.on_screen_switch(function(idx)
-- 	vistty.log.debug("screen switched to " .. idx)
-- end)
--
-- -- on_title_change：终端标题变化（经主线程缓冲批量触发）。
-- vistty.on_title_change(function(title)
-- 	vistty.log.debug("title changed: " .. title)
-- end)
--
-- -- on_resize：窗口/尺寸变化后触发。
-- vistty.on_resize(function(output_id, w, h, cols, rows)
-- 	vistty.log.debug("resize " .. w .. "x" .. h .. " @" .. cols .. "x" .. rows)
-- end)
--
-- -- on_zoom：字体缩放后触发。
-- vistty.on_zoom(function(size)
-- 	vistty.log.debug("zoom to " .. size)
-- end)
