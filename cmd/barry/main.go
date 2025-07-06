package main

import (
	"log"
	"os"

	barrycli "github.com/go-barry/barry/cli"
	clilib "github.com/urfave/cli/v2"
)

func newApp() *clilib.App {
	return &clilib.App{
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
}

func runApp(args []string) error {
	return newApp().Run(args)
}

func main() {
	if err := runApp(os.Args); err != nil {
		log.Fatal(err)
	}
}
