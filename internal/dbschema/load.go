package dbschema

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Candidates are the places migrations usually live, relative to a repo root.
var Candidates = []string{
	"migrations", "db/migrations", "database/migrations", "sql/migrations", "migration",
	"internal/db/migrations", "internal/database/migrations", "internal/migrations",
	"pkg/db/migrations", "schema/migrations", "db/migrate/sql", "sql",
}

// PrismaCandidates are where a Prisma schema usually lives.
var PrismaCandidates = []string{"prisma/schema.prisma", "schema.prisma"}

const maxFile = 4 << 20

var (
	reMigFile = regexp.MustCompile(`^(\d+)[_-](.+?)(\.up)?\.sql$`)
	reGooseUp = regexp.MustCompile(`(?i)--\s*\+goose\s+up`)
	reGooseDn = regexp.MustCompile(`(?i)--\s*\+goose\s+down`)
	reGooseSB = regexp.MustCompile(`(?im)^\s*--\s*\+goose\s+statement(begin|end)\s*$`)
)

// Detect finds a migrations folder or Prisma schema under base (a repo or folder path).
// It returns the path relative to base ("." if base itself holds the migrations).
func Detect(base string) (string, error) {
	if hasSQL(base) {
		return ".", nil
	}
	for _, c := range PrismaCandidates {
		if st, err := os.Stat(filepath.Join(base, c)); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	for _, c := range Candidates {
		if hasSQL(filepath.Join(base, c)) {
			return c, nil
		}
	}
	return "", fmt.Errorf("no migrations found in %s (looked in %s and %s); pass the folder explicitly", base, strings.Join(Candidates, ", "), strings.Join(PrismaCandidates, ", "))
}

func hasSQL(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			return true
		}
	}
	return false
}

// Load reads migrations at path (a folder of .sql files or a schema.prisma file) and replays them.
// label is how the source is shown in the docs (usually the path relative to the workspace root).
func Load(path, label string) (*Schema, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		if strings.HasSuffix(path, ".prisma") {
			data, err := readSmall(path)
			if err != nil {
				return nil, err
			}
			s := ParsePrisma(string(data), label)
			return s, nil
		}
		return nil, fmt.Errorf("%s is not a migrations folder or a .prisma file", label)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	type file struct {
		name, version, title string
		n                    uint64
	}
	var files []file
	tool := "sql"
	goose := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".down.sql") {
			continue
		}
		f := file{name: name, title: strings.TrimSuffix(name, ".sql")}
		if g := reMigFile.FindStringSubmatch(name); g != nil {
			f.version, f.title = g[1], g[2]
			f.n, _ = strconv.ParseUint(g[1], 10, 64)
			if g[3] != "" {
				tool = "golang-migrate"
			}
		}
		if !goose {
			if data, err := readSmall(filepath.Join(path, name)); err == nil && reGooseUp.Match(data) {
				goose = true
			}
		}
		files = append(files, f)
	}
	if goose {
		tool = "goose"
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .sql migrations in %s", label)
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].version != "" && files[j].version != "" && files[i].n != files[j].n {
			return files[i].n < files[j].n
		}
		return files[i].name < files[j].name
	})

	s := New(tool, label)
	for _, f := range files {
		data, err := readSmall(filepath.Join(path, f.name))
		if err != nil {
			return nil, err
		}
		sql := string(data)
		if tool == "goose" {
			sql = gooseUp(sql)
		}
		m := Migration{File: f.name, Version: f.version, Name: strings.ReplaceAll(f.title, "_", " "), Date: dateFromVersion(f.version)}
		for _, stmt := range SplitStatements(sql) {
			s.Apply(stmt, &m)
		}
		s.Migrations = append(s.Migrations, m)
	}
	return s, nil
}

func readSmall(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > maxFile {
		return nil, errors.New(filepath.Base(path) + " is larger than 4 MB; skipped")
	}
	return os.ReadFile(path)
}

// gooseUp keeps only the Up section of a goose migration.
func gooseUp(sql string) string {
	if loc := reGooseUp.FindStringIndex(sql); loc != nil {
		sql = sql[loc[1]:]
	}
	if loc := reGooseDn.FindStringIndex(sql); loc != nil {
		sql = sql[:loc[0]]
	}
	return reGooseSB.ReplaceAllString(sql, "")
}

// dateFromVersion turns a timestamp-style version (20261003…) into 2026-10-03.
func dateFromVersion(v string) string {
	if len(v) < 8 || !strings.HasPrefix(v, "20") {
		return ""
	}
	y, m, d := v[0:4], v[4:6], v[6:8]
	if m < "01" || m > "12" || d < "01" || d > "31" {
		return ""
	}
	return y + "-" + m + "-" + d
}
