package main

type UploadCmd struct {
	Files     []string `arg:"" name:"file" help:"Files or directories to upload to the Snake Can." min:"1"`
	Recursive bool     `short:"r" help:"Recursively process directories."`
}

func (u *UploadCmd) Validate() error {
	return validatePaths(u.Files, u.Recursive)
}

func (u *UploadCmd) Run(cfg *Config) error {
	return uploadPaths(cfg, u.Files, u.Recursive, false)
}
