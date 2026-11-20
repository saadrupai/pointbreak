package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTempWithCRLFAndMissingFile(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	crlf := "# svc — Current Task\r\n\r\n## 2026-11-20\r\n### branch: a — Windows task\r\n- Next: x\r\n"
	put(t, ws.Root, "context/svc/temp.md", crlf)
	if _, err := ws.SaveProgress(SaveInput{Service: "svc", Done: "y"}); err != nil {
		t.Fatalf("CRLF temp.md should parse: %v", err)
	}
	if got := read(t, ws, "svc/temp.md"); strings.Contains(got, "\r") || !strings.Contains(got, "- Done: y") {
		t.Errorf("CRLF not normalised:\n%q", got)
	}

	if err := os.Remove(filepath.Join(ws.Root, "context", "svc", "temp.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.StartTask(StartInput{Service: "svc", Title: "after delete"}); err != nil {
		t.Fatalf("missing temp.md should be recreated: %v", err)
	}
	if got := read(t, ws, "svc/temp.md"); !strings.HasPrefix(got, "# svc — Current Task") {
		t.Errorf("recreated temp.md: %q", got)
	}
}

func TestEndAndSaveWithUnknownTitle(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.StartTask(StartInput{Service: "svc", Title: "Real task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.EndTask(EndInput{Service: "svc", Title: "nope"}); err == nil || !strings.Contains(err.Error(), "no task matching") {
		t.Errorf("end with unknown title: %v", err)
	}
	if _, err := ws.SaveProgress(SaveInput{Service: "svc", Title: "nope", Done: "x"}); err == nil {
		t.Error("save with unknown title should fail")
	}
	if !strings.Contains(read(t, ws, "svc/temp.md"), "Real task") {
		t.Error("failed calls must leave the task in place")
	}
}

func TestEndWithNothingToday(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.EndTask(EndInput{Service: "svc"}); err == nil {
		t.Error("ending with no task should fail")
	}
	put(t, ws.Root, "context/svc/temp.md", "# svc — Current Task\n\n## 2026-11-18\n### branch: x — Old\n")
	if _, err := ws.EndTask(EndInput{Service: "svc"}); err == nil {
		t.Error("no task today: should fail, after archiving the stale day")
	}
	if !strings.Contains(read(t, ws, "svc/logs/2026-11.md"), "Old") || strings.Contains(read(t, ws, "svc/temp.md"), "Old") {
		t.Error("stale day should be archived even when end_task fails")
	}
}

func TestLogWithoutSeparatorStaysNewestFirst(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	put(t, ws.Root, "context/svc/logs/2026-11.md", "# svc — Work Log\n\n## 2026-11-02\n### branch: a — old\n")
	if err := ws.appendToLog("svc", "2026-11-20", []string{"### branch: b — new"}); err != nil {
		t.Fatal(err)
	}
	log := read(t, ws, "svc/logs/2026-11.md")
	if strings.Index(log, "## 2026-11-20") > strings.Index(log, "## 2026-11-02") || !strings.HasPrefix(log, "# svc — Work Log") {
		t.Errorf("newest day must go right after the title:\n%s", log)
	}
	put(t, ws.Root, "context/svc/logs/2026-11.md", "no title at all\n")
	if err := ws.appendToLog("svc", "2026-11-20", []string{"### branch: c — x"}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ws, "svc/logs/2026-11.md"); !strings.HasPrefix(got, "## 2026-11-20") || !strings.Contains(got, "no title at all") {
		t.Errorf("untitled log: %q", got)
	}
}

func TestSearchTreatsQueryAsPlainText(t *testing.T) {
	ws := kitebox(t)
	if out, err := ws.SearchLogs("cancel.go", nil, 0); err != nil || !strings.Contains(out, "orders-svc") {
		t.Errorf("dot should match literally: %v %s", err, out)
	}
	if out, err := ws.SearchLogs("(([", nil, 0); err != nil || !strings.Contains(out, "No log lines") {
		t.Errorf("regex characters must not break search: %v %s", err, out)
	}
	if _, err := ws.SearchLogs("   ", nil, 0); err == nil {
		t.Error("blank query should fail")
	}
	if _, err := ws.SearchLogs("x", []string{"../etc"}, 0); err == nil {
		t.Error("traversal in services should fail")
	}
}

func TestInitWithNoReposAndMissingRules(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00")
	created, err := ws.Init(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 3 {
		t.Errorf("no repos: only README, STANDARDS and system map, got %v", created)
	}
	ws2 := kitebox(t)
	if err := os.Remove(filepath.Join(ws2.Root, "context", "README.md")); err != nil {
		t.Fatal(err)
	}
	out, err := ws2.LoadContext(LoadOptions{Services: []string{"orders-svc"}})
	if err != nil || !strings.Contains(out, "<!-- context/README.md -->\n(missing)") {
		t.Errorf("missing rules should be reported, not fatal: %v", err)
	}
}

func TestStatusWithoutContextFolder(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Status(); err == nil || !strings.Contains(err.Error(), "init_workspace") {
		t.Errorf("status without context/ should point to init_workspace: %v", err)
	}
	if out, err := ws.ListServices(); err != nil || !strings.Contains(out, "Repos without context") {
		t.Errorf("list should show unregistered repos: %v %s", err, out)
	}
}

func TestFutureDatedEntryIsNotArchived(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	put(t, ws.Root, "context/svc/temp.md", "# svc — Current Task\n\n## 2026-11-21\n### branch: x — Clock skew\n")
	if _, err := ws.StartTask(StartInput{Service: "svc", Title: "Today"}); err != nil {
		t.Fatal(err)
	}
	temp := read(t, ws, "svc/temp.md")
	if !strings.Contains(temp, "Clock skew") || !strings.Contains(temp, "Today") {
		t.Errorf("a future-dated entry must stay in temp.md:\n%s", temp)
	}
	if st, _ := ws.Status(); strings.Contains(st, "stale") {
		t.Error("future date is not stale")
	}
}

func TestMultilineAndBulletedCheckpoint(t *testing.T) {
	ws := newWS(t, "2026-11-20 10:00", "svc")
	if _, err := ws.Init(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.StartTask(StartInput{Service: "svc", Title: "T"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.SaveProgress(SaveInput{Service: "svc", Plan: "- one\n* two\n\nthree"}); err != nil {
		t.Fatal(err)
	}
	temp := read(t, ws, "svc/temp.md")
	for _, want := range []string{"- Plan:\n  - one\n  - two\n  - three"} {
		if !strings.Contains(temp, want) {
			t.Errorf("missing %q:\n%s", want, temp)
		}
	}
}
