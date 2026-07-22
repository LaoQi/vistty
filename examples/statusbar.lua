local M = {}

local cpu_temp_cache = nil
local cpu_temp_tick = 0
local cpu_temp_available = true

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

return M
