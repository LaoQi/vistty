local M = {}

local statusbar = require("statusbar")

local ime_active = false
local ime_buf = ""
local ime_page = 0
local ime_cands = nil
local ime_cand_buf = ""
local ime_preedit = ""
local ime_preedit_buf = ""
local ime_panel_width = 0

local function cand_display_width(idx, word)
	return vistty.display_width(tostring(idx)) + vistty.display_width(word) + 1
end

function M.page_slice(cands, page, avail_w)
	if avail_w <= 0 then return {}, 0 end
	if #cands == 0 then return {}, 0 end
	local start = 0
	local cur_page = 0
	while cur_page < page do
		local x = 0
		local count = 0
		while start + count < #cands and count < 9 do
			local w = cand_display_width(count + 1, cands[start + count + 1].word)
			if x + w > avail_w and count > 0 then break end
			x = x + w
			count = count + 1
		end
		if count == 0 then return {}, start end
		start = start + count
		cur_page = cur_page + 1
	end
	local result = {}
	local x = 0
	local i = 0
	while start + i < #cands and i < 9 do
		local w = cand_display_width(i + 1, cands[start + i + 1].word)
		if x + w > avail_w and i > 0 then break end
		x = x + w
		result[#result + 1] = cands[start + i + 1]
		i = i + 1
	end
	return result, start
end

function M.total_pages(cands, avail_w)
	if avail_w <= 0 or #cands == 0 then return 0 end
	local total = 1
	local start = 0
	while start < #cands do
		local x = 0
		local count = 0
		while start + count < #cands and count < 9 do
			local w = cand_display_width(count + 1, cands[start + count + 1].word)
			if x + w > avail_w and count > 0 then break end
			x = x + w
			count = count + 1
		end
		if count == 0 then break end
		start = start + count
		if start < #cands then total = total + 1 end
	end
	return total
end

function M.active()
	return ime_active
end

function M.activate()
	ime_active = true
	ime_buf = ""
	ime_page = 0
end

function M.deactivate()
	ime_active = false
	ime_buf = ""
	ime_page = 0
end

function M.buf()
	return ime_buf
end

function M.preedit()
	if not ime_active or ime_buf == "" then return "" end
	if ime_buf ~= ime_preedit_buf then
		ime_preedit = vistty.pinyin.format_preedit(ime_buf)
		ime_preedit_buf = ime_buf
	end
	return ime_preedit
end

function M.candidates()
	if not ime_active or ime_buf == "" then return {} end
	if ime_buf ~= ime_cand_buf then
		ime_cands = vistty.pinyin.lookup(ime_buf)
		ime_cand_buf = ime_buf
	end
	return ime_cands
end

function M.get_panel_width()
	return ime_panel_width
end

function M.set_panel_width(w)
	ime_panel_width = w
end

local function has_modifier(ev)
	return (ev.mods % (vistty.mods.CTRL * 2)) >= vistty.mods.CTRL
		or (ev.mods % (vistty.mods.ALT * 2)) >= vistty.mods.ALT
		or (ev.mods % (vistty.mods.SUPER * 2)) >= vistty.mods.SUPER
end

local function is_lower_letter(ev)
	return ev.rune >= 97 and ev.rune <= 122
end

local function remove_last_char(s)
	local bytes = {s:byte(1, #s)}
	local n = #bytes
	if n == 0 then return "" end
	local last = bytes[n]
	local count = 1
	if last >= 128 then
		if last >= 240 then count = 4
		elseif last >= 224 then count = 3
		elseif last >= 192 then count = 2
		end
	end
	if count > n then count = n end
	return s:sub(1, n - count)
end

function M.setup_key_handler()
	vistty.input.on_key(function(ev)
		if ev.state ~= vistty.state.PRESS then return end
		if not ime_active then return end
		if has_modifier(ev) then return end

		if is_lower_letter(ev) then
			ime_buf = ime_buf .. string.char(ev.rune)
			ime_page = 0
			return true
		end

		if ime_buf == "" then return end

		if ev.code == vistty.keys.BACKSPACE then
			ime_buf = remove_last_char(ime_buf)
			ime_page = 0
			return true
		end

		if ev.code == vistty.keys.ESCAPE then
			ime_buf = ""
			ime_page = 0
			return true
		end

		if ev.code == vistty.keys.ENTER then
			vistty.term.send(ime_buf)
			ime_buf = ""
			ime_page = 0
			return true
		end

		if ev.code == vistty.keys.SPACE then
			local cands = M.candidates()
			if #cands > 0 then
				vistty.term.send(cands[1].word)
			else
				vistty.term.send(ime_buf)
			end
			ime_buf = ""
			ime_page = 0
			return true
		end

		for i = 1, 9 do
			if ev.code == vistty.keys["NUM" .. i] then
				local cands = M.candidates()
				local avail = M.get_panel_width() - vistty.display_width(M.preedit() .. "_")
				local page_cands = M.page_slice(cands, ime_page, avail)
				if i <= #page_cands then
					vistty.term.send(page_cands[i].word)
				end
				ime_buf = ""
				ime_page = 0
				return true
			end
		end

		if ev.code == vistty.keys.MINUS or ev.code == vistty.keys.LEFT
		   or ev.code == vistty.keys.UP then
			if ime_page > 0 then ime_page = ime_page - 1 end
			return true
		end
		if ev.code == vistty.keys.EQUAL or ev.code == vistty.keys.RIGHT
		   or ev.code == vistty.keys.DOWN or ev.code == vistty.keys.TAB then
			local cands = M.candidates()
			local avail = M.get_panel_width() - vistty.display_width(M.preedit() .. "_")
			local total = M.total_pages(cands, avail)
			if ime_page + 1 < total then
				ime_page = ime_page + 1
			else
				ime_page = 0
			end
			return true
		end
	end)
end

function M.setup_render_handler()
	vistty.ui.enable("bottom", 1)
	vistty.ui.on_render(function(ctx)
		local w, h = ctx:size()
		local rightW = statusbar.right_width(w)
		local sepW = 2
		local imeW = w - rightW - sepW
		if imeW < 0 then imeW = 0 end
		M.set_panel_width(imeW)

		ctx:rect(0, 0, w, h, {bg=vistty.colors.DARKGRAY})

		if ime_active then
			local pre = M.preedit()
			if pre == "" then
				ctx:text(2, 0, "中", {fg=vistty.colors.CYAN})
			else
				ctx:text(0, 0, pre .. "_", {fg=vistty.colors.CYAN})
				local cands = M.candidates()
				local preW = vistty.display_width(pre .. "_")
				local avail = imeW - preW
				if avail < 0 then avail = 0 end
				local page_cands = M.page_slice(cands, ime_page, avail)

				local x = preW
				for i, c in ipairs(page_cands) do
					local idx = tostring(i)
					local idxW = vistty.display_width(idx)
					ctx:text(x, 0, idx, {fg=vistty.colors.GRAY})
					ctx:text(x + idxW, 0, c.word, {fg=vistty.colors.WHITE})
					x = x + idxW + vistty.display_width(c.word) + 1
				end

				local total = M.total_pages(cands, avail)
				if total > 1 then
					local pg = "(" .. (ime_page + 1) .. "/" .. total .. ")"
					local pgW = vistty.display_width(pg)
					if pgW < imeW then
						ctx:text(imeW - pgW - 1, 0, pg, {fg=vistty.colors.GRAY})
					end
				end
			end
		else
			ctx:text(2, 0, "EN", {fg=vistty.colors.GRAY})
		end

		ctx:text(imeW, 0, "│", {fg="#555555"})

		statusbar.render_right(ctx, w, h)
		return true
	end)
end

function M.init()
	M.setup_key_handler()
	M.setup_render_handler()
end

return M
