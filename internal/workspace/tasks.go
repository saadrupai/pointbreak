package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var dayRE = regexp.MustCompile(`^## (\d{4}-\d{2}-\d{2})\s*$`)

// tempFile is a parsed temp.md: a title line and day sections, newest wherever the file put it.
type tempFile struct {
	title string
	days  []day
}

type day struct {
	date  string
	lines []string // lines under the "## date" header, without trailing blank lines
}

func parseTemp(s, svc string) tempFile {
	t := tempFile{title: fmt.Sprintf("# %s — Current Task", svc)}
	var cur *day
	for _, ln := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(ln, "# ") && cur == nil {
			t.title = ln
			continue
		}
		if m := dayRE.FindStringSubmatch(ln); m != nil {
			t.days = append(t.days, day{date: m[1]})
			cur = &t.days[len(t.days)-1]
			continue
		}
		if cur != nil {
			cur.lines = append(cur.lines, ln)
		}
	}
	for i := range t.days {
		t.days[i].lines = trimBlank(t.days[i].lines)
	}
	return t
}

func (t tempFile) String() string {
	var b strings.Builder
	b.WriteString(t.title + "\n\n")
	if len(t.days) == 0 {
		b.WriteString(Idle + "\n")
		return b.String()
	}
	for i, d := range t.days {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + d.date + "\n")
		for _, ln := range d.lines {
			b.WriteString(ln + "\n")
		}
	}
	return b.String()
}

