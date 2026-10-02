// Package cli is the command-line front end: it starts the MCP server, and
// also exposes the same tools to a shell or CI job.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vedansh-5/graphcontext/pkg/daemon"
	"github.com/vedansh-5/graphcontext/pkg/indexer"
	"github.com/vedansh-5/graphcontext/pkg/mcp_server"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

const usage = `graphcontext - code graph for a repository

Usage:
  graphcontext                       start the MCP server on stdio
  graphcontext serve                 same as above
  graphcontext tools                 list the tools and their arguments
  graphcontext run <tool> [-C dir] [key=value ...]
                                     run one tool and print its JSON answer
  graphcontext index [dir]           index a project and print a summary
  graphcontext watch [dir]           keep a project's index up to date
  graphcontext help                  show this message

Examples:
  graphcontext run search_symbols query=login
  graphcontext run impact_of_change -C ~/code/app symbol=AuthService.login
  graphcontext run diff_impact git_ref=main...HEAD
`

// Main runs the command line and returns the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return serve(stderr)
	}
	switch args[0] {
	case "serve":
		return serve(stderr)
	case "tools":
		return listTools(stdout)
	case "run":
		return runTool(args[1:], stdout, stderr)
	case "index":
		return index(args[1:], stdout, stderr)
	case "watch":
		return watch(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func serve(stderr io.Writer) int {
	if err := mcp_server.StartStdioServer(); err != nil && err != context.Canceled {
		fmt.Fprintf(stderr, "MCP server stopped: %v\n", err)
		return 1
	}
	return 0
}

func listTools(stdout io.Writer) int {
	for _, t := range mcp_server.Tools() {
		fmt.Fprintf(stdout, "%s\n  %s\n", t.Name, t.Description)
		for _, a := range t.Args {
			if a.Name == "project_path" {
				continue
			}
			req := ""
			if a.Required {
				req = ", required"
			}
			fmt.Fprintf(stdout, "    %s (%s%s): %s\n", a.Name, a.Type, req, a.Description)
		}
		fmt.Fprintln(stdout)
	}
	return 0
}

func runTool(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: graphcontext run <tool> [-C dir] [key=value ...]")
		return 2
	}
	name := args[0]
	dir := "."
	toolArgs := map[string]string{}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-C" {
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "-C needs a directory")
				return 2
			}
			i++
			dir = args[i]
			continue
		}
		key, value, ok := strings.Cut(arg, "=")
		if !ok || key == "" {
			fmt.Fprintf(stderr, "expected key=value, got %q\n", arg)
			return 2
		}
		toolArgs[key] = value
	}
	if _, set := toolArgs["project_path"]; !set {
		abs, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(stderr, "resolve %s: %v\n", dir, err)
			return 1
		}
		toolArgs["project_path"] = abs
	}

	mcp_server.SetLive(false)
	defer mcp_server.Shutdown()

	text, toolErr, err := mcp_server.CallTool(context.Background(), name, toolArgs)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if toolErr {
		fmt.Fprintln(stderr, text)
		return 1
	}
	// Tools answer in compact JSON to save an agent's context. A person at a
	// terminal reads it better indented.
	var pretty bytes.Buffer
	if json.Indent(&pretty, []byte(text), "", "  ") == nil {
		text = pretty.String()
	}
	fmt.Fprintln(stdout, text)
	return 0
}

func projectDir(args []string) (string, error) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}

func index(args []string, stdout, stderr io.Writer) int {
	dir, err := projectDir(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	st, err := store.OpenForRepo(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer st.Close()

	start := time.Now()
	changed, err := indexer.EnsureFresh(dir, st)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	files, err := st.FileRecords()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	nodes, edges, err := st.Counts()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	state := "already up to date"
	if changed {
		state = "indexed"
	}
	fmt.Fprintf(stdout, "%s: %s in %s\n", dir, state, time.Since(start).Round(time.Millisecond))
	fmt.Fprintf(stdout, "%d files, %d symbols, %d edges\n", len(files), nodes, edges)
	fmt.Fprintf(stdout, "cache: %s\n", st.Path())
	return 0
}

func watch(args []string, stdout, stderr io.Writer) int {
	dir, err := projectDir(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	d, err := daemon.New(daemon.Config{RepoRoot: dir})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer d.Stop()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	events := d.SubscribeSync()
	if err := d.Start(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	status := d.Status()
	fmt.Fprintf(stdout, "watching %s: %d symbols, %d edges (ctrl-c to stop)\n",
		d.RepoRoot(), status.NodeCount, status.EdgeCount)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(stdout, "stopped")
			return 0
		case evt, ok := <-events:
			if !ok {
				return 0
			}
			if evt.Error != nil {
				fmt.Fprintf(stderr, "sync failed: %v\n", evt.Error)
				continue
			}
			if !evt.Changed || evt.Batch == nil {
				continue
			}
			fmt.Fprintf(stdout, "%s  %d file(s) changed, now %d symbols, %d edges (%s)\n",
				evt.Timestamp.Local().Format("15:04:05"), len(evt.Batch.Changes),
				evt.NodeCount, evt.EdgeCount, evt.Duration.Round(time.Millisecond))
		}
	}
}
