package ui

type InputField struct {
	text        string
	cursorPos   int
	placeholder string
}

func NewInputField(placeholder string) *InputField {
	return &InputField{placeholder: placeholder}
}

func (f *InputField) Text() string {
	return f.text
}

func (f *InputField) SetText(text string) {
	f.text = text
	f.cursorPos = len([]rune(text))
}

func (f *InputField) InsertText(text string) {
	runes := []rune(f.text)
	ins := []rune(text)
	newRunes := make([]rune, 0, len(runes)+len(ins))
	newRunes = append(newRunes, runes[:f.cursorPos]...)
	newRunes = append(newRunes, ins...)
	newRunes = append(newRunes, runes[f.cursorPos:]...)
	f.text = string(newRunes)
	f.cursorPos += len(ins)
}

func (f *InputField) DeleteBackward() {
	if f.cursorPos <= 0 {
		return
	}
	runes := []rune(f.text)
	newRunes := make([]rune, 0, len(runes)-1)
	newRunes = append(newRunes, runes[:f.cursorPos-1]...)
	newRunes = append(newRunes, runes[f.cursorPos:]...)
	f.text = string(newRunes)
	f.cursorPos--
}

func (f *InputField) CursorPos() int {
	return f.cursorPos
}

func (f *InputField) Placeholder() string {
	return f.placeholder
}

func (f *InputField) DisplayText() string {
	if f.text == "" {
		return f.placeholder
	}
	return f.text
}
