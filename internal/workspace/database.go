package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/saadrupai/pointbreak/internal/dbschema"
)

// SharedDir holds docs for databases shared by several services: context/_db/<name>.md.
const SharedDir = "_db"

const (
	sharedStart = "<!-- pointbreak:shared-db:start (sync_database rewrites this block) -->"
	sharedEnd   = "<!-- pointbreak:shared-db:end -->"
	usersStart  = "<!-- pointbreak:users:start (sync_database rewrites this block) -->"
	usersEnd    = "<!-- pointbreak:users:end -->"
)

var reSharedLink = regexp.MustCompile(`context/` + SharedDir + `/([A-Za-z0-9][A-Za-z0-9._-]*)\.md`)

// SyncInput selects what sync_database documents.
//
// Own database: set Service. Shared database: set Database, Source and Users ("*" = every service).
type SyncInput struct {
	Service         string
	Database        string
	Source          string   // migrations folder or schema.prisma, relative to the workspace root
	Users           []string // services using a shared database
	DryRun          bool
	ReplaceSections bool
}

// SyncDatabase replays migrations and writes the schema into the right database doc.
func (w *Workspace) SyncDatabase(in SyncInput) (string, error) {
	if in.Database == "" && in.Service == "" {
		return "", errors.New("pass service (a service with its own database) or database + source + users (a shared database)")
	}
	if in.Database != "" {
		return w.syncShared(in)
	}
	if err := w.requireService(in.Service); err != nil {
		return "", err
	}
	src, label, err := w.resolveSource(in.Source, in.Service)
	if err != nil {
		return "", err
	}
	schema, err := dbschema.Load(src, label)
	if err != nil {
		return "", err
	}
	target := w.ctxPath(in.Service, "database.md")
	doc, err := w.mergeSchemaDoc(target, fmt.Sprintf("# %s — Database", in.Service), schema, in.ReplaceSections, false)
	if err != nil {
		return "", err
	}
	if in.DryRun {
		return fmt.Sprintf("Dry run, nothing written. %s.\n\n%s", schema.Summary(), schema.Render(w.today())), nil
	}
	if err := os.WriteFile(target, []byte(doc), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated %s/%s/database.md from %s: %s.", ContextDir, in.Service, label, schema.Summary()), nil
}

func (w *Workspace) syncShared(in SyncInput) (string, error) {
	if err := CheckName(in.Database); err != nil {
		return "", fmt.Errorf("invalid database name: %w", err)
	}
	if in.Source == "" {
		return "", errors.New("source is required for a shared database: the folder (relative to the workspace root) that holds its migrations, e.g. db-migrations or orders-svc/migrations")
	}
	src, label, err := w.resolveSource(in.Source, "")
	if err != nil {
		return "", err
	}
	schema, err := dbschema.Load(src, label)
	if err != nil {
		return "", err
	}
	users, err := w.sharedUsers(in.Database, in.Users)
	if err != nil {
		return "", err
	}
	usage := map[string][]string{}
	for _, u := range users {
		usage[u] = tableUsage(filepath.Join(w.Root, u), schema.TableNames())
	}

	if err := os.MkdirAll(w.ctxPath(SharedDir), 0o755); err != nil {
		return "", err
	}
	target := w.ctxPath(SharedDir, in.Database+".md")
	doc, err := w.mergeSchemaDoc(target, fmt.Sprintf("# %s — Shared Database", in.Database), schema, in.ReplaceSections, true)
	if err != nil {
		return "", err
	}
	doc = replaceBlock(doc, usersStart, usersEnd, w.usersBlock(users, usage), "## Tables")

	if in.DryRun {
		return fmt.Sprintf("Dry run, nothing written. Shared database %s used by %s. %s.\n\n%s", in.Database, strings.Join(users, ", "), schema.Summary(), schema.Render(w.today())), nil
	}
	if err := os.WriteFile(target, []byte(doc), 0o644); err != nil {
		return "", err
	}
	for _, u := range users {
		if err := w.writeSharedPointer(u, in.Database, usage[u]); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Updated %s/%s/%s.md from %s: %s. Linked it from the database.md of: %s.",
		ContextDir, SharedDir, in.Database, label, schema.Summary(), strings.Join(users, ", ")), nil
}

// resolveSource returns an absolute path and a display label for the migrations source.
// With no explicit source it looks inside the service repo.
func (w *Workspace) resolveSource(source, service string) (string, string, error) {
	if source != "" {
		clean := filepath.Clean(source)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", "", fmt.Errorf("source must be a path inside the workspace, relative to %s", w.Root)
		}
		full := filepath.Join(w.Root, clean)
		if _, err := os.Stat(full); err != nil {
			return "", "", fmt.Errorf("source %s not found in the workspace", clean)
		}
		if st, _ := os.Stat(full); st.IsDir() {
			if rel, err := dbschema.Detect(full); err == nil && rel != "." {
				return filepath.Join(full, rel), filepath.ToSlash(filepath.Join(clean, rel)), nil
			}
		}
		return full, filepath.ToSlash(clean), nil
	}
	repo := filepath.Join(w.Root, service)
	if _, err := os.Stat(repo); err != nil {
		return "", "", fmt.Errorf("repo %s not found in the workspace; pass source", service)
	}
	rel, err := dbschema.Detect(repo)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(repo, rel), filepath.ToSlash(filepath.Join(service, rel)), nil
}

