package records

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit/kittest"
)

var tt *testing.T

func must[T any](v T, err error) T {
	tt.Helper()
	if err != nil {
		tt.Fatalf("unexpected error: %v", err)
	}
	return v
}

func noerr(err error) {
	tt.Helper()
	if err != nil {
		tt.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordsEndToEnd(t *testing.T) {
	fx := kittest.Setup(t)
	tt = t
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	a := fx.Admin

	base := must(repo.CreateBase(ctx, a, BaseInput{Name: "Tracker", Template: "project_tracker"}))
	if len(base.Tables) != 2 {
		t.Fatalf("template tables = %d", len(base.Tables))
	}
	var projects, tasks Table
	for _, tb := range base.Tables {
		if tb.Name == "Projects" {
			projects = tb
		} else {
			tasks = tb
		}
	}
	// The link has a reverse field on Projects and "Open tasks" counts through it.
	res := must(repo.ListRecords(ctx, a, projects.ID, Query{}))
	if res.Total != 2 {
		t.Fatalf("projects = %d", res.Total)
	}
	var countField Field
	for _, f := range projects.Fields {
		if f.Type == TCount {
			countField = f
		}
	}
	for _, r := range res.Records {
		n := r.Fields[countField.ID]
		if r.Fields[projects.Fields[0].ID] == "Website relaunch" && n != 2.0 {
			t.Errorf("Website relaunch open tasks = %v", n)
		}
	}

	// Add fields: lookup, rollup, formula.
	var linkF Field
	for _, f := range tasks.Fields {
		if f.Type == TLink {
			linkF = f
		}
	}
	var estF, statusF Field
	for _, f := range tasks.Fields {
		if f.Name == "Estimate (h)" {
			estF = f
		}
		if f.Name == "Status" {
			statusF = f
		}
	}
	// Reverse link field on projects.
	var revF Field
	for _, f := range must(repo.GetBase(ctx, a, base.ID)).Tables {
		if f.ID == projects.ID {
			for _, x := range f.Fields {
				if x.Type == TLink {
					revF = x
				}
			}
		}
	}
	if revF.ID == "" || revF.optBool("reverse") == false {
		t.Fatalf("no reverse field: %+v", revF)
	}
	rollF := must(repo.CreateField(ctx, a, projects.ID, FieldInput{Name: "Hours", Type: TRollup,
		Options: map[string]any{"link_field_id": revF.ID, "target_field_id": estF.ID, "fn": "sum"}}))
	lookF := must(repo.CreateField(ctx, a, tasks.ID, FieldInput{Name: "Project name", Type: TLookup,
		Options: map[string]any{"link_field_id": linkF.ID, "target_field_id": projects.Fields[0].ID}}))
	fmlF := must(repo.CreateField(ctx, a, tasks.ID, FieldInput{Name: "Label", Type: TFormula,
		Options: map[string]any{"formula": `{Task} & " (" & {Status} & ")"`}}))
	if !strings.Contains(fmlF.optStr("formula"), "{Task}") {
		t.Errorf("API shows the formula with names, got %q", fmlF.optStr("formula"))
	}
	// Circular and bad references are refused.
	if _, err := repo.CreateField(ctx, a, tasks.ID, FieldInput{Name: "Bad", Type: TFormula, Options: map[string]any{"formula": `{Nope} + 1`}}); err == nil {
		t.Error("unknown field reference should be refused")
	}
	if _, _, err := repo.UpdateField(ctx, a, fmlF.ID, FieldInput{Options: map[string]any{"formula": `{Label} & "x"`}}); err == nil {
		t.Error("self reference should be refused")
	}

	tres := must(repo.ListRecords(ctx, a, tasks.ID, Query{Sort: []SortKey{{Field: tasks.Fields[0].ID}}}))
	if tres.Total != 3 {
		t.Fatalf("tasks = %d", tres.Total)
	}
	first := tres.Records[0] // "Collect goals"
	if first.Fields[tasks.Fields[0].ID] != "Collect goals" {
		t.Fatalf("sorted first = %v", first.Fields[tasks.Fields[0].ID])
	}
	if got := first.Fields[fmlF.ID]; got != "Collect goals (To do)" {
		t.Errorf("formula = %v", got)
	}
	l, _ := first.Fields[lookF.ID].([]any)
	if len(l) != 1 || l[0] != "Q3 planning" {
		t.Errorf("lookup = %v", first.Fields[lookF.ID])
	}
	// Estimates then rollup.
	must(repo.UpdateRecords(ctx, a, tasks.ID, []RecordInput{
		{ID: tres.Records[1].ID, Fields: map[string]any{estF.ID: 2.5}},
		{ID: tres.Records[2].ID, Fields: map[string]any{estF.ID: "4"}},
	}))
	pres := must(repo.ListRecords(ctx, a, projects.ID, Query{}))
	sums := map[string]any{}
	for _, r := range pres.Records {
		sums[fmt.Sprint(r.Fields[projects.Fields[0].ID])] = r.Fields[rollF.ID]
	}
	if sums["Website relaunch"] != 6.5 || sums["Q3 planning"] != 0.0 {
		t.Errorf("rollup sums = %v", sums)
	}

	// Editing from the reverse side changes the forward link.
	web := pres.Records[0]
	if web.Fields[projects.Fields[0].ID] != "Website relaunch" {
		web = pres.Records[1]
	}
	q3 := pres.Records[0].ID
	if q3 == web.ID {
		q3 = pres.Records[1].ID
	}
	must(repo.UpdateRecords(ctx, a, projects.ID, []RecordInput{{ID: q3, Fields: map[string]any{revF.ID: []any{tres.Records[1].ID}}}}))
	again := must(repo.GetRecord2(ctx, a, tres.Records[1].ID))
	lk, _ := again.Fields[linkF.ID].([]any)
	if len(lk) != 1 || lk[0].(map[string]any)["id"] != q3 {
		t.Errorf("forward link after reverse edit = %v", again.Fields[linkF.ID])
	}

	// Filter on a computed field and on a link name.
	f1 := must(repo.ListRecords(ctx, a, tasks.ID, Query{Filter: &Node{Field: lookF.ID, Op: "contains", Value: "q3"}}))
	if f1.Total != 1 { // setting the reverse list replaced the links of that project
		t.Errorf("lookup filter matched %d", f1.Total)
	}
	f2 := must(repo.ListRecords(ctx, a, tasks.ID, Query{Filter: &Node{Field: statusF.ID, Op: "is", Value: "To do"}, Search: "design"}))
	if f2.Total != 1 {
		t.Errorf("select+search matched %d", f2.Total)
	}

	// Validation.
	if _, err := repo.CreateRecords(ctx, a, tasks.ID, []RecordInput{{Fields: map[string]any{estF.ID: "abc"}}}); err == nil {
		t.Error("bad number should be refused")
	}
	if _, err := repo.CreateRecords(ctx, a, tasks.ID, []RecordInput{{Fields: map[string]any{fmlF.ID: "x"}}}); err == nil {
		t.Error("writing a formula field should be refused")
	}
	if _, err := repo.CreateRecords(ctx, a, tasks.ID, []RecordInput{{Fields: map[string]any{linkF.ID: []any{NewID()}}}}); err == nil {
		t.Error("link to a missing record should be refused")
	}
	if _, err := repo.CreateRecords(ctx, a, tasks.ID, []RecordInput{{Fields: map[string]any{linkF.ID: []any{web.ID, q3}}}}); err == nil {
		t.Error("single-link field takes one record")
	}

	// History and comments.
	hist := must(repo.History(ctx, a, tres.Records[1].ID))
	if len(hist) == 0 || hist[0].Changes[0]["field"] == nil {
		t.Errorf("history = %+v", hist)
	}
	must(repo.AddComment(ctx, a, tres.Records[1].ID, "Looks good"))
	if cs := must(repo.ListComments(ctx, a, tres.Records[1].ID)); len(cs) != 1 {
		t.Error("comment missing")
	}

	// Type conversion: select -> text keeps names; text -> number clears what does not fit.
	f, _, err := repo.UpdateField(ctx, a, statusF.ID, FieldInput{Type: TText})
	if err != nil || f.Type != TText {
		t.Fatalf("convert to text: %v", err)
	}
	r2 := must(repo.ListRecords(ctx, a, tasks.ID, Query{Sort: []SortKey{{Field: tasks.Fields[0].ID}}}))
	if r2.Records[0].Fields[statusF.ID] != "To do" {
		t.Errorf("converted value = %v", r2.Records[0].Fields[statusF.ID])
	}
	_, cleared, err := repo.UpdateField(ctx, a, statusF.ID, FieldInput{Type: TNumber})
	if err != nil || cleared != 3 {
		t.Errorf("convert to number cleared %d (%v)", cleared, err)
	}
	// Formula referencing the field survives; deleting the field removes dependants.
	if err := repo.DeleteField(ctx, a, statusF.ID); err != nil {
		t.Fatal(err)
	}
	b2 := must(repo.GetBase(ctx, a, base.ID))
	for _, tb := range b2.Tables {
		if tb.ID == tasks.ID {
			for _, x := range tb.Fields {
				if x.ID == fmlF.ID || x.ID == statusF.ID {
					t.Errorf("field %s should be gone", x.Name)
				}
			}
		}
	}
	if err := repo.DeleteField(ctx, a, tasks.Fields[0].ID); err == nil {
		t.Error("the first field cannot be deleted")
	}

	// Table delete removes link fields pointing to it.
	if err := repo.DeleteTable(ctx, a, projects.ID); err != nil {
		t.Fatal(err)
	}
	b3 := must(repo.GetBase(ctx, a, base.ID))
	for _, x := range b3.Tables[0].Fields {
		if x.Type == TLink || x.ID == lookF.ID {
			t.Errorf("field %s should be gone with its table", x.Name)
		}
	}
	if err := repo.DeleteTable(ctx, a, tasks.ID); err == nil {
		t.Error("the last table cannot be deleted")
	}
}

// GetRecord2 returns just the record (test helper).
func (r *Repository) GetRecord2(ctx context.Context, a knowkit.Actor, id string) (*Record, error) {
	rc, _, err := r.GetRecord(ctx, a, id)
	return rc, err
}

func TestRecordsAccessControl(t *testing.T) {
	fx := kittest.Setup(t)
	tt = t
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	b := must(repo.CreateBase(ctx, fx.User, BaseInput{Name: "Private"}))
	tbl := b.Tables[0]

	// Admin of the org sees everything; another org sees nothing; a third user nothing yet.
	if _, err := repo.GetBase(ctx, fx.Other, b.ID); err == nil {
		t.Error("another org must not see the base")
	}
	if _, err := repo.ListRecords(ctx, fx.Other, tbl.ID, Query{}); err == nil {
		t.Error("another org must not read records")
	}
	third := knowkit.Actor{Org: fx.Org, User: must(uuidOf(ctx, fx, "third"))}
	if _, err := repo.GetBase(ctx, third, b.ID); err == nil {
		t.Error("an unshared user must not see the base")
	}
	if l := must(repo.ListBases(ctx, third, false)); len(l) != 0 {
		t.Error("an unshared base must not be listed")
	}
	// Share as viewer: can read, cannot write.
	noerr(repo.SetMembers(ctx, fx.User, b.ID, []Member{{UserID: third.User, Role: "viewer"}}))
	if _, err := repo.ListRecords(ctx, third, tbl.ID, Query{}); err != nil {
		t.Errorf("viewer read: %v", err)
	}
	if _, err := repo.CreateRecords(ctx, third, tbl.ID, []RecordInput{{Fields: map[string]any{"Name": "x"}}}); err == nil {
		t.Error("viewer must not write")
	}
	if _, err := repo.CreateField(ctx, third, tbl.ID, FieldInput{Name: "n", Type: TText}); err == nil {
		t.Error("viewer must not add fields")
	}
	// Org-wide edit access.
	must(repo.UpdateBase(ctx, fx.User, b.ID, BaseInput{Name: "Private", OrgAccess: "edit"}))
	if _, err := repo.CreateRecords(ctx, third, tbl.ID, []RecordInput{{Fields: map[string]any{"Name": "x"}}}); err != nil {
		t.Errorf("org editor write: %v", err)
	}
	if err := repo.TrashBase(ctx, third, b.ID, true); err == nil {
		t.Error("an editor must not trash the base")
	}
	// Trash blocks writes and hides the base; restore brings it back.
	noerr(repo.TrashBase(ctx, fx.User, b.ID, true))
	if _, err := repo.CreateRecords(ctx, fx.User, tbl.ID, []RecordInput{{Fields: map[string]any{"Name": "y"}}}); err == nil {
		t.Error("trashed base must refuse writes")
	}
	noerr(repo.TrashBase(ctx, fx.User, b.ID, false))
	if l := must(repo.ListBases(ctx, fx.User, false)); len(l) != 1 {
		t.Errorf("bases after restore = %d", len(l))
	}
	// Records of one base cannot be linked from another.
	other := must(repo.CreateBase(ctx, fx.User, BaseInput{Name: "Other"}))
	if _, err := repo.CreateField(ctx, fx.User, tbl.ID, FieldInput{Name: "x", Type: TLink, Options: map[string]any{"table_id": other.Tables[0].ID}}); err == nil {
		t.Error("linking to a table of another base must be refused")
	}
}

func uuidOf(ctx context.Context, fx *kittest.Fixture, name string) (string, error) {
	var id string
	err := fx.Pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'test',$2,$3,$2) RETURNING id::text`,
		fx.Org, name+fmt.Sprint(time.Now().UnixNano()), name+"@example.test").Scan(&id)
	return id, err
}

func TestImportExportBackup(t *testing.T) {
	fx := kittest.Setup(t)
	tt = t
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	a := fx.User
	b := must(repo.CreateBase(ctx, a, BaseInput{Name: "Import"}))

	csvText := "Name,Qty,Price,Due,Done,Email,Status\n"
	for i := 0; i < 40; i++ {
		csvText += fmt.Sprintf("Item %d,%d,$%d.50,2026-04-%02d,%s,u%d@x.co,%s\n", i, i*3, i+1, i%28+1, []string{"yes", "no"}[i%2], i, []string{"New", "Open", "Closed"}[i%3])
	}
	csvText += "=cmd|' /C calc'!A0,1,$1.00,2026-04-01,no,bad@x.co,New\n"
	pv := must(PreviewCSV([]byte(csvText), true))
	types := []string{}
	for _, c := range pv.Columns {
		types = append(types, c.Type)
	}
	want := []string{TText, TNumber, TCurrency, TDate, TCheckbox, TEmail, TSelect}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("detected %v, want %v", types, want)
	}
	res := must(repo.ImportCSV(ctx, a, b.ID, ImportRequest{TableName: "Items", Data: csvText, HasHeader: true, Columns: pv.Columns}))
	if res.Imported != 41 {
		t.Fatalf("imported %d", res.Imported)
	}
	// Exports neutralise formula injection.
	data, _, err := repo.ExportCSV(ctx, a, res.TableID, Query{Sort: []SortKey{{Field: firstField(t, repo, a, b.ID, res.TableID)}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "'=cmd") || strings.Contains(out, "\n=cmd") {
		t.Errorf("export is not protected against formula injection:\n%s", out[:200])
	}
	if !strings.Contains(out, "Item 0,0,") {
		t.Errorf("export content: %s", out[:300])
	}

	// Backup and restore round-trips tables, links, formulas and counts.
	tpl := must(repo.CreateBase(ctx, a, BaseInput{Name: "Sales", Template: "sales_pipeline"}))
	bk := must(repo.ExportBackup(ctx, a, tpl.ID))
	raw, err := json.Marshal(bk)
	if err != nil {
		t.Fatal(err)
	}
	cp := must(repo.RestoreBackup(ctx, a, raw, "Sales copy"))
	if cp.Name != "Sales copy" || cp.ID == tpl.ID || len(cp.Tables) != 2 {
		t.Fatalf("restore: %+v", cp.Name)
	}
	var deals Table
	for _, tb := range cp.Tables {
		if tb.Name == "Deals" {
			deals = tb
		}
	}
	rs := must(repo.ListRecords(ctx, a, deals.ID, Query{}))
	var companyField Field
	for _, f := range deals.Fields {
		if f.Type == TLink {
			companyField = f
		}
	}
	for _, r := range rs.Records {
		l, _ := r.Fields[companyField.ID].([]any)
		if len(l) != 1 {
			t.Errorf("restored link lost: %v", r.Fields[companyField.ID])
		}
	}
	// Garbage is refused.
	if _, err := repo.RestoreBackup(ctx, a, []byte(`{"version":9}`), ""); err == nil {
		t.Error("bad backup should be refused")
	}
}

func firstField(t *testing.T, repo *Repository, a knowkit.Actor, baseID, tableID string) string {
	b := must(repo.GetBase(context.Background(), a, baseID))
	for _, tb := range b.Tables {
		if tb.ID == tableID {
			return tb.Fields[0].ID
		}
	}
	t.Fatal("table missing")
	return ""
}

func TestRecordsPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	fx := kittest.Setup(t)
	tt = t
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	a := fx.User
	b := must(repo.CreateBase(ctx, a, BaseInput{Name: "Big"}))
	var sb strings.Builder
	sb.WriteString("Name,Amount,Group\n")
	const n = 100000
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "Row %d,%d,G%d\n", i, (i*7919)%100000, i%50)
	}
	start := time.Now()
	pv := must(PreviewCSV([]byte(sb.String()), true))
	pv.Columns[2].Type = TText
	res := must(repo.ImportCSV(ctx, a, b.ID, ImportRequest{TableName: "Big", Data: sb.String(), HasHeader: true, Columns: pv.Columns}))
	t.Logf("import of %d rows: %v", res.Imported, time.Since(start))
	if res.Imported != n {
		t.Fatalf("imported %d", res.Imported)
	}
	bb := must(repo.GetBase(ctx, a, b.ID))
	var tbl Table
	for _, x := range bb.Tables {
		if x.ID == res.TableID {
			tbl = x
		}
	}
	amount := tbl.Fields[1].ID
	group := tbl.Fields[2].ID
	timeIt := func(name string, q Query) *QueryResult {
		s := time.Now()
		r := must(repo.ListRecords(ctx, a, tbl.ID, q))
		d := time.Since(s)
		t.Logf("%-28s total=%-6d %v", name, r.Total, d)
		return r
	}
	timeIt("first page", Query{Limit: 100})
	timeIt("deep page (offset 90000)", Query{Limit: 100, Offset: 90000})
	r := timeIt("sort by number (SQL)", Query{Limit: 100, Sort: []SortKey{{Field: amount, Desc: true}}})
	if r.Records[0].Fields[amount] != 99999.0 && r.Records[0].Fields[amount] != 99998.0 {
		t.Errorf("top amount = %v", r.Records[0].Fields[amount])
	}
	timeIt("filter number > 99000", Query{Limit: 100, Filter: &Node{Field: amount, Op: ">", Value: 99000.0}})
	timeIt("filter+sort+group", Query{Limit: 100, Filter: &Node{Field: group, Op: "is", Value: "G7"}, Sort: []SortKey{{Field: amount}}, GroupBy: []string{group}})
	timeIt("search", Query{Limit: 100, Search: "Row 77777"})
}

// The SQL path and the in-memory path must give the same answer for the same query.
func TestSQLAndMemoryPathsAgree(t *testing.T) {
	fx := kittest.Setup(t)
	tt = t
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	a := fx.User
	b := must(repo.CreateBase(ctx, a, BaseInput{Name: "Agree"}))
	tbl := b.Tables[0]
	mk := func(name, typ string, opts map[string]any) Field {
		return *must(repo.CreateField(ctx, a, tbl.ID, FieldInput{Name: name, Type: typ, Options: opts}))
	}
	num := mk("Num", TNumber, map[string]any{"precision": 2})
	date := mk("Due", TDate, nil)
	sel := mk("Pick", TSelect, choices("Zed", "Alpha", "Mid"))
	multi := mk("Tags", TMulti, choices("a", "b", "c"))
	chk := mk("Ok", TCheckbox, nil)
	txt := mk("Note", TText, nil)
	prim := b.Tables[0].Fields[0].ID
	var in []RecordInput
	words := []string{"item10", "item2", "Item1", "beta", "Alpha x", ""}
	for i := 0; i < 60; i++ {
		f := map[string]any{prim: fmt.Sprintf("row %d", i), num.ID: float64(i%7) * 1.5, txt.ID: words[i%len(words)]}
		if i%3 != 0 {
			f[date.ID] = fmt.Sprintf("2026-03-%02d", i%28+1)
		}
		if i%4 != 0 {
			f[sel.ID] = []string{"Zed", "Alpha", "Mid"}[i%3]
		}
		if i%5 != 0 {
			f[multi.ID] = []any{[]string{"a", "b", "c"}[i%3], "c"}
		}
		f[chk.ID] = i%2 == 0
		in = append(in, RecordInput{Fields: f})
	}
	must(repo.CreateRecords(ctx, a, tbl.ID, in))
	queries := []Query{
		{Sort: []SortKey{{Field: num.ID, Desc: true}, {Field: txt.ID}}},
		{Sort: []SortKey{{Field: sel.ID}, {Field: date.ID, Desc: true}}},
		{Sort: []SortKey{{Field: txt.ID}}},
		{GroupBy: []string{sel.ID}, Sort: []SortKey{{Field: num.ID}}},
		{Filter: &Node{Field: num.ID, Op: ">=", Value: 3.0}, Sort: []SortKey{{Field: num.ID}}},
		{Filter: &Node{Field: num.ID, Op: "!=", Value: 3.0}},
		{Filter: &Node{Field: txt.ID, Op: "contains", Value: "ITEM"}},
		{Filter: &Node{Field: txt.ID, Op: "doesNotContain", Value: "item"}},
		{Filter: &Node{Field: txt.ID, Op: "isNot", Value: "beta"}},
		{Filter: &Node{Field: txt.ID, Op: "isEmpty"}},
		{Filter: &Node{Field: date.ID, Op: "isNotEmpty"}},
		{Filter: &Node{Field: date.ID, Op: "isAfter", Value: map[string]any{"mode": "exactDate", "date": "2026-03-10"}}},
		{Filter: &Node{Field: date.ID, Op: "isWithin", Value: map[string]any{"mode": "pastMonth"}}},
		{Filter: &Node{Field: sel.ID, Op: "isAnyOf", Value: []any{"Zed", "Mid"}}},
		{Filter: &Node{Field: sel.ID, Op: "isNot", Value: "Zed"}},
		{Filter: &Node{Field: multi.ID, Op: "hasAllOf", Value: []any{"a", "c"}}},
		{Filter: &Node{Field: multi.ID, Op: "hasNoneOf", Value: []any{"b"}}},
		{Filter: &Node{Field: chk.ID, Op: "is", Value: true}},
		{Filter: &Node{Field: chk.ID, Op: "is", Value: false}},
		{Filter: &Node{Logic: "or", Conds: []Node{{Field: num.ID, Op: "<", Value: 2.0}, {Logic: "and", Conds: []Node{{Field: chk.ID, Op: "is", Value: true}, {Field: sel.ID, Op: "is", Value: "Mid"}}}}}, Sort: []SortKey{{Field: prim}}},
		{Search: "alpha"}, {Search: "item1"}, {Search: "zed"},
	}
	ids := func(r *QueryResult) string {
		var s []string
		for _, x := range r.Records {
			s = append(s, x.ID)
		}
		return strings.Join(s, ",")
	}
	for i, q := range queries {
		q.Limit = 500
		repo.forceMemory = false
		fast := must(repo.ListRecords(ctx, a, tbl.ID, q))
		repo.forceMemory = true
		slow := must(repo.ListRecords(ctx, a, tbl.ID, q))
		repo.forceMemory = false
		if fast.Total != slow.Total {
			t.Errorf("query %d: total sql=%d memory=%d", i, fast.Total, slow.Total)
			continue
		}
		// Rows with equal sort keys may come in any order within a tie only if no tie-break; both break ties by seq.
		if ids(fast) != ids(slow) {
			t.Errorf("query %d (%+v): order differs between SQL and memory", i, q)
		}
	}
}
