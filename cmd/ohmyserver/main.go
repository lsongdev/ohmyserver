package main

import (
	"log"
	"os"

	"github.com/lsongdev/ohmyserver/internal/host"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "update" {
		if err := runUpdateCommand(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
