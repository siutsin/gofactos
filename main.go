// This file keeps the executable entry point separate from application logic.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/siutsin/gofactos/internal/app"
)

// main delegates process setup and failures to the reusable CLI application.
func main() {
	ctx := context.Background()
	if err := app.NewCommand().Run(ctx, os.Args); err != nil {
		slog.ErrorContext(ctx, "failed to run app", "error", err)
		os.Exit(1)
	}
}
