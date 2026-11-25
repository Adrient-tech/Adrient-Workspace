package accounting

import (
	"context"
	"errors"
	"testing"
)

func TestDepScheduleSLMAndWDV(t *testing.T) {
	s := DepSchedule(100000, 10000, 36, "slm", 0, "2026-04-15", "")
	if len(s) != 36 || s[0].Period != "2026-04" || s[0].Dep != 2500 || s[35].Closing != 10000 {
		t.Fatalf("slm: len=%d first=%+v last=%+v", len(s), s[0], s[len(s)-1])
	}
	// rounding remainder lands on the last month; total always cost - salvage
	r := DepSchedule(1000, 0, 7, "slm", 0, "2026-01-01", "")
	var sum float64
	for _, x := range r {
		sum += x.Dep
	}
	if rupees(paise(sum)) != 1000 || r[6].Closing != 0 {
		t.Fatalf("remainder: sum=%v last=%+v", sum, r[6])
	}
	w := DepSchedule(100000, 5000, 60, "wdv", 40, "2026-04-01", "")
	if w[0].Dep != 3333.33 || w[len(w)-1].Closing != 5000 {
		t.Fatalf("wdv: first %+v last %+v", w[0], w[len(w)-1])
	}
	for i := 1; i < len(w); i++ {
		if w[i].Dep > w[i-1].Dep+0.01 && i < len(w)-1 {
			t.Fatalf("wdv depreciation must decline: %v then %v", w[i-1].Dep, w[i].Dep)
		}
	}
	d := DepSchedule(120000, 0, 12, "slm", 0, "2026-01-01", "2026-04-20")
	if len(d) != 4 {
		t.Fatalf("disposal stops depreciation after the disposal month: %d", len(d))
	}
	if DepSchedule(100, 100, 12, "slm", 0, "2026-01-01", "") != nil {
		t.Fatal("salvage >= cost must yield nothing")
	}
}

func TestAssetDepreciationPostsOnceAndDisposes(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	bank := idOf(t, repo, org, "bank")
	a, err := repo.CreateAsset(ctx, org, user, Asset{Name: "Laptop", Tag: "A-1", PurchaseDate: "2026-04-10", Cost: 120000, Salvage: 0, LifeMonths: 12, Method: "slm", PaidFromID: bank, Class: "HQ"})
	if err != nil || a.BookValue != 120000 {
		t.Fatalf("create: %v %+v", err, a)
	}
	if _, err := repo.CreateAsset(ctx, org, user, Asset{Name: "Bad", PurchaseDate: "2026-04-10", Cost: 100, Salvage: 100, LifeMonths: 12}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("salvage: %v", err)
	}
	res, err := repo.RunDepreciation(ctx, org, user, "2026-06")
	if err != nil || res.Posted != 3 {
		t.Fatalf("run: %v %+v", err, res)
	}
	for i := 0; i < 2; i++ {
		if res, _ = repo.RunDepreciation(ctx, org, user, "2026-06"); res.Posted != 0 {
			t.Fatalf("depreciation must be idempotent: %+v", res)
		}
	}
	if balanceOf(t, repo, org, "accum_depreciation") != -30000 || balanceOf(t, repo, org, "depreciation_expense") != 30000 {
		t.Fatalf("accum %v exp %v", balanceOf(t, repo, org, "accum_depreciation"), balanceOf(t, repo, org, "depreciation_expense"))
	}
	pl := BuildPnLByClass(mustAccts(t, repo, org), mustPostings(t, repo, org), "2026-04-01", "2026-06-30")
	if pl.Totals["HQ"].Expenses != 0 && pl.Totals["HQ"].Expenses != 30000 {
		t.Fatalf("class expense %+v", pl.Totals)
	}
	sched, _ := repo.AssetSchedule(ctx, org, a.ID)
	if !sched[0].Posted || !sched[2].Posted || sched[3].Posted {
		t.Fatalf("posted flags wrong: %+v", sched[:4])
	}
	// a locked period is counted, not an error, and retried after reopening
	if _, err := repo.SaveSettings(ctx, org, user, Settings{FYStartMonth: 4, Currency: "INR", Intrastate: true, ClosingDate: "2026-07-31"}); err != nil {
		t.Fatal(err)
	}
	if res, _ = repo.RunDepreciation(ctx, org, user, "2026-08"); res.Posted != 1 || res.Locked != 1 {
		t.Fatalf("locked: %+v", res)
	}
	if _, err := repo.SaveSettings(ctx, org, user, Settings{FYStartMonth: 4, Currency: "INR", Intrastate: true}); err != nil {
		t.Fatal(err)
	}
	requireBalanced(t, repo, org)
	// dispose: sold for 70,000 on 2026-09-05 => book value then 120000-6*10000=60000... gain 10000
	d, err := repo.DisposeAsset(ctx, org, user, a.ID, "2026-09-05", 70000, bank)
	if err != nil || d.Status != "disposed" {
		t.Fatalf("dispose: %v %+v", err, d)
	}
	if balanceOf(t, repo, org, "fixed_assets") != 0 || balanceOf(t, repo, org, "accum_depreciation") != 0 {
		t.Fatalf("disposal must clear cost and accumulated: %v %v", balanceOf(t, repo, org, "fixed_assets"), balanceOf(t, repo, org, "accum_depreciation"))
	}
	if g := balanceOf(t, repo, org, "asset_disposal"); g != 10000 {
		t.Fatalf("gain on disposal want 10000 got %v", g)
	}
	if _, err := repo.DisposeAsset(ctx, org, user, a.ID, "2026-09-06", 0, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("double dispose: %v", err)
	}
	requireBalanced(t, repo, org)
}

func mustAccts(t *testing.T, repo *Repository, org string) []Account {
	a, err := repo.ListAccounts(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func mustPostings(t *testing.T, repo *Repository, org string) []Posting {
	p, err := repo.LoadPostings(context.Background(), org, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
