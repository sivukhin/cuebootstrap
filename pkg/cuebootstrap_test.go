package pkg

import (
	"testing"

	"cuelang.org/go/cue/load"

	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	instances := load.Instances([]string{"../config.cue"}, &load.Config{})
	decl, err := extractDecls(instances)
	require.Nil(t, err)
	registry, err := LoadRegistry(decl)
	require.Nil(t, err)
	t.Log(registry)
}
