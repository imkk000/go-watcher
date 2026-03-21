package walkcmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

func TestWalkCmd(t *testing.T) {
	want := CmdInfo{
		Name:  "root",
		Usage: "Command Usage",
		Commands: []CmdInfo{{
			Name: "encode",
			Commands: []CmdInfo{{
				Name: "json",
				Flags: []CmdFlag{
					{Type: "string", Name: "string_val"},
					{Type: "bool", Name: "bool_val"},
					{Type: "float32", Name: "float32_val"},
					{Type: "float64", Name: "float64_val"},
					{Type: "[]float64", Name: "float64_slices_val"},
					{Type: "uint64", Name: "uint64_val"},
					{Type: "uint32", Name: "uint32_val", Usage: "set to zero"},
				},
			}},
		}},
	}

	rootCmd := &cli.Command{
		Name:  "root",
		Usage: "Command Usage",
		Commands: []*cli.Command{
			{Name: "help"},
			{Name: "completion"},
			{
				Name: "encode",
				Commands: []*cli.Command{{
					Name: "json",
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "string_val"},
						&cli.BoolFlag{Name: "bool_val"},
						&cli.Float32Flag{Name: "float32_val"},
						&cli.Float64Flag{Name: "float64_val"},
						&cli.Float64SliceFlag{Name: "float64_slices_val"},
						&cli.Uint64Flag{Name: "uint64_val"},
						&cli.Uint32Flag{Name: "uint32_val", Usage: "set to zero"},
					},
				}},
			},
		},
	}
	info := Walk(rootCmd)

	assert.Equal(t, want, info)
}