// sharedUsers resolves which services use a shared database.
func (w *Workspace) sharedUsers(db string, users []string) ([]string, error) {
	all, err := w.Services()
	if err != nil {
		return nil, err
	}
	if len(users) == 1 && users[0] == "*" {
		return all, nil
	}
	if len(users) == 0 { // services already linked to this database
		link := fmt.Sprintf("%s/%s/%s.md", ContextDir, SharedDir, db)
		for _, s := range all {
			if b, err := os.ReadFile(w.ctxPath(s, "database.md")); err == nil && strings.Contains(string(b), link) {
				users = append(users, s)
			}
		}
		if len(users) == 0 {
			return nil, errors.New(`users is required the first time: the services that use this database, or ["*"] for all services`)
		}
		return users, nil
	}
	for _, u := range users {
		if err := w.requireService(u); err != nil {
			return nil, err
		}
	}
	return users, nil
}

// mergeSchemaDoc puts the generated block into an existing doc without touching human sections.
func (w *Workspace) mergeSchemaDoc(path, title string, s *dbschema.Schema, replaceSections, shared bool) (string, error) {
	gen := s.Render(w.today())
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	cur := string(data)
	if err := checkMarkers(cur, dbschema.GenStart, dbschema.GenEnd, relTo(w.Root, path)); err != nil {
		return "", err
	}
	if shared {
		if err := checkMarkers(cur, usersStart, usersEnd, relTo(w.Root, path)); err != nil {
			return "", err
		}
	}
	switch {
	case strings.Contains(cur, dbschema.GenStart[:30]):
		return replaceBlock(cur, dbschema.GenStart, dbschema.GenEnd, gen, ""), nil
	case strings.TrimSpace(cur) == "" || strings.Contains(cur, "`<table_name>`") || strings.Contains(cur, "<one line: what a row represents>"):
		var b strings.Builder
		b.WriteString(title + "\n\n## Engine\n")
		fmt.Fprintf(&b, "- Migrations: %s, `%s`\n- Engine, database name and access library: (unverified, fill in)\n\n", s.Tool, s.Dir)
		if shared {
			b.WriteString(usersStart + "\n" + usersEnd + "\n\n")
		}
		b.WriteString(gen + "\n")
		if !shared {
			b.WriteString("## Data owned elsewhere\n- (fill in: ids that point to other services' data)\n\n")
		} else {
			b.WriteString("## Ownership\n- (fill in: which service is allowed to write each table)\n\n")
		}
		b.WriteString("## Data notes\n- (fill in: money units, PII columns, hot tables, retention)\n")
		return b.String(), nil
	case replaceSections:
		return replaceSectionsWith(cur, gen), nil
	default:
		return "", fmt.Errorf("%s is hand-written and has no generated block. Rerun with replace_sections: true to replace only its Tables, Relations, Enums and Migration log sections (all other sections stay), or dry_run: true to preview", relTo(w.Root, path))
	}
}

