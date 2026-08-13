package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"chpp/modbus-api-middleware/internal/updatebridge"
)

var version = "0.2.6-bootstrap"

func main() {
	dbPath := flag.String("db", "middleware.db", "SQLite path")
	flag.String("gateway-id", "", "compatibility flag; read from SQLite")
	flag.String("listen", "", "compatibility flag; bridge does not open a web listener")
	flag.Int("cleanup-retention-days", 0, "compatibility flag; bridge does not run cleanup")
	flag.Bool("require-license", false, "compatibility flag; bridge authenticates with the stored API key")
	flag.String("license-file", "", "compatibility flag; bridge does not read the license file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("chpp-middleware update bridge %s\n", version)
		return
	}
	if service, err := runMaybeService(*dbPath); err != nil {
		log.Fatal(err)
	} else if service {
		return
	}
	if err := (&updatebridge.Bridge{DBPath: *dbPath, Version: version}).Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
