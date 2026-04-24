package main

import (
	"context"
	"fmt"

	"github.com/alash3al/stash/internal/bootstrap"
	httphandlers "github.com/alash3al/stash/internal/handlers/http"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/urfave/cli/v3"
)

func serverCmd(ctx context.Context, cmd *cli.Command) error {
	port := cmd.String("port")
	host := cmd.String("host")

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	e := echo.New()
	e.Use(middleware.Recover())
	httphandlers.RegisterRoutes(e, bc)

	fmt.Printf("Starting server on %s:%s\n", host, port)
	return e.Start(host + ":" + port)
}
