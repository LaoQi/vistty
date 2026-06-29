package ime

import (
	"testing"
)

type mockIM struct {
	name      string
	active    bool
	activated int
	deact     int
	resetCnt  int
	lastEv    KeyEvent
	preedit   string
	cands     []Candidate
}

func (m *mockIM) Name() string         { return m.name }
func (m *mockIM) Activate()            { m.active = true; m.activated++ }
func (m *mockIM) Deactivate()          { m.active = false; m.deact++ }
func (m *mockIM) IsActive() bool       { return m.active }
func (m *mockIM) ProcessKey(ev KeyEvent) Response {
	m.lastEv = ev
	return Response{Consumed: true, Preedit: m.preedit, Candidates: m.cands}
}
func (m *mockIM) Preedit() string        { return m.preedit }
func (m *mockIM) Candidates() []Candidate { return m.cands }
func (m *mockIM) Reset()                  { m.resetCnt++ }

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.Active() != nil {
		t.Fatalf("new registry active should be nil, got %v", r.Active())
	}
	if got := r.List(); len(got) != 0 {
		t.Fatalf("new registry list should be empty, got %v", got)
	}
}

func TestRegistryRegisterAndList(t *testing.T) {
	r := NewRegistry()
	a := &mockIM{name: "a"}
	b := &mockIM{name: "b"}
	r.Register(a)
	r.Register(b)
	got := r.List()
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Fatalf("list len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("list[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRegistryRegisterOverwrite(t *testing.T) {
	r := NewRegistry()
	a := &mockIM{name: "x", preedit: "a"}
	r.Register(a)
	b := &mockIM{name: "x", preedit: "b"}
	r.Register(b)
	if len(r.List()) != 1 {
		t.Fatalf("overwrite should keep single entry, got %v", r.List())
	}
}

func TestRegistryActivate(t *testing.T) {
	r := NewRegistry()
	m := &mockIM{name: "pinyin", preedit: "ni"}
	r.Register(m)
	if !r.Activate("pinyin") {
		t.Fatal("Activate should return true for registered method")
	}
	if r.Active() != m {
		t.Fatal("Active should return pinyin method")
	}
	if m.activated != 1 {
		t.Fatalf("Activate should call Activate once, got %d", m.activated)
	}
	if !m.active {
		t.Fatal("mock should be active")
	}
}

func TestRegistryActivateUnknown(t *testing.T) {
	r := NewRegistry()
	if r.Activate("nope") {
		t.Fatal("Activate should return false for unknown method")
	}
	if r.Active() != nil {
		t.Fatal("Active should be nil after failed activate")
	}
}

func TestRegistryActivateSwitches(t *testing.T) {
	r := NewRegistry()
	a := &mockIM{name: "a"}
	b := &mockIM{name: "b"}
	r.Register(a)
	r.Register(b)
	r.Activate("a")
	r.Activate("b")
	if a.deact != 1 {
		t.Fatalf("a should be deactivated once, got %d", a.deact)
	}
	if b.activated != 1 {
		t.Fatalf("b should be activated once, got %d", b.activated)
	}
	if r.Active() != b {
		t.Fatal("Active should be b")
	}
}

func TestRegistryDeactivate(t *testing.T) {
	r := NewRegistry()
	m := &mockIM{name: "x"}
	r.Register(m)
	r.Activate("x")
	r.Deactivate()
	if r.Active() != nil {
		t.Fatal("Active should be nil after Deactivate")
	}
	if m.deact != 1 {
		t.Fatalf("Deactivate should call Deactivate once, got %d", m.deact)
	}
}

func TestRegistryDeactivateNoop(t *testing.T) {
	r := NewRegistry()
	r.Deactivate()
	if r.Active() != nil {
		t.Fatal("Active should be nil")
	}
}

func TestRegistryProcessKeyRoutesToActive(t *testing.T) {
	r := NewRegistry()
	m := &mockIM{name: "x", preedit: "abc", cands: []Candidate{{Word: "w", Code: "c"}}}
	r.Register(m)
	r.Activate("x")
	ev := KeyEvent{Rune: 'a', Code: 30, State: true}
	resp := r.ProcessKey(ev)
	if !resp.Consumed {
		t.Fatal("ProcessKey should return Consumed=true")
	}
	if resp.Preedit != "abc" {
		t.Fatalf("Preedit = %q, want abc", resp.Preedit)
	}
	if len(resp.Candidates) != 1 || resp.Candidates[0].Word != "w" {
		t.Fatalf("Candidates = %v, want [{w c}]", resp.Candidates)
	}
	if m.lastEv != ev {
		t.Fatalf("lastEv = %v, want %v", m.lastEv, ev)
	}
}

func TestRegistryProcessKeyNoActive(t *testing.T) {
	r := NewRegistry()
	m := &mockIM{name: "x"}
	r.Register(m)
	resp := r.ProcessKey(KeyEvent{State: true})
	if resp.Consumed {
		t.Fatal("ProcessKey without active should return Consumed=false")
	}
	if resp.Commit != "" || resp.Preedit != "" || resp.Candidates != nil {
		t.Fatalf("empty Response expected, got %+v", resp)
	}
}

func TestLuaIMM(t *testing.T) {
	activated := 0
	deactivated := 0
	resetCnt := 0
	var gotEv KeyEvent
	m := NewLuaIMM("lua", LuaIMMHooks{
		OnActivate:   func() { activated++ },
		OnDeactivate: func() { deactivated++ },
		ProcessKey: func(ev KeyEvent) Response {
			gotEv = ev
			return Response{Consumed: true, Commit: "ok"}
		},
		Preedit:    func() string { return "pre" },
		Candidates: func() []Candidate { return []Candidate{{Word: "x"}} },
		OnReset:    func() { resetCnt++ },
	})

	if m.Name() != "lua" {
		t.Fatalf("Name = %q, want lua", m.Name())
	}
	if m.IsActive() {
		t.Fatal("should start inactive")
	}
	m.Activate()
	if !m.IsActive() {
		t.Fatal("should be active after Activate")
	}
	if activated != 1 {
		t.Fatalf("OnActivate called %d times", activated)
	}
	resp := m.ProcessKey(KeyEvent{Rune: 'b', State: true})
	if !resp.Consumed || resp.Commit != "ok" {
		t.Fatalf("ProcessKey = %+v", resp)
	}
	if gotEv.Rune != 'b' {
		t.Fatalf("gotEv.Rune = %q", gotEv.Rune)
	}
	if m.Preedit() != "pre" {
		t.Fatalf("Preedit = %q", m.Preedit())
	}
	if c := m.Candidates(); len(c) != 1 || c[0].Word != "x" {
		t.Fatalf("Candidates = %v", c)
	}
	m.Deactivate()
	if m.IsActive() {
		t.Fatal("should be inactive after Deactivate")
	}
	if deactivated != 1 {
		t.Fatalf("OnDeactivate called %d times", deactivated)
	}
	m.Reset()
	if resetCnt != 1 {
		t.Fatalf("OnReset called %d times", resetCnt)
	}
}

func TestLuaIMMNilHooks(t *testing.T) {
	m := NewLuaIMM("empty", LuaIMMHooks{})
	m.Activate()
	m.Deactivate()
	if m.Preedit() != "" {
		t.Fatal("nil preedit should return empty")
	}
	if m.Candidates() != nil {
		t.Fatal("nil candidates should return nil")
	}
	resp := m.ProcessKey(KeyEvent{State: true})
	if resp.Consumed {
		t.Fatal("nil ProcessKey should return Consumed=false")
	}
	m.Reset()
}

func TestLuaIMMImplementsInterface(t *testing.T) {
	var _ InputMethod = (*LuaIMM)(nil)
	var _ InputMethod = (*mockIM)(nil)
}
