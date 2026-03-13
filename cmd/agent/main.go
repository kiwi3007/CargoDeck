// Playerr Agent — lightweight client that runs on a remote device (e.g. Steam Deck).
// It connects to the Playerr server via SSE, receives install jobs, executes them,
// and reports progress back.
//
// Usage:
//
//	cargodeck-agent --server http://unraid:5002 --token <token> [--name steam-deck]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kiwi3007/cargodeck/cmd/agent/internal/agentclient"
)

var version = "dev"

func main() {
	server := flag.String("server", "", "Playerr server URL (e.g. http://192.168.1.10:5002)")
	token := flag.String("token", "", "Agent auth token (from Settings → Agents)")
	name := flag.String("name", "", "Agent display name (default: hostname)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	testConn := flag.Bool("test-connection", false, "Test server connectivity and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("cargodeck-agent", version)
		os.Exit(0)
	}

	if *testConn {
		if *server == "" || *token == "" {
			os.Exit(1)
		}
		c, err := agentclient.New(agentclient.Config{ServerURL: *server, Token: *token, Name: "test"})
		if err != nil {
			os.Exit(1)
		}
		if err := c.TestConnection(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if *server == "" {
		log.Fatal("[Agent] --server is required (e.g. --server http://192.168.1.10:5002)")
	}
	if *token == "" {
		log.Fatal("[Agent] --token is required (find it in Playerr Settings → Agents)")
	}

	agentName := *name
	if agentName == "" {
		h, err := os.Hostname()
		if err != nil {
			h = "unknown"
		}
		agentName = h
	}

	client, err := agentclient.New(agentclient.Config{
		ServerURL: *server,
		Token:     *token,
		Name:      agentName,
		Version:   version,
	})
	if err != nil {
		log.Fatalf("[Agent] Init failed: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("[Agent] Starting Playerr Agent %s — connecting to %s", version, *server)
	go client.Run()

	<-quit
	log.Println("[Agent] Shutting down.")
	client.Stop()
}