func relTo(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}

// replaceBlock swaps the text between start and end markers (inclusive). When the markers are
// missing, the block goes before the line starting with `before`, or after the title line.
func replaceBlock(doc, start, end, block, before string) string {
	block = strings.TrimRight(block, "\n")
	if !strings.HasPrefix(block, start) {
		block = start + "\n" + block + "\n" + end
	}
	si := strings.Index(doc, start[:min(30, len(start))])
	if si >= 0 {
		if ei := strings.Index(doc[si:], end); ei >= 0 {
			return doc[:si] + block + doc[si+ei+len(end):]
		}
	}
	lines := strings.Split(doc, "\n")
	at := -1
	if before != "" {
		for i, l := range lines {
			if strings.HasPrefix(l, before) || strings.HasPrefix(l, dbschema.GenStart[:30]) {
				at = i
				break
			}
		}
	}
	if at < 0 {
		at = 0
		for i, l := range lines {
			if strings.HasPrefix(l, "# ") {
				at = i + 1
				break
			}
		}
		return strings.Join(append(append(append([]string{}, lines[:at]...), "", block), lines[at:]...), "\n")
	}
	return strings.Join(append(append(append([]string{}, lines[:at]...), block, ""), lines[at:]...), "\n")
}

var generatedSections = []string{"## tables", "## relations", "## enums", "## migration log"}

// replaceSectionsWith removes the generated-type sections of a hand-written doc and puts gen
// where the first of them was. Every other section is kept as written.
func replaceSectionsWith(doc, gen string) string {
	lines := strings.Split(doc, "\n")
	var out []string
	inserted, skipping := false, false
	for _, l := range lines {
		if strings.HasPrefix(l, "## ") {
			lower := strings.ToLower(l)
			skipping = false
			for _, g := range generatedSections {
				if strings.HasPrefix(lower, g) {
					skipping = true
				}
			}
			if skipping && !inserted {
				out = append(out, strings.TrimRight(gen, "\n"), "")
				inserted = true
			}
		}
		if !skipping {
			out = append(out, l)
		}
	}
	if !inserted {
		out = append(out, "", strings.TrimRight(gen, "\n"))
	}
	return strings.Join(out, "\n")
}

func (w *Workspace) usersBlock(users []string, usage map[string][]string) string {
	var b strings.Builder
	b.WriteString(usersStart + "\n## Used by\n")
	fmt.Fprintf(&b, "_Tables found in each service's code by a scan on %s (unverified: string matches, not runtime proof)._\n\n", w.today())
	b.WriteString("| Service | Tables referenced in code |\n|---|---|\n")
	for _, u := range users {
		t := strings.Join(usage[u], ", ")
		if t == "" {
			t = "none found"
		}
		fmt.Fprintf(&b, "| [%s](../%s/architecture.md) | %s |\n", u, u, t)
	}
	b.WriteString(usersEnd)
	return b.String()
}

// writeSharedPointer adds or refreshes the shared-database block in a service's database.md.
func (w *Workspace) writeSharedPointer(svc, db string, tables []string) error {
	path := w.ctxPath(svc, "database.md")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	cur := string(data)
	if err := checkMarkers(cur, sharedStart, sharedEnd, relTo(w.Root, path)); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(sharedStart + "\n## Shared database\n")
	fmt.Fprintf(&b, "This service uses the shared database `%s`. Full schema, relations and migration log: `%s/%s/%s.md`.\n\n", db, ContextDir, SharedDir, db)
	if len(tables) > 0 {
		fmt.Fprintf(&b, "Tables referenced in this service's code (unverified, code scan on %s): %s.\n", w.today(), strings.Join(tables, ", "))
	} else {
		b.WriteString("No table names from this database were found in this service's code (unverified).\n")
	}
	b.WriteString(sharedEnd)
	block := b.String()

	if strings.TrimSpace(cur) == "" || strings.Contains(cur, "`<table_name>`") {
		cur = fmt.Sprintf("# %s — Database\n\n%s\n\n## Tables this service owns\n- (fill in: which shared tables this service writes; other services only read them)\n\n## Data notes\n- (fill in)\n", svc, block)
	} else {
		cur = replaceBlock(cur, sharedStart, sharedEnd, block, "")
	}
	return os.WriteFile(path, []byte(cur), 0o644)
}

