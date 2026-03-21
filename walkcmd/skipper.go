package walkcmd

type skipper map[string]bool

var cmdSkipper = skipper{
	"help":       true,
	"h":          true,
	"version":    true,
	"completion": true,
}

func (c skipper) Is(cmd string) bool {
	return c[cmd]
}
