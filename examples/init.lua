vistty.config = {
	backend   = "auto",
	shell     = "/bin/bash",
	font      = "",
	fontsize  = 24,
	primary   = "",
	error_log = "",
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

vistty.input.bind(vistty.keys.R, function()
	if super() then vistty.reload(); return true end
end)
vistty.input.bind(vistty.keys.Q, function()
	if super() then vistty.exit(); return true end
end)

local ime = require("ime")

vistty.input.on_key(function(ev)
	if ev.state ~= vistty.state.PRESS then return end
	if (ev.mods % (vistty.mods.CTRL * 2)) >= vistty.mods.CTRL
	   and ev.code == vistty.keys.SPACE then
		if ime.active() then
			ime.deactivate()
		else
			ime.activate()
		end
		return true
	end
end)

ime.init()

-- === 生命周期钩子示例（默认注释，按需启用） ===
--
-- -- on_activate：后端确定后触发（backend_name 已注入），可注册后端专属快捷键。
-- vistty.on_activate(function(name)
-- 	vistty.log("activated with backend: " .. name)
-- end)
--
-- -- on_exit：程序退出前触发，可保存状态或清理资源。
-- vistty.on_exit(function()
-- 	vistty.log("vistty exiting, bye")
-- end)
--
-- -- on_tab_new / on_tab_close / on_tab_switch：标签生命周期。
-- vistty.on_tab_new(function(idx, title)
-- 	vistty.log("tab #" .. idx .. " created: " .. title)
-- end)
-- vistty.on_tab_close(function(idx, title)
-- 	vistty.log("tab #" .. idx .. " closed: " .. title)
-- end)
-- vistty.on_tab_switch(function(newIdx, oldIdx)
-- 	vistty.log("tab switch " .. oldIdx .. " -> " .. newIdx)
-- end)
--
-- -- on_screen_switch：屏幕焦点切换后触发。
-- vistty.on_screen_switch(function(idx)
-- 	vistty.log("screen switched to " .. idx)
-- end)
--
-- -- on_title_change：终端标题变化（经主线程缓冲批量触发）。
-- vistty.on_title_change(function(title)
-- 	vistty.log("title changed: " .. title)
-- end)
--
-- -- on_resize：窗口/尺寸变化后触发。
-- vistty.on_resize(function(output_id, w, h, cols, rows)
-- 	vistty.log("resize " .. w .. "x" .. h .. " @" .. cols .. "x" .. rows)
-- end)
--
-- -- on_zoom：字体缩放后触发。
-- vistty.on_zoom(function(size)
-- 	vistty.log("zoom to " .. size)
-- end)
