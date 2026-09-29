// EdgeCDN core — the main-control module.
//
// Usage:
//
//	core -config core/config.json   # optional; defaults work out of the box
//	core -static web/dist           # serve the built frontend at /
//	core -log core.log              # also append log lines to a file (detached runs)
package main

import (
	"flag"
	"io"
	"log"
	"os"

	"edgecdn/core/internal/app"
)

func main() {
	configPath := flag.String("config", "core/config.json", "core config JSON path (optional)")
	staticDir := flag.String("static", "", "directory with built web frontend to serve at / (optional)")
	logPath := flag.String("log", "", "append log output to this file as well (optional)")
	flag.Parse()
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatalf("open log file %s: %v", *logPath, err)
		}
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	if err := app.Run(*configPath, *staticDir); err != nil {
		log.Fatal(err)
	}
}
