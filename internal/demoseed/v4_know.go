package demoseed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/contracts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/records"
	"github.com/Adrient-tech/Adrient-Workspace/internal/signatures"
	"github.com/Adrient-tech/Adrient-Workspace/internal/wiki"
)

// runV4 is seed step 4: Records, Wiki, Contracts and Signatures. Dates are
// relative to today. Everything is fictional.
func (s *seeder) runV4() {
	a := knowkit.Actor{Org: s.org, User: s.admin, Admin: true}
	recRepo := records.NewRepository(s.pool)
	// Databases inside wiki pages take their access from the wiki space; the seeder acts as admin.
	recRepo.Deps.ExternalRole = func(ctx context.Context, org, user, source, ref string) int { return records.RoleAdmin }
	s.v4Records(recRepo, a)
	s.v4Wiki(recRepo, a)
	s.v4Guides()
	ids := s.v4Contracts(a)
	s.v4Signatures(a, ids)
}

// ---- Records ------------------------------------------------------------

type recKit struct {
	s    *seeder
	repo *records.Repository
	a    knowkit.Actor
}

func (k recKit) field(table, name, typ string, opts map[string]any) *records.Field {
	f, err := k.repo.CreateField(k.s.ctx, k.a, table, records.FieldInput{Name: name, Type: typ, Options: opts})
	if err != nil {
		k.s.warn("records field "+name, err)
		return &records.Field{Options: map[string]any{}}
	}
	return f
}

func (k recKit) rename(f records.Field, name string) {
	if _, _, err := k.repo.UpdateField(k.s.ctx, k.a, f.ID, records.FieldInput{Name: name}); err != nil {
		k.s.warn("records rename", err)
	}
}

func (k recKit) table(baseID, tableID, name, desc string) {
	if err := k.repo.UpdateTable(k.s.ctx, k.a, tableID, name, desc, nil); err != nil {
		k.s.warn("records table", err)
	}
}

func (k recKit) rows(table string, rows []map[string]any) []string {
	var ids []string
	for i := 0; i < len(rows); i += 100 {
		j := min(i+100, len(rows))
		in := make([]records.RecordInput, 0, j-i)
		for _, m := range rows[i:j] {
			in = append(in, records.RecordInput{Fields: m})
		}
		out, err := k.repo.CreateRecords(k.s.ctx, k.a, table, in)
		if err != nil {
			k.s.warn("records rows", err)
			continue
		}
		for _, r := range out {
			ids = append(ids, r.ID)
		}
	}
	k.s.count("records_rows", len(ids))
	return ids
}

func (k recKit) view(table, name, typ string, cfg map[string]any) {
	if _, err := k.repo.SaveView(k.s.ctx, k.a, table, "", records.View{Name: name, Type: typ, Config: cfg}); err != nil {
		k.s.warn("records view "+name, err)
	}
}

func choicesOf(names ...string) map[string]any {
	l := make([]any, len(names))
	for i, n := range names {
		l[i] = map[string]any{"name": n}
	}
	return map[string]any{"choices": l}
}

// cid finds the id of a named option of a select field.
func cid(f *records.Field, name string) string {
	for _, c := range f.Choices() {
		if c.Name == name {
			return c.ID
		}
	}
	return ""
}

func cond(field, op string, v any) map[string]any {
	return map[string]any{"field": field, "op": op, "value": v}
}

func andOf(c ...map[string]any) map[string]any {
	l := make([]any, len(c))
	for i, x := range c {
		l[i] = x
	}
	return map[string]any{"logic": "and", "conds": l}
}

func (s *seeder) ago(days int) string   { return day(s.today.AddDate(0, 0, -days)) }
func (s *seeder) ahead(days int) string { return day(s.today.AddDate(0, 0, days)) }

func (s *seeder) newBase(repo *records.Repository, a knowkit.Actor, name, desc, color string) (*records.Base, bool) {
	b, err := repo.CreateBase(s.ctx, a, records.BaseInput{Name: name, Description: desc, Color: color, OrgAccess: "edit", Template: "blank"})
	if err != nil {
		s.warn("records base "+name, err)
		return nil, false
	}
	s.count("records_bases", 1)
	return b, true
}

func (s *seeder) v4Records(repo *records.Repository, a knowkit.Actor) {
	k := recKit{s, repo, a}
	s.recSuppliers(k)
	s.recLaunch(k)
	s.recEquipment(k)
}

