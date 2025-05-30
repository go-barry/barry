package cli

import (
	"github.com/callumeddisford/barry"

	"github.com/urfave/cli/v2"
)

var DevCommand = &cli.Command{
	Name:  "dev",
	Usage: "Start Barry in dev mode (no caching, live reload)",
	Action: func(c *cli.Context) error {
		cfg := barry.RuntimeConfig{
			Env:         "dev",
			EnableCache: false,
			Port:        8080,
		}
		barry.Start(cfg)
		return nil
	},
}

var ProdCommand = &cli.Command{
	Name:  "prod",
	Usage: "Start Barry in production mode (caching on by default)",
	Action: func(c *cli.Context) error {
		cfg := barry.RuntimeConfig{
			Env:         "prod",
			EnableCache: true,
			Port:        8080,
		}
		barry.Start(cfg)
		return nil
	},
}
