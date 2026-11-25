package records

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
)

const (
	maxImportBytes = 25 << 20
	maxImportRows  = 200000
)

// detectDelimiter picks , ; or tab from the first line.
func detectDelimiter(data []byte) rune {
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	best, bestN := ',', 0
	for _, d := range []rune{',', ';', '\t'} {
		if n := bytes.Count(line, []byte(string(d))); n > bestN {
			best, bestN = d, n
		}
	}
	return best
}

func parseCSV(data []byte) ([][]string, rune, error) {
	if len(data) > maxImportBytes {
		return nil, 0, knowkit.Invalid("the file is larger than %d MB", maxImportBytes>>20)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	d := detectDelimiter(data)
	rd := csv.NewReader(bytes.NewReader(data))
	rd.Comma = d
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, knowkit.Invalid("cannot read the file as CSV: %v", err)
		}
		empty := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		rows = append(rows, rec)
		if len(rows) > maxImportRows+1 {
			return nil, 0, knowkit.Invalid("more than %d rows", maxImportRows)
		}
	}
	return rows, d, nil
}

var reSafeNumber = regexp.MustCompile(`^-?[0-9][0-9,]*(\.[0-9]+)?$`)

var reCurrency = regexp.MustCompile(`^[-(]?[$€£₹¥]\s?[\d,]+(\.\d+)?\)?$`)

// detectType guesses a field type from sample values.
func detectType(values []string, rows int) string {
	var vals []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return TText
	}
	all := func(f func(string) bool) bool {
		for _, v := range vals {
			if !f(v) {
				return false
			}
		}
		return true
	}
	isNum := func(v string) bool {
		_, err := parseNumberText(Field{Type: TNumber}, v)
		return err == nil && !strings.HasSuffix(v, "%") && !reCurrency.MatchString(v)
	}
	switch {
	case all(func(v string) bool {
		switch strings.ToLower(v) {
		case "true", "false", "yes", "no", "1", "0", "x", "checked":
			return true
		}
		return false
	}) && !all(func(v string) bool { return v == "1" || v == "0" }):
		return TCheckbox
	case all(func(v string) bool { return reCurrency.MatchString(v) }):
		return TCurrency
	case all(func(v string) bool { return strings.HasSuffix(v, "%") && isNumPct(v) }):
		return TPercent
	case all(func(v string) bool {
		return isNum(v) && !(len(v) > 1 && v[0] == '0' && !strings.Contains(v, ".")) && len(v) < 16
	}):
		return TNumber
	case all(func(v string) bool {
		t, ok := parseDate(v)
		return ok && (strings.ContainsAny(v, "-/")) && t.Year() > 1000
	}):
		if all(func(v string) bool { return strings.ContainsAny(v, ":T") }) {
			return TDate // with time, the field option is set by the caller
		}
		return TDate
	case all(func(v string) bool { return reEmail.MatchString(v) }):
		return TEmail
	case all(func(v string) bool { return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") }):
		return TURL
	}
	distinct := map[string]bool{}
	long := false
	for _, v := range vals {
		distinct[strings.ToLower(v)] = true
		if len(v) > 120 || strings.Contains(v, "\n") {
			long = true
		}
	}
	if long {
		return TLongText
	}
	if rows >= 15 && len(distinct) <= 10 && len(distinct)*5 <= len(vals) {
		return TSelect
	}
	return TText
}

func isNumPct(v string) bool {
	_, err := parseNumberText(Field{Type: TPercent}, v)
	return err == nil
}

// ImportColumn maps one CSV column to a field.
type ImportColumn struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	FieldID string `json:"field_id,omitempty"` // append mode: the existing field
	Skip    bool   `json:"skip,omitempty"`
}

// ImportPreview is what the import dialog shows.
type ImportPreview struct {
	Delimiter string         `json:"delimiter"`
	Rows      int            `json:"rows"`
	Columns   []ImportColumn `json:"columns"`
	Sample    [][]string     `json:"sample"`
}