var (
	codeExt  = map[string]bool{".go": true, ".ts": true, ".js": true, ".py": true, ".java": true, ".kt": true, ".rb": true, ".sql": true, ".cs": true, ".php": true, ".rs": true, ".scala": true, ".ex": true, ".prisma": true}
	skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "bin": true, "target": true, ".next": true, "migrations": true, "migration": true, "testdata": true}
)

// tableUsage scans a repo's source for SQL-style or ORM references to each table.
// Results look like "orders (12)", sorted by count. Never reads files over 1 MB.
func tableUsage(repo string, tables []string) []string {
	if len(tables) == 0 {
		return nil
	}
	res := map[string]*regexp.Regexp{}
	for _, t := range tables {
		q := regexp.QuoteMeta(t)
		res[t] = regexp.MustCompile(`(?i)(\b(?:from|join|into|update|table)\s+["` + "`" + `]?(?:\w+\.)?` + q + `["` + "`" + `]?\b` +
			`|TableName\(\)\s*string\s*\{\s*return\s*"` + q + `"` +
			`|__tablename__\s*=\s*['"]` + q + `['"]` +
			`|@@map\(\s*"` + q + `"\s*\)` +
			`|@Table\(\s*name\s*=\s*"` + q + `"` +
			`|\.Table\(\s*"` + q + `"\s*\)` +
			`|\bdb\.` + q + `\b)`)
	}
	counts := map[string]int{}
	_ = filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && p != repo) {
				return filepath.SkipDir
			}
			return nil
		}
		if !codeExt[filepath.Ext(p)] {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > 1<<20 {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for t, re := range res {
			counts[t] += len(re.FindAllIndex(data, -1))
		}
		return nil
	})
	var found []string
	for t, n := range counts {
		if n > 0 {
			found = append(found, t)
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if counts[found[i]] != counts[found[j]] {
			return counts[found[i]] > counts[found[j]]
		}
		return found[i] < found[j]
	})
	out := make([]string, len(found))
	for i, t := range found {
		out[i] = fmt.Sprintf("`%s` (%d)", t, counts[t])
	}
	return out
}

// sharedDocsFor returns the shared database docs a service's database.md links to.
func (w *Workspace) sharedDocsFor(svc string) []string {
	data, err := os.ReadFile(w.ctxPath(svc, "database.md"))
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, g := range reSharedLink.FindAllStringSubmatch(string(data), -1) {
		if !seen[g[1]] {
			seen[g[1]] = true
			out = append(out, g[1])
		}
	}
	return out
}

// checkMarkers refuses to edit a doc whose generated block lost one of its markers,
// instead of guessing where the block ends and duplicating or eating human text.
func checkMarkers(doc, start, end, name string) error {
	prefix := start[:min(30, len(start))]
	hasStart, hasEnd := strings.Count(doc, prefix), strings.Count(doc, end)
	switch {
	case hasStart == 0 && hasEnd == 0:
		return nil
	case hasStart == 1 && hasEnd == 1 && strings.Index(doc, prefix) < strings.Index(doc, end):
		return nil
	default:
		return fmt.Errorf("%s has a broken generated block (%d start and %d end markers for %q). Restore the missing marker or delete the whole block, then sync again", name, hasStart, hasEnd, strings.Fields(end)[0]+" "+strings.Fields(end)[1])
	}
}
