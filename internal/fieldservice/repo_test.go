package fieldservice

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opstest"
)

type fakeContacts struct{ list map[string]opshttp.ContactRef }

func (f *fakeContacts) Search(_ context.Context, _, q string, _ int) ([]opshttp.ContactRef, error) {
	var out []opshttp.ContactRef
	for _, c := range f.list {
		out = append(out, c)
	}
	return out, nil
}
func (f *fakeContacts) Get(_ context.Context, _, id string) (opshttp.ContactRef, error) {
	c, ok := f.list[id]
	if !ok {
		return c, opshttp.ErrNotFound
	}
	return c, nil
}
func (f *fakeContacts) Create(_ context.Context, _, _ string, c opshttp.ContactRef) (opshttp.ContactRef, error) {
	c.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", len(f.list)+1)
	f.list[c.ID] = c
	return c, nil
}

type fakeInv struct{ docs []opshttp.InvoiceDraft }

func (f *fakeInv) Create(_ context.Context, _, _ string, d opshttp.InvoiceDraft) (opshttp.DocRef, error) {
	f.docs = append(f.docs, d)
	return opshttp.DocRef{ID: fmt.Sprintf("00000000-0000-0000-0000-%012d", 900+len(f.docs)), Kind: d.Kind, Number: fmt.Sprintf("D-%d", len(f.docs)), Status: "draft", Balance: 10}, nil
}
func (f *fakeInv) Get(_ context.Context, _, id string) (opshttp.DocRef, error) {
	return opshttp.DocRef{ID: id, Status: "sent", Balance: 10}, nil
}
func (f *fakeInv) ConvertEstimate(context.Context, string, string, string) (opshttp.DocRef, error) {
	return opshttp.DocRef{}, nil
}

type fakeCal struct {
	events  map[string]opshttp.CalEvent
	owners  map[string]string
	deleted int
}

func (f *fakeCal) Create(_ context.Context, _, owner string, e opshttp.CalEvent) (string, error) {
	id := fmt.Sprintf("ev%d", len(f.events)+f.deleted+1)
	f.events[id], f.owners[id] = e, owner
	return id, nil
}
func (f *fakeCal) Update(_ context.Context, _, id string, e opshttp.CalEvent) error {
	f.events[id] = e
	return nil
}
func (f *fakeCal) Delete(_ context.Context, _, id string) error {
	delete(f.events, id)
	f.deleted++
	return nil
}

