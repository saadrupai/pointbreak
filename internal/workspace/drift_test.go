package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeCommits returns a CommitsSince that reports commits only for the given repos.
func fakeCommits(byRepo map[string][]time.Time) func(string, time.Time) []time.Time {
	return func(dir string, since time.Time) []time.Time {
		var out []time.Time
		for _, c := range byRepo[filepath.Base(dir)] {
			if c.After(since) {
				out = append(out, c)
			}
		}
		return out
	}
}

func setDocTime(t *testing.T, ws *Workspace, svc string, at time.Time) {
	t.Helper()
	for _, f := range DriftDocs {
		p := ws.ctxPath(svc, f)
		if _, err := os.Stat(p); err == nil {
			if err := os.Chtimes(p, at, at); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDriftReportsCommitsAfterDocsUpdate(t *testing.T) {
	ws := kitebox(t)
	docs := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	setDocTime(t, ws, "orders-svc", docs)
	setDocTime(t, ws, "payments-svc", docs)
	setDocTime(t, ws, "notifications-svc", docs)
	ws.CommitsSince = fakeCommits(map[string][]time.Time{
		"orders-svc": {
			docs.Add(2 * time.Hour),       // inside the grace window: same piece of work
			docs.Add(3 * 24 * time.Hour),  // real drift
			docs.Add(10 * 24 * time.Hour), // real drift, newest
			docs.Add(-5 * 24 * time.Hour), // before the docs: ignored
		},
	})

	got := ws.Drift()
	if len(got) != 1 || got[0].Service != "orders-svc" || got[0].Commits != 2 {
		t.Fatalf("want orders-svc with 2 commits, got %+v", got)
	}
	if got[0].DocsUpdated.Format("2006-01-02") != "2026-11-02" || got[0].Newest.Format("2006-01-02") != "2026-11-12" {
		t.Errorf("dates: %+v", got[0])
	}
	line := got[0].String()
	for _, want := range []string{"orders-svc", "2 commits", "2026-11-02", "2026-11-12"} {
		if !strings.Contains(line, want) {
			t.Errorf("drift line missing %q: %s", want, line)
		}
	}
}

func TestDriftSkipsServicesWithoutRepoOrDocsOrGit(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if err := os.MkdirAll(filepath.Join(ws.Root, "lib-only"), 0o755); err != nil { // a folder, not a repo
		t.Fatal(err)
	}
	if _, err := ws.Init([]string{"svc", "lib-only"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	ws.CommitsSince = func(dir string, since time.Time) []time.Time {
		calls++
		return []time.Time{since.Add(48 * time.Hour)}
	}
	got := ws.Drift()
	if calls != 1 || len(got) != 1 || got[0].Service != "svc" {
		t.Errorf("only svc has a repo; got calls=%d drift=%+v", calls, got)
	}

	ws.CommitsSince = nil // git unavailable or disabled
	if d := ws.Drift(); len(d) != 0 {
		t.Errorf("no git: no drift, got %+v", d)
	}

	ws.CommitsSince = fakeCommits(map[string][]time.Time{"svc": {time.Now().Add(72 * time.Hour)}})
	for _, f := range DriftDocs {
		_ = os.Remove(ws.ctxPath("svc", f))
	}
	if d := ws.Drift(); len(d) != 0 {
		t.Errorf("no docs to compare: no drift, got %+v", d)
	}
}

func TestDriftShowsInStatusLoadContextAndImpact(t *testing.T) {
	ws := kitebox(t)
	docs := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	setDocTime(t, ws, "payments-svc", docs)
	ws.CommitsSince = fakeCommits(map[string][]time.Time{"payments-svc": {docs.Add(4 * 24 * time.Hour)}})

	st, err := ws.Status()
	if err != nil || !strings.Contains(st, "Docs that may be out of date") || !strings.Contains(st, "payments-svc · 1 commit ") {
		t.Errorf("status should flag drift: %v\n%s", err, st)
	}

	out, err := ws.LoadContext(LoadOptions{Services: []string{"payments-svc"}, SkipRules: true})
	if err != nil || !strings.Contains(out, "<!-- pointbreak: docs may be out of date: payments-svc · 1 commit") {
		t.Errorf("load_context should warn for a drifted service: %v\n%.300s", err, out)
	}
	clean, _ := ws.LoadContext(LoadOptions{Services: []string{"orders-svc"}, SkipRules: true})
	if strings.Contains(clean, "out of date") {
		t.Error("load_context must not warn for a service that hasn't drifted")
	}

	imp, err := ws.Impact("payment.refunded")
	if err != nil || !strings.Contains(imp, "Docs that may be out of date") || !strings.Contains(imp, "payments-svc") {
		t.Errorf("impact should note drifted docs: %v\n%s", err, imp)
	}
}

func TestNoDriftLeavesOutputUnchanged(t *testing.T) {
	ws := kitebox(t) // newWS sets CommitsSince to report nothing
	st, _ := ws.Status()
	imp, _ := ws.Impact("payment.refunded")
	out, _ := ws.LoadContext(LoadOptions{Services: []string{"orders-svc"}})
	for name, s := range map[string]string{"status": st, "impact": imp, "load_context": out} {
		if strings.Contains(s, "out of date") {
			t.Errorf("%s mentions drift with no commits:\n%.300s", name, s)
		}
	}
}

func TestGitCommitsSinceOnNonRepoIsEmpty(t *testing.T) {
	if got := gitCommitsSince(t.TempDir(), time.Now().Add(-time.Hour)); len(got) != 0 {
		t.Errorf("a folder that isn't a repo has no commits, got %v", got)
	}
}
