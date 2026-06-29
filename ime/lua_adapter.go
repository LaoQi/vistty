package ime

type LuaIMMHooks struct {
	OnActivate   func()
	OnDeactivate func()
	ProcessKey   func(ev KeyEvent) Response
	Preedit      func() string
	Candidates   func() []Candidate
	OnReset      func()
}

type LuaIMM struct {
	name        string
	active      bool
	onActivate  func()
	onDeactivate func()
	processKey  func(ev KeyEvent) Response
	preedit     func() string
	candidates  func() []Candidate
	onReset     func()
}

func NewLuaIMM(name string, hooks LuaIMMHooks) *LuaIMM {
	return &LuaIMM{
		name:         name,
		onActivate:   hooks.OnActivate,
		onDeactivate: hooks.OnDeactivate,
		processKey:   hooks.ProcessKey,
		preedit:      hooks.Preedit,
		candidates:   hooks.Candidates,
		onReset:      hooks.OnReset,
	}
}

func (l *LuaIMM) Name() string { return l.name }

func (l *LuaIMM) Activate() {
	l.active = true
	if l.onActivate != nil {
		l.onActivate()
	}
}

func (l *LuaIMM) Deactivate() {
	l.active = false
	if l.onDeactivate != nil {
		l.onDeactivate()
	}
}

func (l *LuaIMM) IsActive() bool { return l.active }

func (l *LuaIMM) ProcessKey(ev KeyEvent) Response {
	if l.processKey != nil {
		return l.processKey(ev)
	}
	return Response{}
}

func (l *LuaIMM) Preedit() string {
	if l.preedit != nil {
		return l.preedit()
	}
	return ""
}

func (l *LuaIMM) Candidates() []Candidate {
	if l.candidates != nil {
		return l.candidates()
	}
	return nil
}

func (l *LuaIMM) Reset() {
	if l.onReset != nil {
		l.onReset()
	}
}
