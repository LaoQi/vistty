package screen

type Selection struct {
	StartRow int
	StartCol int
	EndRow   int
	EndCol   int
	Active   bool
}

func NewSelection() *Selection {
	return &Selection{}
}

func (s *Selection) SetStart(row, col int) {
	s.StartRow = row
	s.StartCol = col
}

func (s *Selection) SetEnd(row, col int) {
	s.EndRow = row
	s.EndCol = col
}

func (s *Selection) Contains(row, col int) bool {
	if !s.Active {
		return false
	}
	sr, sc, er, ec := s.Normalized()
	if row < sr || row > er {
		return false
	}
	if row == sr && col < sc {
		return false
	}
	if row == er && col > ec {
		return false
	}
	return true
}

func (s *Selection) Clear() {
	*s = Selection{}
}

func (s *Selection) IsEmpty() bool {
	return s.StartRow == s.EndRow && s.StartCol == s.EndCol
}

func (s *Selection) Normalized() (startRow, startCol, endRow, endCol int) {
	if s.StartRow < s.EndRow || (s.StartRow == s.EndRow && s.StartCol <= s.EndCol) {
		return s.StartRow, s.StartCol, s.EndRow, s.EndCol
	}
	return s.EndRow, s.EndCol, s.StartRow, s.StartCol
}
