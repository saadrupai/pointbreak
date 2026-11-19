package dbschema

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	rePrismaBlock = regexp.MustCompile(`(?s)\b(model|enum)\s+(\w+)\s*\{(.*?)\n\}`)
	rePrismaMap   = regexp.MustCompile(`@@map\(\s*"([^"]+)"\s*\)`)
	reFieldMap    = regexp.MustCompile(`@map\(\s*"([^"]+)"\s*\)`)
	reDefault     = regexp.MustCompile(`@default\((.*?)\)(?:\s|$)`)
	reDBType      = regexp.MustCompile(`@db\.(\w+(?:\([^)]*\))?)`)
	reRelation    = regexp.MustCompile(`@relation\(([^)]*)\)`)
	reRelFields   = regexp.MustCompile(`fields:\s*\[([^\]]*)\]`)
	reRelRefs     = regexp.MustCompile(`references:\s*\[([^\]]*)\]`)
	reRelOnDelete = regexp.MustCompile(`onDelete:\s*(\w+)`)
	reBlockAttr   = regexp.MustCompile(`@@(index|unique|id)\(\s*\[([^\]]*)\]`)
)

var prismaTypes = map[string]string{
	"String": "text", "Int": "integer", "BigInt": "bigint", "Boolean": "boolean", "DateTime": "timestamp",
	"Float": "double precision", "Decimal": "decimal", "Json": "jsonb", "Bytes": "bytea",
}

// ParsePrisma reads a Prisma schema into the same Schema model the SQL replay produces.
func ParsePrisma(src, label string) *Schema {
	s := New("prisma", label)
	blocks := rePrismaBlock.FindAllStringSubmatch(src, -1)
	models := map[string]string{} // model name → table name
	for _, b := range blocks {
		if b[1] == "enum" {
			var vals []string
			for _, ln := range strings.Split(b[3], "\n") {
				if f := strings.Fields(strings.TrimSpace(ln)); len(f) > 0 && !strings.HasPrefix(f[0], "//") && !strings.HasPrefix(f[0], "@@") {
					vals = append(vals, f[0])
				}
			}
			s.addEnum(b[2], vals)
			continue
		}
		table := b[2]
		if g := rePrismaMap.FindStringSubmatch(b[3]); g != nil {
			table = g[1]
		}
		models[b[2]] = table
	}
	m := Migration{File: label, Name: "Prisma schema"}
	for _, b := range blocks {
		if b[1] != "model" {
			continue
		}
		t := s.table(models[b[2]])
		colName := map[string]string{} // prisma field → column
		for _, ln := range strings.Split(b[3], "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" || strings.HasPrefix(ln, "//") {
				continue
			}
			if strings.HasPrefix(ln, "@@") {
				if g := reBlockAttr.FindStringSubmatch(ln); g != nil {
					cols := mapCols(strings.Split(g[2], ","), colName)
					switch g[1] {
					case "index":
						s.addIndex(&Index{Table: t.Name, Columns: cols})
					case "unique":
						s.addIndex(&Index{Table: t.Name, Columns: cols, Unique: true})
					case "id":
						t.PK = cols
					}
				}
				continue
			}
			f := strings.Fields(ln)
			if len(f) < 2 {
				continue
			}
			field, ptype := f[0], f[1]
			base := strings.TrimSuffix(strings.TrimSuffix(ptype, "?"), "[]")
			if _, isModel := models[base]; isModel {
				if r := reRelation.FindStringSubmatch(ln); r != nil {
					fs, rs := reRelFields.FindStringSubmatch(r[1]), reRelRefs.FindStringSubmatch(r[1])
					if fs != nil && rs != nil {
						from, to := strings.Split(fs[1], ","), strings.Split(rs[1], ",")
						for i := range from {
							if i < len(to) {
								od := ""
								if g := reRelOnDelete.FindStringSubmatch(r[1]); g != nil {
									od = strings.ToLower(g[1])
								}
								s.addFK(FK{Table: t.Name, Column: strings.TrimSpace(from[i]), RefTable: models[base], RefColumn: strings.TrimSpace(to[i]), OnDelete: od})
							}
						}
					}
				}
				continue // relation field, not a column
			}
			col := field
			if g := reFieldMap.FindStringSubmatch(ln); g != nil {
				col = g[1]
			}
			colName[field] = col
			typ := prismaTypes[base]
			if _, isEnum := s.Enums[base]; isEnum {
				typ = base
			}
			if g := reDBType.FindStringSubmatch(ln); g != nil {
				typ = strings.ToLower(g[1])
			}
			if typ == "" {
				typ = strings.ToLower(base)
			}
			if strings.HasSuffix(ptype, "[]") {
				typ += "[]"
			}
			c := &Column{Name: col, Type: typ, Nullable: strings.HasSuffix(ptype, "?")}
			if strings.Contains(ln, "@id") {
				c.PK, c.Nullable = true, false
				t.PK = []string{col}
			}
			if strings.Contains(ln, "@unique") {
				c.Unique = true
			}
			if g := reDefault.FindStringSubmatch(ln); g != nil {
				c.Default = g[1]
				if g[1] == "autoincrement()" {
					c.Notes = append(c.Notes, "auto-increment")
				}
			}
			if strings.Contains(ln, "@updatedAt") {
				c.Notes = append(c.Notes, "set on update")
			}
			t.Columns = append(t.Columns, c)
		}
		// fix FK columns that used Prisma field names
		for i := range s.FKs {
			if s.FKs[i].Table == t.Name {
				if c, ok := colName[s.FKs[i].Column]; ok {
					s.FKs[i].Column = c
				}
			}
		}
		m.Changes = append(m.Changes, fmt.Sprintf("model `%s`", t.Name))
	}
	s.Migrations = []Migration{m}
	return s
}

func mapCols(raw []string, names map[string]string) []string {
	var out []string
	for _, r := range raw {
		r = strings.TrimSpace(strings.Split(strings.TrimSpace(r), "(")[0])
		if r == "" {
			continue
		}
		if c, ok := names[r]; ok {
			r = c
		}
		out = append(out, r)
	}
	return out
}
