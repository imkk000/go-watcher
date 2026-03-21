package walkcmd

import (
	"reflect"

	"github.com/urfave/cli/v3"
)

type CmdInfo struct {
	Name     string    `json:"name"`
	Usage    string    `json:"usage"`
	Commands []CmdInfo `json:"commands,omitempty"`
	Flags    []CmdFlag `json:"flags,omitempty"`
}

type CmdFlag struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Usage string `json:"usage"`
}

func Walk(c *cli.Command) CmdInfo {
	info := CmdInfo{
		Name:  c.Name,
		Usage: c.Usage,
	}

	for _, cmd := range c.Commands {
		if cmdSkipper.Is(cmd.Name) {
			continue
		}
		info.Commands = append(info.Commands, Walk(cmd))
	}

	for _, flag := range c.Flags {
		var flags []CmdFlag
		for _, name := range flag.Names() {
			if cmdSkipper.Is(name) {
				continue
			}
			t := reflect.TypeOf(flag.Get()).String()
			docFlag := flag.(cli.DocGenerationFlag)
			flags = append(flags, CmdFlag{
				Type:  t,
				Name:  name,
				Usage: docFlag.GetUsage(),
			})
		}

		info.Flags = append(info.Flags, flags...)
	}

	return info
}
