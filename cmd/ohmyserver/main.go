package main

import (
	"log"

	"github.com/lsongdev/ohmyserver/internal/host"
)

func main() {
	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
