package pkg

import (
	"fmt"
	"reflect"
	"sort"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
)

type NodeProps struct {
	CanBeNull      bool
	CanBeUndefined bool
	CanBeObject    bool
	CanBeArray     bool
	CanBeString    bool
	CanBeNumber    bool
	CanBeBool      bool
	ObjectFields   map[string]*Node
	ArrayElement   *Node
	Numbers        []float64
	Strings        []string
	Bools          []bool
}

type Node struct {
	NodeProps
	DiscriminationField  string
	DiscriminationValues map[string]*Node
}

func mapKeys(maps ...any) []string {
	keys := make(map[string]struct{}, 0)
	for _, m := range maps {
		value := reflect.ValueOf(m)
		if value.Type().Kind() == reflect.Map {
			for _, key := range value.MapKeys() {
				specificKey := key
				if specificKey.Kind() == reflect.Interface {
					specificKey = specificKey.Elem()
				}
				if specificKey.Kind() == reflect.String {
					keys[specificKey.String()] = struct{}{}
				}
			}
		}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	return result
}

func LoadInto(node *Node, aValue any) error {
	if aValue == nil {
		node.CanBeNull = true
		return nil
	}
	switch theValue := aValue.(type) {
	case float64:
		node.Numbers = append(node.Numbers, theValue)
		node.CanBeNumber = true
	case int:
		node.Numbers = append(node.Numbers, float64(theValue))
		node.CanBeNumber = true
	case string:
		node.Strings = append(node.Strings, theValue)
		node.CanBeString = true
	case bool:
		node.Bools = append(node.Bools, theValue)
		node.CanBeBool = true
	case map[any]any, map[string]any:
		node.CanBeObject = true
		if reflect.ValueOf(theValue).Len() == 0 {
			break
		}
		fresh := false
		if node.ObjectFields == nil {
			fresh = true
			node.ObjectFields = make(map[string]*Node, 0)
		}
		for _, key := range mapKeys(theValue, node.ObjectFields) {
			field, _ := node.ObjectFields[key]
			if field == nil {
				field = new(Node)
				if !fresh {
					field.CanBeUndefined = true
				}
			}
			var value any
			var valueOk bool
			if mStr, ok := theValue.(map[string]any); ok {
				value, valueOk = mStr[key]
			} else if mAny, ok := theValue.(map[any]any); ok {
				value, valueOk = mAny[key]
			} else {
				return fmt.Errorf("unexpected map type: %T", theValue)
			}
			if valueOk {
				if err := LoadInto(field, value); err != nil {
					return err
				}
			} else {
				field.CanBeUndefined = true
			}
			node.ObjectFields[key] = field
		}
	case []any:
		node.CanBeArray = true
		if len(theValue) == 0 {
			break
		}
		if node.ArrayElement == nil {
			node.ArrayElement = new(Node)
		}
		for _, element := range theValue {
			if err := LoadInto(node.ArrayElement, element); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected value type: %T", aValue)
	}
	return nil
}

func astOptions(options []ast.Expr) (ast.Expr, error) {
	if len(options) > 1 {
		return ast.NewBinExpr(token.OR, options...), nil
	}
	if len(options) == 1 {
		return options[0], nil
	}
	return nil, fmt.Errorf("can't create ast options from zero options")
}

func equals[T comparable](s []T) bool {
	if len(s) == 0 {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return true
}

const (
	nullComplexity = 1 << iota
	stringComplexity
	numberComplexity
	boolComplexity
	arrayComplexity
	objectComplexity
)

func nodeComplexity(node *Node, c map[*Node]int) int {
	if _, ok := c[node]; ok {
		return 0
	}
	if node.CanBeNull {
		c[node] += nullComplexity
	}
	if node.CanBeArray {
		c[node] += arrayComplexity
		if node.ArrayElement != nil {
			c[node] += nodeComplexity(node.ArrayElement, c)
		}
	}
	if node.CanBeObject {
		c[node] += objectComplexity
		for _, value := range node.ObjectFields {
			c[node] += nodeComplexity(value, c)
		}
	}
	if node.CanBeNumber {
		c[node] += numberComplexity
	}
	if node.CanBeString {
		c[node] += stringComplexity
	}
	if node.CanBeBool {
		c[node] += boolComplexity
	}
	return c[node]
}

func TreeComplexity(node *Node) map[*Node]int {
	c := make(map[*Node]int, 0)
	nodeComplexity(node, c)
	return c
}

func Format(registry *Registry, node *Node, complexity map[*Node]int, noDefaults bool) (ast.Expr, error) {
	return format(registry, node, complexity, noDefaults, true)
}

func format(
	registry *Registry,
	node *Node,
	complexity map[*Node]int,
	noDefaults bool,
	isRoot bool,
) (ast.Expr, error) {
	if name, ok := registry.SchemaName[node]; !isRoot && ok {
		return ast.NewIdent(name), nil
	}
	expressions := make([]ast.Expr, 0)
	if node.CanBeArray {
		if node.ArrayElement != nil {
			format, err := format(registry, node.ArrayElement, complexity, noDefaults, false)
			if err != nil {
				return nil, fmt.Errorf("unable to format array element: %w", err)
			}
			expressions = append(expressions, ast.NewList(&ast.UnaryExpr{Op: token.ELLIPSIS, X: format}))
		} else {
			expressions = append(expressions, ast.NewList())
		}
	}
	if node.CanBeObject {
		if node.ObjectFields != nil {
			fields := make([]any, 0)
			keys := mapKeys(node.ObjectFields)
			sort.Slice(keys, func(i, j int) bool {
				c1, c2 := complexity[node.ObjectFields[keys[i]]], complexity[node.ObjectFields[keys[j]]]
				if c1 != c2 {
					return c1 < c2
				}
				return keys[i] < keys[j]
			})
			for _, key := range keys {
				value := node.ObjectFields[key]
				fields = append(fields, ast.NewIdent(key))
				if value.CanBeUndefined {
					fields = append(fields, token.OPTION)
				}
				format, err := format(registry, value, complexity, noDefaults, false)
				if err != nil {
					return nil, fmt.Errorf("unable to format field %v: %w", key, err)
				}
				fields = append(fields, format)
			}
			expressions = append(expressions, ast.NewStruct(fields...))
		} else {
			expressions = append(expressions, ast.NewStruct())
		}
	}
	if node.CanBeNumber {
		numbers := make([]ast.Expr, 0)
		numbers = append(numbers, ast.NewIdent("number"))
		if complexity[node] == 1 && equals(node.Numbers) && !noDefaults {
			numbers = append(numbers, &ast.UnaryExpr{Op: token.MUL, X: ast.NewLit(token.INT, fmt.Sprintf("%v", node.Numbers[0]))})
		}
		options, err := astOptions(numbers)
		if err != nil {
			return nil, fmt.Errorf("unable to create number options: %w", err)
		}
		expressions = append(expressions, options)
	}
	if node.CanBeString {
		strings := make([]ast.Expr, 0)
		strings = append(strings, ast.NewIdent("string"))
		if complexity[node] == 1 && equals(node.Strings) && !noDefaults {
			strings = append(strings, &ast.UnaryExpr{Op: token.MUL, X: ast.NewString(node.Strings[0])})
		}
		options, err := astOptions(strings)
		if err != nil {
			return nil, fmt.Errorf("unable to create string options: %w", err)
		}
		expressions = append(expressions, options)
	}
	if node.CanBeBool {
		bools := make([]ast.Expr, 0)
		bools = append(bools, ast.NewIdent("bool"))
		if complexity[node] == 1 && equals(node.Bools) && !noDefaults {
			tokenType, tokenValue := token.TRUE, "true"
			if !node.Bools[0] {
				tokenType, tokenValue = token.FALSE, "false"
			}
			bools = append(bools, &ast.UnaryExpr{Op: token.MUL, X: ast.NewLit(tokenType, tokenValue)})
		}
		options, err := astOptions(bools)
		if err != nil {
			return nil, fmt.Errorf("unable to create bool options: %w", err)
		}
		expressions = append(expressions, options)
	}
	if node.CanBeNull {
		expressions = append(expressions, ast.NewNull())
	}
	if len(expressions) == 0 {
		return nil, fmt.Errorf("unexpected nodes structure: found empty node %v", node)
	}
	return astOptions(expressions)
}
