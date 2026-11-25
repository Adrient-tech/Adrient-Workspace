package tickets

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestInboundParsing(t *testing.T) {
	n, a, ok := ParseAddress(`"Ravi K" <Ravi@Acme.Example>`)
	if !ok || n != "Ravi K" || a != "ravi@acme.example" {
		t.Fatalf("%q %q %v", n, a, ok)
	}
	if _, _, ok := ParseAddress("not an address"); ok {
		t.Fatal("accepted junk")
	}
	if k, num, ok := TicketRefIn("Re: VPN down [help-42]"); !ok || k != "HELP" || num != 42 {
		t.Fatalf("%s %d %v", k, num, ok)
	}
	if _, _, ok := TicketRefIn("no reference here [x]"); ok {
		t.Fatal("matched junk")
	}
	got := StripQuoted("Thanks, it works now.\n\nOn Mon, 3 Nov 2026, Support wrote:\n> did you try\n> turning it off")
	if got != "Thanks, it works now." {
		t.Fatalf("stripped %q", got)
	}
	if got := StripQuoted("line one\n> quoted\nline two\n-- \nSig"); got != "line one\nline two" {
		t.Fatalf("%q", got)
	}
	if cleanSubject("Re: Fwd: Printer jam [HELP-3]") != "Printer jam" {
		t.Fatal(cleanSubject("Re: Fwd: Printer jam [HELP-3]"))
	}
	if !IsAutomated(InboundMail{Subject: "Automatic reply: away"}, "a@b.test") || !IsAutomated(InboundMail{}, "no-reply@b.test") || !IsAutomated(InboundMail{AutoReply: true}, "a@b.test") {
		t.Fatal("automation not detected")
	}
	if IsAutomated(InboundMail{Subject: "Help"}, "ravi@b.test") {
		t.Fatal("false positive")
	}
}

