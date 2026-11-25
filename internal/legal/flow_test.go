package legal

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit/kittest"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

type fakeInv struct{ drafts []opshttp.InvoiceDraft }

func (f *fakeInv) Create(ctx context.Context, org, user string, d opshttp.InvoiceDraft) (opshttp.DocRef, error) {
	f.drafts = append(f.drafts, d)
	var total float64
	for _, l := range d.Lines {
		total += l.Qty * l.Price * (1 + l.TaxRate/100)
	}
	return opshttp.DocRef{ID: "11111111-1111-1111-1111-111111111111", Number: fmt.Sprintf("INV-%04d", len(f.drafts)), Total: float64(int64(total*100+0.5)) / 100}, nil
}
func (f *fakeInv) Get(ctx context.Context, org, id string) (opshttp.DocRef, error) {
	return opshttp.DocRef{}, nil
}
func (f *fakeInv) ConvertEstimate(ctx context.Context, org, user, id string) (opshttp.DocRef, error) {
	return opshttp.DocRef{}, nil
}

func TestLegalFlow(t *testing.T) {
	fx := kittest.Setup(t)
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	repo.Deps.Now = func() time.Time { return time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC) }
	inv := &fakeInv{}
	repo.Deps.Invoices = inv
	events, tasks := map[string]string{}, map[string]string{}
	repo.Deps.Calendar = func(ctx context.Context, org, owner, id, title, desc, day string) (string, error) {
		if id == "" {
			id = fmt.Sprintf("00000000-0000-0000-0000-%012d", len(events)+1)
		}
		events[id] = day + "|" + title
		return id, nil
	}
	repo.Deps.DeleteEvent = func(ctx context.Context, org, id string) error { delete(events, id); return nil }
	repo.Deps.Task = func(ctx context.Context, org, assignee, id, title, notes, due string, done bool) (string, error) {
		if id == "" {
			id = fmt.Sprintf("00000000-0000-0000-0001-%012d", len(tasks)+1)
		}
		tasks[id] = due + "|" + title
		return id, nil
	}
	repo.Deps.DeleteTask = func(ctx context.Context, org, id string) error { delete(tasks, id); return nil }
	a, u := fx.Admin, fx.User

	if _, err := repo.SaveSettings(ctx, u, Settings{NumberFormat: "year-number", IncrementMin: 6, PracticeAreas: []string{"Corporate"}}); !errors.Is(err, knowkit.ErrForbidden) {
		t.Fatalf("non-admin saved settings: %v", err)
	}
	if _, err := repo.SaveSettings(ctx, a, Settings{NumberFormat: "year-number", IncrementMin: 6, DefaultRate: 500000, Currency: "inr", TermsDays: 30, TaxRateBp: 1800, TaxName: "GST", PracticeAreas: []string{"Corporate", "Litigation"}}); err != nil {
		t.Fatal(err)
	}
	cl, err := repo.CreateClient(ctx, a, ClientInput{Kind: "company", Name: "Kaveri Textiles Pvt Ltd", Email: "legal@kaveri.example", Phone: "+91 98450 11122"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := repo.CreateMatter(ctx, u, MatterInput{Title: "Supply agreement dispute", ClientID: cl.ID, PracticeArea: "Litigation", SolDate: "2027-01-10",
		Parties: []Party{{Name: "Bharat Dyes LLP", Role: "Opposing party"}}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Number != "2026-00001" || m.Status != "intake" || m.ResponsibleID != u.User {
		t.Fatalf("matter %+v", m)
	}
	if len(m.Deadlines) != 1 || m.Deadlines[0].Kind != "limitation" || len(events) != 1 || len(tasks) != 1 {
		t.Fatalf("limitation deadline: %+v events=%v tasks=%v", m.Deadlines, events, tasks)
	}
	if _, err := repo.Move(ctx, u, m.ID, MoveInput{Status: "open"}); err == nil {
		t.Fatal("opened without a conflict check")
	}
	if _, err := repo.Move(ctx, u, m.ID, MoveInput{Status: "pending"}); err == nil {
		t.Fatal("intake -> pending must be refused")
	}
	if _, err := repo.RecordConflict(ctx, u, m.ID, "Bharat Dyes"); err != nil {
		t.Fatal(err)
	}
	m, err = repo.Move(ctx, u, m.ID, MoveInput{Status: "open"})
	if err != nil || m.Status != "open" || m.OpenDate != "2026-11-21" {
		t.Fatalf("open: %+v %v", m, err)
	}
	// 7 minutes rounds up to 12 at a 6-minute increment: 0.2 h x Rs 5,000 = Rs 1,000.
	e, err := repo.AddEntry(ctx, u, m.ID, EntryInput{Kind: "time", Duration: "7m", Description: "Call with client"})
	if err != nil || e.Minutes != 12 || e.Rate != 500000 || e.Amount != 100000 {
		t.Fatalf("entry %+v %v", e, err)
	}
	if _, err := repo.UpdateMatter(ctx, u, m.ID, MatterInput{Title: m.Title, ClientID: cl.ID, PracticeArea: "Litigation", SolDate: "2027-01-10", BillingMethod: "hourly", Rate: 600000}); err != nil {
		t.Fatal(err)
	}
	e2, err := repo.AddEntry(ctx, u, m.ID, EntryInput{Kind: "time", Duration: "1.5", Description: "Draft reply"})
	if err != nil || e2.Amount != 900000 || e2.Rate != 600000 {
		t.Fatalf("entry2 %+v %v", e2, err)
	}
	ex, err := repo.AddEntry(ctx, u, m.ID, EntryInput{Kind: "expense", Price: 120050, Qty: "2.5", Description: "Court fee stamps"})
	if err != nil || ex.Amount != 300125 {
		t.Fatalf("expense %+v %v", ex, err)
	}
	if _, err := repo.Move(ctx, u, m.ID, MoveInput{Status: "closed"}); err == nil {
		t.Fatal("closed with unbilled work")
	}
	pv, err := repo.DraftBill(ctx, u, m.ID, BillInput{Preview: true})
	if err != nil || pv.Fees != 1_000_000 || pv.Expenses != 300125 {
		t.Fatalf("preview %+v %v", pv, err)
	}
	if got, _ := repo.Matter(ctx, u, m.ID); got.Unbilled != 1_300_125 {
		t.Fatalf("preview changed the matter: %d", got.Unbilled)
	}
	b, err := repo.DraftBill(ctx, u, m.ID, BillInput{})
	if err != nil || b.Status != "draft" || b.Number != "2026-00001/B01" || len(b.Lines) != 3 {
		t.Fatalf("bill %+v %v", b, err)
	}
	// fees 10,000.00 + tax 1,800.00 + expenses 3,001.25 = 14,801.25
	if b.Fees != 1_000_000 || b.Tax != 180000 || b.Total != 1_480_125 {
		t.Fatalf("totals %+v", b)
	}
	if _, err := repo.DraftBill(ctx, u, m.ID, BillInput{}); err == nil {
		t.Fatal("billing the same work twice")
	}
	if _, err := repo.UpdateEntry(ctx, u, e.ID, EntryInput{Duration: "2h"}); err == nil {
		t.Fatal("edited a billed entry")
	}
	b, err = repo.IssueBill(ctx, u, b.ID)
	if err != nil || b.Status != "issued" || b.InvoiceNumber != "INV-0001" || len(inv.drafts) != 1 {
		t.Fatalf("issue %+v %v", b, err)
	}
	d := inv.drafts[0]
	if d.ClientName != "Kaveri Textiles Pvt Ltd" || len(d.Lines) != 3 || d.Lines[0].TaxRate != 18 || d.Lines[2].TaxRate != 0 || d.Lines[2].Price != 3001.25 {
		t.Fatalf("invoice draft %+v", d)
	}
	vb, err := repo.VoidBill(ctx, u, b.ID)
	if err != nil || vb.Status != "void" || vb.Note == "" {
		t.Fatalf("void %+v %v", vb, err)
	}
	if got, _ := repo.Matter(ctx, u, m.ID); got.Unbilled != 1_300_125 {
		t.Fatalf("after void unbilled = %d", got.Unbilled)
	}
	m, err = repo.Move(ctx, u, m.ID, MoveInput{Status: "closed", Force: true, Note: "Settled"})
	if err != nil || m.Status != "closed" || m.ClosedDate != "2026-11-21" || len(events) != 0 {
		t.Fatalf("close: %+v %v events=%v", m, err, events)
	}
	if _, err := repo.AddEntry(ctx, u, m.ID, EntryInput{Kind: "time", Duration: "1"}); err == nil {
		t.Fatal("work added to a closed matter")
	}
	if m, err = repo.Move(ctx, u, m.ID, MoveInput{Status: "open"}); err != nil || m.ClosedDate != "" || len(events) != 1 {
		t.Fatalf("reopen: %+v %v events=%v", m, err, events)
	}

	// Flat fee: billed once, and again after the bill is voided.
	fm, err := repo.CreateMatter(ctx, a, MatterInput{Title: "Trademark filing", ClientID: cl.ID, BillingMethod: "flat", FlatFee: 2500000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Move(ctx, a, fm.ID, MoveInput{Status: "open", WaiveConflict: "Existing client, same family"}); err != nil {
		t.Fatal(err)
	}
	fb, err := repo.DraftBill(ctx, a, fm.ID, BillInput{})
	if err != nil || fb.Fees != 2500000 || len(fb.Lines) != 1 {
		t.Fatalf("flat bill %+v %v", fb, err)
	}
	if _, err := repo.DraftBill(ctx, a, fm.ID, BillInput{}); err == nil {
		t.Fatal("flat fee billed twice")
	}
	if _, err := repo.VoidBill(ctx, a, fb.ID); err != nil {
		t.Fatal(err)
	}
	if fb2, err := repo.DraftBill(ctx, a, fm.ID, BillInput{}); err != nil || fb2.Fees != 2500000 {
		t.Fatalf("flat fee after void %+v %v", fb2, err)
	}

	// Restricted matters are invisible to others, including in conflict results.
	rm, err := repo.CreateMatter(ctx, a, MatterInput{Title: "Confidential acquisition", ClientID: cl.ID, Access: "restricted",
		Parties: []Party{{Name: "Zenith Ventures", Role: "Counterparty"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Matter(ctx, u, rm.ID); !errors.Is(err, knowkit.ErrNotFound) {
		t.Fatalf("restricted matter visible: %v", err)
	}
	list, _ := repo.Matters(ctx, u, MatterFilter{})
	for _, x := range list {
		if x.ID == rm.ID {
			t.Fatal("restricted matter in the list")
		}
	}
	cr, err := repo.ConflictCheck(ctx, u, "Zenith")
	if err != nil || len(cr.Hits) != 1 || len(cr.Hits[0].Matters) != 1 || !cr.Hits[0].Matters[0].Restricted || cr.Hits[0].Matters[0].Title != "" {
		t.Fatalf("restricted conflict hit %+v %v", cr, err)
	}
	if _, err := repo.Matter(ctx, fx.Other, m.ID); !errors.Is(err, knowkit.ErrNotFound) {
		t.Fatal("matter visible across orgs")
	}
	if cr, _ := repo.ConflictCheck(ctx, fx.Other, "Kaveri"); len(cr.Hits) != 0 {
		t.Fatal("conflict check crossed orgs")
	}

	// Deadlines: reminders move with the date and go with the deadline.
	dl, err := repo.AddDeadline(ctx, u, m.ID, DeadlineInput{Title: "File written statement", Kind: "filing", Due: "2026-12-05"})
	if err != nil || dl.Urgency != "later" || !dl.Reminders {
		t.Fatalf("deadline %+v %v", dl, err)
	}
	before := len(events)
	for i := 0; i < 3; i++ {
		if _, err := repo.UpdateDeadline(ctx, u, dl.ID, DeadlineInput{Title: "File written statement", Kind: "filing", Due: "2026-11-25"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(events) != before || len(tasks) != 2 {
		t.Errorf("sync not idempotent: events=%v tasks=%v", events, tasks)
	}
	if d, _ := repo.SetDeadlineDone(ctx, u, dl.ID, true); d.Urgency != "done" || len(events) != before-1 {
		t.Errorf("done: %+v events=%v", d, events)
	}
	if err := repo.DeleteDeadline(ctx, u, dl.ID); err != nil || len(tasks) != 1 {
		t.Errorf("delete: %v tasks=%v", err, tasks)
	}

	var contractID string
	if err := fx.Pool.QueryRow(ctx, `INSERT INTO adrient.contracts_contracts (org_id, title, owner_id) VALUES ($1,'Retainer agreement',$2) RETURNING id::text`, fx.Org, a.User).Scan(&contractID); err != nil {
		t.Fatal(err)
	}
	if d, err := repo.AddDocument(ctx, u, m.ID, DocInput{Kind: "contract", RefID: contractID, Category: "Engagement"}); err != nil || d.Name != "Retainer agreement" {
		t.Fatalf("link contract: %+v %v", d, err)
	}
	if _, err := repo.AddDocument(ctx, u, m.ID, DocInput{Kind: "signature", RefID: contractID}); err == nil {
		t.Fatal("linked a missing signature request")
	}
	if _, err := repo.AddNote(ctx, u, m.ID, "call", "Client agreed to settle"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.HoursByUser(ctx, u, "", ""); !errors.Is(err, knowkit.ErrForbidden) {
		t.Fatal("hours report open to everyone")
	}
	if rows, err := repo.HoursByUser(ctx, a, "", ""); err != nil || len(rows) == 0 {
		t.Fatalf("hours %v %v", rows, err)
	}
	if ov, err := repo.Overview(ctx, a); err != nil || ov.OpenMatters < 2 {
		t.Fatalf("overview %+v %v", ov, err)
	}
}
