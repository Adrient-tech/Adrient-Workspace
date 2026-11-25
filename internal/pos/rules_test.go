package pos

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
)

func TestValidatePIN(t *testing.T) {
	for _, ok := range []string{"4821", "90317", "12487965"[:6]} {
		if err := ValidatePIN(ok); err != nil {
			t.Errorf("%s should be allowed: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "123", "123456789", "1111", "1234", "4321", "98765", "12a456", "0000", "7777777"} {
		if ValidatePIN(bad) == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestPinLockout(t *testing.T) {
	now := time.Now()
	failed := 0
	var lock *time.Time
	for i := 1; i <= PinLockAfter; i++ {
		failed, lock = NextAttemptState(failed, now)
		if i < PinLockAfter && lock != nil {
			t.Fatalf("locked too early at try %d", i)
		}
	}
	if lock == nil || failed != 0 || !PinLocked(lock, now.Add(time.Minute)) || PinLocked(lock, now.Add(PinLockFor+time.Second)) {
		t.Fatalf("lock state: %v %v", failed, lock)
	}
}

func TestTiersAndStamps(t *testing.T) {
	tiers := []Tier{{"Gold", 50000, 2}, {"Silver", 10000, 1.5}, {"Bronze", 0, 1}}
	if tr, ok := TierFor(tiers, 9999.99); !ok || tr.Name != "Bronze" {
		t.Fatalf("%v", tr)
	}
	if tr, _ := TierFor(tiers, 10000); tr.Name != "Silver" {
		t.Fatalf("boundary: %v", tr)
	}
	if tr, _ := TierFor(tiers, 1e6); tr.Name != "Gold" {
		t.Fatalf("%v", tr)
	}
	if _, ok := TierFor(nil, 5); ok {
		t.Fatal("no tiers")
	}
	// 1 point per 10 currency units: 1000 -> 100, Gold doubles it.
	if p := EarnPoints(1000, 0.1, Tier{Multiplier: 2}, true); p != 200 {
		t.Fatalf("points %v", p)
	}
	if p := EarnPoints(1000, 0.1, Tier{}, false); p != 100 {
		t.Fatalf("points %v", p)
	}
	rule := StampRule{Enabled: true, MinSale: 100, Goal: 3, RewardCredit: 50}
	st, rw := 0, 0
	for i, want := range []struct{ s, r int }{{1, 0}, {2, 0}, {0, 1}, {1, 0}} {
		var got int
		st, got = AddStamp(rule, st, 150)
		rw += got
		if st != want.s || got != want.r {
			t.Fatalf("sale %d: stamps %d rewards %d", i, st, got)
		}
	}
	if s, r := AddStamp(rule, 1, 99.99); s != 1 || r != 0 {
		t.Fatal("small sale must not stamp")
	}
	if s, _ := AddStamp(StampRule{}, 1, 999); s != 1 {
		t.Fatal("disabled")
	}
}

func TestGiftCodes(t *testing.T) {
	c := NewGiftCode()
	if len(c) != 14 || strings.ContainsAny(c, "01OI") || c != strings.ToUpper(c) {
		t.Fatalf("code %q", c)
	}
	if NewGiftCode() == c {
		t.Fatal("codes must differ")
	}
	if code, ok := GiftRef(" Gift:abcd-2345 "); !ok || code != "ABCD-2345" {
		t.Fatalf("%q %v", code, ok)
	}
	if _, ok := GiftRef("card 1234"); ok {
		t.Fatal("not a gift ref")
	}
}

func TestSaleDiscountPercent(t *testing.T) {
	lines := []CalcLine{{Qty: 2, UnitPrice: 100}, {Qty: 1, UnitPrice: 100, DiscountKind: "percent", DiscountValue: 10}}
	// gross 300, line discount 10 -> 3.33%, order 10% of 290 = 29 -> total 39/300 = 13%
	if got := SaleDiscountPercent(lines, "percent", 10); got != 13 {
		t.Fatalf("%v", got)
	}
	if SaleDiscountPercent(nil, "", 0) != 0 {
		t.Fatal("empty")
	}
}

type posFixture struct {
	repo    *Repository
	org     string
	user    string
	reg     string
	shift   string
	product string
	inv     *inventory.Repository
}

func newPosFixture(t *testing.T) posFixture {
	pool, org, user := setupDB(t)
	ctx := context.Background()
	inv := inventory.NewRepository(pool)
	repo := NewRepository(pool, inv)
	prod, err := inv.CreateProduct(ctx, org, user, inventory.ProductFields{Name: "Widget", SKU: "W1", Price: 100, OpeningQty: 100})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := repo.SaveRegister(ctx, org, "", "Front", "")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := repo.OpenShift(ctx, org, user, reg.Id, 500)
	if err != nil {
		t.Fatal(err)
	}
	return posFixture{repo, org, user, reg.Id, sh.Id, prod.ID, inv}
}

func (f posFixture) sale(qty float64, pay ...*adrientv1.PosPaymentInput) *adrientv1.PosSaleInput {
	if len(pay) == 0 {
		pay = []*adrientv1.PosPaymentInput{{Method: "cash", Amount: 1000}}
	}
	return &adrientv1.PosSaleInput{RegisterId: f.reg, Lines: []*adrientv1.PosLineInput{{ProductId: f.product, Qty: qty, UnitPrice: 100}}, Payments: pay}
}

func TestOfflineSyncIdempotent(t *testing.T) {
	f := newPosFixture(t)
	ctx := context.Background()
	key := "offline-0001-abcdef"
	r1, err := f.repo.CompleteIdempotent(ctx, f.org, f.user, key, f.sale(2), "")
	if err != nil || r1.Status != "done" || r1.Replay || r1.SaleID == "" {
		t.Fatalf("first: %v %+v", err, r1)
	}
	// The phone retries after losing the response: same sale, no second row, stock moved once.
	r2, err := f.repo.CompleteIdempotent(ctx, f.org, f.user, key, f.sale(2), "")
	if err != nil || !r2.Replay || r2.SaleID != r1.SaleID {
		t.Fatalf("replay: %v %+v", err, r2)
	}
	if got := stockOf(t, f.inv, f.org, f.product); got != 98 {
		t.Fatalf("stock %v, want 98 (sold once)", got)
	}
	// Ten parallel retries of a new key create exactly one sale.
	var wg sync.WaitGroup
	ids := make(chan string, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for try := 0; try < 20; try++ {
				r, err := f.repo.CompleteIdempotent(ctx, f.org, f.user, "parallel-key-0001", f.sale(1), "")
				if errors.Is(err, ErrSyncBusy) {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				if err == nil {
					ids <- r.SaleID
				}
				return
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 {
		t.Fatalf("parallel retries made %d sales", len(seen))
	}
	if got := stockOf(t, f.inv, f.org, f.product); got != 97 {
		t.Fatalf("stock %v, want 97", got)
	}
	// A permanently invalid sale is remembered as failed and not retried into a sale.
	bad := f.sale(1, &adrientv1.PosPaymentInput{Method: "cash", Amount: 1})
	rb, err := f.repo.CompleteIdempotent(ctx, f.org, f.user, "bad-sale-key-0001", bad, "")
	if err != nil || rb.Status != "failed" || rb.Error == "" {
		t.Fatalf("bad: %v %+v", err, rb)
	}
	rb2, _ := f.repo.CompleteIdempotent(ctx, f.org, f.user, "bad-sale-key-0001", f.sale(1), "")
	if rb2.Status != "failed" || !rb2.Replay {
		t.Fatalf("a failed key stays failed: %+v", rb2)
	}
	if _, err := f.repo.CompleteIdempotent(ctx, f.org, f.user, "short", f.sale(1), ""); err == nil {
		t.Fatal("short key refused")
	}
}

func TestPinVoidAndApproval(t *testing.T) {
	f := newPosFixture(t)
	ctx := context.Background()
	var mgr string
	if err := f.repo.pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test','subject-mgr','mgr@adrient.localtest.me','Mia Manager') RETURNING id::text`, f.org).Scan(&mgr); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SetPin(ctx, f.org, mgr, "1234", PinInfo{}); err == nil {
		t.Fatal("weak PIN accepted")
	}
	if err := f.repo.SetPin(ctx, f.org, mgr, "4829", PinInfo{CanVoid: true, CanRefund: true, CanDiscount: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SetPin(ctx, f.org, f.user, "4829", PinInfo{CanVoid: true}); err == nil {
		t.Fatal("a PIN another manager uses must be refused")
	}
	a, err := f.repo.VerifyPin(ctx, f.org, f.user, "4829", "void")
	if err != nil || a.UserID != mgr || a.Name != "Mia Manager" {
		t.Fatalf("verify: %v %+v", err, a)
	}
	// Void needs a reason and the PIN.
	r, err := f.repo.CompleteIdempotent(ctx, f.org, f.user, "void-sale-key-001", f.sale(3), "")
	if err != nil || r.Status != "done" {
		t.Fatal(err)
	}
	if _, err := f.repo.VoidSale(ctx, f.org, f.user, r.SaleID, "", "", "4829"); err == nil {
		t.Fatal("void without reason")
	}
	if _, err := f.repo.VoidSale(ctx, f.org, f.user, r.SaleID, "", "customer changed mind", "0000"); !errors.Is(err, ErrBadPin) {
		t.Fatalf("wrong pin: %v", err)
	}
	if got := stockOf(t, f.inv, f.org, f.product); got != 97 {
		t.Fatalf("stock before void %v", got)
	}
	v, err := f.repo.VoidSale(ctx, f.org, f.user, r.SaleID, "", "customer changed mind", "4829")
	if err != nil || v.GetStatus() != "refunded" {
		t.Fatalf("void: %v %v", err, v.GetStatus())
	}
	if got := stockOf(t, f.inv, f.org, f.product); got != 100 {
		t.Fatalf("void must restock: %v", got)
	}
	if _, err := f.repo.VoidSale(ctx, f.org, f.user, r.SaleID, "", "twice", "4829"); err == nil {
		t.Fatal("cannot void twice")
	}
	// Discount approval: over 20% needs a PIN.
	if _, err := f.repo.SaveExt(ctx, f.org, ExtSettings{DiscountPinOverPct: 20, RefundNeedsPin: true, VoidNeedsPin: true}); err != nil {
		t.Fatal(err)
	}
	big := f.sale(1)
	big.DiscountKind, big.DiscountValue = "percent", 30
	rd, _ := f.repo.CompleteIdempotent(ctx, f.org, f.user, "disc-sale-key-001", big, "")
	if rd.Status != "failed" || !strings.Contains(rd.Error, "needs a manager PIN") {
		t.Fatalf("discount without approval: %+v", rd)
	}
	big2 := f.sale(1)
	big2.DiscountKind, big2.DiscountValue = "percent", 30
	rd, _ = f.repo.CompleteIdempotent(ctx, f.org, f.user, "disc-sale-key-002", big2, "4829")
	if rd.Status != "done" {
		t.Fatalf("approved discount: %+v", rd)
	}
	// Five wrong PINs lock the person typing, even for the right PIN.
	for i := 0; i < PinLockAfter; i++ {
		f.repo.VerifyPin(ctx, f.org, f.user, "9999", "void")
	}
	if _, err := f.repo.VerifyPin(ctx, f.org, f.user, "4829", "void"); !errors.Is(err, ErrPinLocked) {
		t.Fatalf("should be locked: %v", err)
	}
}

func TestGiftCardRedeemAtomic(t *testing.T) {
	f := newPosFixture(t)
	ctx := context.Background()
	card, err := f.repo.IssueGiftCard(ctx, f.org, f.user, 150, "Sam", "", "")
	if err != nil || card.Balance != 150 {
		t.Fatalf("issue: %v %+v", err, card)
	}
	gift := func(amt float64) *adrientv1.PosPaymentInput {
		return &adrientv1.PosPaymentInput{Method: "other", Amount: amt, Ref: "gift:" + strings.ToLower(card.Code)}
	}
	// 100 sale paid by the card.
	r, err := f.repo.CompleteIdempotent(ctx, f.org, f.user, "gift-sale-key-001", f.sale(1, gift(100)), "")
	if err != nil || r.Status != "done" {
		t.Fatalf("%v %+v", err, r)
	}
	g, _ := f.repo.GetGiftCard(ctx, f.org, card.Code)
	if g.Balance != 50 {
		t.Fatalf("balance %v", g.Balance)
	}
	// A 100 sale against the 50 left fails and the card is untouched; stock too.
	r, _ = f.repo.CompleteIdempotent(ctx, f.org, f.user, "gift-sale-key-002", f.sale(1, gift(100)), "")
	if r.Status != "failed" || !strings.Contains(r.Error, "only 50.00 left") {
		t.Fatalf("overspend: %+v", r)
	}
	g, _ = f.repo.GetGiftCard(ctx, f.org, card.Code)
	if g.Balance != 50 || stockOf(t, f.inv, f.org, f.product) != 99 {
		t.Fatalf("failed sale changed state: %v %v", g.Balance, stockOf(t, f.inv, f.org, f.product))
	}
	// Part gift card, part cash.
	r, _ = f.repo.CompleteIdempotent(ctx, f.org, f.user, "gift-sale-key-003", f.sale(1, gift(50), &adrientv1.PosPaymentInput{Method: "cash", Amount: 50}), "")
	if r.Status != "done" {
		t.Fatalf("split: %+v", r)
	}
	g, _ = f.repo.GetGiftCard(ctx, f.org, card.Code)
	if g.Balance != 0 {
		t.Fatalf("balance %v", g.Balance)
	}
	if _, err := f.repo.SetGiftCardStatus(ctx, f.org, card.Code, "disabled"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.TopUpGiftCard(ctx, f.org, card.Code, 10); err == nil {
		t.Fatal("a disabled card cannot be topped up")
	}
}

func TestTierPointsStampsAndZReport(t *testing.T) {
	f := newPosFixture(t)
	ctx := context.Background()
	cust := "11111111-1111-1111-1111-111111111111"
	if _, err := f.repo.SaveSettings(ctx, f.org, &adrientv1.PosSettings{LoyaltyEarnPerUnit: 0.1, LoyaltyPointValue: 0.1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SaveExt(ctx, f.org, ExtSettings{RefundNeedsPin: true, VoidNeedsPin: true,
		Tiers:  []Tier{{Name: "Bronze", MinSpend: 0, Multiplier: 1}, {Name: "Silver", MinSpend: 500, Multiplier: 2}},
		Stamps: StampRule{Enabled: true, MinSale: 100, Goal: 3, RewardCredit: 75}}); err != nil {
		t.Fatal(err)
	}
	mk := func(key string, qty float64) *adrientv1.PosSale {
		in := f.sale(qty)
		in.CustomerId, in.CustomerName = cust, "Sam"
		r, err := f.repo.CompleteIdempotent(ctx, f.org, f.user, key, in, "")
		if err != nil || r.Status != "done" {
			t.Fatalf("%s: %v %+v", key, err, r)
		}
		return r.Sale
	}
	s1 := mk("tier-sale-key-0001", 3) // 300: Bronze, 30 points
	if s1.GetLoyaltyEarned() != 30 {
		t.Fatalf("bronze points %v", s1.GetLoyaltyEarned())
	}
	mk("tier-sale-key-0002", 3) // 300 more: lifetime 600 now Silver
	s3 := mk("tier-sale-key-0003", 3)
	if s3.GetLoyaltyEarned() != 60 {
		t.Fatalf("silver doubles points: %v", s3.GetLoyaltyEarned())
	}
	li, err := f.repo.Loyalty(ctx, f.org, cust)
	if err != nil || li.Tier != "Silver" || li.Stamps != 0 || li.Rewards != 1 || li.Spend != 900 {
		t.Fatalf("loyalty: %v %+v", err, li)
	}
	bal, _ := f.repo.GetBalance(ctx, f.org, cust)
	if bal.GetStoreCredit() != 75 {
		t.Fatalf("stamp reward is paid as store credit: %v", bal.GetStoreCredit())
	}
	// Z report of the open shift: 3 sales of 300 in cash, 500 opening float.
	z, err := f.repo.ZReport(ctx, f.org, f.shift)
	if err != nil || z.Sales != 3 || z.Gross != 900 || z.CashSales != 900 || z.Expected != 1400 || z.Closed {
		t.Fatalf("z: %v %+v", err, z)
	}
	if len(z.ByMethod) != 1 || z.ByMethod[0].Method != "cash" || z.ByMethod[0].Count != 3 {
		t.Fatalf("by method %+v", z.ByMethod)
	}
}
