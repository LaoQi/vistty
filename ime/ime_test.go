package ime

import (
	"testing"
)

type mockIM struct {
	name string
}

func (m *mockIM) Name() string { return m.name }
func (m *mockIM) Lookup(input string) []Candidate {
	return []Candidate{{Word: "w", Code: input}}
}
func (m *mockIM) FormatPreedit(input string) string {
	return "pre:" + input
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
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
	a := &mockIM{name: "x"}
	r.Register(a)
	b := &mockIM{name: "x"}
	r.Register(b)
	if len(r.List()) != 1 {
		t.Fatalf("overwrite should keep single entry, got %v", r.List())
	}
}

func TestRegistryLookup(t *testing.T) {
	r := NewRegistry()
	m := &mockIM{name: "x"}
	r.Register(m)
	cands := r.Lookup("x", "ni")
	if len(cands) != 1 || cands[0].Word != "w" || cands[0].Code != "ni" {
		t.Fatalf("Lookup = %v, want [{w ni}]", cands)
	}
}

func TestRegistryLookupUnknown(t *testing.T) {
	r := NewRegistry()
	cands := r.Lookup("nope", "ni")
	if cands != nil {
		t.Fatalf("Lookup unknown should return nil, got %v", cands)
	}
}

func TestRegistryFormatPreedit(t *testing.T) {
	r := NewRegistry()
	m := &mockIM{name: "x"}
	r.Register(m)
	pre := r.FormatPreedit("x", "ni")
	if pre != "pre:ni" {
		t.Fatalf("FormatPreedit = %q, want pre:ni", pre)
	}
}

func TestRegistryFormatPreeditUnknown(t *testing.T) {
	r := NewRegistry()
	pre := r.FormatPreedit("nope", "ni")
	if pre != "ni" {
		t.Fatalf("FormatPreedit unknown should return input, got %q", pre)
	}
}

func TestRegistryClearLuaMethods(t *testing.T) {
	r := NewRegistry()
	a := &mockIM{name: "go"}
	r.Register(a)
	l := NewLuaIMM("lua", LuaIMMHooks{
		Lookup:        func(input string) []Candidate { return nil },
		FormatPreedit: func(input string) string { return input },
	})
	r.Register(l)
	if len(r.List()) != 2 {
		t.Fatalf("should have 2 methods, got %v", r.List())
	}
	r.ClearLuaMethods()
	if len(r.List()) != 1 {
		t.Fatalf("should have 1 method after ClearLuaMethods, got %v", r.List())
	}
	cands := r.Lookup("go", "ni")
	if cands == nil {
		t.Fatal("go method should still work")
	}
}

func TestLuaIMM(t *testing.T) {
	m := NewLuaIMM("lua", LuaIMMHooks{
		Lookup:        func(input string) []Candidate { return []Candidate{{Word: "x", Code: input}} },
		FormatPreedit: func(input string) string { return "f:" + input },
	})

	if m.Name() != "lua" {
		t.Fatalf("Name = %q, want lua", m.Name())
	}
	cands := m.Lookup("ni")
	if len(cands) != 1 || cands[0].Word != "x" {
		t.Fatalf("Lookup = %v", cands)
	}
	pre := m.FormatPreedit("ni")
	if pre != "f:ni" {
		t.Fatalf("FormatPreedit = %q, want f:ni", pre)
	}
}

func TestLuaIMMNilHooks(t *testing.T) {
	m := NewLuaIMM("empty", LuaIMMHooks{})
	cands := m.Lookup("ni")
	if cands != nil {
		t.Fatal("nil Lookup should return nil")
	}
	pre := m.FormatPreedit("ni")
	if pre != "ni" {
		t.Fatalf("nil FormatPreedit should return input, got %q", pre)
	}
}

func TestLuaIMMImplementsInterface(t *testing.T) {
	var _ InputMethod = (*LuaIMM)(nil)
	var _ InputMethod = (*mockIM)(nil)
}