func TestInboundEmailToTicket(t *testing.T) {
	e := newDeskEnv(t)
	alias := "it-" + strings.ToLower(strings.ReplaceAll(e.proj.Key, "-", "")) + "@support.example"
	g, err := e.r.SaveGroup(e.ctx, e.org, e.proj.ID, Group{Name: "Mail team", Strategy: StrategyRoundRobin, EmailAlias: alias, Members: []Member{{UserID: e.users[1], Capacity: 5}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.SaveGroup(e.ctx, e.org, e.proj.ID, Group{Name: "Bad", EmailAlias: "nope"}); err == nil {
		t.Fatal("invalid alias accepted")
	}
	if _, err := e.r.SaveGroup(e.ctx, e.org, e.proj.ID, Group{Name: "Dup", EmailAlias: alias}); err == nil {
		t.Fatal("duplicate alias accepted")
	}
	var sent []string
	e.r.Deps.Mail = func(_ context.Context, to []string, subject, _, _ string) error {
		sent = append(sent, to[0]+"|"+subject)
		return nil
	}
	fake := &FakeMailSource{}
	e.r.Inbound = fake
	now := time.Now()
	fake.Queue = []InboundMail{
		{Key: "<m1@x>", From: "Ravi <ravi@acme.example>", To: []string{alias}, Subject: "Printer jam on floor 2", Body: "It jams every ten pages.", Date: now},
		{Key: "<m1@x>", From: "Ravi <ravi@acme.example>", To: []string{alias}, Subject: "Printer jam on floor 2", Body: "duplicate delivery", Date: now},
		{Key: "<m2@x>", From: "no-reply@vendor.example", To: []string{alias}, Subject: "Newsletter", Body: "buy now"},
		{Key: "<m3@x>", From: "Ravi <ravi@acme.example>", To: []string{"someone-else@support.example"}, Subject: "Wrong address", Body: "x"},
	}
	// every message of the fake source belongs to this org
	for i := range fake.Queue {
		fake.Queue[i].OrgID = e.org
	}
	tk, cm, err := e.r.PollInbound(e.ctx)
	if err != nil || tk != 1 || cm != 0 {
		t.Fatalf("poll: %d tickets %d comments %v", tk, cm, err)
	}
	list, _, _ := e.r.ListPage(e.ctx, e.org, ListOpts{ProjectID: e.proj.ID, View: "all", Limit: 10})
	if len(list) != 1 || list[0].Title != "Printer jam on floor 2" || list[0].Source != "email" || list[0].GroupID != g.ID || list[0].Assignee != e.users[1] {
		t.Fatalf("ticket from mail: %+v", list)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "["+list[0].Ref+"]") {
		t.Fatalf("acknowledgement: %v", sent)
	}
	// requester reply by subject reference becomes a comment, quoted text removed
	fake.Queue = []InboundMail{{Key: "<m4@x>", OrgID: e.org, From: "ravi@acme.example", To: []string{alias}, Subject: "Re: Printer jam on floor 2 [" + list[0].Ref + "]",
		Body: "Still jamming.\n\nOn Mon, Support wrote:\n> checked", Date: now}}
	if tk, cm, _ = e.r.PollInbound(e.ctx); tk != 0 || cm != 1 {
		t.Fatalf("reply: %d %d", tk, cm)
	}
	_, comments, _ := e.r.GetTicket(e.ctx, e.org, list[0].ID)
	if len(comments) != 1 || comments[0].Body != "Still jamming." || comments[0].IsInternal {
		t.Fatalf("comments %+v", comments)
	}
	// someone else quoting the reference must not write into the ticket
	fake.Queue = []InboundMail{{Key: "<m5@x>", OrgID: e.org, From: "stranger@evil.example", To: []string{alias}, Subject: "Hi [" + list[0].Ref + "]", Body: "injected"}}
	if tk, cm, _ = e.r.PollInbound(e.ctx); tk != 1 || cm != 0 {
		t.Fatalf("stranger: %d %d", tk, cm)
	}
	_, comments, _ = e.r.GetTicket(e.ctx, e.org, list[0].ID)
	if len(comments) != 1 {
		t.Fatal("a stranger wrote into the ticket")
	}
	// mail in another organisation addressed to this alias does nothing
	fake.Queue = []InboundMail{{Key: "<m6@x>", OrgID: e.otherOrg, From: "x@y.example", To: []string{alias}, Subject: "cross org", Body: "x"}}
	if tk, _, _ = e.r.PollInbound(e.ctx); tk != 0 {
		t.Fatal("cross-organisation mail created a ticket")
	}
	// an agent's public reply is mailed to the requester of an e-mailed ticket
	sent = nil
	agent := e.users[1]
	if _, err := e.r.AddComment(e.ctx, e.org, list[0].ID, &agent, "Bo", "Replaced the roller.", false); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "ravi@acme.example|Re: Printer jam") {
		t.Fatalf("reply mail: %v", sent)
	}
	// internal notes are never mailed
	sent = nil
	_, _ = e.r.AddComment(e.ctx, e.org, list[0].ID, &agent, "Bo", "note", true)
	if len(sent) != 0 {
		t.Fatal("internal note was mailed")
	}
}

func TestInboundLocalInboxSource(t *testing.T) {
	e := newDeskEnv(t)
	alias := "lb-" + strings.ToLower(e.proj.Key) + "@support.example"
	if _, err := e.r.SaveGroup(e.ctx, e.org, e.proj.ID, Group{Name: "Inbox team", EmailAlias: alias}); err != nil {
		t.Fatal(err)
	}
	_, err := e.r.pool.Exec(e.ctx, `INSERT INTO adrient.mail_messages (org_id, owner_id, folder, from_addr, from_name, to_addrs, subject, body)
		VALUES ($1,$2,'inbox','kim@acme.example','Kim','["`+alias+`"]','Laptop will not boot','Black screen')`, e.org, e.users[0])
	if err != nil {
		t.Fatal(err)
	}
	tk, _, err := e.r.PollInbound(e.ctx)
	if err != nil || tk != 1 {
		t.Fatalf("local inbox poll: %d %v", tk, err)
	}
	if tk, _, _ = e.r.PollInbound(e.ctx); tk != 0 {
		t.Fatal("the same mail was processed twice")
	}
}
