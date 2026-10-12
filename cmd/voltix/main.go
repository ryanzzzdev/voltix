package main

import (
	"flag"
	"fmt"
	"log"
)

// version is overridden at release time; keep a readable default for local builds.
var version = "dev"

func main() {
	configPath := flag.String("config", "", "path to the controller configuration file")
	flag.Parse()

	if *configPath != "" {
		log.Printf("voltix %s starting with config %s", version, *configPath)
	} else {
		log.Printf("voltix %s starting without config", version)
	}
	fmt.Println("voltix ready")
}