func (s *seeder) recSuppliers(k recKit) {
	b, ok := s.newBase(k.repo, k.a, "Suppliers register", "Approved vendors, their terms and the purchase orders placed with them.", "#126C5A")
	if !ok {
		return
	}
	sup := b.Tables[0]
	k.table(b.ID, sup.ID, "Suppliers", "One row per approved vendor")
	k.rename(sup.Fields[0], "Supplier")
	po, err := k.repo.CreateTable(s.ctx, k.a, b.ID, "Purchase orders")
	if err != nil {
		s.warn("records po table", err)
		return
	}
	cats := []string{"Raw material", "Packaging", "Logistics", "Electricals", "Stationery", "Services"}
	category := k.field(sup.ID, "Category", "single_select", choicesOf(cats...))
	k.field(sup.ID, "City", "text", nil)
	k.field(sup.ID, "Contact email", "email", nil)
	k.field(sup.ID, "Phone", "phone", nil)
	k.field(sup.ID, "GSTIN", "text", nil)
	k.field(sup.ID, "Credit days", "number", map[string]any{"precision": 0})
	rating := k.field(sup.ID, "Rating", "rating", map[string]any{"max": 5})
	active := k.field(sup.ID, "Approved", "checkbox", nil)
	k.field(sup.ID, "Onboarded", "date", nil)
	k.field(sup.ID, "Website", "url", nil)
	k.field(sup.ID, "Notes", "long_text", nil)

	k.rename(mustPrimary(k, b.ID, po.ID), "PO number")
	status := k.field(po.ID, "Status", "single_select", choicesOf("Draft", "Sent", "Part received", "Received", "Cancelled"))
	amount := k.field(po.ID, "Amount", "currency", map[string]any{"precision": 2, "symbol": "₹"})
	k.field(po.ID, "Ordered on", "date", nil)
	exp := k.field(po.ID, "Expected on", "date", nil)
	k.field(po.ID, "Delivery window", "duration", nil)
	link := k.field(po.ID, "Supplier", "link", map[string]any{"table_id": sup.ID, "allow_multiple": false})
	rev, _ := link.Options["inverse_field_id"].(string)
	k.field(po.ID, "Days to expected", "formula", map[string]any{"formula": "DATETIME_DIFF({Expected on}, TODAY(), 'days')"})
	if rev != "" {
		k.field(sup.ID, "Open POs", "count", map[string]any{"link_field_id": rev})
		k.field(sup.ID, "Total ordered", "rollup", map[string]any{"link_field_id": rev, "target_field_id": amount.ID, "fn": "sum"})
	}
	k.field(sup.ID, "Added", "created_time", nil)

	used := map[string]bool{}
	var rows []map[string]any
	for i := 0; i < 32; i++ {
		name := s.r.company()
		for used[name] {
			name = s.r.company()
		}
		used[name] = true
		f, l := s.r.personName()
		city := cities[s.r.Intn(len(cities))]
		row := map[string]any{
			"Supplier": name, "Category": cats[i%len(cats)], "City": city,
			"Contact email": demoEmail(f, l), "Phone": s.r.phone(),
			"GSTIN":       fmt.Sprintf("%02dAB%cPK%04dM1Z%d", 24+i%12, 'C'+rune(i%20), 1000+s.r.Intn(8999), s.r.Intn(9)),
			"Credit days": []int{15, 30, 45, 60}[i%4], "Rating": 2 + s.r.Intn(4), "Approved": i%7 != 0,
			"Onboarded": s.ago(30 + s.r.Intn(700)), "Website": "https://www." + slug(name) + ".example",
		}
		if i%5 == 0 {
			row["Notes"] = "Preferred vendor for bulk orders. Review rates every quarter."
		}
		rows = append(rows, row)
	}
	supIDs := s.recIDs(k, sup.ID, rows)
	if len(supIDs) == 0 {
		return
	}
	var porows []map[string]any
	st := []string{"Draft", "Sent", "Part received", "Received", "Received", "Received", "Cancelled"}
	for i := 0; i < 48; i++ {
		ordered := 5 + s.r.Intn(120)
		stt := st[s.r.Intn(len(st))]
		if ordered < 15 && stt == "Received" {
			stt = "Sent"
		}
		porows = append(porows, map[string]any{
			"PO number": fmt.Sprintf("PO-26-%04d", 101+i), "Status": stt,
			"Amount": float64(8000 + s.r.Intn(240000)), "Ordered on": s.ago(ordered), "Expected on": s.ago(ordered - 10 - s.r.Intn(12)),
			"Delivery window": float64((3 + s.r.Intn(10)) * 86400), "Supplier": []any{supIDs[(i*5+i/3)%len(supIDs)]},
		})
	}
	k.rows(po.ID, porows)
	k.view(sup.ID, "Approved suppliers", "grid", map[string]any{"filter": andOf(cond(active.ID, "is", true)), "sort": []any{map[string]any{"field": rating.ID, "desc": true}}})
	k.view(sup.ID, "By category", "grid", map[string]any{"group_by": []any{category.ID}})
	k.view(po.ID, "Pipeline", "kanban", map[string]any{"stack_by": status.ID})
	k.view(po.ID, "Delivery calendar", "calendar", map[string]any{"date_field": exp.ID})
	k.view(po.ID, "Open orders", "grid", map[string]any{"filter": andOf(cond(status.ID, "isAnyOf", []any{cid(status, "Sent"), cid(status, "Part received")}))})
}

// recIDs creates rows; for tables with a link back it fills links in the second table.
func (s *seeder) recIDs(k recKit, table string, rows []map[string]any) []string {
	return k.rows(table, rows)
}

func mustPrimary(k recKit, baseID, tableID string) records.Field {
	b, err := k.repo.GetBase(k.s.ctx, k.a, baseID)
	if err == nil {
		for _, t := range b.Tables {
			if t.ID == tableID && len(t.Fields) > 0 {
				return t.Fields[0]
			}
		}
	}
	return records.Field{}
}

func (s *seeder) recLaunch(k recKit) {
	b, ok := s.newBase(k.repo, k.a, "Product launch tracker", "Launches in flight and the tasks that get each one out of the door.", "#3B5BDB")
	if !ok {
		return
	}
	ln := b.Tables[0]
	k.table(b.ID, ln.ID, "Launches", "")
	k.rename(ln.Fields[0], "Launch")
	tk, err := k.repo.CreateTable(s.ctx, k.a, b.ID, "Tasks")
	if err != nil {
		s.warn("records tasks table", err)
		return
	}
	k.rename(mustPrimary(k, b.ID, tk.ID), "Task")
	stages := []string{"Idea", "Planning", "Build", "Pre-launch", "Launched"}
	stage := k.field(ln.ID, "Stage", "single_select", choicesOf(stages...))
	k.field(ln.ID, "Product line", "single_select", choicesOf("Apparel", "Kitchen", "Electricals", "Grocery"))
	k.field(ln.ID, "Owner", "text", nil)
	date := k.field(ln.ID, "Launch date", "date", nil)
	k.field(ln.ID, "Budget", "currency", map[string]any{"precision": 0, "symbol": "₹"})
	k.field(ln.ID, "Progress", "percent", map[string]any{"precision": 0})
	k.field(ln.ID, "Priority", "rating", map[string]any{"max": 3})
	k.field(ln.ID, "Channels", "multi_select", choicesOf("WhatsApp", "Email", "Instagram", "Retail stores", "Marketplace"))
	k.field(ln.ID, "Brief", "long_text", nil)
	k.field(ln.ID, "Launch code", "barcode", nil)

	tstatus := k.field(tk.ID, "Status", "single_select", choicesOf("To do", "In progress", "Blocked", "Done"))
	due := k.field(tk.ID, "Due", "date", nil)
	k.field(tk.ID, "Estimate (h)", "number", map[string]any{"precision": 1})
	k.field(tk.ID, "Assignee", "text", nil)
	k.field(tk.ID, "Done", "checkbox", nil)
	link := k.field(tk.ID, "Launch", "link", map[string]any{"table_id": ln.ID, "allow_multiple": false})
	rev, _ := link.Options["inverse_field_id"].(string)
	k.field(tk.ID, "Task no.", "autonumber", nil)
	if rev != "" {
		k.field(ln.ID, "Open tasks", "count", map[string]any{"link_field_id": rev})
		k.field(ln.ID, "Effort (h)", "rollup", map[string]any{"link_field_id": rev, "target_field_id": mustFieldID(k, b.ID, tk.ID, "Estimate (h)"), "fn": "sum"})
	}
	launches := []struct{ name, line, stage, brief string }{
		{"Monsoon rain jackets", "Apparel", "Launched", "Light waterproof jackets in four colours, sold through retail stores and the marketplace."},
		{"Insulated steel bottle 750ml", "Kitchen", "Pre-launch", "Double-wall bottle with a leak-proof cap. Photo shoot done, pricing under review."},
		{"Festive gift hampers", "Grocery", "Build", "Diwali hampers with dry fruits, sweets and a brass diya. Packaging samples arrive next week."},
		{"Smart LED batten 20W", "Electricals", "Planning", "Wi-Fi batten with app control. Needs BIS certification before launch."},
		{"Cotton kurta range", "Apparel", "Build", "Eight designs from a Surat mill, sizes S to XXL."},
		{"Pressure cooker 5L", "Kitchen", "Idea", "Induction-base cooker; supplier shortlist pending."},
		{"Basmati rice 10kg bag", "Grocery", "Launched", "Premium aged basmati in a resealable bag."},
		{"Desk lamp with wireless charger", "Electricals", "Planning", "Targeted at work-from-home offices."},
		{"Winter blanket collection", "Apparel", "Idea", "Fleece and wool blends for the November season."},
		{"Copper water bottle", "Kitchen", "Pre-launch", "Hammered copper finish; lab report on copper content received."},
		{"Spice sampler box", "Grocery", "Planning", "Twelve regional spice blends in a gift box."},
		{"Extension board with surge guard", "Electricals", "Build", "Four sockets, USB-C port and 2 metre cable."},
	}
	owners := []string{"Asha Menon", "Rohit Iyer", "Meenal Joshi", "Tarun Gill", "Divya Nambiar"}
	chans := []string{"WhatsApp", "Email", "Instagram", "Retail stores", "Marketplace"}
	var lrows []map[string]any
	for i, l := range launches {
		row := map[string]any{"Launch": l.name, "Product line": l.line, "Stage": l.stage, "Owner": owners[i%len(owners)],
			"Launch date": s.ahead(-40 + i*13), "Budget": float64(150000 + 75000*(i%6)), "Priority": 1 + i%3, "Brief": l.brief,
			"Channels": []any{chans[i%5], chans[(i+2)%5]}, "Launch code": fmt.Sprintf("LCH-%03d-%d", 101+i, 2026)}
		switch l.stage {
		case "Launched":
			row["Progress"] = 1.0
		case "Pre-launch":
			row["Progress"] = 0.85
		case "Build":
			row["Progress"] = 0.55
		case "Planning":
			row["Progress"] = 0.25
		default:
			row["Progress"] = 0.05
		}
		lrows = append(lrows, row)
	}
	lids := k.rows(ln.ID, lrows)
	if len(lids) == 0 {
		return
	}
	titles := []string{"Finalise product specification", "Get supplier quotes", "Approve sample", "Photo shoot", "Write product copy", "Set price and margin", "Create GST HSN mapping",
		"Packaging artwork", "Print labels", "Upload to marketplace", "Brief the sales team", "Plan WhatsApp broadcast", "Prepare launch email", "Stock first batch", "Quality check"}
	statuses := []string{"To do", "In progress", "Blocked", "Done", "Done", "Done"}
	var trows []map[string]any
	for i := 0; i < 48; i++ {
		li := i % len(lids)
		stt := statuses[s.r.Intn(len(statuses))]
		trow := map[string]any{"Task": titles[i%len(titles)], "Status": stt, "Due": s.ahead(-25 + s.r.Intn(60)), "Estimate (h)": float64(1 + s.r.Intn(16)),
			"Assignee": owners[s.r.Intn(len(owners))], "Launch": []any{lids[li]}}
		if stt == "Done" {
			trow["Done"] = true
		}
		trows = append(trows, trow)
	}
	k.rows(tk.ID, trows)
	k.view(ln.ID, "By stage", "kanban", map[string]any{"stack_by": stage.ID})
	k.view(ln.ID, "Launch calendar", "calendar", map[string]any{"date_field": date.ID})
	k.view(tk.ID, "Board", "kanban", map[string]any{"stack_by": tstatus.ID})
	k.view(tk.ID, "Due soon", "grid", map[string]any{"filter": andOf(cond(tstatus.ID, "isNot", cid(tstatus, "Done"))), "sort": []any{map[string]any{"field": due.ID}}})
}