// PreviewCSV reads a file and guesses its columns.
func PreviewCSV(data []byte, hasHeader bool) (*ImportPreview, error) {
	rows, d, err := parseCSV(data)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, knowkit.Invalid("the file has no rows")
	}
	width := 0
	for _, r := range rows[:min(len(rows), 1000)] {
		width = max(width, len(r))
	}
	if width > MaxFieldsPerTable {
		return nil, knowkit.Invalid("the file has %d columns; at most %d are supported", width, MaxFieldsPerTable)
	}
	body := rows
	var header []string
	if hasHeader {
		header, body = rows[0], rows[1:]
	}
	p := &ImportPreview{Delimiter: string(d), Rows: len(body)}
	for c := 0; c < width; c++ {
		name := ""
		if c < len(header) {
			name = strings.TrimSpace(header[c])
		}
		if name == "" {
			name = "Field " + itoa(c+1)
		}
		var col []string
		for _, r := range body[:min(len(body), 1000)] {
			if c < len(r) {
				col = append(col, r[c])
			}
		}
		p.Columns = append(p.Columns, ImportColumn{Index: c, Name: name, Type: detectType(col, len(body))})
	}
	if len(p.Columns) > 0 && !knowkit.OneOf(p.Columns[0].Type, TText, TLongText, TNumber) {
		p.Columns[0].Type = TText
	}
	for _, r := range body[:min(len(body), 5)] {
		p.Sample = append(p.Sample, r)
	}
	return p, nil
}

func itoa(n int) string { return fmtNum(float64(n)) }

// ImportRequest runs an import.
type ImportRequest struct {
	TableName string         `json:"table_name"`
	TableID   string         `json:"table_id"` // append to this table when set
	Data      string         `json:"data"`
	HasHeader bool           `json:"has_header"`
	Columns   []ImportColumn `json:"columns"`
}

// ImportResult says what an import did.
type ImportResult struct {
	TableID  string   `json:"table_id"`
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Notes    []string `json:"notes,omitempty"`
}

