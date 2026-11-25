package tickets

import (
	"testing"
	"time"
)

func TestCustomFieldValidation(t *testing.T) {
	defs := []FieldDef{
		{Key: "asset_tag", Label: "Asset tag", Kind: "text", RequiredOn: []string{"resolved"}},
		{Key: "cost", Label: "Cost", Kind: "number"},
		{Key: "site", Label: "Site", Kind: "select", Options: []string{"Pune", "Kochi"}},
		{Key: "due", Label: "Due", Kind: "date"},
		{Key: "approved", Label: "Approved", Kind: "checkbox"},
		{Key: "change_window", Label: "Change window", Kind: "text", TicketType: "change", RequiredOn: []string{"in_progress"}},
	}
	good, err := ValidateCustom(defs, "incident", map[string]string{"cost": "12.50", "site": "Pune", "due": "2026-12-01", "approved": "true", "legacy": "kept", "empty": " "})
	if err != nil || good["cost"] != "12.50" || good["legacy"] != "kept" || len(good) != 5 {
		t.Fatalf("%v %v", good, err)
	}
	for name, bad := range map[string]map[string]string{"number": {"cost": "twelve"}, "select": {"site": "Delhi"}, "date": {"due": "01/12/2026"}, "bool": {"approved": "maybe"}} {
		if _, err := ValidateCustom(defs, "incident", bad); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if m := MissingCustom(defs, "incident", "resolved", map[string]string{}); len(m) != 1 || m[0] != "Asset tag" {
		t.Fatalf("%v", m)
	}
	if m := MissingCustom(defs, "incident", "in_progress", nil); len(m) != 0 {
		t.Fatal("a change-only field applied to an incident")
	}
	if m := MissingCustom(defs, "change", "in_progress", nil); len(m) != 1 {
		t.Fatal("change field not required")
	}
	if m := MissingCustom(defs, "incident", "resolved", map[string]string{"asset_tag": "A-1"}); len(m) != 0 {
		t.Fatal("filled field reported missing")
	}
	bad := FieldDef{Key: "Bad Key!", Label: "x", Kind: "text"}
	if bad.Validate() == nil {
		t.Fatal("bad key accepted")
	}
	sel := FieldDef{Key: "k", Label: "x", Kind: "select", Options: []string{"a"}}
	if sel.Validate() == nil {
		t.Fatal("a select with one option accepted")
	}
}

func TestCustomFieldsAndSkillRouting(t *testing.T) {
	e := newDeskEnv(t)
	fd, err := e.r.SaveFieldDef(e.ctx, e.org, e.proj.ID, FieldDef{Key: "asset_tag", Label: "Asset tag", Kind: "text", RequiredOn: []string{"resolved"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.SaveFieldDef(e.ctx, e.org, e.proj.ID, FieldDef{Key: "asset_tag", Label: "Dup", Kind: "text"}); err == nil {
		t.Fatal("duplicate key accepted")
	}
	if _, err := e.r.SaveFieldDef(e.ctx, e.otherOrg, e.proj.ID, FieldDef{Key: "x", Label: "x", Kind: "text"}); err == nil {
		t.Fatal("field added to another organisation's project")
	}
	tk := e.ticket(t, "Laptop broken", CreateOpts{})
	resolved := "resolved"
	if _, err := e.r.UpdateTicket(e.ctx, e.org, tk.ID, &resolved, nil, nil, nil, nil, false); err == nil {
		t.Fatal("required custom field not enforced")
	}
	if _, err := e.r.SetFields(e.ctx, e.org, tk.ID, FieldsPatch{Custom: &map[string]string{"asset_tag": "LT-204"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.UpdateTicket(e.ctx, e.org, tk.ID, &resolved, nil, nil, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := e.r.DeleteFieldDef(e.ctx, e.org, fd.ID); err != nil {
		t.Fatal(err)
	}

	// skills: only the member with the skill is chosen; with nobody qualified the ticket stays unassigned
	g, err := e.r.SaveGroup(e.ctx, e.org, e.proj.ID, Group{Name: "Skilled", Strategy: StrategyRoundRobin, Members: []Member{
		{UserID: e.users[0], Capacity: 9, Skills: []string{"billing"}}, {UserID: e.users[1], Capacity: 9, Skills: []string{"network"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.SaveAutomation(e.ctx, e.org, e.users[0], e.proj.ID, Automation{Name: "VPN needs network skill", Kind: KindDispatch, Enabled: true,
		Conds: []Condition{{"title", "contains", "vpn"}}, Actions: []Action{{"set_group", g.ID}, {"require_skill", "Network"}}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		v := e.ticket(t, "VPN problem", CreateOpts{})
		d, _ := e.r.GetDesk(e.ctx, e.org, v.ID)
		if d.Assignee != e.users[1] || len(d.NeedSkills) != 1 || d.NeedSkills[0] != "network" {
			t.Fatalf("vpn ticket %d: assignee %q skills %v", i, d.Assignee, d.NeedSkills)
		}
	}
	if _, err := e.r.SaveAutomation(e.ctx, e.org, e.users[0], e.proj.ID, Automation{Name: "Legal", Kind: KindDispatch, Enabled: true,
		Conds: []Condition{{"title", "contains", "contract"}}, Actions: []Action{{"set_group", g.ID}, {"require_skill", "legal"}}}); err != nil {
		t.Fatal(err)
	}
	c := e.ticket(t, "Contract review", CreateOpts{})
	d, _ := e.r.GetDesk(e.ctx, e.org, c.ID)
	if d.Assignee != "" || d.GroupID != g.ID {
		t.Fatalf("nobody has the skill, ticket must wait in the team: %+v", d)
	}
	events, _ := e.r.Events(e.ctx, e.org, c.ID)
	seen := false
	for _, ev := range events {
		seen = seen || ev.Kind == "routing"
	}
	if !seen {
		t.Fatal("the missing skill was not reported in the trail")
	}
	// out of office setting
	if err := e.r.SetOOO(e.ctx, e.org, e.users[1], ptrTime(time.Now().Add(-time.Hour))); err == nil {
		t.Fatal("past date accepted")
	}
	until := time.Now().Add(48 * time.Hour)
	if err := e.r.SetOOO(e.ctx, e.org, e.users[1], &until); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.r.GetOOO(e.ctx, e.org, e.users[1]); got == nil {
		t.Fatal("out of office not stored")
	}
	if err := e.r.SetOOO(e.ctx, e.org, e.users[1], nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.r.GetOOO(e.ctx, e.org, e.users[1]); got != nil {
		t.Fatal("out of office not cleared")
	}
}
