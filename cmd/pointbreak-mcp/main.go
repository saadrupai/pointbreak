// Command pointbreak-mcp serves the Pointbreak tools over stdio for any MCP client.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/saadrupai/pointbreak"
	"github.com/saadrupai/pointbreak/internal/mcp"
	"github.com/saadrupai/pointbreak/internal/tools"
	"github.com/saadrupai/pointbreak/internal/workspace"
)

var version = "0.4.0"

const instructions = "Pointbreak keeps per-service memory for a microservice workspace in context/. " +
	"Start work with load_context for the services involved, then start_task in each. " +
	"Checkpoint with save_progress, finish with end_task. Use impact before changing an API, event, queue or table."

func main() {
	wsFlag := flag.String("workspace", "", "workspace root (default: $POINTBREAK_WORKSPACE, else the nearest parent with context/README.md, else the current directory)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("pointbreak-mcp", version)
		return
	}

	root := *wsFlag
	if root == "" {
		root = os.Getenv("POINTBREAK_WORKSPACE")
	}
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fail(err)
		}
		root = findWorkspace(cwd)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		fail(err)
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		fail(fmt.Errorf("workspace %s is not a directory", root))
	}

	tpl, err := fs.Sub(pointbreak.Templates, "templates/context")
	if err != nil {
		fail(err)
	}
	ws := workspace.New(root, tpl)
	srv := mcp.NewServer("pointbreak", version, instructions, tools.All(ws))
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "pointbreak-mcp:", err)
	os.Exit(1)
}

// findWorkspace walks up from dir to the nearest directory holding context/README.md,
// so the server works when an agent is opened inside a service repo. Falls back to dir.
func findWorkspace(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, workspace.ContextDir, "README.md")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}
