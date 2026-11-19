package dbschema

import (
	"fmt"
	"regexp"
	"strings"
)

// SplitStatements splits SQL into statements on top-level semicolons, dropping comments and
// respecting quotes and dollar-quoted bodies ($$ … $$, $tag$ … $tag$).
func SplitStatements(sql string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, s)
		}
		b.Reset()
	}
	r := []rune(sql)
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case c == '-' && i+1 < len(r) && r[i+1] == '-':
			for i < len(r) && r[i] != '\n' {
				i++
			}
			b.WriteRune(' ')
		case c == '/' && i+1 < len(r) && r[i+1] == '*':
			i += 2
			for i+1 < len(r) && !(r[i] == '*' && r[i+1] == '/') {
				i++
			}
			i++
			b.WriteRune(' ')
		case c == '\'' || c == '"' || c == '`':
			b.WriteRune(c)
			for i++; i < len(r); i++ {
				b.WriteRune(r[i])
				if r[i] == c {
					if i+1 < len(r) && r[i+1] == c { // doubled quote escape
						i++
						b.WriteRune(r[i])
						continue
					}
					break
				}
			}
		case c == '$':
			j := i + 1
			for j < len(r) && (r[j] == '_' || isAlnum(r[j])) {
				j++
			}
			if j < len(r) && r[j] == '$' {
				tag := string(r[i : j+1])
				end := strings.Index(string(r[j+1:]), tag)
				if end < 0 {
					b.WriteString(string(r[i:]))
					i = len(r)
					break
				}
				stop := j + 1 + len([]rune(string(r[j+1:])[:end])) + len([]rune(tag))
				b.WriteString(string(r[i:stop]))
				i = stop - 1
				break
			}
			b.WriteRune(c)
		case c == ';':
			flush()
		default:
			b.WriteRune(c)
		}
	}
	flush()
	return out
}

func isAlnum(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

var spaceRE = regexp.MustCompile(`\s+`)

func norm(s string) string { return strings.TrimSpace(spaceRE.ReplaceAllString(s, " ")) }

// ident removes quotes and a public. schema prefix.
func ident(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimSuffix(s, ","), "(")
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = strings.Trim(p, "\"`[]")
	}
	if len(parts) == 2 && strings.EqualFold(parts[0], "public") {
		return parts[1]
	}
	return strings.Join(parts, ".")
}

// splitTop splits on commas outside parentheses and quotes.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	var quote rune
	for i, c := range s {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		out = append(out, t)
	}
	return out
}

// parenBody returns the text inside the first top-level (…) at or after from, and the index after it.
func parenBody(s string, from int) (string, int, bool) {
	open := strings.IndexByte(s[from:], '(')
	if open < 0 {
		return "", 0, false
	}
	open += from
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return s[open+1 : i], i + 1, true
			}
		}
	}
	return "", 0, false
}

func identList(s string) []string {
	var out []string
	for _, p := range splitTop(s) {
		f := strings.Fields(p)
		if len(f) > 0 {
			out = append(out, ident(f[0]))
		}
	}
	return out
}

