package main

import (
	"fmt"

	"github.com/alecthomas/kong"
)

var cli struct {
	Hello HelloCmd `cmd:"" help:"Print a greeting."`
	Login LoginCmd `cmd:"" help:"Authenticate with a Snake Can server."`
	Rm    RmCmd    `cmd:"" help:"Upload files to the Snake Can."`
}

type HelloCmd struct{}

func (h *HelloCmd) Run() error {
	fmt.Println("hello, snake fans.")
	return nil
}

func main() {
	cfg, _ := loadConfig()

	ctx := kong.Parse(&cli,
		kong.Name("snake"),
		kong.Description("The Snake Can CLI."),
	)
	ctx.FatalIfErrorf(ctx.Run(cfg))
}
