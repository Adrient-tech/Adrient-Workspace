package demoseed

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/eventmeet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/forms"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/keep"
	"github.com/Adrient-tech/Adrient-Workspace/internal/meet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pos"
	"github.com/Adrient-tech/Adrient-Workspace/internal/projects"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tickets"
)

// ---- Drive --------------------------------------------------------------

type dfile struct {
	name, mime string
	data       []byte
}

// tinyPDF writes a one-page PDF with a title and text lines (Helvetica).
func tinyPDF(title string, lines []string) []byte {
	var content strings.Builder
	esc := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
	fmt.Fprintf(&content, "BT /F1 20 Tf 56 780 Td (%s) Tj ET\n", esc.Replace(title))
	y := 740
	for _, l := range lines {
		fmt.Fprintf(&content, "BT /F1 11 Tf 56 %d Td (%s) Tj ET\n", y, esc.Replace(l))
		y -= 18
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()),
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	var offs []int
	for i, o := range objs {
		offs = append(offs, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return b.Bytes()
}

// swatchPNG draws a diagonal gradient between two colours (no text).
func swatchPNG(w, h int, a, b color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x+y) / float64(w+h)
			img.Set(x, y, color.RGBA{uint8(float64(a.R)*(1-t) + float64(b.R)*t), uint8(float64(a.G)*(1-t) + float64(b.G)*t), uint8(float64(a.B)*(1-t) + float64(b.B)*t), 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func csvOf(rows ...string) []byte { return []byte(strings.Join(rows, "\n") + "\n") }

func (s *seeder) v2Drive() {
	repo := drive.NewRepository(s.pool)
	type folder struct {
		name  string
		subs  []string
		files []dfile
	}
	green, mint := color.RGBA{18, 108, 90, 255}, color.RGBA{215, 239, 232, 255}
	folders := []folder{
		{"Finance", []string{"FY 2026-27", "Invoices", "Bank statements"}, []dfile{
			{"Chart of accounts.csv", "text/csv", csvOf("Code,Name,Type", "1000,Cash,asset", "1100,Bank,asset", "1200,Accounts receivable,asset", "2000,Accounts payable,liability", "4000,Sales,income", "5000,Cost of goods sold,expense")},
			{"GST filing calendar.pdf", "application/pdf", tinyPDF("GST filing calendar FY 2026-27", []string{"GSTR-1: 11th of every month", "GSTR-3B: 20th of every month", "GSTR-9 annual return: 31 December", "TDS return (24Q): quarterly, 31st of month after quarter"})},
			{"October MIS.pdf", "application/pdf", tinyPDF("Monthly MIS, October", []string{"Revenue: INR 62.4 lakh", "Gross margin: 31.2 percent", "Net profit: INR 11.8 lakh", "Receivables over 60 days: 3 accounts"})}}},
		{"Sales", []string{"Proposals", "Contracts", "Price lists"}, []dfile{
			{"Price list Q4.csv", "text/csv", csvOf("SKU,Item,Unit price", "DEMO-0001,Cotton Shirt Classic,899", "DEMO-0002,Steel Bottle Classic,165", "DEMO-0003,LED Bulb 9W Classic,89", "DEMO-0004,Notebook A5 Classic,45")},
			{"Standard terms of sale.pdf", "application/pdf", tinyPDF("Standard terms of sale", []string{"1. Prices are in INR and exclude GST.", "2. Payment within 30 days of invoice date.", "3. Goods remain our property until paid in full.", "4. Delivery dates are estimates."})},
			{"Pipeline review.md", "text/markdown", []byte("# Pipeline review\n\n- 12 open deals, INR 38 lakh\n- 4 in negotiation\n- Slipped renewals: 2\n")}}},
		{"HR", []string{"Policies", "Onboarding"}, []dfile{
			{"Leave policy.pdf", "application/pdf", tinyPDF("Leave policy", []string{"Casual leave: 8 days per year", "Earned leave: 15 days per year", "Sick leave: 7 days per year", "Requests are approved by the reporting manager."})},
			{"New joiner checklist.md", "text/markdown", []byte("# New joiner checklist\n\n- [ ] Laptop and accounts\n- [ ] Welcome email\n- [ ] Buddy assigned\n- [ ] First week plan\n")}}},
		{"Operations", []string{"Warehouse SOPs", "Vendor documents"}, []dfile{
			{"Receiving SOP.pdf", "application/pdf", tinyPDF("Receiving SOP", []string{"1. Check the PO and delivery challan.", "2. Count and inspect each carton.", "3. Record the GRN in the system.", "4. Put away to the assigned bin."})},
			{"Stock count sheet.csv", "text/csv", csvOf("Aisle,Bin,SKU,Counted,System", "A,A-01,DEMO-0001,120,121", "A,A-02,DEMO-0002,88,88", "B,B-07,DEMO-0003,310,309")}}},
		{"Marketing", []string{"Brand", "Campaigns"}, []dfile{
			{"Brand green.png", "image/png", swatchPNG(640, 400, green, mint)},
			{"Diwali banner.png", "image/png", swatchPNG(800, 400, color.RGBA{244, 160, 60, 255}, color.RGBA{190, 40, 70, 255})},
			{"Campaign results.csv", "text/csv", csvOf("Channel,Reach,Leads,Orders", "WhatsApp,6200,540,66", "Email,4100,310,48", "Instagram,5300,260,62", "Walk-in,2800,150,38")}}},
		{"Projects", []string{"Warehouse automation", "Customer onboarding"}, []dfile{
			{"Project brief.md", "text/markdown", []byte("# Warehouse automation\n\nGoal: cut dispatch time by 30 percent.\n\nOwner: Asha Menon\n")}}},
	}
	nf, nfiles := 0, 0
	var firstFile string
	for _, f := range folders {
		fo, err := repo.CreateFolder(s.ctx, s.org, s.admin, "", f.name)
		if err != nil {
			s.warn("drive folder", err)
			continue
		}
		nf++
		for _, sub := range f.subs {
			if _, err := repo.CreateFolder(s.ctx, s.org, s.admin, fo.ID, sub); err != nil {
				s.warn("drive subfolder", err)
				continue
			}
			nf++
		}
		if s.opt.Blobs == nil {
			continue
		}
		for _, d := range f.files {
			key, err := drive.NewStorageKey()
			if err != nil {
				s.warn("drive key", err)
				continue
			}
			if err := s.opt.Blobs.Put(s.ctx, key, d.mime, int64(len(d.data)), bytes.NewReader(d.data)); err != nil {
				s.warn("drive blob", err)
				continue
			}
			file, err := repo.CreateFile(s.ctx, s.org, s.admin, fo.ID, d.name, d.mime, key, int64(len(d.data)))
			if err != nil {
				s.warn("drive file", err)
				continue
			}
			nfiles++
			if firstFile == "" || nfiles%3 == 0 {
				firstFile = file.ID
				_ = repo.StarFile(s.ctx, s.admin, file.ID)
			}
		}
	}
	if s.opt.Blobs == nil {
		s.rep.Warnings = append(s.rep.Warnings, "drive: no object store configured (ADRIENT_RUSTFS_*), demo folders were created without files")
	}
	s.count("drive_folders", nf)
	s.count("drive_files", nfiles)
}

// ---- Calendar and Meet --------------------------------------------------

func (s *seeder) v2Calendar() {
	cal := calendar.NewRepository(s.pool)
	mt := meet.NewRepository(s.pool)
	em := eventmeet.NewRepository(s.pool)
	// Monday of this week.
	wd := int(s.today.Weekday())
	if wd == 0 {
		wd = 7
	}
	mon := time.Date(s.today.Year(), s.today.Month(), s.today.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, 1-wd)
	at := func(dayOff, h, m int) time.Time {
		return mon.AddDate(0, 0, dayOff).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	}
	mails := func(names ...string) []string {
		var out []string
		for _, n := range names {
			if p := teamByFirst(n); p != nil {
				out = append(out, p.email())
			}
		}
		return out
	}
	type ev struct {
		title, loc, color, rec string
		day, h, m, mins        int
		allDay, meet           bool
		who                    []string
	}
	evs := []ev{
		{"Daily standup", "", "#126C5A", "FREQ=DAILY;COUNT=5", 0, 9, 30, 15, false, true, []string{"Imran", "Arjun", "Faisal"}},
		{"Weekly sales review", "", "#2563eb", "FREQ=WEEKLY", 0, 11, 0, 60, false, true, []string{"Rohan", "Divya", "Sneha"}},
		{"Supplier call: Kaveri Textiles", "", "#d97706", "", 0, 15, 0, 45, false, true, []string{"Karan"}},
		{"Payroll review", "Finance room", "#7c3aed", "", 1, 10, 0, 60, false, false, []string{"Priya", "Neha"}},
		{"Customer demo: Godavari Pharma", "", "#126C5A", "", 1, 14, 30, 60, false, true, []string{"Rohan", "Divya"}},
		{"Hiring panel: Senior Backend Engineer", "", "#db2777", "", 1, 16, 0, 60, false, true, []string{"Imran", "Neha"}},
		{"Warehouse safety drill", "North gate", "#dc2626", "", 2, 15, 0, 30, false, false, []string{"Arjun", "Karan"}},
		{"1:1 Asha and Rohan", "", "#2563eb", "FREQ=WEEKLY", 2, 12, 0, 30, false, true, []string{"Rohan"}},
		{"Vendor review: Narmada Packaging", "Conference room 2", "#d97706", "", 2, 11, 30, 60, false, false, []string{"Karan", "Priya"}},
		{"GST filing deadline", "", "#dc2626", "", 3, 0, 0, 0, true, false, nil},
		{"Quarterly business review", "Board room", "#126C5A", "", 4, 11, 0, 120, false, true, []string{"Rohan", "Priya", "Imran", "Neha", "Divya"}},
		{"Team lunch", "Cafeteria", "#16a34a", "", 4, 13, 30, 60, false, false, nil},
		{"Pune store opening prep", "Pune store", "#7c3aed", "", 4, 16, 0, 60, false, false, []string{"Karan", "Arjun"}},
		{"Month-end close", "", "#0891b2", "", 3, 10, 0, 90, false, true, []string{"Priya"}},
		{"Product roadmap workshop", "", "#126C5A", "", 3, 14, 0, 90, false, true, []string{"Imran", "Sneha", "Rohan"}},
		{"Diwali campaign retro", "", "#db2777", "", 1, 12, 0, 45, false, true, []string{"Sneha", "Divya"}},
		{"Company all-hands", "", "#2563eb", "", 5, 10, 0, 60, false, true, []string{"Rohan", "Priya", "Imran", "Neha", "Arjun", "Divya", "Karan", "Sneha", "Faisal"}},
		{"Next week: Pune store inauguration", "Pune store", "#16a34a", "", 7, 10, 0, 120, false, false, nil},
	}
	n, nm := 0, 0
	for _, e := range evs {
		start := at(e.day, e.h, e.m)
		end := start.Add(time.Duration(e.mins) * time.Minute)
		if e.allDay {
			start = mon.AddDate(0, 0, e.day)
			end = start.AddDate(0, 0, 1)
		}
		created, err := cal.Create(s.ctx, s.org, s.admin, calendar.Fields{Title: e.title, Description: "Demo event", Location: e.loc, StartAt: start, EndAt: end, AllDay: e.allDay, Color: e.color,
			Recurrence: e.rec, Attendees: mails(e.who...), ItemType: "event", Reminders: []int32{10}})
		if err != nil {
			s.warn("calendar v2", err)
			continue
		}
		n++
		if e.meet {
			room, err := mt.Create(s.ctx, s.org, s.admin, e.title)
			if err != nil {
				s.warn("meet room", err)
				continue
			}
			if err := em.Set(s.ctx, s.org, created.ID, room.ID, room.Code); err != nil {
				s.warn("meet link", err)
				continue
			}
			nm++
		}
	}
	s.count("calendar_events_v2", n)
	s.count("meet_rooms", nm)
}

// ---- Projects -----------------------------------------------------------

func (s *seeder) v2Projects() {
	pr := projects.NewRepository(s.pool)
	teams, _ := pr.ListTeams(s.ctx, s.org)
	var tm projects.Team
	for _, t := range teams {
		if t.Name == "Delivery" {
			tm = t
		}
	}
	if tm.ID == "" {
		var err error
		if tm, err = pr.CreateTeam(s.ctx, s.org, "Delivery", "DLV", "", ""); err != nil {
			s.warn("projects team v2", err)
			return
		}
	}
	for _, id := range s.userIDs() {
		_ = pr.AddTeamMember(s.ctx, s.org, tm.ID, id)
	}
	plist, _ := pr.ListProjects(s.ctx, s.org)
	byName := map[string]projects.Project{}
	for _, p := range plist {
		byName[p.Name] = p
	}
	type pdef struct {
		name, desc, color, lead string
		target                  int
		issues                  [][4]string // title, status, assignee first name, priority(1-4)
	}
	defs := []pdef{
		{"Warehouse automation rollout", "Barcode scanning, bin locations and automatic reorder across both warehouses.", "#126C5A", "Asha", 45, [][4]string{
			{"Map bin locations for aisles A to D", "done", "Arjun", "2"}, {"Label all bins with barcodes", "done", "Arjun", "2"},
			{"Pilot barcode scanning at the Pune store", "in_progress", "Faisal", "1"}, {"Set reorder points for the top 150 SKUs", "in_progress", "Karan", "2"},
			{"Integrate scanner app with goods receipt", "in_progress", "Imran", "1"}, {"Train shift A and shift B", "todo", "Arjun", "3"},
			{"Cut-over weekend plan", "todo", "Asha", "2"}, {"Go-live review", "backlog", "Asha", "3"}}},
		{"GST filing automation", "Generate GSTR-1 and GSTR-3B drafts from invoices and bills.", "#2563eb", "Priya", 30, [][4]string{
			{"Reconcile October purchase invoices", "done", "Priya", "2"}, {"Map HSN codes for all products", "in_progress", "Priya", "2"},
			{"Credit note handling in GST summary", "in_progress", "Imran", "1"}, {"Review draft returns with the accountant", "todo", "Priya", "2"},
			{"Set up e-way bill export", "backlog", "Imran", "3"}}},
		{"Customer onboarding revamp", "A simple welcome journey for new accounts: contract, first order, review.", "#d97706", "Divya", 60, [][4]string{
			{"Interview five recent customers", "done", "Divya", "3"}, {"Draft the welcome email sequence", "in_progress", "Sneha", "2"},
			{"Build the onboarding checklist in Tasks", "todo", "Divya", "2"}, {"Customer health score in CRM", "todo", "Rohan", "3"},
			{"Pilot with three new accounts", "backlog", "Divya", "2"}}},
	}
	extra := func(issue, start, due string) {
		_, _ = s.pool.Exec(s.ctx, `INSERT INTO adrient.project_issue_extras (issue_id, org_id, start_date, due_date) VALUES ($1::uuid,$2::uuid,$3::date,$4::date)
			ON CONFLICT (issue_id) DO UPDATE SET start_date=EXCLUDED.start_date, due_date=EXCLUDED.due_date`, issue, s.org, start, due)
	}
	uid := func(f string) string {
		if f == "Asha" {
			return s.admin
		}
		if p := teamByFirst(f); p != nil {
			return p.UserID
		}
		return ""
	}
	nIssues := 0
	for pi, d := range defs {
		p, ok := byName[d.name]
		if !ok {
			var err error
			if p, err = pr.CreateProject(s.ctx, s.org, projects.ProjectFields{Name: d.name, Description: d.desc}); err != nil {
				s.warn("projects project v2", err)
				continue
			}
		}
		target := day(s.today.AddDate(0, 0, d.target))
		if _, err := pr.UpdateProject(s.ctx, s.org, projects.ProjectFields{Name: d.name, Description: d.desc, Color: d.color, State: "started", LeadID: uid(d.lead), TargetDate: target}, p.ID); err != nil {
			s.warn("projects update", err)
		}
		// Replace the thin placeholder issues of the first run with a real plan.
		if old, err := pr.ListIssues(s.ctx, s.org, projects.IssueFilter{ProjectID: p.ID}); err == nil {
			for _, o := range old {
				if o.CreatorID == s.user || o.CreatorID == s.admin {
					if strings.HasPrefix(o.Title, "Scope and plan") || o.Title == "Vendor shortlist" || o.Title == "Pilot at Pune store" || o.Title == "Train the team" || o.Title == "Go live review" {
						_ = pr.DeleteIssue(s.ctx, s.org, o.ID)
					}
				}
			}
		}
		_, _ = s.pool.Exec(s.ctx, `INSERT INTO adrient.project_milestones (org_id, project_id, name, due_date, done) VALUES ($1::uuid,$2::uuid,'Pilot complete',$3::date,false),($1::uuid,$2::uuid,'Go live',$4::date,false)`,
			s.org, p.ID, day(s.today.AddDate(0, 0, d.target/2)), target)
		for i, is := range d.issues {
			prio := int32(is[3][0] - '0')
			iss, err := pr.CreateIssue(s.ctx, s.org, tm.ID, s.admin, projects.IssueFields{Title: is[0], Description: "Demo issue for " + d.name + ".", Status: is[1], Priority: prio, AssigneeID: uid(is[2]), ProjectID: p.ID, Estimate: int32(1 + i%5)})
			if err != nil {
				s.warn("projects issue v2", err)
				continue
			}
			nIssues++
			off := -20 + pi*3 + i*6
			extra(iss.ID, day(s.today.AddDate(0, 0, off)), day(s.today.AddDate(0, 0, off+9+(i%3)*4)))
		}
	}
	s.count("project_issues_v2", nIssues)
}

// userIDs returns the admin plus the team's account ids.
func (s *seeder) userIDs() []string {
	ids := []string{s.admin}
	for _, m := range team {
		if m.UserID != "" {
			ids = append(ids, m.UserID)
		}
	}
	return ids
}

// ---- Tickets ------------------------------------------------------------

func (s *seeder) v2Tickets() {
	repo := tickets.NewRepository(s.pool)
	proj, err := repo.CreateProject(s.ctx, s.org, s.admin, "HELP", "IT and facilities helpdesk", "Requests from staff to IT, admin and facilities.", "team")
	if err != nil {
		s.warn("tickets project", err)
		return
	}
	if err := repo.SaveSLAPolicies(s.ctx, s.org, proj.ID, []tickets.SLAPolicy{
		{Priority: "urgent", FirstResponseMin: 30, ResolveMin: 240}, {Priority: "high", FirstResponseMin: 120, ResolveMin: 480},
		{Priority: "normal", FirstResponseMin: 480, ResolveMin: 2880}, {Priority: "low", FirstResponseMin: 1440, ResolveMin: 7200}}); err != nil {
		s.warn("tickets sla", err)
	}
	type tk struct {
		title, body, prio, status, who, agent string
		hoursAgo                              float64
	}
	list := []tk{
		{"Cannot sign in to the Pune store POS", "The POS terminal shows 'session expired' after every login. Store opens tomorrow.", "urgent", "open", "Karan", "Faisal", 1.5},
		{"Barcode scanner not pairing", "Scanner 4 in aisle C does not pair over Bluetooth.", "high", "in_progress", "Arjun", "Faisal", 9},
		{"Laptop for new joiner Nikhil", "Need a laptop, charger and accounts before Monday.", "high", "in_progress", "Neha", "Faisal", 30},
		{"Printer on first floor jams", "A4 printer jams every 10 pages.", "normal", "open", "Priya", "", 20},
		{"VPN keeps disconnecting", "VPN drops every 15 minutes when working from home.", "normal", "in_progress", "Rohan", "Faisal", 46},
		{"Add Narmada Packaging to the vendor list", "Create the vendor record with GST number 27AAAAA0000A1Z5.", "normal", "resolved", "Karan", "Priya", 70},
		{"Air conditioner in the board room", "The AC makes a loud noise and does not cool.", "normal", "open", "Neha", "", 52},
		{"Request: second monitor", "Please arrange a second monitor for the finance desk.", "low", "open", "Priya", "", 120},
		{"Reset password for Divya", "Locked out after too many attempts.", "high", "resolved", "Divya", "Faisal", 6},
		{"Wi-Fi slow at the Pune store", "Speed test shows 3 Mbps.", "high", "resolved", "Karan", "Faisal", 100},
		{"Shared mailbox for accounts@", "We need a shared mailbox for the accounts team.", "normal", "in_progress", "Priya", "Faisal", 40},
		{"Office keys for the new store", "Need six sets of keys and two access cards.", "low", "open", "Karan", "", 200},
		{"Install Tally on the finance PC", "License is available, needs installation.", "normal", "resolved", "Priya", "Faisal", 90},
		{"CCTV camera offline at the loading dock", "Camera 3 shows no signal since this morning.", "urgent", "in_progress", "Arjun", "Faisal", 3},
		{"Update the website contact number", "Footer still shows the old phone number.", "low", "closed", "Sneha", "Imran", 300},
		{"Mobile hotspot for field team", "Two hotspot devices for the Nashik visit.", "low", "closed", "Rohan", "Faisal", 260},
		{"Email signature template", "Create a standard signature with the new logo.", "low", "resolved", "Neha", "Sneha", 150},
		{"Backup drive almost full", "The NAS is at 94 percent.", "high", "open", "Faisal", "", 14},
		{"Conference room projector not detected", "Laptop does not detect the projector in room 2.", "normal", "open", "Imran", "", 26},
		{"Request: standing desk", "Doctor advised a standing desk, please advise on the process.", "low", "open", "Divya", "", 75},
	}
	now := time.Now()
	n := 0
	for _, t := range list {
		req := teamByFirst(t.who)
		var ruid *string
		name := "Staff member"
		email := ""
		if req != nil {
			ruid, name, email = &req.UserID, req.name(), req.email()
		}
		tk, err := repo.CreateTicket(s.ctx, s.org, proj.ID, t.title, t.body, t.prio, ruid, name, email, "web")
		if err != nil {
			s.warn("tickets create", err)
			continue
		}
		n++
		created := now.Add(-time.Duration(t.hoursAgo * float64(time.Hour)))
		var agentID *string
		agentName := ""
		if a := teamByFirst(t.agent); a != nil {
			agentID, agentName = &a.UserID, a.name()
		} else if t.agent == "Priya" || t.agent == "" {
			// unassigned stays unassigned
		}
		if agentID != nil {
			st := t.status
			_, _ = repo.UpdateTicket(s.ctx, s.org, tk.ID, &st, nil, nil, nil, agentID, false)
			reply := created.Add(time.Duration(10+n*7) * time.Minute)
			if c, err := repo.AddComment(s.ctx, s.org, tk.ID, agentID, agentName, "Hi, I'm looking into this now.", false); err == nil {
				_, _ = s.pool.Exec(s.ctx, `UPDATE adrient.ticket_comments SET created_at=$2 WHERE id=$1::uuid`, c.ID, reply)
			}
		} else if t.status != "open" {
			st := t.status
			_, _ = repo.UpdateTicket(s.ctx, s.org, tk.ID, &st, nil, nil, nil, nil, false)
		}
		resolved := "NULL"
		if t.status == "resolved" || t.status == "closed" {
			resolved = "$2 + interval '3 hours'"
		}
		_, _ = s.pool.Exec(s.ctx, `UPDATE adrient.tickets SET created_at=$2, updated_at=$2, resolved_at=`+resolved+` WHERE id=$1::uuid`, tk.ID, created)
	}
	s.count("tickets", n)
}

// ---- Forms --------------------------------------------------------------

func (s *seeder) v2Forms() {
	repo := forms.NewRepository(s.pool)
	type fdef struct {
		title, desc string
		qs          []forms.Question
		settings    forms.Settings
		answers     func(i int) map[string]any
		count       int
	}
	pick := func(i int, opts ...string) string { return opts[i%len(opts)] }
	defs := []fdef{
		{"Customer satisfaction survey", "Tell us how we did. It takes one minute.", []forms.Question{
			{ID: "q1", Type: forms.TypeMultipleChoice, Title: "How did you order?", Required: true, Options: []string{"Phone", "WhatsApp", "Website", "Store visit"}},
			{ID: "q2", Type: forms.TypeLinearScale, Title: "How likely are you to recommend us?", Required: true, ScaleMin: 1, ScaleMax: 10, ScaleMinLabel: "Not likely", ScaleMaxLabel: "Very likely"},
			{ID: "q3", Type: forms.TypeCheckboxes, Title: "What did you like?", Options: []string{"Quality", "Price", "Delivery speed", "Support"}},
			{ID: "q4", Type: forms.TypeParagraph, Title: "Anything we should improve?"}},
			forms.Settings{ShowProgressBar: true, ConfirmationMessage: "Thank you for your feedback!"},
			func(i int) map[string]any {
				return map[string]any{"q1": pick(i, "WhatsApp", "Phone", "Website", "Store visit", "WhatsApp"), "q2": fmt.Sprint(6 + (i*3)%5), "q3": []string{pick(i, "Quality", "Price", "Delivery speed"), pick(i+1, "Support", "Quality")},
					"q4": pick(i, "Delivery was quick.", "Please add more payment options.", "Packaging could be better.", "Happy with the service.", "")}
			}, 34},
		{"Employee feedback pulse", "Anonymous quarterly pulse for the team.", []forms.Question{
			{ID: "q1", Type: forms.TypeDropdown, Title: "Your department", Required: true, Options: []string{"Engineering", "Sales", "Finance", "Operations", "People and Culture"}},
			{ID: "q2", Type: forms.TypeRating, Title: "How satisfied are you with your work?", Required: true, ScaleMax: 5},
			{ID: "q3", Type: forms.TypeMultipleChoice, Title: "Workload this quarter", Options: []string{"Too light", "About right", "Too heavy"}},
			{ID: "q4", Type: forms.TypeParagraph, Title: "One thing we should start or stop doing"}},
			forms.Settings{ShowProgressBar: true},
			func(i int) map[string]any {
				return map[string]any{"q1": pick(i, "Engineering", "Sales", "Operations", "Finance", "People and Culture", "Operations"), "q2": fmt.Sprint(3 + i%3), "q3": pick(i, "About right", "Too heavy", "About right", "Too light"),
					"q4": pick(i, "Start Friday demos.", "Stop long status meetings.", "More learning budget.", "Better shift handover.")}
			}, 22},
		{"Event registration: Pune store opening", "Join us on 1 December at 10 am.", []forms.Question{
			{ID: "q1", Type: forms.TypeShortAnswer, Title: "Full name", Required: true},
			{ID: "q2", Type: forms.TypeShortAnswer, Title: "Company", Required: false},
			{ID: "q3", Type: forms.TypeMultipleChoice, Title: "Number of guests", Required: true, Options: []string{"Just me", "2", "3", "4 or more"}},
			{ID: "q4", Type: forms.TypeDropdown, Title: "Food preference", Options: []string{"Vegetarian", "Jain", "Non-vegetarian"}}},
			forms.Settings{CollectEmail: true, ConfirmationMessage: "See you there!"},
			func(i int) map[string]any {
				f, l := firstNames[i%len(firstNames)], lastNames[(i*3)%len(lastNames)]
				return map[string]any{"q1": f + " " + l, "q2": coPrefix[i%len(coPrefix)] + " " + coSuffix[i%len(coSuffix)], "q3": pick(i, "Just me", "2", "2", "3", "4 or more"), "q4": pick(i, "Vegetarian", "Jain", "Non-vegetarian", "Vegetarian")}
			}, 41},
	}
	made := 0
	for _, d := range defs {
		f, err := repo.Create(s.ctx, s.org, s.admin, forms.Fields{Title: d.title, Description: d.desc, Questions: d.qs, Settings: d.settings, Accepting: true})
		if err != nil {
			s.warn("forms create", err)
			continue
		}
		for i := 0; i < d.count; i++ {
			email := ""
			if d.settings.CollectEmail {
				email = fmt.Sprintf("guest%d@%s", i+1, demoDomain)
			}
			if _, err := repo.SubmitResponse(s.ctx, s.org, f.ID, "", email, d.answers(i), nil); err != nil {
				s.warn("forms response", err)
				break
			}
			made++
		}
		// Spread the responses over the last three weeks.
		_, _ = s.pool.Exec(s.ctx, `UPDATE adrient.form_responses SET submitted_at = now() - (random()*interval '21 days') WHERE form_id=$1::uuid`, f.ID)
	}
	s.count("forms", len(defs))
	s.count("form_responses", made)
}

// ---- Keep ---------------------------------------------------------------

func (s *seeder) v2Keep() {
	repo := keep.NewRepository(s.pool)
	ck := func(items ...string) []keep.ChecklistItem {
		var out []keep.ChecklistItem
		for i, t := range items {
			out = append(out, keep.ChecklistItem{Text: strings.TrimPrefix(t, "x:"), Checked: strings.HasPrefix(t, "x:") || i == 0 && false})
		}
		return out
	}
	notes := []keep.Fields{
		{Title: "Pune store opening, 1 December", Body: "Puja at 10 am. Signage Wednesday, shelving Tuesday, POS ready.", Color: "sand", Pinned: true, Checklist: ck("x:Shelving delivered", "x:POS terminals installed", "Signage installed", "Staff uniforms", "Invite local press")},
		{Title: "Ideas for the Q4 offsite", Body: "Lonavala or Alibaug. Two days, one workshop on the roadmap, one outdoor activity.", Color: "mint", Pinned: true},
		{Title: "Weekly priorities", Color: "fog", Checklist: ck("x:Approve payroll draft", "Review Coromandel pilot terms", "Sign Vindhya credit note", "Call Kaveri Textiles", "Prepare QBR deck")},
		{Title: "Supplier call notes: Kaveri Textiles", Body: "Power outage at the dyeing unit. 400 pieces by road this weekend, 800 on 28 Nov. Ask for a 2% delay credit.", Color: "peach"},
		{Title: "Books to read", Body: "The Goal (Goldratt)\nHigh Output Management (Grove)\nThe Mom Test (Fitzpatrick)", Color: "dusk"},
		{Title: "Warehouse automation: open questions", Body: "- Which scanner model for the second warehouse?\n- Do we need bin-level reorder points?\n- Training schedule for shift B", Color: "sage"},
		{Title: "Gift ideas for top customers", Body: "Steel bottle with logo, dry fruit box, diary. Budget INR 600 each.", Color: "blossom"},
		{Title: "Passwords reminder", Body: "Never keep passwords in notes. Use the team password manager and ask Faisal for access.", Color: "coral"},
	}
	n := 0
	for _, f := range notes {
		if _, err := repo.Create(s.ctx, s.org, s.admin, f); err != nil {
			s.warn("keep", err)
			continue
		}
		n++
	}
	s.count("keep_notes", n)
}

// ---- Sales: a full financial year of invoices ----------------------------

// v2Sales adds paid invoices from April to now so the profit and loss report
// shows a healthy year next to the payroll and bills, then posts them.
func (s *seeder) v2Sales(acct *accounting.Repository) {
	invc := invoicing.NewRepository(s.pool)
	clients := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		clients = append(clients, coPrefix[(i*3+1)%len(coPrefix)]+" "+coSuffix[(i*7+2)%len(coSuffix)])
	}
	fy := time.Date(s.today.Year(), time.April, 1, 0, 0, 0, 0, time.Local)
	if s.today.Before(fy) {
		fy = fy.AddDate(-1, 0, 0)
	}
	days := int(s.today.Sub(fy).Hours() / 24)
	made := 0
	for k := 0; k < 44; k++ {
		issue := fy.AddDate(0, 0, 1+(k*days)/46)
		if issue.After(s.today.AddDate(0, 0, -2)) {
			issue = s.today.AddDate(0, 0, -2-k%5)
		}
		var lines []invoicing.Line
		for l := 0; l < 2+s.r.Intn(2); l++ {
			lines = append(lines, invoicing.Line{Description: prodKinds[s.r.Intn(len(prodKinds))] + " bulk order", Qty: float64(80 + s.r.Intn(260)), Unit: "pcs", Price: float64(300 + s.r.Intn(900)), TaxRate: 18, TaxName: "GST"})
		}
		doc, err := invc.SaveDocument(s.ctx, s.org, s.admin, "", invoicing.DocInput{Kind: "invoice", ClientName: clients[k%len(clients)], ClientEmail: fmt.Sprintf("accounts%d@%s", k%9, demoDomain), Currency: "INR",
			IssueDate: day(issue), DueDate: day(issue.AddDate(0, 0, 30)), Lines: lines, Notes: "Demo invoice", Schedule: []invoicing.Schedule{}, Attachments: []invoicing.Attachment{}})
		if err != nil {
			s.warn("sales v2 invoice", err)
			continue
		}
		made++
		if _, _, err := invc.MarkSent(s.ctx, s.org, doc.ID); err != nil {
			s.warn("sales v2 sent", err)
		}
		if k%7 != 6 {
			if _, err := invc.AddPayment(s.ctx, s.org, s.admin, doc.ID, invoicing.Payment{Amount: doc.Total, Method: "bank_transfer", PaidAt: day(issue.AddDate(0, 0, 8+k%15)), Note: "Demo payment"}); err != nil {
				s.warn("sales v2 payment", err)
			}
		}
	}
	// A shop that sells in rupees: switch the POS currency when nothing has been sold yet.
	_, _ = s.pool.Exec(s.ctx, `INSERT INTO adrient.pos_settings (org_id, store_name, currency) VALUES ($1,'Adrient Trading','INR') ON CONFLICT (org_id) DO UPDATE SET currency='INR' WHERE NOT EXISTS (SELECT 1 FROM adrient.pos_sales WHERE org_id=$1)`, s.org)
	s.count("invoices_v2", made)
	inv := inventory.NewRepository(s.pool)
	posRepo := pos.NewRepository(s.pool, inv)
	if _, err := acct.Sync(s.ctx, s.org, s.admin, posRepo, invc, true); err != nil {
		s.warn("accounting sync v2", err)
	}
}