// ImportCSV creates a table from the file (or appends to table_id).
func (r *Repository) ImportCSV(ctx context.Context, a knowkit.Actor, baseID string, req ImportRequest) (*ImportResult, error) {
	if _, err := r.getBase(ctx, a, baseID, RoleEdit); err != nil {
		return nil, err
	}
	rows, _, err := parseCSV([]byte(req.Data))
	if err != nil {
		return nil, err
	}
	body := rows
	if req.HasHeader && len(rows) > 0 {
		body = rows[1:]
	}
	res := &ImportResult{}
	tableID := req.TableID
	if tableID == "" {
		name := strings.TrimSpace(req.TableName)
		if name == "" {
			name = "Imported table"
		}
		t, err := r.CreateTable(ctx, a, baseID, name)
		if err != nil {
			return nil, err
		}
		tableID = t.ID
		first := true
		cols := make([]ImportColumn, 0, len(req.Columns))
		for _, c := range req.Columns {
			if c.Skip {
				continue
			}
			if first {
				first = false
				// the primary field already exists: rename it for the first column
				sch, err := loadSchema(ctx, r.pool, a.Org, baseID)
				if err != nil {
					return nil, err
				}
				p, _ := sch.primary(tableID)
				ptype := c.Type
				if !knowkit.OneOf(ptype, TText, TLongText, TNumber) {
					ptype = TText
				}
				if _, _, err := r.UpdateField(ctx, a, p.ID, FieldInput{Name: c.Name, Type: ptype}); err != nil {
					return nil, err
				}
				c.FieldID = p.ID
				cols = append(cols, c)
				continue
			}
			opts := map[string]any{}
			if c.Type == TSelect || c.Type == TMulti {
				seen := map[string]bool{}
				var ch []any
				for _, row := range body {
					if c.Index < len(row) {
						for _, n := range choiceNames(c.Type, row[c.Index]) {
							if k := strings.ToLower(n); !seen[k] && len(ch) < 100 {
								seen[k] = true
								ch = append(ch, map[string]any{"name": n})
							}
						}
					}
				}
				opts["choices"] = ch
			}
			if c.Type == TCurrency || c.Type == TNumber || c.Type == TPercent {
				opts["precision"] = decimalsIn(body, c.Index)
			}
			if !ValidType(c.Type) || IsComputed(c.Type) || c.Type == TLink || c.Type == TAttachment {
				c.Type = TText
			}
			f, err := r.CreateField(ctx, a, tableID, FieldInput{Name: c.Name, Type: c.Type, Options: opts})
			if err != nil {
				return nil, err
			}
			c.FieldID = f.ID
			cols = append(cols, c)
		}
		req.Columns = cols
	}
	b, err := r.tableBase(ctx, a, tableID, RoleEdit)
	if err != nil {
		return nil, err
	}
	e, err := r.newEngine(ctx, a, b.ID)
	if err != nil {
		return nil, err
	}
	// Append mode: add any select options the file introduces.
	for _, c := range req.Columns {
		f, ok := e.sch.field(c.FieldID)
		if !ok || f.TableID != tableID || (f.Type != TSelect && f.Type != TMulti) {
			continue
		}
		var add []any
		for _, ch := range f.Choices() {
			add = append(add, map[string]any{"id": ch.ID, "name": ch.Name, "color": ch.Color})
		}
		known := map[string]bool{}
		for _, ch := range f.Choices() {
			known[strings.ToLower(ch.Name)] = true
		}
		changed := false
		for _, row := range body {
			if c.Index < len(row) {
				for _, n := range choiceNames(f.Type, row[c.Index]) {
					if !known[strings.ToLower(n)] && len(add) < 200 {
						known[strings.ToLower(n)] = true
						add = append(add, map[string]any{"name": n})
						changed = true
					}
				}
			}
		}
		if changed {
			opts := map[string]any{}
			for k, v := range f.Options {
				opts[k] = v
			}
			opts["choices"] = add
			if _, _, err := r.UpdateField(ctx, a, f.ID, FieldInput{Options: opts}); err != nil {
				return nil, err
			}
		}
	}
	if e, err = r.newEngine(ctx, a, b.ID); err != nil {
		return nil, err
	}
	var have int64
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.records_records WHERE table_id=$1`, tableID).Scan(&have)
	if have+int64(len(body)) > MaxRecordsPerTable {
		return nil, knowkit.Invalid("a table can hold at most %d records", MaxRecordsPerTable)
	}
	type outRow struct {
		id   string
		data []byte
	}
	var batch []outRow
	cellProblems := 0
	for _, row := range body {
		data := map[string]any{}
		for _, c := range req.Columns {
			f, ok := e.sch.field(c.FieldID)
			if !ok || f.TableID != tableID || IsComputed(f.Type) || f.Type == TLink || f.Type == TAttachment || c.Index >= len(row) {
				continue
			}
			v, _, perr := ParseText(f, row[c.Index])
			if perr != nil {
				cellProblems++
				continue
			}
			if v != nil {
				data[f.ID] = v
			}
		}
		raw, _ := json.Marshal(data)
		batch = append(batch, outRow{NewID(), raw})
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var next int64
	if err := tx.QueryRow(ctx, `UPDATE adrient.records_tables SET next_seq = next_seq + $2 WHERE id=$1 RETURNING next_seq - $2`, tableID, len(batch)).Scan(&next); err != nil {
		return nil, err
	}
	src := make([][]any, len(batch))
	for i, o := range batch {
		src[i] = []any{o.id, a.Org, tableID, next + int64(i), o.data, a.User, a.User}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"adrient", "records_records"},
		[]string{"id", "org_id", "table_id", "seq", "data", "created_by", "updated_by"}, pgx.CopyFromRows(src)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	res.TableID, res.Imported = tableID, len(batch)
	if cellProblems > 0 {
		res.Skipped = cellProblems
		res.Notes = append(res.Notes, itoa(cellProblems)+" cell(s) did not fit their field type and were left empty")
	}
	r.audit(ctx, "import.csv", "records_table", tableID)
	return res, nil
}

func choiceNames(t, cell string) []string {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return nil
	}
	if t == TMulti {
		return splitList(cell)
	}
	return []string{cell}
}

func decimalsIn(rows [][]string, col int) int {
	d := 0
	for _, r := range rows[:min(len(rows), 1000)] {
		if col < len(r) {
			if i := strings.IndexByte(r[col], '.'); i >= 0 {
				n := len(strings.TrimRight(r[col][i+1:], "%) "))
				d = max(d, n)
			}
		}
	}
	return min(d, 6)
}

// ExportCSV renders a table (optionally narrowed by a view query) as CSV.
func (r *Repository) ExportCSV(ctx context.Context, a knowkit.Actor, tableID string, q Query, fieldIDs []string) ([]byte, string, error) {
	b, err := r.tableBase(ctx, a, tableID, RoleView)
	if err != nil {
		return nil, "", err
	}
	e, err := r.newEngine(ctx, a, b.ID)
	if err != nil {
		return nil, "", err
	}
	fields := e.sch.byTbl[tableID]
	if len(fieldIDs) > 0 {
		var pick []Field
		for _, id := range fieldIDs {
			if f, ok := e.sch.field(id); ok && f.TableID == tableID {
				pick = append(pick, f)
			}
		}
		fields = pick
	}
	q.Offset, q.Limit = 0, MaxRecordsPerTable
	res, err := r.list(ctx, a, tableID, q, MaxRecordsPerTable)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&buf)
	head := make([]string, len(fields))
	for i, f := range fields {
		head[i] = f.Name
	}
	_ = w.Write(head)
	for _, rc := range res.Records {
		row := make([]string, len(fields))
		for i, f := range fields {
			v := rc.Fields[f.ID]
			row[i] = csvSafe(exportText(f, v))
		}
		_ = w.Write(row)
	}
	w.Flush()
	name := e.sch.tables[tableID].Name
	return buf.Bytes(), name, w.Error()
}

// exportText is the plain text of a value for CSV (links as names, formulas as text).
func exportText(f Field, v any) string {
	if v == nil {
		return ""
	}
	if f.Type == TLink {
		l, _ := v.([]any)
		names := make([]string, 0, len(l))
		for _, e := range l {
			if m, ok := e.(map[string]any); ok {
				names = append(names, toStr(m["name"]))
			}
		}
		return strings.Join(names, ", ")
	}
	if IsComputed(f.Type) {
		if fe, ok := v.(FormulaError); ok {
			return "#ERROR! " + fe.Msg
		}
		return toStr(v)
	}
	switch f.Type {
	case TCheckbox:
		return "true"
	case TPercent:
		return Display(f, v, nil)
	case TNumber, TCurrency:
		n, _ := v.(float64)
		return strings.TrimPrefix(Display(Field{Type: TNumber, Options: f.Options}, n, nil), "")
	}
	return Display(f, v, nil)
}

// csvSafe stops spreadsheet formula injection: a cell that starts with = + - @ is prefixed with a quote.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		if reSafeNumber.MatchString(s) {
			return s // a plain number such as -5
		}
		return "'" + s
	}
	return s
}

// ---- backup ----------------------------------------------------------------

// Backup is a whole base as JSON.
type Backup struct {
	Version int           `json:"version"`
	Base    BaseInput     `json:"base"`
	Tables  []backupTable `json:"tables"`
}

type backupTable struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Fields  []Field        `json:"fields"`
	Views   []View         `json:"views"`
	Records []backupRecord `json:"records"`
}

type backupRecord struct {
	ID    string              `json:"id"`
	Seq   int64               `json:"seq"`
	Data  map[string]any      `json:"data"`
	Links map[string][]string `json:"links,omitempty"`
}

// ExportBackup dumps a base (all tables, fields with raw formulas, views, records and links).
func (r *Repository) ExportBackup(ctx context.Context, a knowkit.Actor, baseID string) (*Backup, error) {
	b, err := r.getBase(ctx, a, baseID, RoleView)
	if err != nil {
		return nil, err
	}
	s, err := loadSchema(ctx, r.pool, a.Org, baseID)
	if err != nil {
		return nil, err
	}
	bk := &Backup{Version: 1, Base: BaseInput{Name: b.Name, Description: b.Description, Color: b.Color}}
	tids := make([]string, 0, len(s.tables))
	for id := range s.tables {
		tids = append(tids, id)
	}
	sortByPos(tids, s)
	for _, tid := range tids {
		t := s.tables[tid]
		bt := backupTable{ID: tid, Name: t.Name, Fields: s.byTbl[tid]}
		vrows, err := r.pool.Query(ctx, `SELECT id::text, name, type, config, position FROM adrient.records_views WHERE table_id=$1 ORDER BY position`, tid)
		if err != nil {
			return nil, err
		}
		for vrows.Next() {
			v := View{TableID: tid}
			if err := vrows.Scan(&v.ID, &v.Name, &v.Type, &v.Config, &v.Position); err != nil {
				vrows.Close()
				return nil, err
			}
			bt.Views = append(bt.Views, v)
		}
		vrows.Close()
		rrows, err := r.pool.Query(ctx, `SELECT id::text, seq, data FROM adrient.records_records WHERE table_id=$1 ORDER BY seq`, tid)
		if err != nil {
			return nil, err
		}
		idx := map[string]int{}
		for rrows.Next() {
			var x backupRecord
			if err := rrows.Scan(&x.ID, &x.Seq, &x.Data); err != nil {
				rrows.Close()
				return nil, err
			}
			idx[x.ID] = len(bt.Records)
			bt.Records = append(bt.Records, x)
		}
		rrows.Close()
		bk.Tables = append(bk.Tables, bt)
		// Forward links only (reverse fields read the same rows).
		for _, f := range s.byTbl[tid] {
			if f.Type != TLink || f.optBool("reverse") {
				continue
			}
			lrows, err := r.pool.Query(ctx, `SELECT src::text, dst::text FROM adrient.records_links WHERE org_id=$1 AND field_id=$2 ORDER BY pos`, a.Org, f.ID)
			if err != nil {
				return nil, err
			}
			for lrows.Next() {
				var src, dst string
				_ = lrows.Scan(&src, &dst)
				if i, ok := idx[src]; ok {
					x := &bt.Records[i]
					if x.Links == nil {
						x.Links = map[string][]string{}
					}
					x.Links[f.ID] = append(x.Links[f.ID], dst)
				}
			}
			lrows.Close()
		}
		bk.Tables[len(bk.Tables)-1] = bt
	}
	return bk, nil
}

func sortByPos(ids []string, s *schema) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && s.tables[ids[j]].Position < s.tables[ids[j-1]].Position; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}

// RestoreBackup creates a new base from a backup. Every id is replaced by a fresh one.
func (r *Repository) RestoreBackup(ctx context.Context, a knowkit.Actor, raw []byte, name string) (*Base, error) {
	if len(raw) > 200<<20 {
		return nil, knowkit.Invalid("the backup is too large")
	}
	// New ids for everything: replace each old uuid in the whole JSON text.
	var probe Backup
	if err := json.Unmarshal(raw, &probe); err != nil || probe.Version != 1 {
		return nil, knowkit.Invalid("this is not an Adrient Records backup")
	}
	re := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	mapped := map[string]string{}
	text := re.ReplaceAllStringFunc(string(raw), func(old string) string {
		k := strings.ToLower(old)
		if n, ok := mapped[k]; ok {
			return n
		}
		n := NewID()
		mapped[k] = n
		return n
	})
	var bk Backup
	if err := json.Unmarshal([]byte(text), &bk); err != nil {
		return nil, knowkit.Invalid("the backup is damaged")
	}
	if strings.TrimSpace(name) != "" {
		bk.Base.Name = name
	}
	if err := cleanBase(&bk.Base); err != nil {
		return nil, err
	}
	if len(bk.Tables) == 0 || len(bk.Tables) > MaxTablesPerBase {
		return nil, knowkit.Invalid("a base needs between 1 and %d tables", MaxTablesPerBase)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var baseID string
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.records_bases (org_id, name, description, color, owner_id, org_access) VALUES ($1,$2,$3,$4,$5,'none') RETURNING id::text`,
		a.Org, bk.Base.Name, bk.Base.Description, bk.Base.Color, a.User).Scan(&baseID); err != nil {
		return nil, err
	}
	for pos, t := range bk.Tables {
		var maxSeq int64
		for _, x := range t.Records {
			maxSeq = max(maxSeq, x.Seq)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.records_tables (id, org_id, base_id, name, position, next_seq) VALUES ($1,$2,$3,$4,$5,$6)`, t.ID, a.Org, baseID, t.Name, pos, maxSeq+1); err != nil {
			return nil, err
		}
		for _, f := range t.Fields {
			if !ValidType(f.Type) {
				return nil, knowkit.Invalid("the backup has an unknown field type")
			}
			opts, _ := json.Marshal(f.Options)
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.records_fields (id, org_id, base_id, table_id, name, type, description, options, position) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
				f.ID, a.Org, baseID, t.ID, f.Name, f.Type, f.Description, opts, f.Position); err != nil {
				return nil, err
			}
		}
		for _, v := range t.Views {
			cfg, _ := json.Marshal(v.Config)
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.records_views (org_id, table_id, name, type, config, position, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7)`, a.Org, t.ID, v.Name, v.Type, cfg, v.Position, a.User); err != nil {
				return nil, err
			}
		}
		src := make([][]any, len(t.Records))
		for i, x := range t.Records {
			d, _ := json.Marshal(x.Data)
			src[i] = []any{x.ID, a.Org, t.ID, x.Seq, d, a.User, a.User}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"adrient", "records_records"}, []string{"id", "org_id", "table_id", "seq", "data", "created_by", "updated_by"}, pgx.CopyFromRows(src)); err != nil {
			return nil, err
		}
	}
	for _, t := range bk.Tables {
		var rows [][]any
		for _, x := range t.Records {
			for fid, ids := range x.Links {
				for i, dst := range ids {
					rows = append(rows, []any{a.Org, fid, x.ID, dst, i})
				}
			}
		}
		if len(rows) > 0 {
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{"adrient", "records_links"}, []string{"org_id", "field_id", "src", "dst", "pos"}, pgx.CopyFromRows(rows)); err != nil {
				return nil, knowkit.Invalid("the backup has links to records that are not in it")
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	r.audit(ctx, "base.restore_backup", "records_base", baseID)
	return r.GetBase(ctx, a, baseID)
}
