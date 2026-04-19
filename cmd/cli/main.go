package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "stash",
		Usage: "Stash - Memory layer for AI applications",
		Commands: []*cli.Command{
			{
				Name:   "env",
				Usage:  "Show environment variables and configuration",
				Action: EnvCmd,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "file",
						Aliases: []string{"f"},
						Value:   ".env",
						Usage:   "Config file to load",
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}