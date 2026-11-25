package timetracking

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opstest"
)

type fakeInv struct{ drafts []opshttp.InvoiceDraft }

func (f *fakeInv) Create(_ context.Context, _, _ string, d opshttp.InvoiceDraft) (opshttp.DocRef, error) {
	f.drafts = append(f.drafts, d)
	tot := 0.0
	for _, l := range d.Lines {
		tot += l.Qty * l.Price
	}
	return opshttp.DocRef{ID: "00000000-0000-0000-0000-00000000000" + string(rune('0'+len(f.drafts))), Kind: d.Kind, Number: "INV-1", Status: "sent", Total: tot, Balance: tot}, nil
}
func (f *fakeInv) Get(_ context.Context, _, id string) (opshttp.DocRef, error) {
	return opshttp.DocRef{ID: id, Number: "INV-1", Status: "paid"}, nil
}
func (f *fakeInv) ConvertEstimate(context.Context, string, string, string) (opshttp.DocRef, error) {
	return opshttp.DocRef{}, nil
}

func TestRepositoryFlow(t *testing.T) {
	pool, org, admin := opstest.DB(t)
	ctx := context.Background()
	inv := &fakeInv{}
	repo := NewRepository(pool, hr.NewRepository(pool), inv, nil)
	now := time.Date(2099, 1, 15, 10, 0, 0, 0, time.Local)
	repo.now = func() time.Time { return now }

	uAsha := opstest.User(t, pool, org, "asha")
	eAsha := opstest.Employee(t, pool, org, uAsha, "Asha", "")
	adm, _ := repo.CallerFor(ctx, org, admin, true)
	asha, _ := repo.CallerFor(ctx, org, uAsha, false)
	if !adm.Admin || asha.Admin {
		t.Fatal("roles")
	}

	rate, round := 100.0, 15
	cl, err := repo.SaveClient(ctx, adm, "", Client{Name: "Acme", Rate: &rate, Rounding: &round})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveClient(ctx, asha, "", Client{Name: "x"}); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("members cannot create clients: %v", err)
	}
	p, err := repo.SaveProject(ctx, adm, "", Project{Name: "Website", ClientID: cl.ID, Billable: true})
	if err != nil {
		t.Fatal(err)
	}
	internal, _ := repo.SaveProject(ctx, adm, "", Project{Name: "Admin", Billable: false})

	// Members log their own time; amounts and rates are hidden from them.
	r1, err := repo.SaveEntry(ctx, asha, "", Entry{ProjectID: p.ID, Date: "2099-01-14", Seconds: 20 * 60, Billable: true, Task: "Design"})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Entry.Amount != nil || r1.Entry.Billed != 1800 {
		t.Fatalf("entry = %+v", r1.Entry)
	}
	if _, err := repo.SaveEntry(ctx, asha, "", Entry{ProjectID: p.ID, Date: "2099-03-01", Seconds: 60}); !errors.Is(err, opshttp.ErrInvalid) {
		t.Fatalf("future days refused: %v", err)
	}
	if _, err := repo.SaveEntry(ctx, asha, "", Entry{ProjectID: p.ID, Date: "2099-01-14", Seconds: 90000}); !errors.Is(err, opshttp.ErrInvalid) {
		t.Fatalf("over 24h refused: %v", err)
	}
	if _, err := repo.SaveEntry(ctx, asha, "", Entry{ProjectID: internal.ID, Date: "2099-01-14", Seconds: 3600, Billable: true}); err != nil {
		t.Fatal(err)
	}
	list, _ := repo.ListEntries(ctx, adm, EntryFilter{From: "2099-01-01", To: "2099-01-31"})
	if len(list) != 2 || list[0].Amount == nil {
		t.Fatalf("admin sees all with amounts: %+v", list)
	}
	mine, _ := repo.ListEntries(ctx, asha, EntryFilter{From: "2099-01-01", To: "2099-01-31"})
	if len(mine) != 2 || mine[0].Amount != nil {
		t.Fatal("members see their own entries without amounts")
	}

	// Weekly grid cell keeps one entry per project and day.
	if _, err := repo.SetCell(ctx, asha, p.ID, "2099-01-13", 2*3600); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetCell(ctx, asha, p.ID, "2099-01-13", 3*3600); err != nil {
		t.Fatal(err)
	}
	day, _ := repo.ListEntries(ctx, asha, EntryFilter{From: "2099-01-13", To: "2099-01-13"})
	if len(day) != 1 || day[0].Seconds != 3*3600 {
		t.Fatalf("cell must keep one entry: %+v", day)
	}

	// Timer: 90 minutes.
	if _, _, err := repo.StartTimer(ctx, asha, p.ID, "Review", true); err != nil {
		t.Fatal(err)
	}
	repo.now = func() time.Time { return now.Add(90 * time.Minute) }
	saved, err := repo.StopTimer(ctx, asha)
	if err != nil || saved.Entry.Seconds < 5390 {
		t.Fatalf("timer saved %+v %v", saved, err)
	}
	if _, err := repo.StopTimer(ctx, asha); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("no timer running: %v", err)
	}

	// Invoice: only the Website project bills (the internal one is non-billable).
	res, err := repo.CreateInvoice(ctx, adm, cl.ID, "2099-01-01", "2099-01-31", nil)
	if err != nil || res.Entries != 3 {
		t.Fatalf("invoice %+v %v", res, err)
	}
	if len(inv.drafts) != 1 || len(inv.drafts[0].Lines) != 1 || inv.drafts[0].Lines[0].Price != 100 {
		t.Fatalf("draft = %+v", inv.drafts)
	}
	if _, err := repo.SaveEntry(ctx, asha, r1.Entry.ID, Entry{ProjectID: p.ID, Date: "2099-01-14", Seconds: 60}); !errors.Is(err, opshttp.ErrConflict) {
		t.Fatalf("invoiced entry is locked: %v", err)
	}
	if _, err := repo.CreateInvoice(ctx, adm, cl.ID, "", "", nil); !errors.Is(err, opshttp.ErrInvalid) {
		t.Fatalf("nothing left to bill: %v", err)
	}
	invs, _ := repo.Invoices(ctx, adm)
	if len(invs) != 1 || invs[0].Status != "paid" || invs[0].Entries != 3 {
		t.Fatalf("invoices = %+v", invs)
	}

	// Payroll lines need an HR record and a pay rate.
	s, _, _ := repo.GetSettings(ctx, org)
	s.PayrollRates = map[string]float64{eAsha: 200}
	if _, err := repo.SaveSettings(ctx, adm, s); err != nil {
		t.Fatal(err)
	}
	hrAdm := adm
	hrAdm.HRAdmin = true
	lines, skipped, err := repo.PayrollLines(ctx, hrAdm, "2099-01")
	if err != nil || len(lines) != 1 || lines[0].EmployeeID != eAsha || len(skipped) != 0 || lines[0].Hours < 7.4 {
		t.Fatalf("payroll lines %+v %v %v", lines, skipped, err)
	}
	if _, _, err := repo.PayrollLines(ctx, asha, "2099-01"); !errors.Is(err, opshttp.ErrForbidden) {
		t.Fatalf("only HR admins: %v", err)
	}
}
