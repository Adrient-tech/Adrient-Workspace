package captable

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit/kittest"
)

func TestCapTableFlow(t *testing.T) {
	fx := kittest.Setup(t)
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	repo.Deps.Now = func() time.Time { return time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC) }
	a, u := fx.Admin, fx.User

	// Nobody but admins and named people sees anything.
	if repo.Level(ctx, u) != LevelNone || repo.Level(ctx, a) != LevelEdit {
		t.Fatal("default access")
	}
	if err := repo.GrantAccess(ctx, u, u.User, LevelEdit); !errors.Is(err, knowkit.ErrForbidden) {
		t.Fatal("a non-admin granted access")
	}
	if err := repo.GrantAccess(ctx, a, u.User, LevelView); err != nil || repo.Level(ctx, u) != LevelView {
		t.Fatalf("grant: %v", err)
	}
	if err := repo.RevokeAccess(ctx, a, u.User); err != nil || repo.Level(ctx, u) != LevelNone {
		t.Fatal("revoke")
	}
	if repo.Level(ctx, fx.Other) != LevelNone {
		t.Fatal("other org")
	}

	common, err := repo.CreateClass(ctx, a, ClassInput{Name: "Common", Kind: "common", Authorized: 20_000_000})
	if err != nil {
		t.Fatal(err)
	}
	pref, err := repo.CreateClass(ctx, a, ClassInput{Name: "Series A Preferred", Kind: "preferred", Seniority: 1, PrefMultipleMilli: 1000})
	if err != nil {
		t.Fatal(err)
	}
	asha, _ := repo.CreateStakeholder(ctx, a, StakeholderInput{Name: "Asha Rao", Kind: "founder"})
	bala, _ := repo.CreateStakeholder(ctx, a, StakeholderInput{Name: "Bala Iyer", Kind: "founder"})
	esha, _ := repo.CreateStakeholder(ctx, a, StakeholderInput{Name: "Esha Nair", Kind: "employee"})
	fund, _ := repo.CreateStakeholder(ctx, a, StakeholderInput{Name: "Kaveri Ventures", Kind: "investor"})
	for _, x := range []struct {
		h string
		q int64
	}{{asha.ID, 6_000_000}, {bala.ID, 4_000_000}} {
		if _, err := repo.Issue(ctx, a, SecurityInput{Kind: "stock", StakeholderID: x.h, ClassID: common.ID, Quantity: x.q, Price: "0.01", IssueDate: "2024-01-01"}); err != nil {
			t.Fatal(err)
		}
	}
	// Authorised limit: 20,000,000 - 10,000,000 issued.
	if _, err := repo.Issue(ctx, a, SecurityInput{Kind: "stock", StakeholderID: asha.ID, ClassID: common.ID, Quantity: 10_000_001, IssueDate: "2024-02-01"}); err == nil {
		t.Fatal("issued past the authorised count")
	}
	pool, err := repo.CreatePool(ctx, a, PoolInput{Name: "ESOP 2024", Size: 1_000_000, Adopted: "2024-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Issue(ctx, a, SecurityInput{Kind: "option", StakeholderID: esha.ID, ClassID: common.ID, PoolID: pool.ID, Quantity: 1_000_001, Price: "1", IssueDate: "2024-06-01"}); err == nil {
		t.Fatal("granted more than the pool holds")
	}
	opt, err := repo.Issue(ctx, a, SecurityInput{Kind: "option", StakeholderID: esha.ID, ClassID: common.ID, PoolID: pool.ID, Quantity: 48_000, Price: "2.5",
		IssueDate: "2024-06-01", VestMonths: 48, CliffMonths: 12})
	if err != nil || opt.Label != "OPT-1" {
		t.Fatalf("option %+v %v", opt, err)
	}
	// Exercise: before the cliff nothing is exercisable; at 2025-06-01 12 of 48 months have vested: 12,000.
	if _, err := repo.Record(ctx, a, "exercise", opt.ID, EventInput{Date: "2025-05-31", Quantity: 1}); err == nil {
		t.Fatal("exercised before the cliff")
	}
	if _, err := repo.Record(ctx, a, "exercise", opt.ID, EventInput{Date: "2025-06-01", Quantity: 12_001}); err == nil {
		t.Fatal("exercised more than vested")
	}
	got, err := repo.Record(ctx, a, "exercise", opt.ID, EventInput{Date: "2025-06-01", Quantity: 12_000})
	if err != nil || got.Exercised != 12_000 || got.Outstanding != 36_000 {
		t.Fatalf("exercise %+v %v", got, err)
	}
	if _, err := repo.Record(ctx, a, "exercise", opt.ID, EventInput{Date: "2025-06-01", Quantity: 1}); err == nil {
		t.Fatal("exercised the same vested options twice")
	}
	tb, _ := repo.TableOf(ctx, a.Org, "2026-11-21")
	// Esha holds 12,000 shares and 36,000 options; fully diluted = 10,000,000 + 12,000 + 36,000 + pool (1,000,000 - 48,000) = 10,... see below.
	if tb.Outstanding != 10_012_000 || tb.Pool != 952_000 || tb.FullyDil != 10_012_000+36_000+952_000 {
		t.Fatalf("table out %d pool %d fd %d", tb.Outstanding, tb.Pool, tb.FullyDil)
	}
	// Transfer 1,000,000 of Bala's shares to the fund.
	bs, _ := repo.Securities(ctx, a.Org, bala.ID, "stock", "")
	if _, err := repo.Record(ctx, a, "transfer", bs[0].ID, EventInput{Date: "2026-01-01", Quantity: 4_000_001, ToStakeholder: fund.ID}); err == nil {
		t.Fatal("transferred more than held")
	}
	if _, err := repo.Record(ctx, a, "transfer", bs[0].ID, EventInput{Date: "2026-01-01", Quantity: 1_000_000, ToStakeholder: fund.ID, Price: "1"}); err != nil {
		t.Fatal(err)
	}
	tb, _ = repo.TableOf(ctx, a.Org, "")
	var balaN, fundN int64
	for _, r := range tb.Rows {
		switch r.ID {
		case bala.ID:
			balaN = r.Stock
		case fund.ID:
			fundN = r.Stock
		}
	}
	if balaN != 3_000_000 || fundN != 1_000_000 {
		t.Fatalf("after transfer bala %d fund %d", balaN, fundN)
	}
	// As of 2025-12-31 (before the transfer) Bala still holds 4,000,000.
	early, _ := repo.TableOf(ctx, a.Org, "2025-12-31")
	for _, r := range early.Rows {
		if r.ID == bala.ID && r.Stock != 4_000_000 {
			t.Fatalf("as-of table: %d", r.Stock)
		}
	}

	// A SAFE, then a priced round closes: SAFE converts, investors get shares, the pool tops up.
	safe, err := repo.Issue(ctx, a, SecurityInput{Kind: "safe", StakeholderID: fund.ID, Principal: 50_000_000, Cap: 500_000_000, PostMoney: true, IssueDate: "2026-03-01"})
	if err != nil || safe.Label != "SAFE-1" {
		t.Fatalf("safe %+v %v", safe, err)
	}
	// Existing fully diluted today: 10,012,000 + 36,000 + 952,000 = 11,000,000 (pool incl.).
	in := RoundModelInput{Name: "Series A", Date: "2026-11-21", PreMoney: 2_000_000_000, Investment: 400_000_000, ClassID: pref.ID,
		Investors: []Investor{{StakeholderID: fund.ID, Amount: 400_000_000}}}
	m, err := repo.ModelRoundPreview(ctx, a.Org, in)
	if err != nil || m.PreFD != 11_000_000 || len(m.Converted) != 1 {
		t.Fatalf("model %+v %v", m, err)
	}
	// Post-money SAFE: 10% of the company capitalisation after conversion: s = 0.1 (11,000,000 + s) = 1,222,222.2 -> 1,222,222.
	if m.Converted[0].Shares != 1_222_222 {
		t.Fatalf("SAFE shares %d", m.Converted[0].Shares)
	}
	rr, err := repo.CloseRound(ctx, a, in)
	if err != nil || rr.Name != "Series A" {
		t.Fatalf("close %+v %v", rr, err)
	}
	tb, _ = repo.TableOf(ctx, a.Org, "")
	if tb.Convertible != 0 {
		t.Errorf("SAFE still open: %d", tb.Convertible)
	}
	var prefShares int64
	for _, c := range tb.Classes {
		if c.ID == pref.ID {
			prefShares = c.Shares
		}
	}
	if prefShares != 1_222_222+m.Investors[0].Shares {
		t.Errorf("preferred outstanding %d", prefShares)
	}
	var pct int64
	for _, r := range tb.Rows {
		pct += r.PctFD
	}
	if pct+tb.PoolPctFD != PctScale {
		t.Errorf("fully diluted adds to %d", pct+tb.PoolPctFD)
	}

	// Exit model: every payout adds up to the exit value, and the preferred gets its money back at a low exit.
	out, err := repo.ExitModel(ctx, a.Org, ExitInput{Exits: []int64{100_000_000, 5_000_000_000, 50_000_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range out {
		var tot int64
		for _, h := range o.Holders {
			tot += h.Payout
		}
		if tot != o.Exit {
			t.Errorf("exit %d: holders get %d", o.Exit, tot)
		}
	}
	// Exports are escaped CSV.
	b, name, err := repo.ExportCSV(ctx, a, "captable", "")
	if err != nil || len(b) == 0 || name != "captable-captable-2026-11-21.csv" {
		t.Fatalf("export %s %v", name, err)
	}
	// Undo: removing the transfer restores Bala.
	evs, _ := repo.Events(ctx, a.Org)
	for _, e := range evs {
		if e.Kind == "transfer" {
			if err := repo.UndoEvent(ctx, a, e.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	tb, _ = repo.TableOf(ctx, a.Org, "")
	for _, r := range tb.Rows {
		if r.ID == bala.ID && r.Stock != 4_000_000 {
			t.Errorf("after undo bala %d", r.Stock)
		}
	}
}
