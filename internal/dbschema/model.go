// Package dbschema rebuilds a database schema by replaying migrations (golang-migrate, goose,
// plain SQL) or reading a Prisma schema, and renders it as markdown. It only reads files;
// it never connects to a database.
package dbschema

import "strings"

// Column is one table column as it stands after all migrations.
type Column struct {
	Name     string
	Type     string
	Nullable bool
	Default  string
	PK       bool
	Unique   bool
	Notes    []string
}

// Table is one table and its table-level constraints.
type Table struct {
	Name    string
	Columns []*Column
	PK      []string
	Uniques [][]string
	Checks  []string
}

// Index is a secondary index.
type Index struct {
	Name    string
	Table   string
	Columns []string
	Unique  bool
	Where   string
}

// FK is a foreign key from Table.Column to RefTable.RefColumn.
type FK struct {
	Table, Column, RefTable, RefColumn, OnDelete string
}

// Migration is one migration file and a short list of what it changed.
type Migration struct {
	File, Version, Name, Date string
	Changes                   []string
}

// Schema is the replayed state.
type Schema struct {
	Tool       string // golang-migrate | goose | sql | prisma
	Dir        string // where the migrations were read from, relative to the workspace root
	Tables     map[string]*Table
	TableOrder []string
	Indexes    map[string]*Index
	IndexOrder []string
	Enums      map[string][]string
	EnumOrder  []string
	FKs        []FK
	Migrations []Migration
	Skipped    []string // statements not understood: "file: first words"
}

// New returns an empty schema.
func New(tool, dir string) *Schema {
	return &Schema{Tool: tool, Dir: dir, Tables: map[string]*Table{}, Indexes: map[string]*Index{}, Enums: map[string][]string{}}
}

// TableNames returns the tables in creation order.
func (s *Schema) TableNames() []string { return append([]string{}, s.TableOrder...) }

func (s *Schema) table(name string) *Table {
	if t, ok := s.Tables[name]; ok {
		return t
	}
	t := &Table{Name: name}
	s.Tables[name] = t
	s.TableOrder = append(s.TableOrder, name)
	return t
}

func (s *Schema) dropTable(name string) {
	if _, ok := s.Tables[name]; !ok {
		return
	}
	delete(s.Tables, name)
	s.TableOrder = remove(s.TableOrder, name)
	var fks []FK
	for _, f := range s.FKs {
		if f.Table != name {
			fks = append(fks, f)
		}
	}
	s.FKs = fks
	for _, n := range append([]string{}, s.IndexOrder...) {
		if s.Indexes[n].Table == name {
			s.dropIndex(n)
		}
	}
}

func (s *Schema) renameTable(from, to string) {
	t, ok := s.Tables[from]
	if !ok {
		return
	}
	t.Name = to
	delete(s.Tables, from)
	s.Tables[to] = t
	for i, n := range s.TableOrder {
		if n == from {
			s.TableOrder[i] = to
		}
	}
	for i := range s.FKs {
		if s.FKs[i].Table == from {
			s.FKs[i].Table = to
		}
		if s.FKs[i].RefTable == from {
			s.FKs[i].RefTable = to
		}
	}
	for _, ix := range s.Indexes {
		if ix.Table == from {
			ix.Table = to
		}
	}
}

func (s *Schema) renameColumn(table, from, to string) {
	t, ok := s.Tables[table]
	if !ok {
		return
	}
	if c := t.col(from); c != nil {
		c.Name = to
	}
	for i := range s.FKs {
		if s.FKs[i].Table == table && strings.EqualFold(s.FKs[i].Column, from) {
			s.FKs[i].Column = to
		}
		if s.FKs[i].RefTable == table && strings.EqualFold(s.FKs[i].RefColumn, from) {
			s.FKs[i].RefColumn = to
		}
	}
	for _, ix := range s.Indexes {
		if ix.Table == table {
			for i, c := range ix.Columns {
				if strings.EqualFold(c, from) {
					ix.Columns[i] = to
				}
			}
		}
	}
}

func (s *Schema) dropColumn(table, col string) {
	t, ok := s.Tables[table]
	if !ok {
		return
	}
	t.dropCol(col)
	var fks []FK
	for _, f := range s.FKs {
		if !(f.Table == table && strings.EqualFold(f.Column, col)) {
			fks = append(fks, f)
		}
	}
	s.FKs = fks
}

func (s *Schema) addIndex(ix *Index) {
	if ix.Name == "" {
		ix.Name = ix.Table + "_" + strings.Join(ix.Columns, "_") + "_idx"
	}
	if _, ok := s.Indexes[ix.Name]; !ok {
		s.IndexOrder = append(s.IndexOrder, ix.Name)
	}
	s.Indexes[ix.Name] = ix
}

func (s *Schema) dropIndex(name string) {
	if _, ok := s.Indexes[name]; !ok {
		return
	}
	delete(s.Indexes, name)
	s.IndexOrder = remove(s.IndexOrder, name)
}

func (s *Schema) addEnum(name string, values []string) {
	if _, ok := s.Enums[name]; !ok {
		s.EnumOrder = append(s.EnumOrder, name)
	}
	s.Enums[name] = values
}

func (s *Schema) addFK(f FK) {
	for _, x := range s.FKs {
		if x.Table == f.Table && strings.EqualFold(x.Column, f.Column) && x.RefTable == f.RefTable {
			return
		}
	}
	s.FKs = append(s.FKs, f)
}

func (t *Table) col(name string) *Column {
	for _, c := range t.Columns {
		if strings.EqualFold(c.Name, name) {
			return c
		}
	}
	return nil
}

func (t *Table) dropCol(name string) {
	var keep []*Column
	for _, c := range t.Columns {
		if !strings.EqualFold(c.Name, name) {
			keep = append(keep, c)
		}
	}
	t.Columns = keep
}

func remove(list []string, v string) []string {
	var out []string
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
