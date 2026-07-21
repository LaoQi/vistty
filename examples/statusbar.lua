local M = {}

local ime = require("ime")

local cpu_temp_cache = nil
local cpu_temp_tick = 0
local cpu_temp_available = true

local left_provider = nil
local left_width = 0

function M.get_cpu_temp()
	if not cpu_temp_available then return nil end
	cpu_temp_tick = cpu_temp_tick + 1
	if cpu_temp_tick % 60 ~= 1 and cpu_temp_cache ~= nil then
		return cpu_temp_cache
	end
	local f = io.open("/sys/class/thermal/thermal_zone0/temp", "r")
	if not f then
		cpu_temp_available = false
		cpu_temp_cache = nil
		return nil
	end
	local val = f:read("*n")
	f:close()
	if val then
		cpu_temp_cache = math.floor(val / 1000)
	else
		cpu_temp_available = false
		cpu_temp_cache = nil
	end
	return cpu_temp_cache
end

function M.right_width(w)
	local dateW = vistty.display_width("0000-00-00 00:00:00")
	local temp = M.get_cpu_temp()
	local tempW = 0
	if temp then
		tempW = vistty.display_width(tostring(temp) .. "°C") + 1
	end
	return tempW + dateW
end

function M.register_left(provider)
	left_provider = provider
end

function M.unregister_left()
	left_provider = nil
	left_width = 0
end

function M.left_available_width(output_id)
	if output_id then
		return M._left_widths[output_id] or 0
	end
	return left_width
end

function M.render_right(ctx, w, h)
	local dateStr = os.date("%Y-%m-%d %H:%M:%S")
	local dateW = vistty.display_width(dateStr)
	ctx:text(w - dateW, 0, dateStr, {fg="#64C8FF"})

	local temp = M.get_cpu_temp()
	if temp then
		local tempStr = tostring(temp) .. "°C"
		local tempW = vistty.display_width(tempStr)
		local tempColor = vistty.colors.GOLD
		if temp > 80 then tempColor = vistty.colors.RED end
		ctx:text(w - dateW - tempW - 1, 0, tempStr, {fg=tempColor})
	end
end

function M.init()
	M._left_widths = {}

	vistty.ui.enable("bottom", 1)
	vistty.ui.on_render(function(ctx)
		local w, h = ctx:size()
		local oid = ctx:output_id()
		local focused = vistty.screen.focused_output_id()
		local isFocused = (oid == focused)

		local rightW = M.right_width(w)
		local sepW = 2
		left_width = w - rightW - sepW
		if left_width < 0 then left_width = 0 end
		M._left_widths[oid] = left_width

		ctx:rect(0, 0, w, h, {bg=vistty.colors.DARKGRAY})

		if isFocused and left_provider and left_provider.render then
			left_provider.render(ctx, left_width, h, oid)
		end

		ctx:rect(left_width, 0, w - left_width, h, {bg=vistty.colors.DARKGRAY})
		ctx:text(left_width, 0, "│", {fg="#555555"})

		M.render_right(ctx, w, h)
		return true
	end)

	ime.init(M)
	M.register_left(ime)
end

return M
