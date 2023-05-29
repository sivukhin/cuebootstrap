package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"encoding/json"
	"gopkg.in/yaml.v2"

	"github.com/sivukhin/cuebootstrap/pkg"

	"cuelang.org/go/cue/format"
)

func main() {
	input := flag.String("inputs", "", "glob pattern of input json files")
	flag.Parse()

	files, err := filepath.Glob(*input)
	if *input == "" {
		log.Fatalf("empty glob pattern provided")
	}
	if err != nil {
		log.Fatalf("unable to execute glob pattern %v: %v", *input, err)
	}

	var root pkg.Node
	for _, file := range files {
		bytes, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		var data any
		if strings.HasSuffix(".json", file) {
			err = json.Unmarshal(bytes, &data)
		} else if strings.HasSuffix(".yaml", file) || strings.HasSuffix(".yml", file) {
			err = yaml.Unmarshal(bytes, &data)
		} else {
			err = fmt.Errorf("unexpected extension of file %v", file)
		}
		if err != nil {
			panic(err)
		}
		err = pkg.LoadInto(&root, data)
		if err != nil {
			log.Fatalf("unexpected error for file %v: %v", file, err)
		}
	}
	node, err := pkg.Format(&root, pkg.TreeComplexity(&root))
	if err != nil {
		log.Fatalf("unable to format node: %v", err)
	}
	serialized, err := format.Node(node)
	if err != nil {
		log.Fatalf("unable to serialize node: %v", err)
	}
	fmt.Printf("%v\n", string(serialized))
}
