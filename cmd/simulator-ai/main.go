package main

import (
	"fmt"
	"os"

	"github.com/corezoid/simulator-ai/internal/mcpserver"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	c, e := publicapi.LoadConfig(os.Getenv)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	client, e := publicapi.New(c)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if e = server.ServeStdio(mcpserver.New(client), server.WithWorkerPoolSize(1), server.WithQueueSize(32)); e != nil {
		fmt.Fprintln(os.Stderr, "Simulator AI stdio stopped")
		os.Exit(1)
	}
}
