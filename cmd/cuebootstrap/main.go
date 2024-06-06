package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue/ast"
	"gopkg.in/yaml.v2"

	"github.com/sivukhin/cuebootstrap/pkg"

	"cuelang.org/go/cue/format"
)

func main() {
	input := flag.String("inputs", "", "glob pattern of input json files")
	skeleton := flag.String("skeleton", "", "path to skeleton file")
	flag.Parse()

	files, err := filepath.Glob(*input)
	if *input == "" {
		log.Fatalf("empty glob pattern provided")
	}
	if err != nil {
		log.Fatalf("unable to execute glob pattern %v: %v", *input, err)
	}

	var registry *pkg.Registry
	if *skeleton != "" {
		registry, err = pkg.LoadRegistryFromFile(*skeleton)
		if err != nil {
			log.Fatalf("failed to load registry: %v", err)
		}
	} else {
		registry = pkg.NewRegistry()
		registry.Root = "#root"
		
		registry.AddSchema("#root", &pkg.Node{})
	}

	root := registry.SchemaNode[registry.Root]
	for _, file := range files {
		bytes, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		var data any
		log.Printf("processing file %v", file)
		if strings.HasSuffix(file, ".json") {
			err = json.Unmarshal(bytes, &data)
		} else if strings.HasSuffix(file, ".yaml") || strings.HasSuffix(file, ".yml") {
			err = yaml.Unmarshal(bytes, &data)
		} else {
			err = fmt.Errorf("unexpected extension of file %v", file)
		}
		if err != nil {
			panic(err)
		}
		err = pkg.LoadInto(root, data)
		if err != nil {
			log.Fatalf("unexpected error for file %v: %v", file, err)
		}
	}
	declarations := make([]ast.Node, 0)
	for _, schemaName := range registry.SchemasOrder {
		schemaNode := registry.SchemaNode[schemaName]
		declaration, err := pkg.Format(registry, schemaNode, pkg.TreeComplexity(schemaNode))
		if err != nil {
			log.Fatalf("unable to format schema: %v", err)
		}
		declarations = append(declarations, &ast.Field{Label: ast.NewIdent(schemaName), Value: declaration})
	}
	decls := make([][]byte, 0)
	for i, decl := range declarations {
		serialized, err := format.Node(decl, format.Simplify())
		if err != nil {
			log.Fatalf("unable to serialize declaration %v: %v", registry.SchemasOrder[i], err)
		}
		decls = append(decls, serialized)
	}
	fmt.Printf("%v\n", string(bytes.Join(decls, []byte("\n"))))
}
