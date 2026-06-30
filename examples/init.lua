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
}

local function super()
	return vistty.input.pressed(vistty.keys.LEFT_SUPER) or
	       vistty.input.pressed(vistty.keys.RIGHT_SUPER)
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
	if super() then vistty.tab.prev(); return true end
end)
vistty.input.bind(vistty.keys.RIGHT, function()
	if super() then vistty.tab.next(); return true end
end)
vistty.input.bind(vistty.keys.TAB, function()
	if super() then vistty.screen.next(); return true end
end)
vistty.input.bind_keys({
	vistty.keys.NUM1, vistty.keys.NUM2, vistty.keys.NUM3,
	vistty.keys.NUM4, vistty.keys.NUM5, vistty.keys.NUM6,
	vistty.keys.NUM7, vistty.keys.NUM8, vistty.keys.NUM9,
}, function(n)
	if super() then vistty.screen.switch(n - 1); return true end
end)

-- === 插件热重载 / 退出 ===
-- Super+R 热重载 init.lua（开发期调试用，修改配置后无需重启 vistty）
-- Super+Q 退出 vistty（走两阶段关闭路径，与窗口关闭/信号一致）
vistty.input.bind(vistty.keys.R, function()
	if super() then vistty.reload(); return true end
end)
vistty.input.bind(vistty.keys.Q, function()
	if super() then vistty.exit(); return true end
end)

-- === 拼音输入法 ===
-- Ctrl+Space 切换激活/去激活。on_key 钩子按注册顺序执行，
-- 此钩子仅匹配 Ctrl+Space 时返回 true 吞掉，其他情况返回 nil 放行。
vistty.input.on_key(function(ev)
	if ev.state ~= vistty.state.PRESS then return end
	if (ev.mods % (vistty.mods.CTRL * 2)) >= vistty.mods.CTRL
	   and ev.code == vistty.keys.SPACE then
		if vistty.ime.active() then
			vistty.ime.deactivate()
		else
			vistty.ime.activate("pinyin")
		end
		return true
	end
end)

-- 按键路由到 active 输入法。必须注册在切换钩子之后：
-- IME 未激活直接放行；process_key 返回 consumed=true 则吞掉按键不传终端。
vistty.input.on_key(function(ev)
	if ev.state ~= vistty.state.PRESS then return end
	if not vistty.ime.active() then return end
	local r = vistty.ime.process_key(ev)
	if r.commit and r.commit ~= "" then
		vistty.term.send(r.commit)
	end
	return r.consumed
end)

-- === 底部状态栏 ===
-- 启用 1 行底部面板：IME 激活时渲染 preedit + 候选词，否则渲染时钟与标签数。
-- 颜色可用 vistty.colors 常量或 "#RRGGBB" / "#RRGGBBAA" 字符串
vistty.ui.enable("bottom", 1)
vistty.ui.on_render(function(ctx)
	local w, h = ctx:size()
	ctx:rect(0, 0, w, h, {bg=vistty.colors.DARKGRAY})

	if vistty.ime.active() then
		local pre = vistty.ime.preedit()
		if pre == "" then
			ctx:text(2, 0, "中", {fg=vistty.colors.CYAN})
		else
			ctx:text(0, 0, pre .. "_", {fg=vistty.colors.CYAN})
			local cands = vistty.ime.candidates()
			local x = #pre + 2
			for i, c in ipairs(cands) do
				local idx = tostring(i)
				ctx:text(x, 0, idx, {fg=vistty.colors.GRAY})
				ctx:text(x + #idx, 0, c.word, {fg=vistty.colors.WHITE})
				x = x + #idx + #c.word + 1
			end
		end
	else
		ctx:text(2, 0, os.date("%H:%M:%S"), {fg="#64C8FF"})
		ctx:text(w - 10, 0, "tabs:" .. vistty.tab.count(), {fg=vistty.colors.GOLD})
	end
	return true
end)

-- === 模块化加载 ===
-- 完全开放沙箱，dofile 可用，可将插件拆分到独立文件：
-- dofile(os.getenv("HOME") .. "/.config/vistty/plugins/extra.lua")