func TestRepositoryFlow(t *testing.T) {
	pool, org, admin := opstest.DB(t)
	ctx := context.Background()
	contacts := &fakeContacts{list: map[string]opshttp.ContactRef{}}
	inv := &fakeInv{}
	cal := &fakeCal{events: map[string]opshttp.CalEvent{}, owners: map[string]string{}}
	repo := NewRepository(pool, contacts, inv, cal)
	c := Caller{OrgID: org, UserID: admin, Admin: true}
	crew := Caller{OrgID: org, UserID: opstest.User(t, pool, org, "crew")}
	member := opstest.User(t, pool, org, "tech")

	cust, err := repo.CreateCustomer(ctx, c, opshttp.ContactRef{Name: "Mrs Rao", Email: "rao@x.test"})
	if err != nil {
		t.Fatal(err)
	}
	prop, err := repo.SaveProperty(ctx, c, "", Property{ContactID: cust.ID, Name: "Home", Address: "12 Lake Road"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveService(ctx, crew, "", Service{Name: "Mowing", UnitPrice: 50}); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("catalogue is admin only: %v", err)
	}

	// Request -> quote (an estimate in Invoicing) -> job.
	req, err := repo.SaveRequest(ctx, crew, "", Request{ContactID: cust.ID, PropertyID: prop.ID, Title: "Lawn care", Preferred: "2099-02-02",
		Items: []Item{{Name: "Mowing", Qty: 1, UnitPrice: 50, UnitCost: 20, Taxable: true}, {Name: "Edging", Qty: 2, UnitPrice: 10}}})
	if err != nil || req.Status != ReqNew || req.ClientName != "Mrs Rao" {
		t.Fatalf("request %+v %v", req, err)
	}
	q, err := repo.CreateQuote(ctx, crew, req.ID)
	if err != nil || q.Status != ReqQuoted || q.QuoteNumber == "" || len(inv.docs) != 1 || inv.docs[0].Kind != "estimate" || inv.docs[0].ContactID != cust.ID {
		t.Fatalf("quote %+v %v %+v", q, err, inv.docs)
	}
	if _, err := repo.CreateQuote(ctx, crew, req.ID); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("second quote refused: %v", err)
	}
	job, err := repo.CreateJobFromRequest(ctx, crew, req.ID, Schedule{Start: "2099-02-02", Repeat: "weekly", EndType: "count", EndCount: 3, StartTime: "09:00", EndTime: "11:00"}, []string{member})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Visits) != 3 || job.QuoteID != q.QuoteID || job.DisplayStatus != "active" || job.Totals.Subtotal != 70 {
		t.Fatalf("job %+v", job)
	}
	if len(cal.events) != 3 || cal.owners["ev1"] != member {
		t.Fatalf("each visit lands in the assignee's Calendar: %d %v", len(cal.events), cal.owners)
	}
	if _, err := repo.CreateJobFromRequest(ctx, crew, req.ID, Schedule{}, nil); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("a request converts once: %v", err)
	}

	// Reschedule one visit: the Calendar item moves, nothing is duplicated.
	v0 := job.Visits[0]
	nd := "2099-02-03"
	if _, err := repo.UpdateVisit(ctx, crew, v0.ID, VisitUpdate{Date: nd}); err != nil {
		t.Fatal(err)
	}
	if len(cal.events) != 3 {
		t.Fatalf("calendar items after move: %d", len(cal.events))
	}

	// Complete a visit; invoice the completed visits in one invoice with qty x visits.
	if _, _, err := repo.CompleteVisit(ctx, crew, v0.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CompleteVisit(ctx, crew, job.Visits[1].ID); err != nil {
		t.Fatal(err)
	}
	doc, n, err := repo.InvoiceJob(ctx, crew, job.ID)
	if err != nil || n != 2 || doc.Kind != "invoice" {
		t.Fatalf("invoice %+v %d %v", doc, n, err)
	}
	last := inv.docs[len(inv.docs)-1]
	if last.Lines[0].Qty != 2 || last.Lines[1].Qty != 4 {
		t.Fatalf("lines must be multiplied by the visits: %+v", last.Lines)
	}
	if _, _, err := repo.InvoiceJob(ctx, crew, job.ID); !errors.Is(err, opshttp.ErrInvalid) {
		t.Fatalf("nothing left to invoice: %v", err)
	}
	// Editing the schedule keeps completed visits and rebuilds the open ones.
	j2, err := repo.SaveJob(ctx, crew, job.ID, Job{ContactID: cust.ID, PropertyID: prop.ID, Title: "Lawn care", Schedule: Schedule{Start: "2099-02-02", Repeat: "weekly", EndType: "count", EndCount: 4}, Items: job.Items, Assignees: []string{member}})
	if err != nil {
		t.Fatal(err)
	}
	done := 0
	for _, v := range j2.Visits {
		if v.Status == "completed" {
			done++
		}
	}
	if len(j2.Visits) != 5 || done != 2 { // 3 open on the schedule + the 2 completed (one of them was moved off the schedule)
		t.Fatalf("after edit: %d visits, %d completed", len(j2.Visits), done)
	}
	// Cancelling removes the open visits from Calendar.
	before := len(cal.events)
	if _, err := repo.CancelJob(ctx, crew, job.ID); err != nil {
		t.Fatal(err)
	}
	if len(cal.events) >= before {
		t.Fatalf("cancelled job must leave Calendar: %d -> %d", before, len(cal.events))
	}
	d, err := repo.Dashboard(ctx, c)
	if err != nil || d.NewRequests != 0 {
		t.Fatalf("dashboard %+v %v", d, err)
	}
}
