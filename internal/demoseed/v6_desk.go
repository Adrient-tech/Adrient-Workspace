package demoseed

import (
	"fmt"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/tickets"
)

// runV6 is seed step 6: fills the service desk (groups, SLA rules, automations,
// request forms, knowledge base articles and about 120 tickets), adds six
// fictional Stories and generates Music and Video content with ffmpeg.
func (s *seeder) runV6() {
	s.v6Desk()
	s.v6Stories()
	s.v6Media()
}

func (s *seeder) userByEmail(email string) string {
	var id string
	_ = s.pool.QueryRow(s.ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 AND email=$2`, s.org, email).Scan(&id)
	return id
}

func (s *seeder) v6Desk() {
	repo := tickets.NewRepository(s.pool)
	var projID string
	if err := s.pool.QueryRow(s.ctx, `SELECT id::text FROM adrient.ticket_projects WHERE org_id=$1 AND key='HELP'`, s.org).Scan(&projID); err != nil {
		p, err := repo.CreateProject(s.ctx, s.org, s.admin, "HELP", "IT and facilities helpdesk", "Requests from staff to IT, admin and facilities.", "team")
		if err != nil {
			s.warn("desk project", err)
			return
		}
		projID = p.ID
	}
	// public intake so the portal, forms and knowledge base have a link
	_, _ = s.pool.Exec(s.ctx, `UPDATE adrient.ticket_projects SET intake_mode='public', public_token=COALESCE(public_token,$2) WHERE id=$1`, projID, "pt_demo"+fmt.Sprintf("%x", s.org[:8]))
	ctx := tickets.WithActor(s.ctx, tickets.Actor{ID: s.admin, Name: "Asha Menon"})

	uid := func(first string) string {
		if m := teamByFirst(first); m != nil {
			return s.userByEmail(m.email())
		}
		return ""
	}
	ids := map[string]string{}
	for _, f := range []string{"Faisal", "Imran", "Priya", "Arjun", "Karan", "Neha", "Divya", "Rohan", "Sneha"} {
		ids[f] = uid(f)
	}
	ids["Asha"] = s.admin

	// ---- calendar ----
	week := map[string][][2]string{"mon": {{"09:30", "18:30"}}, "tue": {{"09:30", "18:30"}}, "wed": {{"09:30", "18:30"}}, "thu": {{"09:30", "18:30"}},
		"fri": {{"09:30", "18:30"}}, "sat": {{"10:00", "14:00"}}}
	cal, err := repo.SaveCalendar(ctx, s.org, tickets.CalendarRow{Name: "India office hours", TZ: "Asia/Kolkata", Hours: week, IsDefault: true,
		Holidays: []tickets.Holiday{{Day: day(s.today.AddDate(0, 0, 30)), Name: "Winter break"}, {Day: "2026-12-25", Name: "Christmas Day"}, {Day: "2027-01-26", Name: "Republic Day"}}})
	if err != nil {
		s.warn("desk calendar", err)
	}
	_, _ = repo.SaveCalendar(ctx, s.org, tickets.CalendarRow{Name: "Store support 24x6", TZ: "Asia/Kolkata", Hours: map[string][][2]string{
		"mon": {{"00:00", "24:00"}}, "tue": {{"00:00", "24:00"}}, "wed": {{"00:00", "24:00"}}, "thu": {{"00:00", "24:00"}}, "fri": {{"00:00", "24:00"}}, "sat": {{"00:00", "24:00"}}}})

	// ---- workflow ----
	if _, err := repo.SaveStatusDefs(ctx, s.org, projID, []tickets.StatusDef{
		{Key: "open", Label: "Open", Category: "open"}, {Key: "in_progress", Label: "In progress", Category: "open"},
		{Key: "pending", Label: "Pending requester", Category: "pending", PausesSLA: true},
		{Key: "on_hold", Label: "On hold (vendor)", Category: "hold", PausesSLA: true},
		{Key: "resolved", Label: "Resolved", Category: "solved", Requires: []string{"category"}},
		{Key: "closed", Label: "Closed", Category: "closed"}}); err != nil {
		s.warn("desk statuses", err)
	}

	// ---- groups ----
	mk := func(name, desc, strategy string, members ...tickets.Member) string {
		g, err := repo.SaveGroup(ctx, s.org, projID, tickets.Group{Name: name, Description: desc, Strategy: strategy, CalendarID: cal.ID, Members: members})
		if err != nil {
			s.warn("desk group "+name, err)
		}
		return g.ID
	}
	gIT := mk("IT Support", "Accounts, devices, network and software.", tickets.StrategyLoad,
		tickets.Member{UserID: ids["Faisal"], Capacity: 12, Skills: []string{"network", "hardware"}}, tickets.Member{UserID: ids["Imran"], Capacity: 8, Skills: []string{"software", "access"}},
		tickets.Member{UserID: ids["Asha"], Capacity: 6, Skills: []string{"access"}})
	gFac := mk("Facilities and admin", "Offices, stores, keys, furniture and travel.", tickets.StrategyRoundRobin,
		tickets.Member{UserID: ids["Arjun"], Capacity: 10}, tickets.Member{UserID: ids["Karan"], Capacity: 10})
	gStore := mk("Store operations", "POS, scanners and anything that stops a store trading.", tickets.StrategyManual,
		tickets.Member{UserID: ids["Faisal"], Capacity: 6}, tickets.Member{UserID: ids["Arjun"], Capacity: 6})
	s.count("desk_groups", 3)

	// ---- SLA rules ----
	for _, r := range []tickets.SLARule{
		{Name: "Major incident", Enabled: true, MatchType: "incident", MatchPriority: "urgent", FirstResponseMin: 15, ResolveMin: 120, BusinessHours: false},
		{Name: "Urgent", Enabled: true, MatchPriority: "urgent", FirstResponseMin: 30, ResolveMin: 240, BusinessHours: false},
		{Name: "High", Enabled: true, MatchPriority: "high", FirstResponseMin: 120, ResolveMin: 480, BusinessHours: true},
		{Name: "Normal", Enabled: true, MatchPriority: "normal", FirstResponseMin: 480, ResolveMin: 2880, BusinessHours: true},
		{Name: "Low", Enabled: true, MatchPriority: "low", FirstResponseMin: 1440, ResolveMin: 7200, BusinessHours: true},
	} {
		if _, err := repo.SaveSLARule(ctx, s.org, projID, r); err != nil {
			s.warn("desk sla", err)
		}
	}

	// ---- knowledge base ----
	kb := []struct{ cat, title, body string }{
		{"Accounts", "Reset your password", "Open the sign-in page and choose Forgot password. A reset link is sent to your work email and stays valid for 30 minutes. If you do not get the email, check spam, then ask IT Support."},
		{"Accounts", "Set up two-step sign-in", "Open Settings, Security, then Two-step sign-in. Scan the code with an authenticator app and save the recovery codes in a safe place."},
		{"Accounts", "Request access to a shared folder", "Ask the folder owner to share it with you from Drive. If you do not know the owner, raise a request with the folder name and the reason you need access."},
		{"Network", "Connect to the office Wi-Fi", "Choose the network named Adrient-Staff and sign in with your work account. Guests use Adrient-Guest, which changes its password every Monday."},
		{"Network", "VPN keeps disconnecting", "Switch off power saving for your Wi-Fi adapter, update the VPN client and try the wired connection. If drops continue, send the time of the last three drops."},
		{"Network", "Slow internet at a store", "Run a speed test next to the router. Under 10 Mbps means the line is at fault, so note the speed and raise a ticket with the store name."},
		{"Devices", "Pair a Bluetooth barcode scanner", "Hold the trigger and the pairing button for five seconds until the light blinks blue, then pick the scanner in the POS device list."},
		{"Devices", "Printer jams or prints blank pages", "Open the rear cover, remove the paper path and reload fresh paper. Blank pages usually mean an empty toner cartridge."},
		{"Devices", "Request a new laptop or monitor", "Use the Hardware request form. Managers approve the request and IT delivers it within five working days."},
		{"Software", "Install approved software", "Open the Software portal on your laptop and choose the app. Anything not listed needs a request with a business reason."},
		{"Software", "Email signature template", "Copy the standard signature from the brand guide and paste it into Mail settings. Do not change fonts or colours."},
		{"Facilities", "Book a meeting room", "Create a Calendar event, add the room as a resource and the booking is confirmed automatically if the room is free."},
		{"Facilities", "Office keys and access cards", "Raise a request with the person's name, the building and the start date. Cards are ready two working days after approval."},
		{"Facilities", "Report an AC or power fault", "Call the facilities desk for outages. For noise or poor cooling, raise a ticket with the room and a photo if you can."},
		{"Policies", "What counts as an urgent ticket", "Urgent means a store cannot trade, a customer is blocked or people are unsafe. Everything else is high, normal or low."},
	}
	for _, a := range kb {
		if _, err := repo.SaveArticle(ctx, s.org, s.admin, projID, tickets.Article{Title: a.title, Body: a.body, Published: true, Category: a.cat}); err != nil {
			s.warn("desk kb", err)
		}
	}
	s.count("desk_articles", len(kb))

	// ---- request forms ----
	for _, f := range []tickets.RequestForm{
		{Name: "Hardware request", Description: "Laptops, monitors, phones and accessories.", TicketType: "request", GroupID: gIT, Category: "Hardware", Enabled: true, Fields: []tickets.FormField{
			{Key: "item", Label: "What do you need?", Type: "select", Required: true, Options: []string{"Laptop", "Monitor", "Phone", "Keyboard and mouse", "Other"}},
			{Key: "needed_by", Label: "Needed by", Type: "date"}, {Key: "reason", Label: "Reason", Type: "longtext", Required: true}}},
		{Name: "New joiner setup", Description: "Accounts, laptop and desk for a new colleague.", TicketType: "request", GroupID: gIT, Category: "Onboarding", Enabled: true, Fields: []tickets.FormField{
			{Key: "name", Label: "New joiner's name", Type: "text", Required: true}, {Key: "start", Label: "Start date", Type: "date", Required: true},
			{Key: "dept", Label: "Department", Type: "select", Required: true, Options: []string{"Engineering", "Sales", "Finance", "Operations", "People and Culture"}},
			{Key: "software", Label: "Software needed", Type: "longtext"}}},
		{Name: "Report a store problem", Description: "POS, scanners, power or connectivity at a store.", TicketType: "incident", GroupID: gStore, Category: "Store", Enabled: true, Fields: []tickets.FormField{
			{Key: "store", Label: "Store", Type: "select", Required: true, Options: []string{"Pune", "Nashik", "Indore", "Kochi", "Surat"}},
			{Key: "stopped", Label: "Is the store unable to trade?", Type: "checkbox"}, {Key: "what", Label: "What is happening?", Type: "longtext", Required: true}}},
		{Name: "Facilities request", Description: "Keys, access cards, furniture, travel and repairs.", TicketType: "request", GroupID: gFac, Category: "Facilities", Enabled: true, Fields: []tickets.FormField{
			{Key: "location", Label: "Location", Type: "text", Required: true}, {Key: "what", Label: "What do you need?", Type: "longtext", Required: true}}},
	} {
		if _, err := repo.SaveForm(ctx, s.org, projID, f); err != nil {
			s.warn("desk form", err)
		}
	}
	s.count("desk_forms", 4)

	// ---- tickets ----
	type tpl struct{ title, body, typ, cat, group string }
	tpls := []tpl{
		{"Cannot sign in to the %s store POS", "POS shows 'session expired' after every login.", "incident", "Store", "store"},
		{"Barcode scanner not pairing at %s", "Scanner does not pair over Bluetooth.", "incident", "Hardware", "store"},
		{"Receipt printer offline at %s", "Receipts do not print, the light is red.", "incident", "Hardware", "store"},
		{"Request: second monitor for %s", "Please arrange a second monitor.", "request", "Hardware", "it"},
		{"VPN keeps disconnecting for %s", "VPN drops every 15 minutes.", "question", "Network", "it"},
		{"Reset password for %s", "Locked out after too many attempts.", "question", "Accounts", "it"},
		{"Access to the finance folder for %s", "Needs read access to the finance folder.", "request", "Accounts", "it"},
		{"Install design software for %s", "License is available, needs installation.", "task", "Software", "it"},
		{"Laptop battery swelling, %s", "The battery lifts the trackpad. Unsafe to charge.", "incident", "Hardware", "it"},
		{"Wi-Fi slow near %s desk", "Speed test shows 3 Mbps.", "question", "Network", "it"},
		{"Air conditioner noisy, %s floor", "Loud noise and poor cooling.", "question", "Facilities", "fac"},
		{"Keys and access cards for %s", "Need keys and two access cards.", "request", "Facilities", "fac"},
		{"Standing desk for %s", "Medical advice for a standing desk.", "request", "Facilities", "fac"},
		{"Water leak near %s pantry", "Water on the floor beside the pantry.", "incident", "Facilities", "fac"},
		{"Office travel booking for %s", "Book travel to the Nashik store.", "request", "Facilities", "fac"},
	}
	who := []string{"Pune", "Nashik", "Indore", "Kochi", "Surat", "Priya", "Rohan", "Neha", "Divya", "Karan", "Sneha", "Arjun", "Imran", "second", "third"}
	prios := []string{"low", "normal", "normal", "normal", "high", "high", "urgent"}
	statuses := []string{"open", "open", "open", "in_progress", "in_progress", "in_progress", "pending", "on_hold", "resolved", "resolved", "resolved", "closed"}
	now := time.Now()
	n := 0
	var firstIncident string
	for i := 0; i < 120; i++ {
		t := tpls[i%len(tpls)]
		w := who[(i*7+i/len(tpls))%len(who)]
		prio := prios[(i*5+i/4)%len(prios)]
		status := statuses[(i*11+i/3)%len(statuses)]
		urg, imp := "medium", "medium"
		switch prio {
		case "urgent":
			urg, imp = "high", "high"
		case "high":
			urg, imp = "high", "medium"
		case "low":
			urg, imp = "low", "low"
		}
		g := map[string]string{"it": gIT, "fac": gFac, "store": gStore}[t.group]
		reqEmail, reqName := fmt.Sprintf("%s.%d@%s", slug(w), i, demoDomain), w+" (requester)"
		var ruid *string
		if m := teamByFirst(w); m != nil {
			reqName, reqEmail = m.name(), m.email()
			if id := ids[w]; id != "" {
				ruid = &id
			}
		}
		if i%9 == 0 {
			reqEmail, reqName = fmt.Sprintf("accounts@%s", slug(s.r.company())+".example"), "Accounts desk"
		}
		title := fmt.Sprintf(t.title, w)
		tk, err := repo.CreateTicket(tickets.WithCreateOpts(ctx, tickets.CreateOpts{GroupID: g, Type: t.typ, Category: t.cat, Urgency: urg, Impact: imp}),
			s.org, projID, title, t.body, prio, ruid, reqName, reqEmail, "web")
		if err != nil {
			s.warn("desk ticket", err)
			continue
		}
		n++
		if t.typ == "incident" && (prio == "urgent" || prio == "high") && firstIncident == "" {
			firstIncident = tk.ID
		}
		// store tickets are not auto-routed: assign some by hand
		if tk.AssigneeUserID == nil && g == gStore && i%3 != 0 {
			a := ids["Faisal"]
			if i%2 == 0 {
				a = ids["Arjun"]
			}
			if a != "" {
				_, _ = repo.UpdateTicket(ctx, s.org, tk.ID, nil, nil, nil, nil, &a, false)
			}
		}
		age := time.Duration(2+(i*37)%900) * time.Hour // up to ~38 days
		created := now.Add(-age)
		if status != "open" {
			st := status
			if _, err := repo.UpdateTicket(ctx, s.org, tk.ID, &st, nil, nil, nil, nil, false); err != nil && (status == "resolved" || status == "closed") {
				// resolved needs a category and the template always sets one; fall back quietly
				s.warn("desk status", err)
			}
		}
		if status != "open" || i%2 == 0 {
			agent := tk.AssigneeUserID
			if agent == nil {
				a := ids["Faisal"]
				agent = &a
			}
			if c, err := repo.AddComment(ctx, s.org, tk.ID, agent, "Support agent", "Hi, I am looking into this now.", false); err == nil {
				_, _ = s.pool.Exec(s.ctx, `UPDATE adrient.ticket_comments SET created_at=$2 WHERE id=$1::uuid`, c.ID, created.Add(time.Duration(8+i%50)*time.Minute))
			}
		}
		if i%6 == 0 {
			_, _ = repo.AddComment(ctx, s.org, tk.ID, nil, reqName, "Thanks, any update on this?", false)
			_, _ = repo.AddComment(ctx, s.org, tk.ID, agentPtr(tk.AssigneeUserID, ids["Faisal"]), "Support agent", "Checked the logs. Will confirm once the change is rolled out.", true)
		}
		resolved := "NULL"
		if status == "resolved" || status == "closed" {
			resolved = "$2::timestamptz + interval '5 hours'"
		}
		first := "$2::timestamptz + interval '25 minutes'"
		if status == "open" && i%2 != 0 {
			first = "NULL"
		}
		snooze := "NULL"
		if status == "pending" && i%4 == 0 {
			snooze = "now() + interval '1 day'"
		}
		if _, err := s.pool.Exec(s.ctx, `UPDATE adrient.tickets SET created_at=$2::timestamptz, updated_at=$2::timestamptz + interval '3 hours', resolved_at=`+resolved+`, first_response_at=`+first+`, snoozed_until=`+snooze+`
			WHERE id=$1::uuid`, tk.ID, created); err != nil {
			s.warn("desk ticket dates", err)
		}
		if i%7 == 0 && status != "closed" {
			_, _ = repo.AddTime(ctx, s.org, s.admin, "Asha Menon", tk.ID, tickets.TimeEntry{Minutes: 15 + (i%5)*10, Billable: i%2 == 0, Note: "Investigation"})
		}
	}
	s.count("desk_tickets", n)
	if firstIncident != "" {
		_ = repo.SetMajor(ctx, s.org, firstIncident, true)
		_, _ = s.pool.Exec(s.ctx, `UPDATE adrient.tickets SET status='in_progress', resolved_at=NULL, created_at=now() - interval '55 minutes', updated_at=now() - interval '5 minutes' WHERE id=$1::uuid`, firstIncident)
	}

	// a change request with a two-step approval, and a parent with children
	chg, err := repo.CreateTicket(tickets.WithCreateOpts(ctx, tickets.CreateOpts{GroupID: gIT, Type: "change", Category: "Network", Urgency: "medium", Impact: "high"}),
		s.org, projID, "Change: move the Pune store to the new firewall", "Planned window Sunday 02:00-04:00. Rollback: restore the old rule set.", "high", nil, "Faisal Khan", "faisal.khan@"+demoDomain, "web")
	if err == nil {
		if ids["Imran"] != "" && ids["Priya"] != "" {
			_ = repo.RequestApprovals(ctx, s.org, chg.ID, [][]string{{ids["Imran"]}, {ids["Priya"]}})
			_ = repo.Decide(tickets.WithActor(s.ctx, tickets.Actor{ID: ids["Imran"], Name: "Imran Sheikh"}), s.org, chg.ID, ids["Imran"], "approved", "Rollback plan looks fine.")
		}
	}
	if par, err := repo.CreateTicket(tickets.WithCreateOpts(ctx, tickets.CreateOpts{GroupID: gIT, Type: "task", Category: "Onboarding"}), s.org, projID, "Onboard Nikhil Rao (starts Monday)", "Accounts, laptop, access cards and induction.", "normal", nil, "Neha Kapoor", "neha.kapoor@"+demoDomain, "web"); err == nil {
		for _, c := range []string{"Create accounts and mailbox for Nikhil", "Prepare laptop and charger", "Access card and desk"} {
			if ch, err := repo.CreateTicket(tickets.WithCreateOpts(ctx, tickets.CreateOpts{GroupID: gIT, Type: "task", Category: "Onboarding"}), s.org, projID, c, "", "normal", nil, "Neha Kapoor", "neha.kapoor@"+demoDomain, "web"); err == nil {
				_ = repo.SetParent(ctx, s.org, ch.ID, par.ID)
			}
		}
	}

	// ---- automations ----
	for _, a := range []tickets.Automation{
		{Name: "Store problems go to Store operations", Kind: tickets.KindDispatch, Enabled: true, MatchMode: "any",
			Conds:   []tickets.Condition{{Field: "title", Op: "contains", Value: "POS"}, {Field: "title", Op: "contains", Value: "scanner"}},
			Actions: []tickets.Action{{Type: "set_group", Value: gStore}, {Type: "add_tag", Value: "store"}}},
		{Name: "Urgent incidents notify the group", Kind: tickets.KindTrigger, Event: "create", Enabled: true, MatchMode: "all",
			Conds:   []tickets.Condition{{Field: "priority", Op: "is", Value: "urgent"}, {Field: "type", Op: "is", Value: "incident"}},
			Actions: []tickets.Action{{Type: "notify", Value: "group"}, {Type: "add_tag", Value: "escalated"}}},
		{Name: "Remind when pending for 3 days", Kind: tickets.KindTime, Enabled: true, MatchMode: "all",
			Conds:   []tickets.Condition{{Field: "status", Op: "is", Value: "pending"}, {Field: "hours_since_update", Op: "gt", Value: "72"}},
			Actions: []tickets.Action{{Type: "add_note", Value: "No reply from the requester for 3 days. Close if there is no answer tomorrow."}}},
		{Name: "Auto-close solved tickets after 5 days", Kind: tickets.KindTime, Enabled: true, MatchMode: "all",
			Conds:   []tickets.Condition{{Field: "status", Op: "is", Value: "resolved"}, {Field: "hours_since_update", Op: "gt", Value: "120"}},
			Actions: []tickets.Action{{Type: "set_status", Value: "closed"}}},
		{Name: "VIP requesters get high priority", Kind: tickets.KindTrigger, Event: "create", Enabled: true, MatchMode: "all",
			Conds:   []tickets.Condition{{Field: "tag", Op: "is", Value: "vip"}},
			Actions: []tickets.Action{{Type: "set_priority", Value: "high"}}},
		{Name: "Access requests need the access skill", Kind: tickets.KindDispatch, Enabled: true, MatchMode: "all",
			Conds:   []tickets.Condition{{Field: "category", Op: "is", Value: "Accounts"}},
			Actions: []tickets.Action{{Type: "add_tag", Value: "access"}}},
	} {
		if _, err := repo.SaveAutomation(ctx, s.org, s.admin, projID, a); err != nil {
			s.warn("desk automation", err)
		}
	}
	s.count("desk_automations", 6)
}

func agentPtr(p *string, fallback string) *string {
	if p != nil {
		return p
	}
	return &fallback
}
