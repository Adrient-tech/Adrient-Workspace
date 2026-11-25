package tickets

import (
	"testing"
	"time"
)

func TestScheduledSend(t *testing.T) {
	e := newDeskEnv(t)
	tk := e.ticket(t, "Schedule me", CreateOpts{})
	if _, err := e.r.ScheduleReply(e.ctx, e.org, e.users[0], "Asha", tk.ID, Scheduled{Body: "soon", SendAt: time.Now().Add(10 * time.Second)}); err == nil {
		t.Fatal("a time under a minute away was accepted")
	}
	if _, err := e.r.ScheduleReply(e.ctx, e.org, e.users[0], "Asha", tk.ID, Scheduled{Body: " ", SendAt: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("empty message accepted")
	}
	s, err := e.r.ScheduleReply(e.ctx, e.org, e.users[0], "Asha", tk.ID, Scheduled{Body: "Good morning, a fix is rolling out.", SendAt: time.Now().Add(2 * time.Minute), Solve: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.r.ScheduleReply(e.ctx, e.otherOrg, e.users[0], "Asha", tk.ID, Scheduled{Body: "x", SendAt: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("scheduled into another organisation")
	}
	if n := e.r.sendDueScheduled(e.ctx); n != 0 {
		t.Fatalf("sent early: %d", n)
	}
	// someone else cannot cancel it, the author can
	if err := e.r.CancelScheduled(e.ctx, e.org, e.users[1], false, s.ID); err == nil {
		t.Fatal("another agent cancelled it")
	}
	// make it due
	if _, err := e.r.pool.Exec(e.ctx, `UPDATE adrient.ticket_scheduled SET send_at = now() - interval '1 second' WHERE id=$1`, s.ID); err != nil {
		t.Fatal(err)
	}
	if n := e.r.sendDueScheduled(e.ctx); n != 1 {
		t.Fatalf("sent %d", n)
	}
	if n := e.r.sendDueScheduled(e.ctx); n != 0 {
		t.Fatal("sent twice")
	}
	_, comments, _ := e.r.GetTicket(e.ctx, e.org, tk.ID)
	if len(comments) != 1 || comments[0].Body != "Good morning, a fix is rolling out." || comments[0].AuthorName != "Asha" {
		t.Fatalf("comments %+v", comments)
	}
	d, _ := e.r.GetDesk(e.ctx, e.org, tk.ID)
	if d.FirstResponse == nil {
		t.Fatal("first response not recorded by scheduled send")
	}
	if pending, _ := e.r.ListScheduled(e.ctx, e.org, tk.ID); len(pending) != 0 {
		t.Fatal("sent message still listed")
	}
	// a cancelled message never goes out
	s2, _ := e.r.ScheduleReply(e.ctx, e.org, e.users[0], "Asha", tk.ID, Scheduled{Body: "never", SendAt: time.Now().Add(time.Hour), Internal: true})
	if err := e.r.CancelScheduled(e.ctx, e.org, e.users[0], false, s2.ID); err != nil {
		t.Fatal(err)
	}
	_, _ = e.r.pool.Exec(e.ctx, `UPDATE adrient.ticket_scheduled SET send_at = now() - interval '1 second'`)
	if n := e.r.sendDueScheduled(e.ctx); n != 0 {
		t.Fatal("cancelled message was sent")
	}
}

func TestAttachments(t *testing.T) {
	e := newDeskEnv(t)
	tk := e.ticket(t, "Files", CreateOpts{})
	var fid, other string
	if err := e.r.pool.QueryRow(e.ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, name, mime_type, storage_key, size_bytes) VALUES ($1,$2,'router-config.pdf','application/pdf','k1',2048) RETURNING id::text`, e.org, e.users[0]).Scan(&fid); err != nil {
		t.Fatal(err)
	}
	if err := e.r.pool.QueryRow(e.ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, name, mime_type, storage_key, size_bytes) VALUES ($1,$2,'foreign.pdf','application/pdf','k2',1) RETURNING id::text`, e.otherOrg, e.users[0]).Scan(&other); err != nil {
		t.Fatal(err)
	}
	a, err := e.r.AttachFile(e.ctx, e.org, e.users[0], tk.ID, fid)
	if err != nil || a.Name != "router-config.pdf" || a.Size != 2048 {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := e.r.AttachFile(e.ctx, e.org, e.users[0], tk.ID, other); err == nil {
		t.Fatal("a file of another organisation was attached")
	}
	if _, err := e.r.AttachFile(e.ctx, e.org, e.users[0], tk.ID, "not-a-uuid"); err == nil {
		t.Fatal("garbage id accepted")
	}
	list, _ := e.r.ListAttachments(e.ctx, e.org, tk.ID)
	if len(list) != 1 {
		t.Fatalf("%d", len(list))
	}
	if err := e.r.RemoveAttachment(e.ctx, e.org, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.r.RemoveAttachment(e.ctx, e.otherOrg, a.ID); err == nil {
		t.Fatal("removed twice")
	}
}
