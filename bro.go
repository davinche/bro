package main

import (
	"log"
	"os"

	commands "github.com/davinche/bro/commands"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "bro"
	app.Usage = "scaffold all the things"
	app.Commands = []cli.Command{
		{
			Name:   "init",
			Action: commands.Init,
		},
		{
			Name:   "create",
			Action: commands.Create,
		},
		{
			Name:   "add",
			Action: commands.Add,
		},
		{
			Name:   "reset",
			Action: commands.Reset,
		},
		{
			Name:   "commit",
			Action: commands.Commit,
		},
		{
			Name:   "status",
			Action: commands.Status,
		},
		{
			Name:   "track",
			Action: commands.Track,
		},
		{
			Name:   "clone",
			Action: commands.Clone,
		},
	}

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "threads,t",
			Value: 16,
			Usage: "Number of threads to run things like tree walking",
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
