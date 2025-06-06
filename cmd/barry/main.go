package main

import (
	"log"
	"os"

	barrycli "github.com/go-barry/barry/cli"
	clilib "github.com/urfave/cli/v2"
)

func main() {
	app := &clilib.App{
		Name:  "barry",
		Usage: "A dynamic HTML framework powered by Go",
		Commands: []*clilib.Command{
			barrycli.InitCommand,
			barrycli.DevCommand,
			barrycli.ProdCommand,
			barrycli.CleanCommand,
			barrycli.CheckCommand,
			barrycli.InfoCommand,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
