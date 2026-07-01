package ime

type Candidate struct {
	Word string
	Code string
}

type InputMethod interface {
	Name() string
	Lookup(input string) []Candidate
	FormatPreedit(input string) string
}

type Registry struct {
	methods map[string]InputMethod
}

func NewRegistry() *Registry {
	return &Registry{methods: make(map[string]InputMethod)}
}