var (
	reCreateTable = regexp.MustCompile(`(?i)^create\s+(?:(?:global\s+|local\s+)?(?:temporary|temp)\s+|unlogged\s+)?table\s+(?:if\s+not\s+exists\s+)?([^\s(]+)`)
	reAlterTable  = regexp.MustCompile(`(?i)^alter\s+table\s+(?:if\s+exists\s+)?(?:only\s+)?(\S+)\s+(.*)$`)
	reDropTable   = regexp.MustCompile(`(?i)^drop\s+table\s+(?:if\s+exists\s+)?(.+?)(?:\s+(?:cascade|restrict))?$`)
	reCreateIndex = regexp.MustCompile(`(?i)^create\s+(unique\s+)?index\s+(?:concurrently\s+)?(?:if\s+not\s+exists\s+)?(\S+\s+)?on\s+(?:only\s+)?(\S+?)\s*(?:using\s+\w+\s*)?\(`)
	reDropIndex   = regexp.MustCompile(`(?i)^drop\s+index\s+(?:concurrently\s+)?(?:if\s+exists\s+)?(.+?)(?:\s+(?:cascade|restrict))?$`)
	reCreateEnum  = regexp.MustCompile(`(?i)^create\s+type\s+(\S+)\s+as\s+enum\s*\((.*)\)$`)
	reEnumAdd     = regexp.MustCompile(`(?i)^alter\s+type\s+(\S+)\s+add\s+value\s+(?:if\s+not\s+exists\s+)?'([^']*)'(?:\s+(before|after)\s+'([^']*)')?$`)
	reEnumRename  = regexp.MustCompile(`(?i)^alter\s+type\s+(\S+)\s+rename\s+value\s+'([^']*)'\s+to\s+'([^']*)'$`)
	reDropType    = regexp.MustCompile(`(?i)^drop\s+type\s+(?:if\s+exists\s+)?(\S+)`)
	reRenameTable = regexp.MustCompile(`(?i)^rename\s+table\s+(\S+)\s+to\s+(\S+)$`) // MySQL
	reIgnore      = regexp.MustCompile(`(?i)^(begin|commit|rollback|start\s+transaction|set\s|grant\s|revoke\s|comment\s+on|create\s+(or\s+replace\s+)?(function|procedure|trigger|view|materialized\s+view|extension|schema|sequence|policy|rule)|drop\s+(function|procedure|trigger|view|materialized\s+view|extension|schema|sequence|policy)|alter\s+(sequence|function|view|schema|extension|default\s+privileges)|do\s|select\s|analyze|vacuum|lock\s|refresh\s|use\s|pragma\s)`)
	reData        = regexp.MustCompile(`(?i)^(insert\s+into|update|delete\s+from|truncate)\s+(?:table\s+)?(\S+)`)
	reInValues    = regexp.MustCompile(`(?i)^\(?\s*"?(\w+)"?\s+in\s*\((.*)\)\s*\)?$`)
	reAnyArray    = regexp.MustCompile(`(?i)^\(?\s*"?(\w+)"?(?:::\w+)?\s*=\s*any\s*\(\s*(?:\(\s*)?array\s*\[(.*)\]`)
	reQuoted      = regexp.MustCompile(`'([^']*)'`)
	reRefs        = regexp.MustCompile(`(?i)references\s+([^\s(]+)\s*(?:\(([^)]*)\))?(.*)`)
	reOnDelete    = regexp.MustCompile(`(?i)on\s+delete\s+(cascade|set\s+null|set\s+default|restrict|no\s+action)`)
	reKeyword     = regexp.MustCompile(`(?i)\b(not\s+null|null|default|primary\s+key|unique|references|check|constraint|generated|collate|auto_increment|identity|comment|on\s+update)\b`)
)

