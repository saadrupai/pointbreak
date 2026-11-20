// Package workspace reads and writes the Pointbreak context/ folder of a microservice workspace.
// Every write stays inside <root>/context; service repos are only ever read (git branch name).
package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ContextDir is the folder, relative to the workspace root, that holds all Pointbreak memory.
const ContextDir = "context"

// Idle is the body of a temp.md with no active task.
const Idle = "(none active)"

// ServiceFiles are the per-service files, in the order load_context returns them.
var ServiceFiles = []string{"architecture.md", "database.md", "features.md", "temp.md"}

var (
	nameRE    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	logNameRE = regexp.MustCompile(`^\d{4}-\d{2}\.md$`)
)

// Workspace is one microservice workspace: a root directory holding service repos and context/.
type Workspace struct {
	Root      string
	Templates fs.FS                       // rooted at templates/context
	Now       func() time.Time            // injectable for tests
	Branch    func(repoDir string) string // current git branch, "" if unknown
}

// New returns a Workspace using the real clock and git.
func New(root string, templates fs.FS) *Workspace {
	return &Workspace{Root: root, Templates: templates, Now: time.Now, Branch: gitBranch}
}

func gitBranch(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CheckName rejects anything that is not a plain directory name, so paths can't escape context/.
func CheckName(name string) error {
	if !nameRE.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid service name %q: use the repo's directory name (letters, digits, . _ -)", name)
	}
	return nil
}

func (w *Workspace) ctxPath(parts ...string) string {
	return filepath.Join(append([]string{w.Root, ContextDir}, parts...)...)
}

func (w *Workspace) today() string { return w.Now().Format("2006-01-02") }

// Repos lists direct subdirectories of the root that are git repositories.
func (w *Workspace) Repos() ([]string, error) {
	entries, err := os.ReadDir(w.Root)
	if err != nil {
		return nil, err
	}
	var repos []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() || n == ContextDir || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") {
			continue
		}
		if _, err := os.Stat(filepath.Join(w.Root, n, ".git")); err == nil {
			repos = append(repos, n)
		}
	}
	return repos, nil
}

// Services lists registered services: context/<name>/architecture.md exists.
func (w *Workspace) Services() ([]string, error) {
	entries, err := os.ReadDir(w.ctxPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no %s/ folder in %s: run init_workspace first", ContextDir, w.Root)
	}
	if err != nil {
		return nil, err
	}
	var svcs []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() || strings.HasPrefix(n, "_") || CheckName(n) != nil {
			continue
		}
		if _, err := os.Stat(w.ctxPath(n, "architecture.md")); err == nil {
			svcs = append(svcs, n)
		}
	}
	return svcs, nil
}

func (w *Workspace) requireService(svc string) error {
	if err := CheckName(svc); err != nil {
		return err
	}
	if _, err := os.Stat(w.ctxPath(svc)); err != nil {
		return fmt.Errorf("service %q has no context folder: run init_workspace with services [%q]", svc, svc)
	}
	return nil
}

