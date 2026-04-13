package main

import (
	"context"
	"os"

	"cafeTelkom/internal/app"
)

func StartAPI() {
	app.RunAPI(context.Background(), os.Stdout, os.Stderr)
}

