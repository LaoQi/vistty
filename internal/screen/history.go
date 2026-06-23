package screen

type History struct {
	lines    []*Line
	maxLines int
}

func NewHistory(maxLines int) *History {
	return &History{
		lines:    nil,
		maxLines: maxLines,
	}
}

func (h *History) Push(line *Line) {
	h.lines = append(h.lines, line.Clone())
	if len(h.lines) > h.maxLines {
		for i := 0; i < len(h.lines)-h.maxLines; i++ {
			h.lines[i] = nil
		}
		h.lines = h.lines[len(h.lines)-h.maxLines:]
	}
}

func (h *History) Len() int {
	return len(h.lines)
}

func (h *History) Line(idx int) *Line {
	if idx < 0 || idx >= len(h.lines) {
		return nil
	}
	return h.lines[idx]
}

func (h *History) Clear() {
	h.lines = nil
}
