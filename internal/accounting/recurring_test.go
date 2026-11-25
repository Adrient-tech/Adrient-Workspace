package accounting

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestOccurrenceDateMonthEndNoDrift(t *testing.T) {
	start := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	want := []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31"}
	for i, w := range want {
		if got := OccurrenceDate(start, "monthly", 1, i).Format("2006-01-02"); got != w {
			t.Fatalf("occurrence %d: %s want %s", i, got, w)
		}
	}
	if got := OccurrenceDate(start, "quarterly", 1, 1).Format("2006-01-02"); got != "2026-04-30" {
		t.Fatalf("quarterly %s", got)
	}
	if got := OccurrenceDate(time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), "yearly", 1, 1).Format("2006-01-02"); got != "2025-02-28" {
		t.Fatalf("yearly leap %s", got)
	}
}

func TestRecurringRunnerIsIdempotentAndRetriesFailures(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	rent, bank := idOf(t, repo, org, "misc"), idOf(t, repo, org, "bank")
	tpl, _ := json.Marshal(JournalInput{Memo: "Monthly rent accrual", Lines: []Line{{AccountID: rent, Debit: 1000}, {AccountID: bank, Credit: 1000}}})
	rec, err := repo.SaveRecurring(ctx, org, user, "", Recurring{Kind: "journal", Name: "Rent", Frequency: "monthly", Every: 1, StartDate: "2026-01-31", Active: true, Template: tpl})
	if err != nil {
		t.Fatal(err)
	}
	// Close January so its occurrence fails, then runs as of 31 Mar.
	if _, err := repo.SaveSettings(ctx, org, user, Settings{FYStartMonth: 4, Currency: "INR", Intrastate: true, ClosingDate: "2026-02-01"}); err != nil {
		t.Fatal(err)
	}
	res, err := repo.RunRecurring(ctx, org, user, "2026-03-31")
	if err != nil || res.Created != 2 || res.Failed != 1 {
		t.Fatalf("first run: %v %+v", err, res)
	}
	// Running again retries only the failed occurrence, never duplicates the others.
	res, _ = repo.RunRecurring(ctx, org, user, "2026-03-31")
	if res.Created != 0 || res.Failed != 1 {
		t.Fatalf("second run: %+v", res)
	}
	js, _ := repo.ListJournals(ctx, org, JournalFilter{Source: SourceRecurring})
	if len(js) != 2 {
		t.Fatalf("expected 2 journals, got %d", len(js))
	}
	// Reopen the period: the failed occurrence now posts, once.
	if _, err := repo.SaveSettings(ctx, org, user, Settings{FYStartMonth: 4, Currency: "INR", Intrastate: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		res, _ = repo.RunRecurring(ctx, org, user, "2026-03-31")
		if (i == 0 && res.Created != 1) || (i > 0 && res.Created != 0) {
			t.Fatalf("run %d: %+v", i, res)
		}
	}
	js, _ = repo.ListJournals(ctx, org, JournalFilter{Source: SourceRecurring})
	if len(js) != 3 {
		t.Fatalf("expected 3 journals, got %d", len(js))
	}
	got, _ := repo.GetRecurring(ctx, org, rec.ID)
	if got.Runs != 3 || got.LastRun != "2026-03-31" || got.NextRun != "2026-04-30" {
		t.Fatalf("state: %+v", got)
	}
	// max runs and end date stop the schedule; inactive schedules never run.
	tplB, _ := json.Marshal(BillInput{VendorName: "ISP", Lines: []BillLineIn{{AccountID: rent, Amount: 500}}})
	b, _ := repo.SaveRecurring(ctx, org, user, "", Recurring{Kind: "bill", Name: "Internet", Frequency: "monthly", StartDate: "2026-04-01", MaxRuns: 2, Active: true, AutoPost: true, Template: tplB})
	res, _ = repo.RunRecurring(ctx, org, user, "2026-09-30")
	bills, _ := repo.ListBills(ctx, org, "bill", "", "")
	if len(bills) != 2 || res.Created != 8 {
		t.Fatalf("max runs: %d bills, %+v", len(bills), res)
	}
	if _, err := repo.SaveRecurring(ctx, org, user, b.ID, Recurring{Kind: "bill", Name: "Internet", Frequency: "monthly", StartDate: "2026-04-01", Active: false, Template: tplB}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveRecurring(ctx, org, user, "", Recurring{Kind: "journal", Name: "bad", Frequency: "monthly", StartDate: "2026-04-01", Template: json.RawMessage(`{"lines":[]}`)}); err == nil {
		t.Fatal("unbalanced template must be rejected")
	}
	requireBalanced(t, repo, org)
}