func mustFieldID(k recKit, baseID, tableID, name string) string {
	b, err := k.repo.GetBase(k.s.ctx, k.a, baseID)
	if err == nil {
		for _, t := range b.Tables {
			if t.ID == tableID {
				for _, f := range t.Fields {
					if f.Name == name {
						return f.ID
					}
				}
			}
		}
	}
	return ""
}

func (s *seeder) recEquipment(k recKit) {
	b, ok := s.newBase(k.repo, k.a, "Equipment assets", "Laptops, machines and vehicles with warranty dates and service history.", "#C2410C")
	if !ok {
		return
	}
	as := b.Tables[0]
	k.table(b.ID, as.ID, "Assets", "")
	k.rename(as.Fields[0], "Asset tag")
	lg, err := k.repo.CreateTable(s.ctx, k.a, b.ID, "Maintenance log")
	if err != nil {
		s.warn("records log table", err)
		return
	}
	k.rename(mustPrimary(k, b.ID, lg.ID), "Entry")
	cat := k.field(as.ID, "Category", "single_select", choicesOf("Laptop", "Desktop", "Printer", "Forklift", "Delivery van", "Network", "Furniture"))
	k.field(as.ID, "Make and model", "text", nil)
	k.field(as.ID, "Serial", "barcode", nil)
	loc := k.field(as.ID, "Location", "single_select", choicesOf("Pune HQ", "Nashik warehouse", "Indore branch", "Remote"))
	k.field(as.ID, "Assigned to", "text", nil)
	k.field(as.ID, "Purchased", "date", nil)
	k.field(as.ID, "Cost", "currency", map[string]any{"precision": 0, "symbol": "₹"})
	warranty := k.field(as.ID, "Warranty until", "date", nil)
	state := k.field(as.ID, "Status", "single_select", choicesOf("In use", "In stock", "Under repair", "Retired"))
	k.field(as.ID, "Condition", "rating", map[string]any{"max": 5})
	k.field(as.ID, "Warranty days left", "formula", map[string]any{"formula": "DATETIME_DIFF({Warranty until}, TODAY(), 'days')"})
	k.field(as.ID, "Vendor phone", "phone", nil)
	k.field(as.ID, "Manual", "url", nil)

	k.field(lg.ID, "Date", "date", nil)
	k.field(lg.ID, "Type", "single_select", choicesOf("Service", "Repair", "Inspection", "Upgrade"))
	k.field(lg.ID, "Cost", "currency", map[string]any{"precision": 0, "symbol": "₹"})
	k.field(lg.ID, "Downtime", "duration", nil)
	k.field(lg.ID, "Notes", "long_text", nil)
	link := k.field(lg.ID, "Asset", "link", map[string]any{"table_id": as.ID, "allow_multiple": false})
	rev, _ := link.Options["inverse_field_id"].(string)
	if rev != "" {
		k.field(as.ID, "Service entries", "count", map[string]any{"link_field_id": rev})
		k.field(as.ID, "Service spend", "rollup", map[string]any{"link_field_id": rev, "target_field_id": mustFieldID(k, b.ID, lg.ID, "Cost"), "fn": "sum"})
	}
	cats := []string{"Laptop", "Laptop", "Desktop", "Printer", "Forklift", "Delivery van", "Network", "Furniture"}
	models := map[string][]string{"Laptop": {"Lenovo ThinkPad E14", "HP ProBook 440", "Dell Latitude 3440"}, "Desktop": {"HP ProDesk 400", "Dell OptiPlex 3000"},
		"Printer": {"Canon imageRUNNER 2425", "Epson L3250"}, "Forklift": {"Godrej GX 2.5T", "Voltas 3T electric"}, "Delivery van": {"Tata Ace Gold", "Mahindra Jeeto"},
		"Network": {"Ubiquiti U6 Pro", "TP-Link TL-SG2428"}, "Furniture": {"Godrej ergonomic chair", "Featherlite workstation"}}
	locs := []string{"Pune HQ", "Nashik warehouse", "Indore branch", "Remote"}
	states := []string{"In use", "In use", "In use", "In stock", "Under repair", "Retired"}
	var arows []map[string]any
	for i := 0; i < 42; i++ {
		c := cats[i%len(cats)]
		m := models[c][s.r.Intn(len(models[c]))]
		f, l := s.r.personName()
		bought := 60 + s.r.Intn(1100)
		row := map[string]any{"Asset tag": fmt.Sprintf("AST-%04d", 1001+i), "Category": c, "Make and model": m, "Serial": fmt.Sprintf("SN%08d", 40000000+s.r.Intn(9999999)),
			"Location": locs[s.r.Intn(len(locs))], "Purchased": s.ago(bought), "Warranty until": s.ago(bought - 365*(1+s.r.Intn(3))),
			"Status": states[s.r.Intn(len(states))], "Condition": 2 + s.r.Intn(4), "Vendor phone": s.r.phone(),
			"Manual": "https://support." + slug(strings.Fields(m)[0]) + ".example/manuals", "Cost": float64(18000 + s.r.Intn(90000))}
		switch c {
		case "Forklift":
			row["Cost"] = float64(650000 + s.r.Intn(300000))
		case "Delivery van":
			row["Cost"] = float64(480000 + s.r.Intn(120000))
		case "Network", "Printer":
			row["Cost"] = float64(25000 + s.r.Intn(70000))
		}
		if row["Status"] == "In use" && (c == "Laptop" || c == "Desktop") {
			row["Assigned to"] = f + " " + l
		}
		arows = append(arows, row)
	}
	aids := k.rows(as.ID, arows)
	if len(aids) == 0 {
		return
	}
	types := []string{"Service", "Repair", "Inspection", "Upgrade"}
	notes := []string{"Routine service, filters replaced.", "Battery replaced under warranty.", "Annual safety inspection passed.", "RAM upgraded to 16 GB.", "Fan noise fixed.", "Firmware updated."}
	var lrows []map[string]any
	for i := 0; i < 34; i++ {
		t := types[s.r.Intn(len(types))]
		lrows = append(lrows, map[string]any{"Entry": fmt.Sprintf("%s %s", t, s.ago(3+s.r.Intn(300))), "Date": s.ago(3 + s.r.Intn(300)), "Type": t,
			"Cost": float64(500 + s.r.Intn(18000)), "Downtime": float64((1 + s.r.Intn(8)) * 3600), "Notes": notes[s.r.Intn(len(notes))], "Asset": []any{aids[(i*3)%len(aids)]}})
	}
	k.rows(lg.ID, lrows)
	k.view(as.ID, "In use", "grid", map[string]any{"filter": andOf(cond(state.ID, "is", cid(state, "In use"))), "sort": []any{map[string]any{"field": warranty.ID}}})
	k.view(as.ID, "By location", "grid", map[string]any{"group_by": []any{loc.ID}})
	k.view(as.ID, "Lifecycle", "kanban", map[string]any{"stack_by": state.ID})
	k.view(as.ID, "Warranty calendar", "calendar", map[string]any{"date_field": warranty.ID})
	_ = cat
}

