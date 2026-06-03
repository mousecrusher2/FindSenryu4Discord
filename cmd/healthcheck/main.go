package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultPort      = "9090"
	serverPortSecret = "/run/secrets/findsenryu-server-port"
	requestTimeout   = 3 * time.Second
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: healthcheck [url]")
		return 2
	}

	url := defaultHealthURL()
	if len(args) == 1 {
		url = args[0]
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	if err := check(ctx, url); err != nil {
		fmt.Fprintf(stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	return 0
}

func defaultHealthURL() string {
	port := defaultPort
	if body, err := os.ReadFile(serverPortSecret); err == nil {
		if value := strings.TrimSpace(string(body)); value != "" {
			port = value
		}
	}
	return "http://127.0.0.1:" + port + "/health"
}

func check(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
