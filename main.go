package main

import (
	"github.com/alecthomas/kong"
)

var cli struct {
	Login LoginCmd `cmd:"" help:"Authenticate with a Snake Can server."`
	Ls    LsCmd    `cmd:"" help:"List can files under a directory path."`
	Rm    RmCmd    `cmd:"" help:"Upload files to the Snake Can."`
}

func main() {
	cfg, _ := loadConfig()

	ctx := kong.Parse(&cli,
		kong.Name("snake"),
		kong.Description("The Snake Can CLI."),
	)
	ctx.FatalIfErrorf(ctx.Run(cfg))
}