// ---- Wiki ---------------------------------------------------------------

func mdToNodes(md string) []any {
	_, content := wiki.MarkdownToDoc(md)
	var d struct {
		Content []any `json:"content"`
	}
	_ = json.Unmarshal([]byte(content), &d)
	return d.Content
}

func tableNode(rows [][]string) map[string]any {
	var trs []any
	for ri, r := range rows {
		var cells []any
		for _, c := range r {
			typ := "tableCell"
			if ri == 0 {
				typ = "tableHeader"
			}
			p := map[string]any{"type": "paragraph"}
			if c != "" {
				p["content"] = mdInline(c)
			}
			cells = append(cells, map[string]any{"type": typ, "content": []any{p}})
		}
		trs = append(trs, map[string]any{"type": "tableRow", "content": cells})
	}
	return map[string]any{"type": "table", "content": trs}
}

func mdInline(s string) []any {
	nodes := mdToNodes(s)
	if len(nodes) > 0 {
		if m, ok := nodes[0].(map[string]any); ok {
			if c, ok := m["content"].([]any); ok {
				return c
			}
		}
	}
	return []any{map[string]any{"type": "text", "text": s}}
}

// pageDoc converts Markdown with GitHub-style tables to a TipTap document.
func pageDoc(md string) json.RawMessage {
	lines := strings.Split(md, "\n")
	var out []any
	var chunk []string
	flush := func() {
		if len(chunk) > 0 {
			out = append(out, mdToNodes(strings.Join(chunk, "\n"))...)
			chunk = nil
		}
	}
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			flush()
			var rows [][]string
			for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				t := strings.Trim(strings.TrimSpace(lines[i]), "|")
				if strings.Trim(t, "-| :") == "" {
					continue
				}
				var cells []string
				for _, c := range strings.Split(t, "|") {
					cells = append(cells, strings.TrimSpace(c))
				}
				rows = append(rows, cells)
			}
			i--
			out = append(out, tableNode(rows))
			continue
		}
		chunk = append(chunk, lines[i])
	}
	flush()
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": out})
	return b
}

type wpage struct {
	key, parent, title, icon, md string
}