// Apply replays one statement onto the schema and records what it changed in m.
func (s *Schema) Apply(stmt string, m *Migration) {
	st := norm(stmt)
	if st == "" {
		return
	}
	switch {
	case reIgnore.MatchString(st):
		return
	case reCreateTable.MatchString(st):
		s.createTable(st, m)
	case reAlterTable.MatchString(st):
		s.alterTable(st, m)
	case reRenameTable.MatchString(st):
		g := reRenameTable.FindStringSubmatch(st)
		s.renameTable(ident(g[1]), ident(g[2]))
		m.Changes = append(m.Changes, fmt.Sprintf("renames table `%s` to `%s`", ident(g[1]), ident(g[2])))
	case reDropTable.MatchString(st):
		for _, n := range splitTop(reDropTable.FindStringSubmatch(st)[1]) {
			s.dropTable(ident(n))
			m.Changes = append(m.Changes, fmt.Sprintf("drops table `%s`", ident(n)))
		}
	case reCreateIndex.MatchString(st):
		s.createIndex(st, m)
	case reDropIndex.MatchString(st):
		for _, n := range splitTop(reDropIndex.FindStringSubmatch(st)[1]) {
			s.dropIndex(ident(n))
			m.Changes = append(m.Changes, fmt.Sprintf("drops index `%s`", ident(n)))
		}
	case reCreateEnum.MatchString(st):
		g := reCreateEnum.FindStringSubmatch(st)
		s.addEnum(ident(g[1]), quotedValues(g[2]))
		m.Changes = append(m.Changes, fmt.Sprintf("creates enum `%s`", ident(g[1])))
	case reEnumAdd.MatchString(st):
		g := reEnumAdd.FindStringSubmatch(st)
		name := ident(g[1])
		vals := s.Enums[name]
		pos := len(vals)
		if g[3] != "" {
			for i, v := range vals {
				if v == g[4] {
					pos = i
					if strings.EqualFold(g[3], "after") {
						pos = i + 1
					}
				}
			}
		}
		vals = append(vals[:pos:pos], append([]string{g[2]}, vals[pos:]...)...)
		s.addEnum(name, vals)
		m.Changes = append(m.Changes, fmt.Sprintf("adds `%s` to enum `%s`", g[2], name))
	case reEnumRename.MatchString(st):
		g := reEnumRename.FindStringSubmatch(st)
		name := ident(g[1])
		for i, v := range s.Enums[name] {
			if v == g[2] {
				s.Enums[name][i] = g[3]
			}
		}
		m.Changes = append(m.Changes, fmt.Sprintf("renames enum value `%s` to `%s` in `%s`", g[2], g[3], name))
	case reDropType.MatchString(st):
		name := ident(reDropType.FindStringSubmatch(st)[1])
		if _, ok := s.Enums[name]; ok {
			delete(s.Enums, name)
			s.EnumOrder = remove(s.EnumOrder, name)
			m.Changes = append(m.Changes, fmt.Sprintf("drops enum `%s`", name))
		}
	case reData.MatchString(st):
		g := reData.FindStringSubmatch(st)
		m.Changes = append(m.Changes, fmt.Sprintf("changes data in `%s`", ident(g[2])))
	default:
		s.skip(m, st)
	}
}

func (s *Schema) skip(m *Migration, st string) {
	words := strings.Fields(st)
	if len(words) > 6 {
		words = words[:6]
	}
	s.Skipped = append(s.Skipped, fmt.Sprintf("%s: %s …", m.File, strings.Join(words, " ")))
}

func quotedValues(s string) []string {
	var out []string
	for _, g := range reQuoted.FindAllStringSubmatch(s, -1) {
		out = append(out, g[1])
	}
	return out
}

func (s *Schema) createTable(st string, m *Migration) {
	g := reCreateTable.FindStringSubmatch(st)
	name := ident(g[1])
	body, _, ok := parenBody(st, len(g[0]))
	if !ok {
		if strings.Contains(strings.ToLower(st), " as select") || strings.Contains(strings.ToLower(st), " like ") {
			s.table(name)
			m.Changes = append(m.Changes, fmt.Sprintf("creates table `%s` (columns not parsed)", name))
			return
		}
		s.skip(m, st)
		return
	}
	s.dropTable(name) // CREATE TABLE replaces any earlier definition in replay
	t := s.table(name)
	for _, el := range splitTop(body) {
		s.tableElement(t, el)
	}
	m.Changes = append(m.Changes, fmt.Sprintf("creates table `%s`", name))
}

