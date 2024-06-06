package pkg

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/load"
)

func extractSingleDecl(instances []*build.Instance) (ast.Decl, error) {
	if len(instances) != 1 {
		return nil, fmt.Errorf("found %v instances, expected single", len(instances))
	}
	instance := instances[0]
	if len(instance.Files) != 1 {
		return nil, fmt.Errorf("found %v files, expected single", len(instance.Files))
	}
	file := instance.Files[0]
	if len(file.Decls) != 1 {
		return nil, fmt.Errorf("found %v decls, expected single", len(file.Decls))
	}
	return file.Decls[0], nil
}

func extractName(label ast.Label) (string, bool) {
	name, ok := label.(*ast.Ident)
	if !ok {
		return "", false
	}
	return name.Name, true
}

type Registry struct {
	Root         string
	SchemaNode   map[string]*Node
	SchemaName   map[*Node]string
	SchemasOrder []string
}

func NewRegistry() *Registry {
	return &Registry{SchemaNode: make(map[string]*Node), SchemaName: make(map[*Node]string)}
}

func (registry *Registry) AddSchema(name string, node *Node) {
	registry.SchemaNode[name] = node
	registry.SchemaName[node] = name
	registry.SchemasOrder = append(registry.SchemasOrder, name)
}

func (registry *Registry) createNode(root ast.Decl) (*Node, error) {
	switch lit := root.(type) {
	case *ast.Ident:
		if lit.Name == "_" {
			return &Node{}, nil
		}
		if strings.HasPrefix(lit.Name, "#") {
			schema, ok := registry.SchemaNode[lit.Name]
			if !ok {
				return nil, fmt.Errorf("unknown schema referenced: '%v'", lit.Name)
			}
			return schema, nil
		}
		if lit.Name == "string" {
			return &Node{CanBeString: true}, nil
		}
		if lit.Name == "number" {
			return &Node{CanBeNumber: true}, nil
		}
	case *ast.StructLit:
		node := &Node{CanBeObject: true, ObjectFields: make(map[string]*Node)}

		for _, element := range lit.Elts {
			if field, ok := element.(*ast.Field); ok {
				fieldName, ok := extractName(field.Label)
				if !ok {
					return nil, fmt.Errorf("failed to get field label: %v", field.Label)
				}
				if strings.HasPrefix(fieldName, "#") {
					schemaNode, err := registry.createNode(field.Value)
					if err != nil {
						return nil, fmt.Errorf("failed to fill schema %v: %w", fieldName, err)
					}
					registry.AddSchema(fieldName, schemaNode)
				} else {
					fieldNode, err := registry.createNode(field.Value)
					if err != nil {
						return nil, fmt.Errorf("failed to fill field %v: %w", fieldName, err)
					}
					node.ObjectFields[fieldName] = fieldNode
				}
			}
		}
		return node, nil
	case *ast.ListLit:
		if len(lit.Elts) != 1 {
			return nil, fmt.Errorf("only arrays with single ellipsis are supported (e.g. [...type])")
		}
		ellipsis, ok := lit.Elts[0].(*ast.Ellipsis)
		if !ok {
			return nil, fmt.Errorf("only arrays with single ellipsis are supported (e.g. [...type])")
		}
		arrayElement, err := registry.createNode(ellipsis.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to create array element: %w", err)
		}
		node := &Node{CanBeArray: true, ArrayElement: arrayElement}
		return node, nil
	}
	return nil, nil
}

func LoadRegistryFromFile(path string) (*Registry, error) {
	instances := load.Instances([]string{path}, &load.Config{})
	decl, err := extractSingleDecl(instances)
	if err != nil {
		return nil, fmt.Errorf("config file ('%v') must have single top level declaration", path)
	}
	registry, err := LoadRegistry(decl)
	if err != nil {
		return nil, fmt.Errorf("config file ('%v') is invalid: %w", path, err)
	}
	return registry, nil
}

func LoadRegistry(decl ast.Decl) (*Registry, error) {
	field, ok := decl.(*ast.Field)
	if !ok {
		return nil, fmt.Errorf("invalid config structure: there must be single top level schema field")
	}
	schemaName, ok := extractName(field.Label)
	if !ok {
		return nil, fmt.Errorf("invalid config structure: top level field must be present")
	}
	if !ok || !strings.HasPrefix(schemaName, "#") {
		return nil, fmt.Errorf("invalid config structure: top level field must start with '#' symbol")
	}
	registry := NewRegistry()
	registry.Root = schemaName

	rootNode, err := registry.createNode(field.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}
	registry.AddSchema(schemaName, rootNode)
	return registry, nil
}