func (s *seeder) v4Wiki(recRepo *records.Repository, a knowkit.Actor) {
	repo := wiki.NewRepository(s.pool)
	repo.Deps.Records = recRepo
	sp, err := repo.CreateSpace(s.ctx, a, wiki.SpaceInput{Name: "Company handbook", Icon: "📘", OrgAccess: "edit"})
	if err != nil {
		s.warn("wiki space", err)
		return
	}
	s.count("wiki_spaces", 1)
	pages := wikiPages()
	ids := map[string]string{}
	// CreateSpace adds a starter page; reuse it as the handbook home.
	tree, _ := repo.Tree(s.ctx, a, sp.ID, false)
	if len(tree) > 0 {
		ids["home"] = tree[0].ID
	}
	for _, p := range pages {
		if p.key == "home" && ids["home"] != "" {
			continue
		}
		in := wiki.PageInput{SpaceID: sp.ID, Title: p.title, Icon: p.icon}
		if p.parent != "" {
			in.ParentID = ids[p.parent]
		}
		pg, err := repo.CreatePage(s.ctx, a, in)
		if err != nil {
			s.warn("wiki page "+p.title, err)
			continue
		}
		ids[p.key] = pg.ID
	}
	n := 0
	for _, p := range pages {
		id := ids[p.key]
		if id == "" {
			continue
		}
		md := p.md
		for key, pid := range ids {
			md = strings.ReplaceAll(md, "{{"+key+"}}", "/wiki?page="+pid)
		}
		doc := pageDoc(md)
		title, icon := p.title, p.icon
		if _, err := repo.UpdatePage(s.ctx, a, id, wiki.PageUpdate{Title: &title, Icon: &icon, Content: doc, Version: 1}); err != nil {
			s.warn("wiki content "+p.title, err)
			continue
		}
		n++
	}
	s.count("wiki_pages", n)
	if _, ok := ids["it"]; !ok {
		return
	}
	// A database page: the IT request log.
	db, err := repo.CreatePage(s.ctx, a, wiki.PageInput{SpaceID: sp.ID, ParentID: "", Title: "IT request log", Icon: "🗂", Kind: "database"})
	if err != nil {
		s.warn("wiki database", err)
		return
	}
	s.count("wiki_pages", 1)
	k := recKit{s, recRepo, a}
	if db.RecordsTableID == "" {
		return
	}
	pf := mustFieldByPos(k, db.RecordsBaseID, db.RecordsTableID)
	if pf.ID != "" {
		k.rename(pf, "Request")
	}
	kind := k.field(db.RecordsTableID, "Type", "single_select", choicesOf("Hardware", "Software access", "Network", "Account", "Other"))
	status := k.field(db.RecordsTableID, "Status", "single_select", choicesOf("Open", "In progress", "Resolved"))
	k.field(db.RecordsTableID, "Raised by", "text", nil)
	k.field(db.RecordsTableID, "Raised on", "date", nil)
	k.field(db.RecordsTableID, "Urgent", "checkbox", nil)
	reqs := []struct{ t, k, st string }{
		{"New laptop for finance intern", "Hardware", "In progress"}, {"Access to Tally server", "Software access", "Resolved"}, {"Wi-Fi drops in the Nashik warehouse", "Network", "Open"},
		{"Reset 2FA after phone change", "Account", "Resolved"}, {"Install Adobe Acrobat on 4 desktops", "Software access", "In progress"}, {"Second monitor for the design desk", "Hardware", "Open"},
		{"Shared mailbox for accounts@", "Account", "Resolved"}, {"Printer jam on first floor", "Hardware", "Resolved"}, {"VPN not connecting from home", "Network", "In progress"},
		{"Barcode scanner pairing", "Hardware", "Open"}, {"Add new joiner to payroll group", "Account", "Resolved"}, {"Licence for project management tool", "Software access", "Open"},
	}
	var rows []map[string]any
	for i, r := range reqs {
		f, l := s.r.personName()
		row := map[string]any{"Request": r.t, "Type": r.k, "Status": r.st, "Raised by": f + " " + l, "Raised on": s.ago(1 + i*2)}
		if i%5 == 2 {
			row["Urgent"] = true
		}
		rows = append(rows, row)
	}
	k.rows(db.RecordsTableID, rows)
	k.view(db.RecordsTableID, "Board", "kanban", map[string]any{"stack_by": status.ID})
	_ = kind
}

func mustFieldByPos(k recKit, baseID, tableID string) records.Field {
	return mustPrimary(k, baseID, tableID)
}

