package ime

type LuaIMMHooks struct {
	Lookup        func(input string) []Candidate
	FormatPreedit func(input string) string
}

type LuaIMM struct {
	name          string
	lookup        func(input string) []Candidate
	formatPreedit func(input string) string
}

func NewLuaIMM(name string, hooks LuaIMMHooks) *LuaIMM {
	return &LuaIMM{
		name:          name,
		lookup:        hooks.Lookup,
		formatPreedit: hooks.FormatPreedit,
	}
}

func (l *LuaIMM) Name() string { return l.name }

func (l *LuaIMM) Lookup(input string) []Candidate {
	if l.lookup != nil {
		return l.lookup(input)
	}
	return nil
}

func (l *LuaIMM) FormatPreedit(input string) string {
	if l.formatPreedit != nil {
		return l.formatPreedit(input)
	}
	return input
}
