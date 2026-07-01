package ime

import "sort"

func (r *Registry) Register(m InputMethod) {
	if r.methods == nil {
		r.methods = make(map[string]InputMethod)
	}
	r.methods[m.Name()] = m
}

func (r *Registry) Lookup(name, input string) []Candidate {
	if m, ok := r.methods[name]; ok {
		return m.Lookup(input)
	}
	return nil
}

func (r *Registry) FormatPreedit(name, input string) string {
	if m, ok := r.methods[name]; ok {
		return m.FormatPreedit(input)
	}
	return input
}

func (r *Registry) List() []string {
	names := make([]string, 0, len(r.methods))
	for name := range r.methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) ClearLuaMethods() {
	for name, m := range r.methods {
		if _, ok := m.(*LuaIMM); ok {
			delete(r.methods, name)
		}
	}
}
