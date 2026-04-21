package main

import (
	"log"

	"github.com/danterobles/rss-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
