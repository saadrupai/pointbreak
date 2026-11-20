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
	"src/main/resources/db/migration", "db/migration", // Flyway defaults
}

// PrismaCandidates are where a Prisma schema usually lives.
var PrismaCandidates = []string{"prisma/schema.prisma", "schema.prisma"}

const maxFile = 4 << 20

var (
	reMigFile = regexp.MustCompile(`^(\d+)[_-](.+?)(\.up)?\.sql$`)
	reFlywayV = regexp.MustCompile(`^V(\d+(?:[._]\d+)*)__(.+)\.sql$`)
	reFlywayR = regexp.MustCompile(`^R__(.+)\.sql$`)
	reFlywayU = regexp.MustCompile(`^U\d+(?:[._]\d+)*__.+\.sql$`)
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
		key                  []uint64 // version parts, compared in order
		repeatable           bool     // Flyway R__ files apply after all versioned ones
	}
	var files []file
	tool := "sql"
	goose := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".down.sql") || reFlywayU.MatchString(name) {
			continue
		}
		f := file{name: name, title: strings.TrimSuffix(name, ".sql")}
		switch {
		case reFlywayV.MatchString(name):
			g := reFlywayV.FindStringSubmatch(name)
			f.version, f.title, f.key = g[1], g[2], versionKey(g[1])
			tool = "flyway"
		case reFlywayR.MatchString(name):
			f.title, f.repeatable = reFlywayR.FindStringSubmatch(name)[1], true
			tool = "flyway"
		case reMigFile.MatchString(name):
			g := reMigFile.FindStringSubmatch(name)
			f.version, f.title, f.key = g[1], g[2], versionKey(g[1])
			if g[3] != "" && tool != "flyway" {
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
		a, b := files[i], files[j]
		if a.repeatable != b.repeatable {
			return b.repeatable
		}
		if len(a.key) > 0 && len(b.key) > 0 {
			if c := compareKeys(a.key, b.key); c != 0 {
				return c < 0
			}
		}
		return a.name < b.name
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

// versionKey splits "1.10_2" into [1 10 2] so versions compare numerically part by part.
func versionKey(v string) []uint64 {
	var key []uint64
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '_' }) {
		n, _ := strconv.ParseUint(p, 10, 64)
		key = append(key, n)
	}
	return key
}

func compareKeys(a, b []uint64) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return len(a) - len(b)
}
