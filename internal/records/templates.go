package records

import (
	"context"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// Template describes a starter base.
type Template struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Blurb string `json:"blurb"`
}

// Templates lists the starter bases.
var Templates = []Template{
	{"blank", "Blank base", "One empty table."},
	{"project_tracker", "Project tracker", "Projects and their tasks, linked, with progress rollups."},
	{"sales_pipeline", "Sales pipeline", "Companies, contacts and deals with a board view."},
	{"content_calendar", "Content calendar", "Posts by status and publish date with a calendar view."},
	{"inventory", "Inventory", "Items, suppliers and stock levels."},
}

func choices(names ...string) map[string]any {
	l := make([]any, len(names))
	for i, n := range names {
		l[i] = map[string]any{"name": n}
	}
	return map[string]any{"choices": l}
}

func (r *Repository) applyTemplate(ctx context.Context, a knowkit.Actor, baseID, id string) error {
	mk := func(name string) (string, string, error) {
		t, err := r.CreateTable(ctx, a, baseID, name)
		if err != nil {
			return "", "", err
		}
		s, err := loadSchema(ctx, r.pool, a.Org, baseID)
		if err != nil {
			return "", "", err
		}
		p, _ := s.primary(t.ID)
		return t.ID, p.ID, nil
	}
	field := func(table, name, typ string, opts map[string]any) (string, error) {
		f, err := r.CreateField(ctx, a, table, FieldInput{Name: name, Type: typ, Options: opts})
		if err != nil {
			return "", err
		}
		return f.ID, nil
	}
	rename := func(fid, name string) error {
		_, _, err := r.UpdateField(ctx, a, fid, FieldInput{Name: name})
		return err
	}
	view := func(table, name, typ string, cfg map[string]any) error {
		_, err := r.SaveView(ctx, a, table, "", View{Name: name, Type: typ, Config: cfg})
		return err
	}
	add := func(table string, rows ...map[string]any) ([]Record, error) {
		in := make([]RecordInput, len(rows))
		for i, m := range rows {
			in[i] = RecordInput{Fields: m}
		}
		return r.CreateRecords(ctx, a, table, in)
	}
	switch id {
	case "project_tracker":
		proj, projName, err := mk("Projects")
		if err != nil {
			return err
		}
		tasks, taskName, err := mk("Tasks")
		if err != nil {
			return err
		}
		if err := rename(projName, "Project"); err != nil {
			return err
		}
		if err := rename(taskName, "Task"); err != nil {
			return err
		}
		pstatus, _ := field(proj, "Status", TSelect, choices("Planned", "Active", "Done"))
		_, _ = field(proj, "Due", TDate, nil)
		tstatus, _ := field(tasks, "Status", TSelect, choices("To do", "In progress", "Done"))
		_, _ = field(tasks, "Due", TDate, nil)
		_, _ = field(tasks, "Estimate (h)", TNumber, map[string]any{"precision": 1})
		link, err := field(tasks, "Project", TLink, map[string]any{"table_id": proj, "allow_multiple": false})
		if err != nil {
			return err
		}
		sch, _ := loadSchema(ctx, r.pool, a.Org, baseID)
		var rev string
		if lf, ok := sch.field(link); ok {
			rev = lf.optStr("inverse_field_id")
		}
		_, _ = field(proj, "Open tasks", TCount, map[string]any{"link_field_id": rev})
		if err := view(tasks, "By status", "kanban", map[string]any{"stack_by": tstatus}); err != nil {
			return err
		}
		if err := view(proj, "By status", "kanban", map[string]any{"stack_by": pstatus}); err != nil {
			return err
		}
		ps, err := add(proj, map[string]any{"Project": "Website relaunch", "Status": "Active"}, map[string]any{"Project": "Q3 planning", "Status": "Planned"})
		if err != nil {
			return err
		}
		_, err = add(tasks,
			map[string]any{"Task": "Write copy", "Status": "In progress", "Project": []any{ps[0].ID}},
			map[string]any{"Task": "Design homepage", "Status": "To do", "Project": []any{ps[0].ID}},
			map[string]any{"Task": "Collect goals", "Status": "To do", "Project": []any{ps[1].ID}})
		return err
	case "sales_pipeline":
		co, coName, err := mk("Companies")
		if err != nil {
			return err
		}
		deals, dealName, err := mk("Deals")
		if err != nil {
			return err
		}
		if err := rename(coName, "Company"); err != nil {
			return err
		}
		if err := rename(dealName, "Deal"); err != nil {
			return err
		}
		_, _ = field(co, "Website", TURL, nil)
		_, _ = field(co, "Phone", TPhone, nil)
		stage, _ := field(deals, "Stage", TSelect, choices("Lead", "Qualified", "Proposal", "Won", "Lost"))
		value, _ := field(deals, "Value", TCurrency, map[string]any{"precision": 0})
		_, _ = field(deals, "Close date", TDate, nil)
		link, err := field(deals, "Company", TLink, map[string]any{"table_id": co, "allow_multiple": false})
		if err != nil {
			return err
		}
		sch, _ := loadSchema(ctx, r.pool, a.Org, baseID)
		lf, _ := sch.field(link)
		rev := lf.optStr("inverse_field_id")
		_, _ = field(co, "Pipeline value", TRollup, map[string]any{"link_field_id": rev, "target_field_id": value, "fn": "sum"})
		if err := view(deals, "Pipeline", "kanban", map[string]any{"stack_by": stage}); err != nil {
			return err
		}
		cs, err := add(co, map[string]any{"Company": "Northwind Traders", "Website": "northwind.example.com"}, map[string]any{"Company": "Globex Ltd"})
		if err != nil {
			return err
		}
		_, err = add(deals,
			map[string]any{"Deal": "Annual licence", "Stage": "Proposal", "Value": 120000, "Company": []any{cs[0].ID}},
			map[string]any{"Deal": "Pilot", "Stage": "Qualified", "Value": 30000, "Company": []any{cs[1].ID}})
		return err
	case "content_calendar":
		posts, name, err := mk("Posts")
		if err != nil {
			return err
		}
		if err := rename(name, "Title"); err != nil {
			return err
		}
		status, _ := field(posts, "Status", TSelect, choices("Idea", "Drafting", "Review", "Published"))
		pub, _ := field(posts, "Publish date", TDate, nil)
		_, _ = field(posts, "Channel", TMulti, choices("Blog", "Newsletter", "Social"))
		_, _ = field(posts, "Brief", TLongText, nil)
		if err := view(posts, "Calendar", "calendar", map[string]any{"date_field": pub}); err != nil {
			return err
		}
		if err := view(posts, "By status", "kanban", map[string]any{"stack_by": status}); err != nil {
			return err
		}
		_, err = add(posts, map[string]any{"Title": "Launch announcement", "Status": "Drafting", "Channel": []any{"Blog", "Social"}},
			map[string]any{"Title": "Customer story", "Status": "Idea"})
		return err
	case "inventory":
		sup, supName, err := mk("Suppliers")
		if err != nil {
			return err
		}
		items, itemName, err := mk("Items")
		if err != nil {
			return err
		}
		if err := rename(supName, "Supplier"); err != nil {
			return err
		}
		if err := rename(itemName, "Item"); err != nil {
			return err
		}
		_, _ = field(sup, "Email", TEmail, nil)
		_, _ = field(items, "SKU", TBarcode, nil)
		qty, _ := field(items, "In stock", TNumber, nil)
		reorder, _ := field(items, "Reorder at", TNumber, nil)
		price, _ := field(items, "Unit price", TCurrency, map[string]any{"precision": 2})
		_, _ = field(items, "Stock value", TFormula, map[string]any{"formula": "{In stock} * {Unit price}"})
		_, _ = field(items, "Needs reorder", TFormula, map[string]any{"formula": "IF({In stock} <= {Reorder at}, \"Reorder\", \"OK\")"})
		_, err = field(items, "Supplier", TLink, map[string]any{"table_id": sup, "allow_multiple": false})
		if err != nil {
			return err
		}
		_ = []string{qty, reorder, price}
		ss, err := add(sup, map[string]any{"Supplier": "Acme Supplies", "Email": "orders@acme.example.com"})
		if err != nil {
			return err
		}
		_, err = add(items,
			map[string]any{"Item": "Notebook A5", "SKU": "NB-A5", "In stock": 120, "Reorder at": 40, "Unit price": 85, "Supplier": []any{ss[0].ID}},
			map[string]any{"Item": "Gel pen", "SKU": "PEN-G1", "In stock": 18, "Reorder at": 50, "Unit price": 22, "Supplier": []any{ss[0].ID}})
		return err
	}
	return knowkit.Invalid("unknown template %q", id)
}