func trimBlank(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

// blocks returns [start,end) line ranges of "### " task blocks inside a day.
func blocks(lines []string) [][2]int {
	var out [][2]int
	for i, ln := range lines {
		if strings.HasPrefix(ln, "### ") {
			if len(out) > 0 {
				out[len(out)-1][1] = i
			}
			out = append(out, [2]int{i, len(lines)})
		}
	}
	return out
}

// pickBlock finds the task block whose header contains title (case-insensitive), or the last block.
func pickBlock(lines []string, title string) ([2]int, error) {
	bs := blocks(lines)
	if len(bs) == 0 {
		return [2]int{}, errors.New("today's entry has no task block (### branch: …)")
	}
	if title == "" {
		return bs[len(bs)-1], nil
	}
	for _, b := range bs {
		if strings.Contains(strings.ToLower(lines[b[0]]), strings.ToLower(title)) {
			return b, nil
		}
	}
	return [2]int{}, fmt.Errorf("no task matching %q in today's entry", title)
}

func (w *Workspace) readTemp(svc string) (tempFile, error) {
	data, err := os.ReadFile(w.ctxPath(svc, "temp.md"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return tempFile{}, err
	}
	return parseTemp(string(data), svc), nil
}

func (w *Workspace) writeTemp(svc string, t tempFile) error {
	return os.WriteFile(w.ctxPath(svc, "temp.md"), []byte(t.String()), 0o644)
}

// archiveStale moves days older than today from temp.md into their month logs, verbatim.
func (w *Workspace) archiveStale(svc string, t *tempFile) ([]string, error) {
	today := w.today()
	var keep []day
	var archived []string
	for _, d := range t.days {
		if d.date >= today { // today's work, or a clock-skewed future date: leave it alone
			keep = append(keep, d)
			continue
		}
		lines := append(append([]string{}, d.lines...), "- Note: moved from temp.md automatically; the task was not closed with end_task.")
		if err := w.appendToLog(svc, d.date, lines); err != nil {
			return archived, err
		}
		archived = append(archived, d.date)
	}
	t.days = keep
	return archived, nil
}

func (w *Workspace) todayDay(t *tempFile) *day {
	for i := range t.days {
		if t.days[i].date == w.today() {
			return &t.days[i]
		}
	}
	return nil
}

// StartInput describes a new task.
type StartInput struct {
	Service     string
	Branch      string
	Title       string
	Goal        string
	Ticket      string
	Inputs      []string
	AlsoTouches []string
}

// StartTask adds today's task block to temp.md (archiving older days first) and returns the block.
func (w *Workspace) StartTask(in StartInput) (string, error) {
	if err := w.requireService(in.Service); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.Title) == "" {
		return "", errors.New("title is required")
	}
	branch := strings.TrimSpace(in.Branch)
	if branch == "" && w.Branch != nil {
		branch = w.Branch(filepath.Join(w.Root, in.Service))
	}
	if branch == "" {
		branch = "(not recorded)"
	}
	t, err := w.readTemp(in.Service)
	if err != nil {
		return "", err
	}
	archived, err := w.archiveStale(in.Service, &t)
	if err != nil {
		return "", err
	}
	block := []string{fmt.Sprintf("### branch: %s — %s", branch, strings.TrimSpace(in.Title))}
	if in.Ticket != "" {
		block = append(block, "- Ticket: "+in.Ticket)
	}
	if len(in.Inputs) > 0 {
		block = append(block, "- Inputs: "+strings.Join(in.Inputs, ", "))
	}
	if len(in.AlsoTouches) > 0 {
		block = append(block, "- Also touches: "+strings.Join(in.AlsoTouches, ", "))
	}
	if in.Goal != "" {
		block = append(block, "- Goal: "+in.Goal)
	}
	d := w.todayDay(&t)
	if d == nil {
		t.days = append([]day{{date: w.today()}}, t.days...)
		d = &t.days[0]
	}
	if len(d.lines) > 0 {
		d.lines = append(d.lines, "")
	}
	d.lines = append(d.lines, block...)
	if err := w.writeTemp(in.Service, t); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("Started in %s/%s/temp.md under ## %s:\n%s", ContextDir, in.Service, w.today(), strings.Join(block, "\n"))
	if len(archived) > 0 {
		msg += fmt.Sprintf("\nMoved unfinished entries from %s into the logs.", strings.Join(archived, ", "))
	}
	return msg, nil
}

// SaveInput is a checkpoint on today's task.
type SaveInput struct {
	Service   string
	Title     string // optional: which of today's tasks; default the last one
	Plan      string
	Done      string
	Next      string
	Decisions string
	Questions string
}

// SaveProgress appends a timestamped checkpoint to a task block in today's temp.md entry.
func (w *Workspace) SaveProgress(in SaveInput) (string, error) {
	if err := w.requireService(in.Service); err != nil {
		return "", err
	}
	t, err := w.readTemp(in.Service)
	if err != nil {
		return "", err
	}
	d := w.todayDay(&t)
	if d == nil {
		return "", fmt.Errorf("no task started today in %s: call start_task first", in.Service)
	}
	b, err := pickBlock(d.lines, in.Title)
	if err != nil {
		return "", err
	}
	add := []string{"- Checkpoint " + w.Now().Format("15:04")}
	for _, f := range []struct{ label, val string }{
		{"Plan", in.Plan}, {"Done", in.Done}, {"Next", in.Next},
		{"Decisions", in.Decisions}, {"Open questions", in.Questions},
	} {
		if v := strings.TrimSpace(f.val); v != "" {
			add = append(add, bullet(f.label, v)...)
		}
	}
	if len(add) == 1 {
		return "", errors.New("nothing to save: pass at least one of plan, done, next, decisions, questions")
	}
	block := trimBlank(d.lines[b[0]:b[1]])
	rest := append([]string{}, d.lines[b[1]:]...)
	d.lines = append(append(append(d.lines[:b[0]:b[0]], block...), add...), rest...)
	if err := w.writeTemp(in.Service, t); err != nil {
		return "", err
	}
	return fmt.Sprintf("Saved to %s/%s/temp.md (%s).", ContextDir, in.Service, strings.TrimPrefix(block[0], "### ")), nil
}

// bullet renders "- Label: text"; multi-line text becomes an indented list under the label.
func bullet(label, text string) []string {
	parts := strings.Split(text, "\n")
	if len(parts) == 1 {
		return []string{fmt.Sprintf("- %s: %s", label, text)}
	}
	out := []string{fmt.Sprintf("- %s:", label)}
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, "  "+ensureDash(p))
		}
	}
	return out
}

func ensureDash(s string) string {
	if strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") {
		return "- " + s[2:]
	}
	return "- " + s
}

// EndInput closes a task.
type EndInput struct {
	Service string
	Title   string   // optional: which of today's tasks; default the last one
	Summary []string // condensed bullets for the log; empty = keep the task's lines as-is
}

