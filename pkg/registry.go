package pkg

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/token"
)

func isRootSchema(field *ast.Field) (bool, map[string]struct{}) {
	for _, attr := range field.Attrs {
		key, value := attr.Split()
		if key == "root" {
			flags := strings.Split(value, ",")
			flagsMap := make(map[string]struct{})
			for _, flag := range flags {
				flagsMap[strings.TrimSpace(flag)] = struct{}{}
			}
			return true, flagsMap
		}
	}
	return false, nil
}

func extractDecls(instances []*build.Instance) ([]ast.Decl, error) {
	if len(instances) != 1 {
		return nil, fmt.Errorf("found %v instances, expected single", len(instances))
	}
	instance := instances[0]
	if len(instance.Files) != 1 {
		return nil, fmt.Errorf("found %v files, expected single", len(instance.Files))
	}
	return instance.Files[0].Decls, nil
}

func extractName(label ast.Label) (string, bool) {
	name, ok := label.(*ast.Ident)
	if !ok {
		return "", false
	}
	return name.Name, true
}

type Registry struct {
	Root            string
	SchemaNode      map[string]*Node
	SchemaName      map[*Node]string
	SchemasOrder    []string
	UndefinedIsNull bool
	NullIsUndefined bool
}

func NewRegistry() *Registry {
	return &Registry{SchemaNode: make(map[string]*Node), SchemaName: make(map[*Node]string)}
}

func (registry *Registry) AddSchema(name string, node *Node, isRoot bool) {
	if isRoot {
		registry.Root = name
	}
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

func isDiscardField(field *ast.Field) bool {
	for _, attr := range field.Attrs {
		key, _ := attr.Split()
		if key == "discard" {
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
					registry.AddSchema(fieldName, schemaNode, false)
					err := registry.fillNode(field.Value, &schemaNode)
					if err != nil {
						return fmt.Errorf("failed to fill schema %v: %w", fieldName, err)
					}
				} else {
					fieldNode := &Node{}
					if isDiscriminativeField(field) {
						(**node).DiscriminationField = fieldName
						(**node).DiscriminationValues = make(map[string]*Node)
					} else {
						err := registry.fillNode(field.Value, &fieldNode)
						if err != nil {
							return fmt.Errorf("failed to fill field %v: %w", fieldName, err)
						}
						if (**node).ObjectFields == nil {
							(**node).ObjectFields = make(map[string]*Node)
						}
						fieldNode.Discard = isDiscardField(field)
						if field.Constraint == token.OPTION {
							fieldNode.CanBeUndefined = true
						}
						(**node).ObjectFields[fieldName] = fieldNode
					}
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
	decl, err := extractDecls(instances)
	if err != nil {
		return nil, fmt.Errorf("config file ('%v') must have single top level declaration: %w", path, err)
	}
	registry, err := LoadRegistry(decl)
	if err != nil {
		return nil, fmt.Errorf("config file ('%v') is invalid: %w", path, err)
	}
	return registry, nil
}

func LoadRegistry(decls []ast.Decl) (*Registry, error) {
	registry := NewRegistry()
	for _, decl := range decls {
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
		rootNode := &Node{}
		isRoot, rootFlags := isRootSchema(field)
		if _, ok := rootFlags["undefined-is-null"]; ok {
			registry.UndefinedIsNull = true
		}
		if _, ok := rootFlags["null-is-undefined"]; ok {
			registry.NullIsUndefined = true
		}
		registry.AddSchema(schemaName, rootNode, isRoot)
		err := registry.fillNode(field.Value, &rootNode)
		if err != nil {
			return nil, fmt.Errorf("failed to create schema: %w", err)
		}
	}
	return registry, nil
}