func wikiPages() []wpage {
	return []wpage{
		{"home", "", "Welcome to Northwind Trading", "🏠", `## Welcome

This handbook is the single place for how we work at **Northwind Trading Pvt Ltd**, a small distribution company with offices in Pune, Nashik and Indore.

## Start here

- New to the team? Read [Onboarding]({{onboarding}}) first.
- Something not working? See [IT how-tos]({{it}}).
- Questions about leave, expenses or security? See [Policies]({{policies}}).
- Who does what: [Teams and contacts]({{teams}}).

## Quick facts

| Topic | Where to look |
|---|---|
| Office hours | [Working hours and holidays]({{hours}}) |
| Reimbursements | [Expense policy]({{expense}}) |
| Lost laptop or phishing email | [Information security]({{infosec}}) |

> This handbook is edited by everyone. If something is out of date, fix it and tell People and Culture.`},
		{"story", "home", "Our story and values", "🌱", `## Who we are

Northwind Trading began in 2011 as a two-person stationery supplier in Pune. Today about 60 people supply packaging, electricals and groceries to retailers across Maharashtra, Madhya Pradesh and Gujarat.

## Our values

1. **Be dependable.** Deliver when we say we will, and say early when we cannot.
2. **Keep it simple.** Prefer a clear process over a clever one.
3. **Look after each other.** Share credit, cover for a teammate, ask for help.
4. **Spend like it is our own money.**

## Milestones

| Year | What happened |
|---|---|
| 2011 | Founded in Pune |
| 2016 | First warehouse in Nashik |
| 2020 | Moved orders and invoicing to Adrient Workspace |
| 2024 | Indore branch opened |

Next: [Teams and contacts]({{teams}}).`},
		{"teams", "home", "Teams and contacts", "👥", `## Teams

| Team | Lead | Email | Extension |
|---|---|---|---|
| Sales | Rohit Iyer | sales@demo.adrient.example | 201 |
| Finance | Meenal Joshi | accounts@demo.adrient.example | 301 |
| Operations | Tarun Gill | ops@demo.adrient.example | 401 |
| People and Culture | Divya Nambiar | people@demo.adrient.example | 501 |
| IT | Asha Menon | it@demo.adrient.example | 601 |

## Escalation

- Customer complaint: Sales lead, then the founders.
- Stock or delivery problem: Operations lead, same day.
- Payment or GST query: Finance lead.

See also [IT how-tos]({{it}}) for support hours.`},
		{"hours", "home", "Working hours and holidays", "🕘", `## Working hours

- Offices: Monday to Saturday, 9:30 to 18:30 (second and fourth Saturday off).
- Warehouses: two shifts, 7:00 to 15:00 and 14:00 to 22:00.
- Core hours for meetings: 11:00 to 16:00.

## Holidays 2026-27

| Date | Holiday |
|---|---|
| 26 January | Republic Day |
| 14 March | Holi |
| 15 August | Independence Day |
| 2 October | Gandhi Jayanti |
| 8 November | Diwali (3 days) |
| 25 December | Christmas |

Leave rules are in the [Leave and attendance policy]({{leave}}).`},
		{"onboarding", "", "Onboarding", "🚀", `## Welcome aboard

Your first two weeks are about getting set up, meeting people and shipping one small piece of real work.

- [First week checklist]({{week1}})
- [Tools and accounts]({{tools}})

## Who helps you

| Need | Ask |
|---|---|
| Laptop, email, access | IT (extension 601) |
| Salary, leave, documents | People and Culture |
| What to work on | Your manager and your buddy |

Back to the [handbook home]({{home}}).`},
		{"week1", "onboarding", "First week checklist", "✅", `## Day 1

- [ ] Collect laptop and ID card from the front desk
- [ ] Sign in with your work email and set up two-step verification
- [ ] Meet your manager and your buddy
- [ ] Read [Our story and values]({{story}})

## Days 2 to 3

- [ ] Complete the [Information security]({{infosec}}) module
- [ ] Join the team chat channels
- [ ] Add yourself to the shared calendar

## By Friday

- [ ] Submit your bank and PAN details to People and Culture
- [ ] Review the [Leave and attendance policy]({{leave}})
- [ ] Share one question or idea in the Friday team huddle`},
		{"tools", "onboarding", "Tools and accounts you will get", "🧰", `## Your accounts

| Tool | Used for | How to get access |
|---|---|---|
| Mail and Calendar | Everyday communication | Created for you on day 1 |
| Chat | Team channels and quick questions | Invitation by email |
| Drive | Shared files | Folder access by team |
| Invoicing and Inventory | Sales and stock | Ask your manager to request a role |
| Payroll portal | Payslips | Available after the first pay cycle |

## Tips

- Use Drive instead of attaching large files to mail.
- Keep personal files out of shared folders.

Having trouble? See [Reset your password]({{password}}).`},
		{"it", "", "IT how-tos", "💻", `## Support

IT support is available Monday to Saturday, 9:30 to 18:30. Write to **it@demo.adrient.example** or log a request in the [IT request log]({{itlog}}).

## Guides

- [Reset your password and set up two-step verification]({{password}})
- [Connect to the office VPN]({{vpn}})
- [Request new hardware or software]({{request}})

## Rules of thumb

1. Restart first. It fixes more than you would expect.
2. Never share your password, even with IT.
3. Report anything suspicious straight away.`},
		{"password", "it", "Reset your password and set up 2FA", "🔑", `## Forgot your password

1. Open the sign-in page and choose **Forgot password**.
2. Enter your work email. A reset link arrives within a minute.
3. Choose a password of at least 12 characters.

## Set up two-step verification

1. Install an authenticator app on your phone.
2. Open **Settings**, then **Security**, then **Two-step verification**.
3. Scan the QR code and enter the six-digit code.
4. Save the recovery codes somewhere safe, not on your laptop.

## Changed your phone?

Ask IT to reset your two-step verification. We will verify your identity first, as described in [Information security]({{infosec}}).`},
		{"vpn", "it", "Connect to the office VPN", "🌐", `## Before you start

You need your work laptop and two-step verification on your account.

## Steps

1. Open the VPN app from the system tray.
2. Choose the **Pune HQ** profile.
3. Sign in with your work email and the code from your authenticator.
4. Wait for the status to show **Connected**.

## Troubleshooting

| Problem | Fix |
|---|---|
| Stuck on connecting | Switch Wi-Fi off and on, then retry |
| Wrong code | Check the time on your phone is set automatically |
| Files do not open | Disconnect, reconnect, then reopen Drive |

Still stuck? Log it in the [IT request log]({{itlog}}).`},
		{"request", "it", "Request new hardware or software", "🛒", `## Hardware

Laptops are replaced every four years. For anything extra, such as a second monitor or a headset, ask your manager to approve it and send the request to IT.

## Software

- Standard tools are installed for you.
- Paid software needs approval from your department lead and Finance.
- Free tools must be checked by IT before installation.

## Typical timelines

| Request | Time |
|---|---|
| Accessory from stock | 1 day |
| New laptop | 5 to 7 working days |
| Software licence | 2 to 3 working days after approval |

Spending rules are in the [Expense policy]({{expense}}).`},
		{"policies", "", "Policies", "📜", `## Our policies

- [Leave and attendance policy]({{leave}})
- [Expense reimbursement policy]({{expense}})
- [Information security policy]({{infosec}})

Policies are reviewed every April by People and Culture and Finance. Questions can go to people@demo.adrient.example.`},
		{"leave", "policies", "Leave and attendance policy", "🏖", `## Entitlements per year

| Leave type | Days | Notes |
|---|---|---|
| Casual leave | 8 | Up to 3 days at a time |
| Earned leave | 15 | Accrues monthly, carry forward up to 30 days |
| Sick leave | 7 | Medical certificate after 2 days |
| Maternity | 26 weeks | As per the Maternity Benefit Act |

## How to apply

1. Apply in the HR app at least 3 working days ahead (earlier for more than 3 days).
2. Your manager approves or declines within 2 working days.
3. For sick leave, tell your manager before 10:00.

## Attendance

Late marks: three late arrivals in a month count as half a day of casual leave. See [Working hours and holidays]({{hours}}).`},
		{"expense", "policies", "Expense reimbursement policy", "🧾", `## What we reimburse

- Travel for work at actuals (economy class, 3AC train)
- Client meals up to INR 1,500 per person
- Phone and internet allowance as per grade

## Limits

| Item | Limit |
|---|---|
| Hotel, metro city | INR 4,500 per night |
| Hotel, other city | INR 3,000 per night |
| Local conveyance | Actuals, with receipts |
| Daily allowance | INR 600 |

## How to claim

1. Keep every bill. GST invoices are required for hotels and meals.
2. Submit the claim within 30 days.
3. Finance pays approved claims with the next salary.

Software and hardware are covered in [Request new hardware or software]({{request}}).`},
		{"infosec", "policies", "Information security policy", "🛡", `## The basics

- Lock your screen when you step away.
- Use the password manager. Never reuse passwords.
- Two-step verification is mandatory for everyone.
- Do not store customer data on personal devices.

## Phishing

If an email asks for a password, a payment or a gift card, do not click. Forward it to **security@demo.adrient.example** and delete it.

## If something goes wrong

1. Lost laptop or phone: tell IT immediately. We will lock the account.
2. Wrong email sent: tell your manager and IT the same day.
3. Suspected breach: call extension 601 at once.

Setup steps are in [Reset your password and set up 2FA]({{password}}).`},
	}
}

// ---- Contracts ----------------------------------------------------------

const noEnd = 99999

type contractIDs struct{ lease, nda, vendor, offer, saas string }

