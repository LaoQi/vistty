package ime

import "sort"

func (r *Registry) Register(m InputMethod) {
	if r.methods == nil {
		r.methods = make(map[string]InputMethod)
	}
	r.methods[m.Name()] = m
}

func (r *Registry) Activate(name string) bool {
	m, ok := r.methods[name]
	if !ok {
		return false
	}
	if r.active != nil {
		r.active.Deactivate()
	}
	r.active = m
	m.Activate()
	return true
}

func (r *Registry) Deactivate() {
	if r.active != nil {
		r.active.Deactivate()
		r.active = nil
	}
}

func (r *Registry) Active() InputMethod {
	return r.active
}

func (r *Registry) List() []string {
	names := make([]string, 0, len(r.methods))
	for name := range r.methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) ProcessKey(ev KeyEvent) Response {
	if r.active == nil {
		return Response{}
	}
	return r.active.ProcessKey(ev)
}

func (r *Registry) ClearLuaMethods() {
	if r.active != nil {
		if _, ok := r.active.(*LuaIMM); ok {
			r.active = nil
		}
	}
	for name, m := range r.methods {
		if _, ok := m.(*LuaIMM); ok {
			delete(r.methods, name)
		}
	}
}
