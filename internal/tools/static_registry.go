package tools

import "sort"

type Entry struct {
	Definition Definition
	Handler    Handler
}

type StaticRegistry struct {
	definitions []Definition
	handlers    map[string]Handler
}

func NewStaticRegistry(entries ...Entry) *StaticRegistry {
	registry := &StaticRegistry{
		handlers: make(map[string]Handler),
	}

	for _, entry := range entries {
		definition := entry.Definition
		if entry.Handler != nil {
			handlerDefinition := entry.Handler.Definition()
			if definition.Name == "" {
				definition = handlerDefinition
			}

			registry.handlers[handlerDefinition.Name] = entry.Handler
		}

		if definition.Name == "" {
			continue
		}

		registry.definitions = append(registry.definitions, definition)
	}

	sort.Slice(registry.definitions, func(i, j int) bool {
		return registry.definitions[i].Name < registry.definitions[j].Name
	})

	return registry
}

func (r *StaticRegistry) List() []Definition {
	if r == nil {
		return nil
	}

	definitions := make([]Definition, len(r.definitions))
	copy(definitions, r.definitions)
	return definitions
}

func (r *StaticRegistry) Lookup(name string) (Handler, bool) {
	if r == nil {
		return nil, false
	}

	handler, ok := r.handlers[name]
	return handler, ok
}