// tableElement handles one item inside CREATE TABLE ( … ) or after ALTER TABLE … ADD.
func (s *Schema) tableElement(t *Table, el string) {
	lower := strings.ToLower(el)
	if strings.HasPrefix(lower, "constraint ") {
		f := strings.Fields(el)
		if len(f) < 3 {
			return
		}
		el = strings.Join(f[2:], " ")
		lower = strings.ToLower(el)
	}
	switch {
	case strings.HasPrefix(lower, "primary key"):
		if body, _, ok := parenBody(el, 0); ok {
			t.PK = identList(body)
			for _, c := range t.PK {
				if col := t.col(c); col != nil {
					col.PK, col.Nullable = true, false
				}
			}
		}
	case strings.HasPrefix(lower, "unique"):
		if body, _, ok := parenBody(el, 0); ok {
			cols := identList(body)
			if len(cols) == 1 {
				if col := t.col(cols[0]); col != nil {
					col.Unique = true
					return
				}
			}
			t.Uniques = append(t.Uniques, cols)
		}
	case strings.HasPrefix(lower, "foreign key"):
		body, end, ok := parenBody(el, 0)
		if !ok {
			return
		}
		cols := identList(body)
		if r := reRefs.FindStringSubmatch(el[end:]); r != nil {
			refCols := identList(r[2])
			for i, c := range cols {
				rc := "id"
				if i < len(refCols) {
					rc = refCols[i]
				}
				s.addFK(FK{Table: t.Name, Column: c, RefTable: ident(r[1]), RefColumn: rc, OnDelete: onDelete(r[3])})
			}
		}
	case strings.HasPrefix(lower, "check"):
		body, _, ok := parenBody(el, 0)
		if !ok {
			return
		}
		if col, vals := checkValues(body); col != "" {
			if c := t.col(col); c != nil {
				c.Notes = append(c.Notes, "one of: "+strings.Join(vals, ", "))
				return
			}
		}
		t.Checks = append(t.Checks, norm(body))
	case strings.HasPrefix(lower, "exclude"), strings.HasPrefix(lower, "like "), strings.HasPrefix(lower, "index "), strings.HasPrefix(lower, "key "), strings.HasPrefix(lower, "fulltext"):
		if strings.HasPrefix(lower, "index ") || strings.HasPrefix(lower, "key ") { // MySQL inline index
			f := strings.Fields(el)
			if body, _, ok := parenBody(el, 0); ok && len(f) > 1 {
				s.addIndex(&Index{Name: ident(f[1]), Table: t.Name, Columns: identList(body)})
			}
		}
	default:
		s.columnDef(t, el)
	}
}

func onDelete(s string) string {
	if g := reOnDelete.FindStringSubmatch(s); g != nil {
		return strings.ToLower(norm(g[1]))
	}
	return ""
}

func checkValues(expr string) (string, []string) {
	e := norm(expr)
	if g := reInValues.FindStringSubmatch(e); g != nil {
		if v := quotedValues(g[2]); len(v) > 0 {
			return g[1], v
		}
	}
	if g := reAnyArray.FindStringSubmatch(e); g != nil {
		if v := quotedValues(g[2]); len(v) > 0 {
			return g[1], v
		}
	}
	return "", nil
}

// columnDef parses "name type [constraints…]" and adds or replaces the column.
func (s *Schema) columnDef(t *Table, el string) *Column {
	f := strings.Fields(el)
	if len(f) == 0 {
		return nil
	}
	name := ident(f[0])
	rest := strings.TrimSpace(el[len(f[0]):])
	typ, cons := rest, ""
	if loc := topLevelKeyword(rest); loc >= 0 {
		typ, cons = strings.TrimSpace(rest[:loc]), rest[loc:]
	}
	c := &Column{Name: name, Type: strings.ToLower(norm(typ)), Nullable: true}
	s.applyConstraints(t, c, cons)
	if old := t.col(name); old != nil {
		*old = *c
		return old
	}
	t.Columns = append(t.Columns, c)
	if strings.Contains(c.Type, "serial") {
		c.Nullable = false
	}
	return c
}

// topLevelKeyword returns the index of the first constraint keyword outside parentheses.
func topLevelKeyword(s string) int {
	for _, loc := range reKeyword.FindAllStringIndex(s, -1) {
		if depthAt(s, loc[0]) == 0 {
			return loc[0]
		}
	}
	return -1
}

func depthAt(s string, pos int) int {
	d := 0
	var quote byte
	for i := 0; i < pos; i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			d++
		case c == ')':
			d--
		}
	}
	if quote != 0 {
		return 1
	}
	return d
}

