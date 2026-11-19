package dbschema

import (
	"fmt"
	"sort"
	"strings"
)

// Markers around the generated part of a database doc. Everything outside them is human-owned.
const (
	GenStart = "<!-- pointbreak:generated:start (sync_database rewrites everything up to the end marker; edit outside it) -->"
	GenEnd   = "<!-- pointbreak:generated:end -->"
)

// Render returns the generated block: Tables, Relations, Enums and status values, Migration log.
func (s *Schema) Render(today string) string {
	var b strings.Builder
	b.WriteString(GenStart + "\n")
	last := ""
	if n := len(s.Migrations); n > 0 {
		last = s.Migrations[n-1].File
	}
	fmt.Fprintf(&b, "## Tables\n_Generated from `%s` (%s, %d migration file(s), last `%s`) on %s._\n\n", s.Dir, s.Tool, len(s.Migrations), last, today)

	if len(s.TableOrder) == 0 {
		b.WriteString("(no tables found)\n\n")
	}
	fksByCol := map[string]FK{}
	for _, f := range s.FKs {
		fksByCol[f.Table+"."+strings.ToLower(f.Column)] = f
	}
	for _, name := range s.TableOrder {
		t := s.Tables[name]
		fmt.Fprintf(&b, "### `%s`\n\n| Column | Type | Null | Default | Notes |\n|---|---|---|---|---|\n", name)
		for _, c := range t.Columns {
			var notes []string
			if c.PK {
				notes = append(notes, "primary key")
			}
			if c.Unique {
				notes = append(notes, "unique")
			}
			if f, ok := fksByCol[name+"."+strings.ToLower(c.Name)]; ok {
				n := fmt.Sprintf("FK → `%s.%s`", f.RefTable, f.RefColumn)
				if f.OnDelete != "" {
					n += ", on delete " + f.OnDelete
				}
				notes = append(notes, n)
			}
			notes = append(notes, c.Notes...)
			null := "yes"
			if !c.Nullable {
				null = "no"
			}
			fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s | %s |\n", c.Name, cell(c.Type), null, codeOrEmpty(c.Default), cell(strings.Join(notes, "; ")))
		}
		var extra []string
		if len(t.PK) > 1 {
			extra = append(extra, "Primary key: `("+strings.Join(t.PK, ", ")+")`")
		}
		var idx []string
		for _, in := range s.IndexOrder {
			ix := s.Indexes[in]
			if ix.Table != name {
				continue
			}
			d := fmt.Sprintf("`%s (%s)`", ix.Name, strings.Join(ix.Columns, ", "))
			if ix.Unique {
				d = "unique " + d
			}
			if ix.Where != "" {
				d += " where " + ix.Where
			}
			idx = append(idx, d)
		}
		if len(idx) > 0 {
			extra = append(extra, "Indexes: "+strings.Join(idx, " · "))
		}
		for _, u := range t.Uniques {
			extra = append(extra, "Unique: `("+strings.Join(u, ", ")+")`")
		}
		for _, c := range t.Checks {
			extra = append(extra, "Check: `"+cell(c)+"`")
		}
		if len(extra) > 0 {
			b.WriteString("\n" + strings.Join(extra, "  \n") + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Relations\n")
	if len(s.FKs) == 0 {
		b.WriteString("No foreign keys in the migrations.\n\n")
	} else {
		fks := append([]FK{}, s.FKs...)
		sort.SliceStable(fks, func(i, j int) bool { return fks[i].Table < fks[j].Table })
		for _, f := range fks {
			od := ""
			if f.OnDelete != "" {
				od = " (on delete " + f.OnDelete + ")"
			}
			fmt.Fprintf(&b, "- `%s.%s` → `%s.%s`%s\n", f.Table, f.Column, f.RefTable, f.RefColumn, od)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Enums and status values\n")
	type ev struct{ name, vals string }
	var evs []ev
	for _, n := range s.EnumOrder {
		evs = append(evs, ev{"`" + n + "` (type)", strings.Join(s.Enums[n], ", ")})
	}
	for _, tn := range s.TableOrder {
		for _, c := range s.Tables[tn].Columns {
			for _, note := range c.Notes {
				if strings.HasPrefix(note, "one of: ") {
					evs = append(evs, ev{"`" + tn + "." + c.Name + "` (check)", strings.TrimPrefix(note, "one of: ")})
				}
			}
		}
	}
	if len(evs) == 0 {
		b.WriteString("None found in the migrations.\n\n")
	} else {
		b.WriteString("| Type / column | Values |\n|---|---|\n")
		for _, e := range evs {
			fmt.Fprintf(&b, "| %s | %s |\n", e.name, cell(e.vals))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Migration log\nNewest first.\n\n| Migration | Date | Change |\n|---|---|---|\n")
	for i := len(s.Migrations) - 1; i >= 0; i-- {
		m := s.Migrations[i]
		ch := strings.Join(dedupe(m.Changes), "; ")
		if ch == "" {
			ch = "no schema change found"
		}
		date := m.Date
		if date == "" {
			date = "—"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", strings.TrimSuffix(strings.TrimSuffix(m.File, ".sql"), ".up"), date, cell(ch))
	}
	if len(s.Skipped) > 0 {
		fmt.Fprintf(&b, "\n_Not understood, check by hand (unverified): %d statement(s)._\n", len(s.Skipped))
		for i, sk := range s.Skipped {
			if i == 10 {
				fmt.Fprintf(&b, "- … and %d more\n", len(s.Skipped)-10)
				break
			}
			fmt.Fprintf(&b, "- `%s`\n", cell(sk))
		}
	}
	b.WriteString(GenEnd + "\n")
	return b.String()
}

func cell(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ") }

func codeOrEmpty(s string) string {
	if s == "" {
		return ""
	}
	return "`" + cell(s) + "`"
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Summary is a one-line count for tool replies.
func (s *Schema) Summary() string {
	return fmt.Sprintf("%d tables, %d indexes, %d enums, %d relations from %d migration file(s); %d statement(s) not understood",
		len(s.TableOrder), len(s.IndexOrder), len(s.EnumOrder), len(s.FKs), len(s.Migrations), len(s.Skipped))
}
