package tickets

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFromMailAndChat(t *testing.T) {
	e := newDeskEnv(t)
	var mailID, chatID, chanID string
	if err := e.r.pool.QueryRow(e.ctx, `INSERT INTO adrient.mail_messages (org_id, owner_id, folder, from_addr, from_name, to_addrs, subject, body)
		VALUES ($1,$2,'inbox','dev@acme.example','Dev','["me@x.test"]','Invoice export fails','Error 500 on export.') RETURNING id::text`, e.org, e.users[0]).Scan(&mailID); err != nil {
		t.Fatal(err)
	}
	if err := e.r.pool.QueryRow(e.ctx, `INSERT INTO adrient.chat_channels (org_id, kind, name, member_ids) VALUES ($1,'private','ops', to_jsonb(ARRAY[$2::text])) RETURNING id::text`, e.org, e.users[0]).Scan(&chanID); err != nil {
		t.Fatal(err)
	}
	if err := e.r.pool.QueryRow(e.ctx, `INSERT INTO adrient.chat_messages (channel_id, org_id, sender_id, sender_name, body) VALUES ($1,$2,$3,'Bo','The projector in room 2 is dead
please look') RETURNING id::text`, chanID, e.org, e.users[1]).Scan(&chatID); err != nil {
		t.Fatal(err)
	}
	tm, err := e.r.FromSource(e.ctx, e.org, e.users[0], "mail", mailID, "")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := e.r.GetDesk(e.ctx, e.org, tm.ID)
	if d.Title != "Invoice export fails" || d.RequesterEmail != "dev@acme.example" || d.Source != "mail" {
		t.Fatalf("from mail: %+v", d)
	}
	tc, err := e.r.FromSource(e.ctx, e.org, e.users[0], "chat", chatID, e.proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	d, _ = e.r.GetDesk(e.ctx, e.org, tc.ID)
	if d.Title != "The projector in room 2 is dead" || d.RequesterName != "Bo" || !strings.HasPrefix(d.RequesterEmail, "u1-") {
		t.Fatalf("from chat: %+v", d)
	}
	var n int
	_ = e.r.pool.QueryRow(e.ctx, `SELECT count(*) FROM adrient.record_links WHERE org_id=$1 AND from_id=$2 AND to_app IN ('mail','chat')`, e.org, tm.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("links %d", n)
	}
	// not your mail, not your channel, other org
	if _, err := e.r.FromSource(e.ctx, e.org, e.users[1], "mail", mailID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("someone else's mail: %v", err)
	}
	if _, err := e.r.FromSource(e.ctx, e.org, e.users[2], "chat", chatID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("channel the user is not in: %v", err)
	}
	if _, err := e.r.FromSource(e.ctx, e.otherOrg, e.users[0], "mail", mailID, ""); err == nil {
		t.Fatal("other organisation read a mail")
	}
	if _, err := e.r.FromSource(e.ctx, e.org, e.users[0], "sms", mailID, ""); err == nil {
		t.Fatal("unknown source accepted")
	}
}

func TestAppHooks(t *testing.T) {
	e := newDeskEnv(t)
	var tasks, issues, times []string
	e.r.Deps.CreateTask = func(_ context.Context, _, assignee, title, _, link string) error {
		tasks = append(tasks, assignee+"|"+title+"|"+link)
		return nil
	}
	e.r.Deps.CreateIssue = func(_ context.Context, _, _, _, title, _ string) (string, string, error) {
		issues = append(issues, title)
		return "11111111-1111-1111-1111-111111111111", "ENG-7", nil
	}
	e.r.Deps.LogTime = func(_ context.Context, _, user string, minutes int, date, note string, billable bool) error {
		times = append(times, user+"|"+note)
		if minutes == 99 {
			return errors.New("no project")
		}
		return nil
	}
	if _, err := e.r.SaveAutomation(e.ctx, e.org, e.users[0], e.proj.ID, Automation{Name: "Task", Kind: KindTrigger, Event: "create", Enabled: true,
		Conds: []Condition{{"title", "contains", "audit"}}, Actions: []Action{{"create_task", "Prepare audit pack"}}}); err != nil {
		t.Fatal(err)
	}
	tk := e.ticket(t, "Annual audit request", CreateOpts{})
	if len(tasks) != 1 || !strings.Contains(tasks[0], "Prepare audit pack ("+e.proj.Key+"-") || !strings.HasSuffix(tasks[0], "/tickets?ticket="+tk.ID) || strings.HasPrefix(tasks[0], "|") {
		t.Fatalf("task action: %v", tasks)
	}
	if _, err := e.r.AddTime(e.ctx, e.org, e.users[0], "Asha", tk.ID, TimeEntry{Minutes: 30, Note: "call"}); err != nil || len(times) != 1 {
		t.Fatalf("time hook: %v %v", times, err)
	}
	if _, err := e.r.AddTime(e.ctx, e.org, e.users[0], "Asha", tk.ID, TimeEntry{Minutes: 99}); err != nil {
		t.Fatal(err) // the ticket entry is kept even when Time tracking refuses it
	}
	events, _ := e.r.Events(e.ctx, e.org, tk.ID)
	found := false
	for _, ev := range events {
		if ev.Kind == "time" && strings.Contains(string(mustJSON(ev.Detail)), "not linked") {
			found = true
		}
	}
	if !found {
		t.Fatal("a refused Time tracking entry was not reported")
	}
	id, key, err := e.r.CreateIssueFor(e.ctx, e.org, e.users[0], tk.ID, "")
	if err != nil || key != "ENG-7" || id == "" || len(issues) != 1 {
		t.Fatalf("issue: %v %v", key, err)
	}
	var links int
	_ = e.r.pool.QueryRow(e.ctx, `SELECT count(*) FROM adrient.record_links WHERE org_id=$1 AND from_id=$2 AND to_app='projects'`, e.org, tk.ID).Scan(&links)
	if links != 1 {
		t.Fatalf("issue link %d", links)
	}
	e.r.Deps.CreateIssue = nil
	if _, _, err := e.r.CreateIssueFor(e.ctx, e.org, e.users[0], tk.ID, ""); err == nil {
		t.Fatal("missing Projects must be reported")
	}
	_ = time.Now
}

func mustJSON(v any) []byte { b, _ := jsonMarshal(v); return b }
