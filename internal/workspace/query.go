package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Status lists today's active tasks across services, plus any stale temp.md entries.
func (w *Workspace) Status() (string, error) {
	svcs, err := w.Services()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	active := 0
	for _, svc := range svcs {
		t, err := w.readTemp(svc)
		if err != nil {
			return "", err
		}
		for _, d := range t.days {
			for _, blk := range blocks(d.lines) {
				lines := d.lines[blk[0]:blk[1]]
				next := ""
				for _, l := range lines {
					if s := strings.TrimSpace(l); strings.HasPrefix(s, "- Next:") {
						next = strings.TrimSpace(strings.TrimPrefix(s, "- Next:"))
					}
				}
				stale := ""
				if d.date < w.today() {
					stale = " (stale: from " + d.date + ", not ended)"
				}
				fmt.Fprintf(&b, "- %s · %s%s", svc, strings.TrimPrefix(lines[0], "### "), stale)
				if next != "" {
					fmt.Fprintf(&b, " · next: %s", next)
				}
				b.WriteString("\n")
				active++
			}
		}
	}
	if active == 0 {
		return fmt.Sprintf("No active tasks today (%s) across %d services.", w.today(), len(svcs)), nil
	}
	return fmt.Sprintf("Active tasks (%s):\n%s", w.today(), b.String()), nil
}

// SearchLogs finds log lines containing query (case-insensitive), newest month first.
func (w *Workspace) SearchLogs(query string, services []string, limit int) (string, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return "", fmt.Errorf("query is required")
	}
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	if len(services) == 0 {
		var err error
		if services, err = w.Services(); err != nil {
			return "", err
		}
	}
	var hits []string
	for _, svc := range services {
		if err := w.requireService(svc); err != nil {
			return "", err
		}
		entries, _ := os.ReadDir(w.ctxPath(svc, "logs"))
		var names []string
		for _, e := range entries {
			if logNameRE.MatchString(e.Name()) {
				names = append(names, e.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		for _, n := range names {
			data, err := os.ReadFile(w.ctxPath(svc, "logs", n))
			if err != nil {
				continue
			}
			date, task := "", ""
			for _, ln := range strings.Split(string(data), "\n") {
				if m := dayRE.FindStringSubmatch(ln); m != nil {
					date, task = m[1], ""
					continue
				}
				if strings.HasPrefix(ln, "### ") {
					task = strings.TrimPrefix(ln, "### ")
				}
				if date == "" || !strings.Contains(strings.ToLower(ln), q) {
					continue
				}
				hits = append(hits, fmt.Sprintf("- %s · %s · %s\n  %s", svc, date, task, strings.TrimSpace(ln)))
				if len(hits) >= limit {
					return fmt.Sprintf("%d matches (limit reached):\n%s", len(hits), strings.Join(hits, "\n")), nil
				}
			}
		}
	}
	if len(hits) == 0 {
		return fmt.Sprintf("No log lines mention %q.", query), nil
	}
	return fmt.Sprintf("%d matches:\n%s", len(hits), strings.Join(hits, "\n")), nil
}

// Impact finds where a contract, event, table, endpoint or term appears: rows of the system map
// (with their section) and lines in every service's architecture, database and features docs.
func (w *Workspace) Impact(term string) (string, error) {
	q := strings.ToLower(strings.TrimSpace(term))
	if q == "" {
		return "", fmt.Errorf("term is required: an event subject, RPC, endpoint, table or field")
	}
	var b strings.Builder
	n := 0

	if data, err := os.ReadFile(w.ctxPath("global-architecture.md")); err == nil {
		section, header := "", ""
		var rows []string
		for _, ln := range strings.Split(string(data), "\n") {
			s := strings.TrimSpace(ln)
			switch {
			case strings.HasPrefix(s, "## "), strings.HasPrefix(s, "**") && strings.HasSuffix(s, "**"):
				section, header = strings.Trim(strings.TrimPrefix(s, "## "), "*"), ""
			case strings.HasPrefix(s, "|") && strings.Contains(s, "---"):
				// table separator
			case strings.HasPrefix(s, "|") && header == "":
				header = s
			case strings.Contains(strings.ToLower(s), q):
				label := section
				if header != "" {
					label += " — " + header
				}
				rows = append(rows, fmt.Sprintf("- [%s]\n  %s", label, s))
				n++
			}
			if s == "" {
				header = ""
			}
		}
		if len(rows) > 0 {
			b.WriteString("System map (context/global-architecture.md):\n" + strings.Join(rows, "\n") + "\n\n")
		}
	}

	if entries, err := os.ReadDir(w.ctxPath(SharedDir)); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			data, err := os.ReadFile(w.ctxPath(SharedDir, e.Name()))
			if err != nil {
				continue
			}
			var lines []string
			for i, ln := range strings.Split(string(data), "\n") {
				if strings.Contains(strings.ToLower(ln), q) {
					lines = append(lines, fmt.Sprintf("  %s:%d  %s", e.Name(), i+1, strings.TrimSpace(ln)))
					n++
				}
			}
			if len(lines) > 0 {
				fmt.Fprintf(&b, "shared database %s (context/%s/%s):\n%s\n\n", strings.TrimSuffix(e.Name(), ".md"), SharedDir, e.Name(), strings.Join(lines, "\n"))
			}
		}
	}

	svcs, _ := w.Services()
	for _, svc := range svcs {
		var lines []string
		for _, f := range []string{"architecture.md", "database.md", "features.md"} {
			data, err := os.ReadFile(w.ctxPath(svc, f))
			if err != nil {
				continue
			}
			for i, ln := range strings.Split(string(data), "\n") {
				if strings.Contains(strings.ToLower(ln), q) {
					lines = append(lines, fmt.Sprintf("  %s:%d  %s", f, i+1, strings.TrimSpace(ln)))
					n++
				}
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(&b, "%s:\n%s\n\n", svc, strings.Join(lines, "\n"))
		}
	}
	if n == 0 {
		return fmt.Sprintf("%q does not appear in the system map or any service doc. If it should, the docs may be out of date.", term), nil
	}
	return fmt.Sprintf("%d mentions of %q. Every service listed is a candidate to check before changing it.\n\n%s", n, term, strings.TrimRight(b.String(), "\n")), nil
}

// ListServices reports registered services and repos that still need init_workspace.
func (w *Workspace) ListServices() (string, error) {
	repos, err := w.Repos()
	if err != nil {
		return "", err
	}
	svcs, svcErr := w.Services()
	reg := map[string]bool{}
	for _, s := range svcs {
		reg[s] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Workspace: %s\n", w.Root)
	if svcErr != nil {
		fmt.Fprintf(&b, "%v\n", svcErr)
	}
	for _, s := range svcs {
		log := w.NewestLog(s)
		if log == "" {
			log = "no logs"
		}
		_, repoErr := os.Stat(filepath.Join(w.Root, s))
		repo := "repo present"
		if repoErr != nil {
			repo = "repo missing"
		}
		fmt.Fprintf(&b, "- %s (registered · %s · newest log %s)\n", s, repo, log)
	}
	var missing []string
	for _, r := range repos {
		if !reg[r] {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(&b, "Repos without context (run init_workspace): %s\n", strings.Join(missing, ", "))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
