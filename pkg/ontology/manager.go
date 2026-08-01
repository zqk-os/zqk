package ontology

import "sync"

// OntologyManager defines the core interface for the Ontology Versioning Layer.
type OntologyManager interface {
	Register(o Ontology) error
	ResolveClass(name string) (Class, error)
	ValidateInstance(class string, properties map[string]any) error
}

type defaultManager struct {
	mu         sync.RWMutex
	ontologies map[string]Ontology
	classCache map[string]Class
}

// NewManager creates a new Ontology Manager.
func NewManager() OntologyManager {
	return &defaultManager{
		ontologies: make(map[string]Ontology),
		classCache: make(map[string]Class),
	}
}

func (m *defaultManager) Register(o Ontology) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ontologies[o.ID] = o
	for name, class := range o.Classes {
		m.classCache[name] = class
	}
	return nil
}

func (m *defaultManager) ResolveClass(name string) (Class, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if c, ok := m.classCache[name]; ok {
		return c, nil
	}
	return Class{}, ErrClassNotFound
}

func (m *defaultManager) ValidateInstance(className string, properties map[string]any) error {
	class, err := m.ResolveClass(className)
	if err != nil {
		return err
	}
	for propName, prop := range class.Properties {
		if prop.Required {
			if _, exists := properties[propName]; !exists {
				return ErrInvalidInstance
			}
		}
	}
	return nil
}
