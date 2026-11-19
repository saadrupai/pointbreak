package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saadrupai/pointbreak"
)

func templates(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(pointbreak.Templates, "templates/context")
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

// newWS creates a workspace with git-repo dirs and a fixed clock.
func newWS(t *testing.T, now string, repos ...string) *Workspace {
	t.Helper()
	root := t.TempDir()
	for _, r := range repos {
		if err := os.MkdirAll(filepath.Join(root, r, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ts, err := time.Parse("2006-01-02 15:04", now)
	if err != nil {
		t.Fatal(err)
	}
	ws := New(root, templates(t))
	ws.Now = func() time.Time { return ts }
	ws.Branch = func(string) string { return "feature/x" }
	return ws
}

func read(t *testing.T, ws *Workspace, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ws.Root, ContextDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCheckName(t *testing.T) {
	for _, bad := range []string{"", "../x", "a/b", ".hidden", "a..b", "x y"} {
		if CheckName(bad) == nil {
			t.Errorf("CheckName(%q) accepted a bad name", bad)
		}
	}
	for _, ok := range []string{"orders-svc", "payments_svc", "svc.v2", "A1"} {
		if err := CheckName(ok); err != nil {
			t.Errorf("CheckName(%q) = %v", ok, err)
		}
	}
}

func TestInitCreatesAllAndNeverOverwrites(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "orders-svc", "payments-svc")
	if err := os.MkdirAll(filepath.Join(ws.Root, "notes"), 0o755); err != nil { // not a repo
		t.Fatal(err)
	}
	created, err := ws.Init(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2+2*5 {
		t.Fatalf("created %d files, want 12: %v", len(created), created)
	}
	if got := read(t, ws, "orders-svc/temp.md"); !strings.Contains(got, "# orders-svc — Current Task") || !strings.Contains(got, Idle) {
		t.Errorf("temp.md not filled from template: %q", got)
	}
	if got := read(t, ws, "orders-svc/logs/2026-11.md"); !strings.Contains(got, "Work Log, 2026-11") {
		t.Errorf("log header wrong: %q", got)
	}
	if _, err := os.Stat(filepath.Join(ws.Root, ContextDir, "notes")); err == nil {
		t.Error("non-repo folder was registered")
	}

	custom := filepath.Join(ws.Root, ContextDir, "orders-svc", "architecture.md")
	if err := os.WriteFile(custom, []byte("hand written"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := ws.Init(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("second Init created %v", again)
	}
	if got := read(t, ws, "orders-svc/architecture.md"); got != "hand written" {
		t.Error("Init overwrote an existing file")
	}
}

func TestInitRejectsBadOrMissingService(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "orders-svc")
	if _, err := ws.Init([]string{"../etc"}); err == nil {
		t.Error("accepted path traversal")
	}
	if _, err := ws.Init([]string{"ghost-svc"}); err == nil {
		t.Error("accepted a service with no directory")
	}
}

func TestTaskLifecycle(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "orders-svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}

	msg, err := ws.StartTask(StartInput{Service: "orders-svc", Title: "Partial refunds", Ticket: "ORD-142", Goal: "refund part of an order", AlsoTouches: []string{"payments-svc"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "### branch: feature/x — Partial refunds") {
		t.Errorf("start message: %s", msg)
	}
	temp := read(t, ws, "orders-svc/temp.md")
	for _, want := range []string{"## 2026-11-20", "- Ticket: ORD-142", "- Also touches: payments-svc", "- Goal: refund part of an order"} {
		if !strings.Contains(temp, want) {
			t.Errorf("temp.md missing %q:\n%s", want, temp)
		}
	}
	if strings.Contains(temp, Idle) {
		t.Error("temp.md still says idle")
	}

	if _, err := ws.StartTask(StartInput{Service: "orders-svc", Title: "Fix typo", Branch: "fix/typo"}); err != nil {
		t.Fatal(err)
	}
	temp = read(t, ws, "orders-svc/temp.md")
	if strings.Count(temp, "## 2026-11-20") != 1 || strings.Count(temp, "### branch:") != 2 {
		t.Errorf("second task should share the day header:\n%s", temp)
	}

	if _, err := ws.SaveProgress(SaveInput{Service: "orders-svc", Title: "refunds", Plan: "step one\nstep two", Next: "write tests"}); err != nil {
		t.Fatal(err)
	}
	temp = read(t, ws, "orders-svc/temp.md")
	refundsIdx, typoIdx := strings.Index(temp, "Partial refunds"), strings.Index(temp, "Fix typo")
	planIdx := strings.Index(temp, "  - step one")
	if planIdx < refundsIdx || planIdx > typoIdx {
		t.Errorf("checkpoint not inside the refunds block:\n%s", temp)
	}
	if !strings.Contains(temp, "- Checkpoint 10:00") || !strings.Contains(temp, "- Next: write tests") {
		t.Errorf("checkpoint missing fields:\n%s", temp)
	}

	if _, err := ws.EndTask(EndInput{Service: "orders-svc", Title: "refunds", Summary: []string{"Added refund limit", "- Tests pass"}}); err != nil {
		t.Fatal(err)
	}
	log := read(t, ws, "orders-svc/logs/2026-11.md")
	if !strings.Contains(log, "## 2026-11-20\n### branch: feature/x — Partial refunds\n- Added refund limit\n- Tests pass") {
		t.Errorf("log entry wrong:\n%s", log)
	}
	temp = read(t, ws, "orders-svc/temp.md")
	if strings.Contains(temp, "Partial refunds") || !strings.Contains(temp, "Fix typo") {
		t.Errorf("only the ended task should leave temp.md:\n%s", temp)
	}

	if _, err := ws.EndTask(EndInput{Service: "orders-svc"}); err != nil {
		t.Fatal(err)
	}
	if temp = read(t, ws, "orders-svc/temp.md"); !strings.Contains(temp, Idle) {
		t.Errorf("temp.md should be idle after the last task ends:\n%s", temp)
	}
	log = read(t, ws, "orders-svc/logs/2026-11.md")
	if strings.Count(log, "## 2026-11-20") != 1 {
		t.Errorf("same-day tasks should share one log header:\n%s", log)
	}
	if strings.Index(log, "Fix typo") > strings.Index(log, "Partial refunds") {
		t.Errorf("newest task should be first under the day:\n%s", log)
	}
}

func TestLogNewestDayAtTop(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	if err := ws.appendToLog("svc", "2026-11-05", []string{"### branch: a — older"}); err != nil {
		t.Fatal(err)
	}
	if err := ws.appendToLog("svc", "2026-11-20", []string{"### branch: b — newer"}); err != nil {
		t.Fatal(err)
	}
	log := read(t, ws, "svc/logs/2026-11.md")
	if strings.Index(log, "## 2026-11-20") > strings.Index(log, "## 2026-11-05") {
		t.Errorf("newest day must be at the top:\n%s", log)
	}
	if err := ws.appendToLog("svc", "2026-10-30", []string{"### branch: c — last month"}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ws, "svc/logs/2026-10.md"); !strings.Contains(got, "## 2026-10-30") {
		t.Errorf("entry should go to its own month file:\n%s", got)
	}
}

func TestStaleDayIsArchived(t *testing.T) {
	ws := newWS(t, "2026-11-20 09:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	stale := "# svc — Current Task\n\n## 2026-11-18\n### branch: x — Old work\n- Next: finish it\n"
	if err := os.WriteFile(filepath.Join(ws.Root, ContextDir, "svc", "temp.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := ws.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "stale: from 2026-11-18") {
		t.Errorf("status should flag the stale entry: %s", status)
	}
	msg, err := ws.StartTask(StartInput{Service: "svc", Title: "New work"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "2026-11-18") {
		t.Errorf("start should report the archived day: %s", msg)
	}
	if temp := read(t, ws, "svc/temp.md"); strings.Contains(temp, "Old work") || !strings.Contains(temp, "## 2026-11-20") {
		t.Errorf("temp.md should hold only today:\n%s", temp)
	}
	log := read(t, ws, "svc/logs/2026-11.md")
	if !strings.Contains(log, "### branch: x — Old work") || !strings.Contains(log, "not closed with end_task") {
		t.Errorf("stale entry not archived verbatim:\n%s", log)
	}
}

func TestErrorsGuideTheCaller(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.SaveProgress(SaveInput{Service: "svc", Done: "x"}); err == nil || !strings.Contains(err.Error(), "start_task") {
		t.Errorf("save without a task should point to start_task, got %v", err)
	}
	if _, err := ws.StartTask(StartInput{Service: "ghost", Title: "x"}); err == nil || !strings.Contains(err.Error(), "init_workspace") {
		t.Errorf("unknown service should point to init_workspace, got %v", err)
	}
	if _, err := ws.StartTask(StartInput{Service: "svc"}); err == nil {
		t.Error("missing title accepted")
	}
	if _, err := ws.StartTask(StartInput{Service: "svc", Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.SaveProgress(SaveInput{Service: "svc"}); err == nil {
		t.Error("empty checkpoint accepted")
	}
}

// kitebox copies the bundled example workspace into a temp root.
func kitebox(t *testing.T) *Workspace {
	t.Helper()
	ws := newWS(t, "2026-11-20 10:00", "orders-svc", "payments-svc", "notifications-svc")
	src := filepath.Join("..", "..", "examples", "kitebox", "context")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		dst := filepath.Join(ws.Root, ContextDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestLoadContextOrderAndScope(t *testing.T) {
	ws := kitebox(t)
	one, err := ws.LoadContext(LoadOptions{Services: []string{"orders-svc"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(one, "<!-- context/global-architecture.md -->") {
		t.Error("single service should not load the system map by default")
	}
	order := []string{"context/README.md", "orders-svc/architecture.md", "orders-svc/database.md", "orders-svc/features.md", "orders-svc/temp.md", "orders-svc/logs/2026-11.md"}
	last := -1
	for _, f := range order {
		i := strings.Index(one, "<!-- context/"+strings.TrimPrefix(f, "context/")+" -->")
		if i < 0 {
			t.Fatalf("missing %s", f)
		}
		if i < last {
			t.Errorf("%s out of order", f)
		}
		last = i
	}

	two, err := ws.LoadContext(LoadOptions{Services: []string{"orders-svc", "payments-svc"}, SkipRules: true, Files: []string{"database.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(two, "<!-- context/README.md -->") || !strings.Contains(two, "<!-- context/global-architecture.md -->") {
		t.Error("skip_rules or automatic global map not applied")
	}
	if strings.Contains(two, "architecture.md -->\n# orders") || strings.Contains(two, "features.md") {
		t.Error("files filter not applied")
	}
	if _, err := ws.LoadContext(LoadOptions{Services: []string{"../x"}}); err == nil {
		t.Error("path traversal accepted")
	}
}

func TestImpactFindsConsumers(t *testing.T) {
	ws := kitebox(t)
	out, err := ws.Impact("payment.refunded")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Events", "orders-svc, notifications-svc", "orders-svc:", "notifications-svc:"} {
		if !strings.Contains(out, want) {
			t.Errorf("impact missing %q:\n%s", want, out)
		}
	}
	none, _ := ws.Impact("no-such-thing-xyz")
	if !strings.Contains(none, "does not appear") {
		t.Errorf("unexpected: %s", none)
	}
}

func TestSearchLogs(t *testing.T) {
	ws := kitebox(t)
	out, err := ws.SearchLogs("ORD-131", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range []string{"orders-svc", "payments-svc", "notifications-svc"} {
		if !strings.Contains(out, svc) {
			t.Errorf("ORD-131 touched %s but search missed it:\n%s", svc, out)
		}
	}
	if limited, _ := ws.SearchLogs("ORD-131", nil, 1); !strings.Contains(limited, "limit reached") {
		t.Errorf("limit not applied: %s", limited)
	}
}

func TestStatusAndList(t *testing.T) {
	ws := kitebox(t)
	st, err := ws.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st, "payments-svc · branch: feature/ORD-142-partial-refunds") || !strings.Contains(st, "next: implement `ErrRefundLimitReached`") {
		t.Errorf("status:\n%s", st)
	}
	list, err := ws.ListServices()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(list, "(registered") != 3 {
		t.Errorf("list:\n%s", list)
	}
}