// EndTask moves one of today's tasks from temp.md to the top of this month's log.
// When no tasks remain for today, temp.md goes back to "(none active)".
func (w *Workspace) EndTask(in EndInput) (string, error) {
	if err := w.requireService(in.Service); err != nil {
		return "", err
	}
	t, err := w.readTemp(in.Service)
	if err != nil {
		return "", err
	}
	if _, err := w.archiveStale(in.Service, &t); err != nil {
		return "", err
	}
	d := w.todayDay(&t)
	if d == nil {
		if err := w.writeTemp(in.Service, t); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no task from today in %s/temp.md to end", in.Service)
	}
	b, err := pickBlock(d.lines, in.Title)
	if err != nil {
		return "", err
	}
	block := trimBlank(d.lines[b[0]:b[1]])
	// The log keeps everything: a short summary first, then every detail from temp.md.
	entry := block
	var summary []string
	for _, s := range in.Summary {
		if s = strings.TrimSpace(s); s != "" {
			summary = append(summary, "  "+ensureDash(s))
		}
	}
	if len(summary) > 0 {
		entry = append(append([]string{block[0], "- Summary:"}, summary...), block[1:]...)
	}
	if err := w.appendToLog(in.Service, d.date, entry); err != nil {
		return "", err
	}
	d.lines = trimBlank(append(append([]string{}, d.lines[:b[0]]...), d.lines[b[1]:]...))
	if len(blocks(d.lines)) == 0 {
		var keep []day
		for _, x := range t.days {
			if x.date != d.date {
				keep = append(keep, x)
			}
		}
		t.days = keep
	}
	if err := w.writeTemp(in.Service, t); err != nil {
		return "", err
	}
	logRel := fmt.Sprintf("%s/%s/logs/%s.md", ContextDir, in.Service, d.date[:7])
	left := "temp.md reset to (none active)."
	if len(t.days) > 0 {
		left = "Other tasks from today stay in temp.md."
	}
	return fmt.Sprintf("Logged %q at the top of %s. %s", strings.TrimPrefix(block[0], "### "), logRel, left), nil
}

// appendToLog inserts lines under "## date" at the top of logs/<month>.md (newest first).
func (w *Workspace) appendToLog(svc, date string, lines []string) error {
	if len(date) < 7 {
		return fmt.Errorf("bad date %q", date)
	}
	month := date[:7]
	path := w.ctxPath(svc, "logs", month+".md")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = []byte(fmt.Sprintf("# %s — Work Log, %s\n\nNewest day at the top. One `## YYYY-MM-DD` per day, one `### branch: <branch> — <title>` per task.\n\n---\n", svc, month))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	src := strings.Split(strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n")
	header := "## " + date
	var out []string
	inserted := false
	for i, ln := range src {
		out = append(out, ln)
		if inserted {
			continue
		}
		if ln == header {
			out = append(out, lines...)
			out = append(out, "")
			inserted = true
			continue
		}
		if strings.TrimSpace(ln) == "---" && !containsLine(src[i+1:], header) {
			out = append(out, "", header)
			out = append(out, lines...)
			inserted = true
		}
	}
	if !inserted { // no "---" separator (hand-edited file): put the newest day right after the title
		out = nil
		titled := false
		for _, ln := range src {
			out = append(out, ln)
			if !titled && strings.HasPrefix(ln, "# ") {
				out = append(out, "", header)
				out = append(out, lines...)
				out = append(out, "")
				titled = true
			}
		}
		if !titled {
			out = append(append([]string{header}, lines...), append([]string{""}, src...)...)
		}
	}
	text := strings.Join(collapseBlank(out), "\n") + "\n"
	return os.WriteFile(path, []byte(text), 0o644)
}

func containsLine(lines []string, s string) bool {
	for _, l := range lines {
		if l == s {
			return true
		}
	}
	return false
}

func collapseBlank(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) == "" && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

// EndTaskIn ends the same task in several services (every service the task touched).
// Each service is handled independently; the report lists what happened in each.
func (w *Workspace) EndTaskIn(services []string, title string, summary []string) (string, error) {
	if len(services) == 0 {
		return "", errors.New("pass at least one service")
	}
	var lines []string
	failed := 0
	seen := map[string]bool{}
	for _, svc := range services {
		if seen[svc] {
			continue
		}
		seen[svc] = true
		msg, err := w.EndTask(EndInput{Service: svc, Title: title, Summary: summary})
		if err != nil {
			failed++
			lines = append(lines, fmt.Sprintf("- %s: not ended: %v", svc, err))
			continue
		}
		lines = append(lines, "- "+svc+": "+msg)
	}
	out := strings.Join(lines, "\n")
	if failed == len(seen) {
		return "", errors.New(out)
	}
	return out, nil
}
