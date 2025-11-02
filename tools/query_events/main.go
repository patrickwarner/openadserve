package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/patrickwarner/openadserve/internal/analytics"
	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/observability"
)

func main() {
	logger, err := observability.InitLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	var id string
	var dsn string
	flag.StringVar(&id, "id", "", "request ID")
	flag.StringVar(&dsn, "dsn", "", "ClickHouse DSN")
	flag.Parse()

	if id == "" {
		fmt.Fprintln(os.Stderr, "id required")
		os.Exit(1)
	}
	if dsn == "" {
		cfg := config.Load()
		dsn = cfg.ClickHouseDSN
	}

	a, err := analytics.InitClickHouse(dsn, nil, observability.NewNoOpRegistry(), 10, 2, 5*time.Minute, 1*time.Minute)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect clickhouse: %v\n", err)
		os.Exit(1)
	}
	defer a.Close()

	events, err := a.GetEventsByRequestID(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query events: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(events); err != nil {
		fmt.Fprintf(os.Stderr, "encode events: %v\n", err)
		os.Exit(1)
	}
}
