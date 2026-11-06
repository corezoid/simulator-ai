package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/corezoid/simulator-ai/internal/mcpserver"
	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/corezoid/simulator-ai/internal/version"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		revision := "unknown"
		dirty := false
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					revision = setting.Value
				}
				if setting.Key == "vcs.modified" && setting.Value == "true" {
					dirty = true
				}
			}
		}
		if dirty {
			revision += "-dirty"
		}
		fmt.Printf("simulator-ai %s (%s)\n", version.Current, revision)
		return
	}
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: simulator-ai [--version]")
		os.Exit(2)
	}
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