func (s *Schema) applyConstraints(t *Table, c *Column, cons string) {
	if cons == "" {
		return
	}
	lower := strings.ToLower(cons)
	if strings.Contains(lower, "not null") {
		c.Nullable = false
	}
	if strings.Contains(lower, "primary key") {
		c.PK, c.Nullable = true, false
		t.PK = []string{c.Name}
	}
	if regexp.MustCompile(`(?i)\bunique\b`).MatchString(cons) && !c.PK {
		c.Unique = true
	}
	if i := regexp.MustCompile(`(?i)\bdefault\b`).FindStringIndex(cons); i != nil && depthAt(cons, i[0]) == 0 {
		rest := strings.TrimSpace(cons[i[1]:])
		end := len(rest)
		if loc := topLevelKeyword(rest); loc >= 0 {
			end = loc
		}
		c.Default = strings.TrimSpace(rest[:end])
	}
	if r := reRefs.FindStringSubmatch(cons); r != nil {
		rc := "id"
		if cols := identList(r[2]); len(cols) > 0 {
			rc = cols[0]
		}
		fk := FK{Table: t.Name, Column: c.Name, RefTable: ident(r[1]), RefColumn: rc, OnDelete: onDelete(r[3])}
		s.addFK(fk)
	}
	if i := regexp.MustCompile(`(?i)\bcheck\b`).FindStringIndex(cons); i != nil {
		if body, _, ok := parenBody(cons, i[1]); ok {
			if col, vals := checkValues(body); col != "" {
				c.Notes = append(c.Notes, "one of: "+strings.Join(vals, ", "))
			} else {
				c.Notes = append(c.Notes, "check "+norm(body))
			}
		}
	}
	if regexp.MustCompile(`(?i)generated\s+(always|by\s+default)\s+as\s+identity|auto_increment`).MatchString(cons) {
		c.Notes = append(c.Notes, "auto-increment")
		c.Nullable = false
	}
}

func (s *Schema) alterTable(st string, m *Migration) {
	g := reAlterTable.FindStringSubmatch(st)
	name := ident(g[1])
	t := s.table(name)
	for _, act := range splitTop(g[2]) {
		s.alterAction(t, act, m)
	}
}

var (
	reAddCol     = regexp.MustCompile(`(?i)^add\s+(?:column\s+)?(?:if\s+not\s+exists\s+)?(.+)$`)
	reDropCol    = regexp.MustCompile(`(?i)^drop\s+(?:column\s+)?(?:if\s+exists\s+)?(\S+)(?:\s+(?:cascade|restrict))?$`)
	reDropCons   = regexp.MustCompile(`(?i)^drop\s+(constraint|primary\s+key|foreign\s+key|index|key)\b`)
	reRenameCol  = regexp.MustCompile(`(?i)^rename\s+(?:column\s+)?(\S+)\s+to\s+(\S+)$`)
	reRenameTo   = regexp.MustCompile(`(?i)^rename\s+to\s+(\S+)$`)
	reAlterCol   = regexp.MustCompile(`(?i)^alter\s+(?:column\s+)?(\S+)\s+(.+)$`)
	reModifyCol  = regexp.MustCompile(`(?i)^modify\s+(?:column\s+)?(.+)$`)
	reChangeCol  = regexp.MustCompile(`(?i)^change\s+(?:column\s+)?(\S+)\s+(.+)$`)
	reSetType    = regexp.MustCompile(`(?i)^(?:set\s+data\s+)?type\s+(.+?)(?:\s+using\s+.*)?$`)
	reSetDefault = regexp.MustCompile(`(?i)^set\s+default\s+(.+)$`)
)

