package main

import (
	"fmt"

	"github.com/alecthomas/kong"
)

var cli struct {
	Hello HelloCmd `cmd:"" help:"Print a greeting."`
}

type HelloCmd struct{}

func (h *HelloCmd) Run() error {
	fmt.Println("hello, snake fans.")
	return nil
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("snake"),
		kong.Description("The Snake Can CLI."),
	)
	ctx.FatalIfErrorf(ctx.Run())
}
