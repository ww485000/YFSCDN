// EdgeCDN edge — the data-plane node module.
//
// Usage:
//
//	edge -config edge/config.json
//	edge -config edge/config.json -log edge.log   # also append log lines to a file
//
// Config JSON (all fields optional except node_token):
//
//	{
//	  "core_url": "http://127.0.0.1:8080",
//	  "node_token": "<token from the node record in core>",
//	  "driver": "gopxy",            // or "nginx" (needs nginx_bin)
//	  "listen_addr": ":80",
//	  "tls_addr": ":443",
//	  "data_dir": "edge/data"
//	}
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"edgecdn/edge/internal/cfg"
	"edgecdn/edge/internal/corex"
	"edgecdn/edge/internal/driver"
	"edgecdn/edge/internal/driver/gopxy"
	"edgecdn/edge/internal/driver/nginx"
	"edgecdn/edge/internal/stats"
)

// newDriver builds the data-plane driver named by cfg.Driver.
func newDriver(c *cfg.Config, st *stats.Stats) (driver.Driver, error) {
	switch c.Driver {
	case "nginx":
		return nginx.New(c.NginxBin, c.DataDir, c.ListenAddr, c.TLSAddr)
	case "gopxy", "":
		return gopxy.New(c.ListenAddr, c.TLSAddr, c.DataDir, st)
	default:
		return nil, fmtError("unknown driver " + c.Driver + " (use gopxy or nginx)")
	}
}

func main() {
	configPath := flag.String("config", "edge/config.json", "edge config JSON path")
	logPath := flag.String("log", "", "append log output to this file as well (optional)")
	flag.Parse()
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatalf("open log file %s: %v", *logPath, err)
		}
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}

	c, err := cfg.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	st := stats.New()
	drv, err := newDriver(c, st)
	if err != nil {
		log.Fatal(err)
	}
	if err := drv.Start(); err != nil {
		log.Fatalf("start driver %s: %v", drv.Name(), err)
	}
	defer func() { _ = drv.Stop() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := corex.New(c, drv, st)
	if err := client.Run(ctx); err != nil {
		log.Fatalf("corex: %v", err)
	}
	log.Printf("[edge] stopped")
}

// fmtError is a tiny error wrapper to avoid an errors import in main.
func fmtError(msg string) error {
	return &simpleError{msg: msg}
}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }
