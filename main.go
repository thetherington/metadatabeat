package main

import (
	"os"

	"github.com/thetherington/metadatabeat/cmd"

	_ "github.com/thetherington/metadatabeat/include"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
