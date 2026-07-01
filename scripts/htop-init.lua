vistty.config = {
	backend   = "drm-gbm",
	shell     = "/usr/bin/htop",
	font      = "",
	fontsize  = 14,
	primary   = "",
	error_log = "",
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
vistty.input.bind(vistty.keys.Q, function()
	if super() then vistty.exit(); return true end
end)

vistty.ui.enable("bottom", 1)
vistty.ui.on_render(function(ctx)
	local w, h = ctx:size()
	ctx:rect(0, 0, w, h, {bg=vistty.colors.DARKGRAY})
	ctx:text(2, 0, os.date("%H:%M:%S"), {fg="#64C8FF"})
	ctx:text(w - 10, 0, "tabs:" .. vistty.tab.count(), {fg=vistty.colors.GOLD})
	return true
end)