// Init creates context/ from the templates for the given services (all git repos if empty).
// Existing files are never overwritten. Returns the paths it created, relative to the root.
func (w *Workspace) Init(services []string) ([]string, error) {
	if len(services) == 0 {
		repos, err := w.Repos()
		if err != nil {
			return nil, err
		}
		services = repos
	}
	var created []string
	write := func(rel, content string) error {
		full := filepath.Join(w.Root, rel)
		if _, err := os.Stat(full); err == nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return err
		}
		created = append(created, rel)
		return nil
	}
	tpl := func(name string) (string, error) {
		b, err := fs.ReadFile(w.Templates, name)
		return string(b), err
	}
	for _, f := range []string{"README.md", "STANDARDS.md", "global-architecture.md"} {
		c, err := tpl(f)
		if err != nil {
			return created, err
		}
		if err := write(filepath.Join(ContextDir, f), c); err != nil {
			return created, err
		}
	}
	month := w.Now().Format("2006-01")
	for _, svc := range services {
		if err := CheckName(svc); err != nil {
			return created, err
		}
		if st, err := os.Stat(filepath.Join(w.Root, svc)); err != nil || !st.IsDir() {
			return created, fmt.Errorf("no directory %q in %s", svc, w.Root)
		}
		for _, f := range ServiceFiles {
			c, err := tpl("_service/" + f)
			if err != nil {
				return created, err
			}
			if err := write(filepath.Join(ContextDir, svc, f), strings.ReplaceAll(c, "<service>", svc)); err != nil {
				return created, err
			}
		}
		c, err := tpl("_service/logs/YYYY-MM.md")
		if err != nil {
			return created, err
		}
		c = strings.NewReplacer("<service>", svc, "<YYYY-MM>", month).Replace(c)
		if err := write(filepath.Join(ContextDir, svc, "logs", month+".md"), c); err != nil {
			return created, err
		}
	}
	return created, nil
}

// NewestLog returns the lexically last logs/YYYY-MM.md for a service, or "" if none.
func (w *Workspace) NewestLog(svc string) string {
	entries, err := os.ReadDir(w.ctxPath(svc, "logs"))
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && logNameRE.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[len(names)-1]
}

// LoadOptions selects what LoadContext returns.
type LoadOptions struct {
	Services      []string
	Files         []string // subset of ServiceFiles plus "log"; empty = all
	IncludeGlobal *bool    // nil = automatic (two or more services)
	SkipRules     bool     // true when context/README.md is already imported via CLAUDE.md
	Standards     bool     // add context/STANDARDS.md (planning and building code)
}

// LoadContext concatenates the requested context files in a fixed, stable order:
// rules, global map, then each service (architecture, database, features, temp, newest log).
func (w *Workspace) LoadContext(o LoadOptions) (string, error) {
	if len(o.Services) == 0 {
		return "", errors.New("services is required: pass one or more service names")
	}
	var uniq []string
	seen := map[string]bool{}
	for _, s := range o.Services {
		if !seen[s] {
			seen[s] = true
			uniq = append(uniq, s)
		}
	}
	o.Services = uniq
	want := map[string]bool{}
	for _, f := range o.Files {
		want[f] = true
	}
	all := len(want) == 0
	for _, svc := range o.Services {
		if err := w.requireService(svc); err != nil {
			return "", err
		}
	}
	var b strings.Builder
	add := func(rel string) {
		data, err := os.ReadFile(filepath.Join(w.Root, rel))
		fmt.Fprintf(&b, "<!-- %s -->\n", filepath.ToSlash(rel))
		if err != nil {
			b.WriteString("(missing)\n\n")
			return
		}
		b.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if !o.SkipRules {
		add(filepath.Join(ContextDir, "README.md"))
	}
	if o.Standards {
		add(filepath.Join(ContextDir, "STANDARDS.md"))
	}
	global := len(o.Services) > 1
	if o.IncludeGlobal != nil {
		global = *o.IncludeGlobal
	}
	if global {
		add(filepath.Join(ContextDir, "global-architecture.md"))
	}
	sharedSeen := map[string]bool{}
	for _, svc := range o.Services {
		for _, f := range ServiceFiles {
			if all || want[f] {
				add(filepath.Join(ContextDir, svc, f))
				if f == "database.md" {
					for _, db := range w.sharedDocsFor(svc) {
						if !sharedSeen[db] {
							sharedSeen[db] = true
							add(filepath.Join(ContextDir, SharedDir, db+".md"))
						}
					}
				}
			}
		}
		if all || want["log"] {
			if log := w.NewestLog(svc); log != "" {
				add(filepath.Join(ContextDir, svc, "logs", log))
			} else {
				fmt.Fprintf(&b, "<!-- %s/%s/logs -->\n(no logs yet)\n\n", ContextDir, svc)
			}
		}
	}
	return b.String(), nil
}
