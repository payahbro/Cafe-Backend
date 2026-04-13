package main

import (
	"context"
	"os"

	"cafeTelkom/internal/app"
)

func main() {
	app.RunAPI(context.Background(), os.Stdout, os.Stderr)
}

