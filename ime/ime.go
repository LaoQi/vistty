// Package ime 定义输入法抽象接口与多输入法注册中心。
package ime

type KeyEvent struct {
	Rune  rune
	Code  uint16
	Mods  uint8
	State bool
}

type Candidate struct {
	Word string
	Code string
}

type Response struct {
	Consumed   bool
	Commit     string
	Preedit    string
	Candidates []Candidate
}

type InputMethod interface {
	Name() string
	Activate()
	Deactivate()
	IsActive() bool
	ProcessKey(ev KeyEvent) Response
	Preedit() string
	Candidates() []Candidate
	Reset()
}

type Registry struct {
	methods map[string]InputMethod
	active  InputMethod
}

func NewRegistry() *Registry {
	return &Registry{methods: make(map[string]InputMethod)}
}