func (s *Schema) alterAction(t *Table, act string, m *Migration) {
	lower := strings.ToLower(act)
	switch {
	case reRenameTo.MatchString(act):
		to := ident(reRenameTo.FindStringSubmatch(act)[1])
		from := t.Name
		s.renameTable(from, to)
		m.Changes = append(m.Changes, fmt.Sprintf("renames table `%s` to `%s`", from, to))
	case reRenameCol.MatchString(act) && !strings.HasPrefix(lower, "rename constraint"):
		g := reRenameCol.FindStringSubmatch(act)
		s.renameColumn(t.Name, ident(g[1]), ident(g[2]))
		m.Changes = append(m.Changes, fmt.Sprintf("renames `%s.%s` to `%s`", t.Name, ident(g[1]), ident(g[2])))
	case reDropCons.MatchString(act):
		m.Changes = append(m.Changes, fmt.Sprintf("drops a constraint on `%s`", t.Name))
	case reDropCol.MatchString(act):
		col := ident(reDropCol.FindStringSubmatch(act)[1])
		s.dropColumn(t.Name, col)
		m.Changes = append(m.Changes, fmt.Sprintf("drops `%s.%s`", t.Name, col))
	case reAddCol.MatchString(act):
		def := reAddCol.FindStringSubmatch(act)[1]
		dl := strings.ToLower(def)
		if strings.HasPrefix(dl, "constraint ") || strings.HasPrefix(dl, "primary key") || strings.HasPrefix(dl, "unique") || strings.HasPrefix(dl, "foreign key") || strings.HasPrefix(dl, "check") {
			s.tableElement(t, def)
			m.Changes = append(m.Changes, fmt.Sprintf("adds a constraint on `%s`", t.Name))
			return
		}
		if c := s.columnDef(t, def); c != nil {
			m.Changes = append(m.Changes, fmt.Sprintf("adds `%s.%s`", t.Name, c.Name))
		}
	case reModifyCol.MatchString(act):
		if c := s.columnDef(t, reModifyCol.FindStringSubmatch(act)[1]); c != nil {
			m.Changes = append(m.Changes, fmt.Sprintf("changes `%s.%s`", t.Name, c.Name))
		}
	case reChangeCol.MatchString(act):
		g := reChangeCol.FindStringSubmatch(act)
		old := ident(g[1])
		t.dropCol(old)
		if c := s.columnDef(t, g[2]); c != nil {
			m.Changes = append(m.Changes, fmt.Sprintf("changes `%s.%s`", t.Name, c.Name))
		}
	case reAlterCol.MatchString(act):
		g := reAlterCol.FindStringSubmatch(act)
		c := t.col(ident(g[1]))
		if c == nil {
			s.skip(m, "ALTER TABLE "+t.Name+" "+act)
			return
		}
		op := strings.ToLower(g[2])
		switch {
		case strings.HasPrefix(op, "set not null"):
			c.Nullable = false
		case strings.HasPrefix(op, "drop not null"):
			c.Nullable = true
		case strings.HasPrefix(op, "drop default"):
			c.Default = ""
		case reSetDefault.MatchString(g[2]):
			c.Default = strings.TrimSpace(reSetDefault.FindStringSubmatch(g[2])[1])
		case reSetType.MatchString(g[2]):
			c.Type = strings.ToLower(norm(reSetType.FindStringSubmatch(g[2])[1]))
		default:
			s.skip(m, "ALTER TABLE "+t.Name+" "+act)
			return
		}
		m.Changes = append(m.Changes, fmt.Sprintf("alters `%s.%s`", t.Name, c.Name))
	case strings.HasPrefix(lower, "owner to"), strings.HasPrefix(lower, "set "), strings.HasPrefix(lower, "enable"), strings.HasPrefix(lower, "disable"), strings.HasPrefix(lower, "validate"):
		// storage or ownership settings: not part of the documented schema
	default:
		s.skip(m, "ALTER TABLE "+t.Name+" "+act)
	}
}

func (s *Schema) createIndex(st string, m *Migration) {
	g := reCreateIndex.FindStringSubmatch(st)
	body, end, ok := parenBody(st, len(g[0])-1)
	if !ok {
		s.skip(m, st)
		return
	}
	ix := &Index{Name: ident(strings.TrimSpace(g[2])), Table: ident(g[3]), Unique: g[1] != ""}
	for _, p := range splitTop(body) {
		f := strings.Fields(p)
		if len(f) == 0 {
			continue
		}
		col := p
		if !strings.Contains(p, "(") {
			col = ident(f[0])
			if len(f) > 1 && strings.EqualFold(f[1], "desc") {
				col += " DESC"
			}
		}
		ix.Columns = append(ix.Columns, col)
	}
	if w := regexp.MustCompile(`(?i)\bwhere\b(.*)$`).FindStringSubmatch(st[end:]); w != nil {
		ix.Where = norm(w[1])
	}
	s.addIndex(ix)
	kind := "index"
	if ix.Unique {
		kind = "unique index"
	}
	m.Changes = append(m.Changes, fmt.Sprintf("adds %s `%s` on `%s`", kind, ix.Name, ix.Table))
}
