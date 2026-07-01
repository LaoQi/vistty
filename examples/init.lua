vistty.config = {
	backend   = "auto",
	shell     = "/bin/bash",
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
