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

func isDiscriminativeField(field *ast.Field) bool {
	for _, attr := range field.Attrs {
		key, _ := attr.Split()
		if key == "discriminative" {
			return true
		}
	}
	return false
}

func (registry *Registry) fillNode(root ast.Decl, node **Node) error {
	switch lit := root.(type) {
	case *ast.Ident:
		if lit.Name == "_" {
			return nil
		}
		if strings.HasPrefix(lit.Name, "#") {
			schema, ok := registry.SchemaNode[lit.Name]
			if !ok {
				return fmt.Errorf("unknown schema referenced: '%v'", lit.Name)
			}
			*node = schema
			return nil
		}
		if lit.Name == "string" {
			(**node).CanBeString = true
			return nil
		}
		if lit.Name == "number" {
			(**node).CanBeNumber = true
			return nil
		}
	case *ast.StructLit:
		(**node).CanBeObject = true
		(**node).ObjectFields = make(map[string]*Node)

		for _, element := range lit.Elts {
			if field, ok := element.(*ast.Field); ok {
				fieldName, ok := extractName(field.Label)
				if !ok {
					return fmt.Errorf("failed to get field label: %v", field.Label)
				}
				if strings.HasPrefix(fieldName, "#") {
					if ident, ok := field.Value.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "#") {
						return fmt.Errorf("direct references between schemas are forbidden (e.g. #a: #b)")
					}
					schemaNode := &Node{}
					registry.AddSchema(fieldName, schemaNode)
					err := registry.fillNode(field.Value, &schemaNode)
					if err != nil {
						return fmt.Errorf("failed to fill schema %v: %w", fieldName, err)
					}
				} else {
					fieldNode := &Node{}
					if isDiscriminativeField(field) {
						(**node).DiscriminationField = fieldName
						(**node).DiscriminationValues = make(map[string]*Node)
					}
					err := registry.fillNode(field.Value, &fieldNode)
					if err != nil {
						return fmt.Errorf("failed to fill field %v: %w", fieldName, err)
					}
					(**node).ObjectFields[fieldName] = fieldNode
				}
			}
		}
		return nil
	case *ast.ListLit:
		if len(lit.Elts) != 1 {
			return fmt.Errorf("only arrays with single ellipsis are supported (e.g. [...type])")
		}
		ellipsis, ok := lit.Elts[0].(*ast.Ellipsis)
		if !ok {
			return fmt.Errorf("only arrays with single ellipsis are supported (e.g. [...type])")
		}
		arrayElement := &Node{}
		err := registry.fillNode(ellipsis.Type, &arrayElement)
		if err != nil {
			return fmt.Errorf("failed to create array element: %w", err)
		}
		(**node).CanBeArray = true
		(**node).ArrayElement = arrayElement
		return nil
	}
	return nil
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

	rootNode := &Node{}
	registry.AddSchema(schemaName, rootNode)
	err := registry.fillNode(field.Value, &rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}
	return registry, nil
}