func (s *seeder) v4Contracts(a knowkit.Actor) contractIDs {
	repo := contracts.NewRepository(s.pool)
	var ids contractIDs
	t := true
	type ob struct {
		title, owner string
		due          int
		every        string
		done         bool
	}
	type c struct {
		in    contracts.Input
		obs   []ob
		dec   string
		until int
		key   *string
	}
	mk := func(title, typ, party, billing string, value float64, startDaysAgo, endInDays int, auto bool, notice int, status string, tags ...string) contracts.Input {
		in := contracts.Input{Title: title, Type: typ, PartyName: party, Billing: billing, Value: value, Currency: "INR", Status: status,
			StartDate: s.ago(startDaysAgo), AutoRenew: auto, NoticeDays: notice, Remind: &t, Tags: tags, TermMonths: 12}
		if endInDays != noEnd {
			in.EndDate = s.ahead(endInDays)
		}
		return in
	}
	list := []c{
		{in: mk("Pune HQ office lease", "lease", "Sahyadri Realty LLP", "monthly", 185000, 400, 330, false, 90, "active", "premises"), key: &ids.lease,
			obs: []ob{{"Pay monthly rent by the 5th", "us", 12, "monthly", false}, {"Renew fire safety certificate", "us", 45, "", false}, {"Landlord to service lifts quarterly", "them", 25, "quarterly", false}}},
		{in: mk("Mutual NDA with Kaveri Retail", "nda", "Kaveri Retail Pvt Ltd", "one-time", 0, 120, 245, false, 30, "active", "legal"), key: &ids.nda,
			obs: []ob{{"Return or destroy confidential material on exit", "mutual", 245, "", false}}},
		{in: mk("Packaging supply agreement", "vendor", "Narmada Packaging Industries", "quarterly", 420000, 280, 85, true, 60, "active", "supply", "packaging"), key: &ids.vendor,
			dec: contracts.DecRenewing,
			obs: []ob{{"Quarterly rate review", "mutual", 20, "quarterly", false}, {"Supplier to deliver quality report", "them", -6, "", false}, {"Issue purchase forecast", "us", 8, "", false}}},
		{in: mk("Adrient Workspace subscription", "service", "Adrient Tech", "annual", 360000, 320, 45, true, 30, "active", "software"), key: &ids.saas,
			obs: []ob{{"Review seat count before renewal", "us", 15, "", false}}},
		{in: mk("Cold storage services, Nashik", "service", "Godavari Cold Chain", "monthly", 96000, 700, 12, false, 30, "active", "logistics"), dec: contracts.DecNotRenewing,
			obs: []ob{{"Collect final temperature logs", "them", 10, "", false}, {"Settle last invoice", "us", 18, "", false}}},
		{in: mk("Annual maintenance, forklifts", "service", "Coromandel Engineering Works", "annual", 148000, 310, 55, true, 45, "active", "equipment"),
			obs: []ob{{"Quarterly forklift service visit", "them", -12, "quarterly", false}, {"Share service reports", "them", 30, "", true}}},
		{in: mk("Software licence: accounting add-on", "license", "Deccan Software Pvt Ltd", "annual", 54000, 340, 25, false, 30, "active", "software"), dec: contracts.DecSnoozed, until: 10},
		{in: mk("Employment contract: Operations Manager", "employment", "Tarun Gill", "monthly", 112000, 800, noEnd, false, 0, "active", "employment"),
			obs: []ob{{"Annual appraisal", "mutual", 60, "annually", false}}},
		{in: mk("Courier services agreement", "vendor", "Konkan Logistics", "monthly", 42000, 500, -20, false, 30, "active", "logistics"),
			obs: []ob{{"Reconcile monthly invoices", "us", -3, "monthly", false}}},
		{in: mk("Brand shoot agreement", "service", "Tungabhadra Studios", "one-time", 85000, 90, 0, false, 0, "terminated", "marketing")},
		{in: mk("Warehouse security services", "service", "Himalaya Guard Services", "monthly", 64000, 0, noEnd, false, 30, "draft", "premises"),
			obs: []ob{{"Verify guard background checks", "them", 14, "", false}}},
		{in: mk("Offer letter template: sales trainees", "employment", "Various candidates", "one-time", 0, 0, noEnd, false, 0, "draft", "employment"), key: &ids.offer},
	}
	for i := range list {
		switch list[i].in.Status {
		case "draft":
			list[i].in.StartDate, list[i].in.EndDate = "", ""
		case "terminated":
			list[i].in.EndDate = s.ago(10)
			list[i].in.Notes = "Terminated by mutual agreement after the shoot was delivered."
		}
	}
	n, no := 0, 0
	for _, x := range list {
		f, err := repo.Create(s.ctx, a, x.in)
		if err != nil {
			s.warn("contract "+x.in.Title, err)
			continue
		}
		n++
		if x.key != nil {
			*x.key = f.ID
		}
		for _, o := range x.obs {
			oi := contracts.ObligationInput{Title: o.title, Owner: o.owner, Due: s.ahead(o.due)}
			if o.every != "" {
				oi.Repeat = &contracts.Repeat{Every: o.every, Anchor: oi.Due}
			}
			ob, err := repo.AddObligation(s.ctx, a, f.ID, oi)
			if err != nil {
				s.warn("obligation", err)
				continue
			}
			no++
			if o.done {
				if _, err := repo.CompleteObligation(s.ctx, a, ob.ID, false); err != nil {
					s.warn("obligation done", err)
				}
			}
		}
		if x.dec != "" {
			d := contracts.Decision{Kind: x.dec, Note: "Discussed in the vendor review meeting."}
			if x.dec == contracts.DecSnoozed {
				d.Until = s.ahead(x.until)
			}
			if _, err := repo.Decide(s.ctx, a, f.ID, d); err != nil {
				s.warn("contract decision", err)
			}
		}
	}
	s.count("contracts", n)
	s.count("contract_obligations", no)
	return ids
}

// ---- Signatures ---------------------------------------------------------

type sigStore struct {
	s     *seeder
	drv   *drive.Repository
	files map[string][]byte
	names map[string]string
	n     int
	dir   string
}

func (g *sigStore) put(owner, name string, data []byte) (string, error) {
	if g.s.opt.Blobs != nil {
		if g.dir == "" {
			if f, err := g.drv.CreateFolder(g.s.ctx, g.s.org, owner, "", "Signatures"); err == nil {
				g.dir = f.ID
			}
		}
		key, err := drive.NewStorageKey()
		if err != nil {
			return "", err
		}
		if err := g.s.opt.Blobs.Put(g.s.ctx, key, "application/pdf", int64(len(data)), bytes.NewReader(data)); err != nil {
			return "", err
		}
		f, err := g.drv.CreateFile(g.s.ctx, g.s.org, owner, g.dir, name, "application/pdf", key, int64(len(data)))
		if err != nil {
			return "", err
		}
		g.files[f.ID], g.names[f.ID] = data, name
		return f.ID, nil
	}
	g.n++
	id := fmt.Sprintf("5eed0000-0000-4000-8000-%012d", g.n)
	g.files[id], g.names[id] = data, name
	return id, nil
}

