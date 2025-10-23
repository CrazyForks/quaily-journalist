package main

import (
	"os"

	"quaily-journalist/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
