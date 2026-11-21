package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DriftDocs are the hand-maintained docs that describe a service's code. temp.md and logs
// change with every task, so they don't count as "docs updated".
var DriftDocs = []string{"architecture.md", "database.md", "features.md"}

// DriftGrace ignores commits made shortly after the docs were edited: the usual flow is
// "update docs at the end of a task, then commit", and those commits are the same work.
const DriftGrace = 6 * time.Hour

// Drift says a service's repo has commits newer than its docs, so the docs may be stale.
type Drift struct {
	Service     string
	Commits     int
	DocsUpdated time.Time
	Newest      time.Time
}

func (d Drift) String() string {
	word := "commits"
	if d.Commits == 1 {
		word = "commit"
	}
	return fmt.Sprintf("%s · %d %s since its docs were last updated (%s), newest %s. Check the code before trusting architecture/features/database, and update them.",
		d.Service, d.Commits, word, d.DocsUpdated.Format("2006-01-02"), d.Newest.Format("2006-01-02"))
}

// Drift lists services whose repo has commits after their docs were last edited.
// It only reads: file times under context/ and `git log` in each service repo.
func (w *Workspace) Drift() []Drift {
	svcs, err := w.Services()
	if err != nil {
		return nil
	}
	var out []Drift
	for _, svc := range svcs {
		if d, ok := w.driftFor(svc); ok {
			out = append(out, d)
		}
	}
	return out
}

func (w *Workspace) driftFor(svc string) (Drift, bool) {
	if w.CommitsSince == nil {
		return Drift{}, false
	}
	repo := filepath.Join(w.Root, svc)
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return Drift{}, false // no repo for this service (e.g. a folder inside a monorepo)
	}
	var docs time.Time
	for _, f := range DriftDocs {
		if st, err := os.Stat(w.ctxPath(svc, f)); err == nil && st.ModTime().After(docs) {
			docs = st.ModTime()
		}
	}
	if docs.IsZero() {
		return Drift{}, false
	}
	d := Drift{Service: svc, DocsUpdated: docs}
	for _, c := range w.CommitsSince(repo, docs.Add(DriftGrace)) {
		d.Commits++
		if c.After(d.Newest) {
			d.Newest = c
		}
	}
	return d, d.Commits > 0
}

func driftSection(ds []Drift) string {
	if len(ds) == 0 {
		return ""
	}
	lines := make([]string, len(ds))
	for i, d := range ds {
		lines[i] = "- " + d.String()
	}
	return "\n\nDocs that may be out of date:\n" + strings.Join(lines, "\n")
}

// gitCommitsSince returns commit times on HEAD after since. Any git problem means "none":
// drift is a hint, never a reason to fail.
func gitCommitsSince(dir string, since time.Time) []time.Time {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "log", "--since="+since.Format(time.RFC3339), "--format=%ct", "-n", "500", "HEAD")
	// Never walk up into a parent repo (the workspace root may itself be a git repo).
	cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var times []time.Time
	for _, ln := range strings.Fields(string(out)) {
		if n, err := strconv.ParseInt(ln, 10, 64); err == nil {
			if t := time.Unix(n, 0); t.After(since) {
				times = append(times, t)
			}
		}
	}
	return times
}
