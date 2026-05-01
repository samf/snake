package main

import (
	"github.com/alecthomas/kong"
)

var version = "dev"

var cli struct {
	Login  LoginCmd  `cmd:"" help:"Authenticate with a Snake Can server."`
	Logout LogoutCmd `cmd:"" help:"Sign out and revoke the stored token."`
	Ls     LsCmd     `cmd:"" help:"List can files under a directory path."`
	Rm     RmCmd     `cmd:"" help:"Upload files to the Snake Can."`
	Status StatusCmd `cmd:"" help:"Show authentication and configuration status."`
}

func main() {
	cfg, _ := loadConfig()

	ctx := kong.Parse(&cli,
		kong.Name("snake"),
		kong.Description("The Snake Can CLI."),
	)
	ctx.FatalIfErrorf(ctx.Run(cfg))
}