func (s *seeder) v4Signatures(a knowkit.Actor, ct contractIDs) {
	repo := signatures.NewRepository(s.pool)
	st := &sigStore{s: s, drv: drive.NewRepository(s.pool), files: map[string][]byte{}, names: map[string]string{}}
	repo.Deps.ReadFile = func(ctx context.Context, org, owner, id string) ([]byte, string, error) {
		return st.files[id], st.names[id], nil
	}
	repo.Deps.ReadFileAny = func(ctx context.Context, org, id string) ([]byte, string, error) {
		return st.files[id], st.names[id], nil
	}
	repo.Deps.SaveFile = func(ctx context.Context, org, owner, name, mime string, data []byte) (string, error) {
		return st.put(owner, name, data)
	}
	type sg struct{ name, email string }
	type req struct {
		title, msg, pdfTitle string
		lines                []string
		signers              []sg
		sequential           bool
		sign                 int    // how many signers sign, in order
		decline              string // reason: the next signer declines
		expire               int
		contract             string
	}
	mail := func(f, l string) string { return demoEmail(f, l) }
	reqs := []req{
		{title: "Mutual NDA with Kaveri Retail", msg: "Please sign the NDA so we can share the product roadmap.", pdfTitle: "Mutual Non-Disclosure Agreement",
			lines:   []string{"This agreement is made between Northwind Trading Pvt Ltd and Kaveri Retail Pvt Ltd.", "Both parties will keep shared information confidential for three years.", "Governing law: India. Courts at Pune."},
			signers: []sg{{"Rohit Iyer", mail("Rohit", "Iyer")}, {"Anaya Shetty", mail("Anaya", "Shetty")}}, sign: 2, contract: ct.nda},
		{title: "Packaging supply agreement renewal", msg: "Renewal for the next twelve months at the revised rates.", pdfTitle: "Packaging Supply Agreement (renewal)",
			lines:   []string{"Supplier: Narmada Packaging Industries. Buyer: Northwind Trading Pvt Ltd.", "Term: twelve months from the renewal date.", "Rates are reviewed each quarter by mutual consent."},
			signers: []sg{{"Meenal Joshi", mail("Meenal", "Joshi")}, {"Harsh Mahajan", mail("Harsh", "Mahajan")}}, sign: 1, sequential: true, expire: 14, contract: ct.vendor},
		{title: "Offer letter: Sales Executive", msg: "We are delighted to offer you the role. Please sign to accept.", pdfTitle: "Offer of employment",
			lines:   []string{"Position: Sales Executive, Pune.", "Annual compensation: INR 5,40,000 (fixed).", "Joining date: first working day of next month.", "Notice period: 60 days."},
			signers: []sg{{"Pooja Reddy", mail("Pooja", "Reddy")}}, sign: 1, contract: ct.offer},
		{title: "Pune HQ lease extension", msg: "Extension of the lease for two more years.", pdfTitle: "Lease extension addendum",
			lines:   []string{"Landlord: Sahyadri Realty LLP.", "Extension: 24 months with a 5 percent rent increase.", "Security deposit stays unchanged."},
			signers: []sg{{"Founders desk", mail("Asha", "Menon")}, {"Sahyadri Realty", mail("Nikhil", "Pillai")}}, decline: "The rent increase is higher than we discussed. Please send revised terms.", contract: ct.lease},
		{title: "Service order: annual maintenance", msg: "Please countersign the service order for forklift maintenance.", pdfTitle: "Service order, forklift maintenance",
			lines:   []string{"Vendor: Coromandel Engineering Works.", "Scope: quarterly service of two forklifts.", "Fee: INR 1,48,000 per year plus GST."},
			signers: []sg{{"Tarun Gill", mail("Tarun", "Gill")}, {"Uma Naik", mail("Uma", "Naik")}}, sign: 0, expire: 7},
		{title: "Software subscription order form", msg: "Order form for the workspace subscription renewal.", pdfTitle: "Subscription order form",
			lines:   []string{"Customer: Northwind Trading Pvt Ltd.", "Plan: Business, 60 seats, billed annually.", "Total: INR 3,60,000 plus GST."},
			signers: []sg{{"Meenal Joshi", mail("Meenal", "Joshi")}}, sign: 1, contract: ct.saas},
	}
	cl := signatures.Client{IP: "203.0.113.20", UA: "Demo seed", Base: s.opt.BaseURL}
	if cl.Base == "" {
		cl.Base = "http://workspace.localtest.me:8080"
	}
	done := 0
	for _, r := range reqs {
		pdf := tinyPDF(r.pdfTitle, r.lines)
		fid, err := st.put(a.User, r.pdfTitle+".pdf", pdf)
		if err != nil {
			s.warn("signature file", err)
			continue
		}
		in := signatures.Input{Title: r.title, Message: r.msg, FileID: fid, Sequential: r.sequential, ExpireDays: r.expire, ContractID: r.contract}
		for i, sgn := range r.signers {
			in.Signers = append(in.Signers, signatures.SignerInput{Name: sgn.name, Email: sgn.email})
			in.Fields = append(in.Fields,
				signatures.FieldInput{SignerIndex: i, Type: "signature", Label: "Signature", Page: 1, X: 0.08, Y: 0.60 + 0.12*float64(i), W: 0.32, H: 0.06},
				signatures.FieldInput{SignerIndex: i, Type: "date", Label: "Date", Page: 1, X: 0.55, Y: 0.60 + 0.12*float64(i), W: 0.2, H: 0.04})
		}
		q, err := repo.Create(s.ctx, a, in)
		if err != nil {
			s.warn("signature create "+r.title, err)
			continue
		}
		q, err = repo.Send(s.ctx, a, q.ID, cl.Base)
		if err != nil {
			s.warn("signature send "+r.title, err)
			continue
		}
		done++
		tokens := make([]string, len(q.Signers))
		for i, sgn := range q.Signers {
			if j := strings.LastIndex(sgn.Link, "/sign/"); j >= 0 {
				tokens[i] = sgn.Link[j+len("/sign/"):]
			}
		}
		for i := 0; i < r.sign && i < len(tokens); i++ {
			if _, err := repo.PublicSign(s.ctx, tokens[i], signatures.SignInput{Consent: true, SignatureKind: "typed", SignatureData: q.Signers[i].Name}, cl); err != nil {
				s.warn("signature sign "+r.title, err)
			}
		}
		if r.decline != "" {
			if err := repo.PublicDecline(s.ctx, tokens[len(r.signers)-1], r.decline, cl); err != nil {
				s.warn("signature decline", err)
			}
		}
	}
	s.count("signature_requests", done)
}

// v4Guides adds two interview guides for Talent's scorecards.
func (s *seeder) v4Guides() {
	guides := []struct {
		name, stage, format string
		qs                  []string
	}{
		{"Phone screen", "Screening", "phone", []string{"Why are you interested in this role?", "Walk me through your most recent job.", "What are your salary expectations and notice period?"}},
		{"Technical interview", "Interview", "video", []string{"Describe a hard problem you solved recently.", "How do you prioritise when everything is urgent?", "Tell me about a mistake you made at work and what changed afterwards.", "What would you do in your first 30 days here?"}},
	}
	for _, g := range guides {
		type q struct {
			Text string `json:"text"`
		}
		var qs []q
		for _, x := range g.qs {
			qs = append(qs, q{x})
		}
		b, _ := json.Marshal(qs)
		if _, err := s.pool.Exec(s.ctx, `INSERT INTO adrient.talent_guides (org_id, name, stage, format, questions, created_by) VALUES ($1,$2,$3,$4,$5,$6)`, s.org, g.name, g.stage, g.format, b, s.admin); err != nil {
			s.warn("talent guide", err)
			return
		}
		s.count("talent_guides", 1)
	}
}
