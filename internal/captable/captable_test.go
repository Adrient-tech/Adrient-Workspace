package captable

import (
	"math/big"
	"testing"
)

func sum(xs []int64) (t int64) {
	for _, x := range xs {
		t += x
	}
	return
}

func TestPercentagesLargestRemainder(t *testing.T) {
	got := Percentages([]int64{1, 1, 1})
	want := []int64{333334, 333333, 333333} // 1/3 = 333333.33; the extra unit goes to the first of the equal remainders
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	// 10 : 2 of 12 = 83.3333% and 16.6667%: 833333.33 and 166666.67 -> the bigger remainder gets the spare unit.
	got = Percentages([]int64{10, 2})
	if got[0] != 833333 || got[1] != 166667 || sum(got) != PctScale {
		t.Fatalf("got %v", got)
	}
	if sum(Percentages([]int64{7, 11, 13, 17, 19, 23, 1})) != PctScale {
		t.Fatal("percentages must add up to exactly 100%")
	}
	if z := Percentages([]int64{0, 0}); z[0] != 0 || z[1] != 0 {
		t.Fatal("no shares, no percentages")
	}
}

func TestVesting(t *testing.T) {
	v := Vesting{Start: "2024-01-15", Months: 48, CliffMons: 12, Frequency: "monthly"}
	cases := []struct {
		asOf string
		want int64
	}{
		{"2024-01-14", 0},    // before the start
		{"2025-01-14", 0},    // 11 whole months: before the cliff
		{"2025-01-15", 1200}, // 12 months: 4800*12/48
		{"2025-07-20", 1800}, // 18 whole months
		{"2027-01-15", 3600}, // 36 months
		{"2028-01-15", 4800}, // 48 months: everything
		{"2030-01-01", 4800},
	}
	for _, c := range cases {
		if got := v.Vested(4800, c.asOf); got != c.want {
			t.Errorf("vested on %s = %d, want %d", c.asOf, got, c.want)
		}
	}
	// Quarterly steps round down to a whole quarter: 1000 shares, 36 months, month 5 -> 3 months -> floor(1000*3/36) = 83.
	q := Vesting{Start: "2024-01-01", Months: 36, Frequency: "quarterly"}
	if got := q.Vested(1000, "2024-06-01"); got != 83 {
		t.Errorf("quarterly: %d", got)
	}
	// Whole-share rounding never loses a share: the end vests the full quantity.
	odd := Vesting{Start: "2024-01-01", Months: 36}
	if odd.Vested(1000, "2024-06-01") != 138 || odd.Vested(1000, "2027-01-01") != 1000 { // floor(1000*5/36)=138
		t.Errorf("odd quantity: %d %d", odd.Vested(1000, "2024-06-01"), odd.Vested(1000, "2027-01-01"))
	}
	// Month-end starts do not drift: Jan 31 + 1 month is Feb 29 in a leap year.
	m := Vesting{Start: "2024-01-31", Months: 12}
	if m.Vested(1200, "2024-02-28") != 0 || m.Vested(1200, "2024-02-29") != 100 {
		t.Errorf("month end: %d %d", m.Vested(1200, "2024-02-28"), m.Vested(1200, "2024-02-29"))
	}
	if (Vesting{}).Vested(500, "2020-01-01") != 500 {
		t.Error("no schedule means fully vested")
	}
}

func TestNoteInterest(t *testing.T) {
	n := Convertible{Kind: "note", PrincipalC: 10_000_000, InterestBp: 800, IssueDate: "2024-01-01"}
	// 182 days from 2024-01-01 to 2024-07-01: 10,000,000 x 8% x 182/365 = 398,904.1 -> 398,904.
	if got := n.Value("2024-07-01"); got != 10_398_904 {
		t.Errorf("note value = %d", got)
	}
	if got := n.Value("2023-12-01"); got != 10_000_000 {
		t.Errorf("before issue: %d", got)
	}
	if (Convertible{Kind: "safe", PrincipalC: 5}).Value("2030-01-01") != 5 {
		t.Error("a SAFE earns no interest")
	}
}

func TestPricedRound(t *testing.T) {
	// 10,000,000 shares, $10M pre-money, $2M in: $1.00 a share, 2,000,000 new shares, 16.6667%.
	r, err := ModelRound(RoundInput{PreMoneyC: 1_000_000_000, InvestmentC: 200_000_000,
		Existing: []Holding{{ID: "f", Name: "Founders", Shares: 10_000_000}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.PricePerShareCents != "100.000000" || r.NewShares != 2_000_000 || r.PostFD != 12_000_000 || r.PostMoneyC != 1_200_000_000 {
		t.Fatalf("%+v", r)
	}
	if r.NewInvestorPct != 166667 || r.Holders[0].PctAfter != 833333 {
		t.Errorf("percentages: investor %d founders %d", r.NewInvestorPct, r.Holders[0].PctAfter)
	}
	var tot int64
	for _, h := range r.Holders {
		tot += h.PctAfter
	}
	if tot != PctScale {
		t.Errorf("the post-round column adds to %d", tot)
	}
}

func TestPostMoneySAFE(t *testing.T) {
	// $500k post-money SAFE with a $5M post-money cap is exactly 10% of the company capitalisation after it
	// converts: s = 0.1 (10,000,000 + s) -> s = 1,111,111.11 -> 1,111,111 shares at 45 cents each.
	safe := Convertible{ID: "s1", Name: "Seed SAFE", Kind: "safe", PrincipalC: 50_000_000, CapC: 500_000_000, PostMoney: true}
	r, err := ModelRound(RoundInput{PreMoneyC: 1_000_000_000, InvestmentC: 200_000_000,
		Existing: []Holding{{ID: "f", Name: "Founders", Shares: 10_000_000}}, Convertibles: []Convertible{safe}})
	if err != nil {
		t.Fatal(err)
	}
	c := r.Converted[0]
	if c.Shares != 1_111_111 || c.PriceCents != "45.000000" || !c.CapBinds {
		t.Fatalf("converted %+v", c)
	}
	// The round price counts the SAFE shares in the pre-money: 1e9 / 11,111,111 = 90.000001 cents.
	if r.PricePerShareCents != "90.000001" {
		t.Errorf("price %s", r.PricePerShareCents)
	}
	// New shares = floor(2e8 x 11,111,111 / 1e9) = floor(2,222,222.2) = 2,222,222. Post-round total 13,333,333.
	if r.NewShares != 2_222_222 || r.PostFD != 13_333_333 {
		t.Errorf("new %d post %d", r.NewShares, r.PostFD)
	}
}

func TestDiscountNote(t *testing.T) {
	// A $100k note with a 20% discount and no cap: price p satisfies p x (10,000,000 + 10,000,000/(0.8 p)) = 1e9
	// -> 1e7 p + 1.25e7 = 1e9 -> p = 98.75 cents; the note converts at 79 cents: 126,582 shares.
	n := Convertible{ID: "n1", Name: "Bridge note", Kind: "note", PrincipalC: 10_000_000, DiscountBp: 2000}
	r, err := ModelRound(RoundInput{PreMoneyC: 1_000_000_000, InvestmentC: 200_000_000,
		Existing: []Holding{{ID: "f", Name: "Founders", Shares: 10_000_000}}, Convertibles: []Convertible{n}})
	if err != nil {
		t.Fatal(err)
	}
	if c := r.Converted[0]; c.Shares != 126_582 || c.PriceCents != "79.000000" || c.CapBinds {
		t.Fatalf("converted %+v", c)
	}
	if r.NewShares != 2_025_316 { // floor(2e8 x 10,126,582 / 1e9) = floor(2,025,316.4)
		t.Errorf("new shares %d", r.NewShares)
	}
}

func TestPoolTopUp(t *testing.T) {
	// 10% post-round pool: pool = 0.12 x 10,000,000 / (1 - 0.12) = 1,363,636.4 -> 1,363,636 shares, before the round.
	r, err := ModelRound(RoundInput{PreMoneyC: 1_000_000_000, InvestmentC: 200_000_000, PoolTargetBp: 1000,
		Existing: []Holding{{ID: "f", Name: "Founders", Shares: 10_000_000}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.PoolTopUp != 1_363_636 || r.NewShares != 2_272_727 || r.PostFD != 13_636_363 {
		t.Fatalf("%+v", r)
	}
	var pool RoundHolder
	for _, h := range r.Holders {
		if h.Name == "Option pool (unallocated)" {
			pool = h
		}
	}
	if d := pool.PctAfter - 100_000; d < -1 || d > 1 {
		t.Errorf("pool is %d of 1,000,000, want about 10%%", pool.PctAfter)
	}
}

func TestRoundRefusesNonsense(t *testing.T) {
	ex := []Holding{{Name: "F", Shares: 100}}
	if _, err := ModelRound(RoundInput{PreMoneyC: 0, Existing: ex}); err == nil {
		t.Error("zero pre-money")
	}
	if _, err := ModelRound(RoundInput{PreMoneyC: 1000, Existing: nil}); err == nil {
		t.Error("no shares")
	}
	big := Convertible{Kind: "safe", PrincipalC: 99, CapC: 100, PostMoney: true}
	if _, err := ModelRound(RoundInput{PreMoneyC: 1000, Existing: ex, Convertibles: []Convertible{big}}); err == nil {
		t.Error("post-money SAFEs claiming 99% have no solution")
	}
}

func payout(r *WaterfallResult, id string) int64 {
	for _, p := range r.Payees {
		if p.ID == id {
			return p.PayoutC
		}
	}
	return -1
}

func twoClasses() []ClassTerms {
	return []ClassTerms{
		{ID: "c", Name: "Common", Shares: 6_000_000},
		{ID: "a", Name: "Series A", Preferred: true, Shares: 4_000_000, InvestedC: 400_000_000, Seniority: 1},
	}
}

func TestWaterfallNonParticipating(t *testing.T) {
	// 4,000,000 preferred shares bought for $4M (1x); 6,000,000 common.
	cases := []struct {
		exit       int64
		pref, comm int64
		converted  bool
	}{
		{1_000_000_000, 400_000_000, 600_000_000, false},  // $10M: as converted the preferred gets exactly $4M, so it keeps its preference
		{2_000_000_000, 800_000_000, 1_200_000_000, true}, // $20M: converts, 40% of the company
		{200_000_000, 200_000_000, 0, false},              // $2M: the preference takes everything
		{400_000_000, 400_000_000, 0, false},              // exactly the preference
	}
	for _, c := range cases {
		r, err := Waterfall(c.exit, nil, twoClasses(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if payout(r, "a") != c.pref || payout(r, "c") != c.comm || r.TotalC != c.exit {
			t.Errorf("exit %d: pref %d common %d total %d", c.exit, payout(r, "a"), payout(r, "c"), r.TotalC)
		}
		for _, p := range r.Payees {
			if p.ID == "a" && p.Converted != c.converted {
				t.Errorf("exit %d converted=%v", c.exit, p.Converted)
			}
		}
	}
}

func TestWaterfallParticipatingAndStack(t *testing.T) {
	cl := twoClasses()
	cl[1].Participating = true
	// $10M: preference $4M first; the $6M left is shared over 10,000,000 shares = 60 cents each:
	// preferred 4,000,000 x 60 = $2.4M more, common $3.6M.
	r, _ := Waterfall(1_000_000_000, nil, cl, nil)
	if payout(r, "a") != 640_000_000 || payout(r, "c") != 360_000_000 {
		t.Errorf("participating: %d / %d", payout(r, "a"), payout(r, "c"))
	}
	// Senior stack: Series B ($3M, senior) and Series A ($4M) with $5M to share: B gets $3M, A $2M, common nothing.
	stack := []ClassTerms{
		{ID: "c", Name: "Common", Shares: 6_000_000},
		{ID: "a", Name: "A", Preferred: true, Shares: 4_000_000, InvestedC: 400_000_000, Seniority: 1},
		{ID: "b", Name: "B", Preferred: true, Shares: 3_000_000, InvestedC: 300_000_000, Seniority: 2},
	}
	r, _ = Waterfall(500_000_000, nil, stack, nil)
	if payout(r, "b") != 300_000_000 || payout(r, "a") != 200_000_000 || payout(r, "c") != 0 {
		t.Errorf("stack: %d %d %d", payout(r, "b"), payout(r, "a"), payout(r, "c"))
	}
	// Equal seniority shares a short pot pro rata to what is owed: $3M and $4M owed, $3.5M available -> 1.5M and 2M.
	stack[2].Seniority = 1
	r, _ = Waterfall(350_000_000, nil, stack, nil)
	if payout(r, "b") != 150_000_000 || payout(r, "a") != 200_000_000 {
		t.Errorf("pari passu: %d %d", payout(r, "b"), payout(r, "a"))
	}
}

func TestWaterfallOptionsAndClaims(t *testing.T) {
	// 1,000,000 common + 100,000 options at $1.00 and a $5M exit: the exercise money joins the pot,
	// 510,000,000 / 1,100,000 = 463.636... cents a share. Options net 36,363,636.36 after paying the strike.
	cl := []ClassTerms{{ID: "c", Name: "Common", Shares: 1_000_000}}
	op := []OptionPool{{ID: "o", Name: "Options", Shares: 100_000, StrikeC: big.NewRat(100, 1)}}
	r, _ := Waterfall(500_000_000, nil, cl, op)
	if payout(r, "o") != 36_363_636 || payout(r, "c") != 463_636_364 || r.TotalC != 500_000_000 {
		t.Errorf("options: %d common %d total %d", payout(r, "o"), payout(r, "c"), r.TotalC)
	}
	// Underwater options are not exercised: strike $10, per-share value $5.
	op[0].StrikeC = big.NewRat(1000, 1)
	r, _ = Waterfall(500_000_000, nil, cl, op)
	if payout(r, "o") != 0 || payout(r, "c") != 500_000_000 {
		t.Errorf("underwater: %d %d", payout(r, "o"), payout(r, "c"))
	}
	// A note repaid first, then the common: $0.5M claim, $5M exit.
	cl2 := []ClassTerms{{ID: "c", Name: "Common", Shares: 1_000_000}}
	r, _ = Waterfall(500_000_000, []Claim{{ID: "n", Name: "Note", AmountC: 50_000_000}}, cl2, nil)
	if payout(r, "n") != 50_000_000 || payout(r, "c") != 450_000_000 {
		t.Errorf("note: %d %d", payout(r, "n"), payout(r, "c"))
	}
	// A short exit: the note takes it all.
	r, _ = Waterfall(30_000_000, []Claim{{ID: "n", Name: "Note", AmountC: 50_000_000}}, cl2, nil)
	if payout(r, "n") != 30_000_000 || payout(r, "c") != 0 {
		t.Errorf("short: %d %d", payout(r, "n"), payout(r, "c"))
	}
}

func TestWaterfallAlwaysAddsUp(t *testing.T) {
	cl := []ClassTerms{
		{ID: "c", Name: "Common", Shares: 7_000_001},
		{ID: "a", Name: "A", Preferred: true, Shares: 1_234_567, InvestedC: 123_456_789, Seniority: 1, Participating: true},
		{ID: "b", Name: "B", Preferred: true, Shares: 999_999, InvestedC: 999_999_00, Seniority: 2},
	}
	op := []OptionPool{{ID: "o1", Shares: 333_333, StrikeC: big.NewRat(37, 1)}, {ID: "o2", Shares: 77_777, StrikeC: big.NewRat(1234, 7)}}
	for _, exit := range []int64{1, 99, 12_345_678, 987_654_321, 5_000_000_000, 123_456_789_012} {
		r, err := Waterfall(exit, nil, cl, op)
		if err != nil {
			t.Fatal(err)
		}
		if r.TotalC != exit {
			t.Errorf("exit %d paid out %d", exit, r.TotalC)
		}
		for _, p := range r.Payees {
			if p.PayoutC < 0 {
				t.Errorf("negative payout %+v", p)
			}
		}
	}
}

func TestCapTableAsOf(t *testing.T) {
	holders := []Stakeholder{{ID: "a", Name: "Asha", Kind: "founder"}, {ID: "b", Name: "Bala", Kind: "founder"}, {ID: "e", Name: "Esha", Kind: "employee"}}
	secs := []Security{
		{ID: "s1", Kind: "stock", StakeholderID: "a", ClassID: "common", Quantity: 6_000_000, IssueDate: "2023-01-01"},
		{ID: "s2", Kind: "stock", StakeholderID: "b", ClassID: "common", Quantity: 4_000_000, IssueDate: "2023-01-01"},
		{ID: "o1", Kind: "option", StakeholderID: "e", PoolID: "p", ClassID: "common", Quantity: 100_000, IssueDate: "2024-01-01",
			Vesting: Vesting{Start: "2024-01-01", Months: 48, CliffMons: 12}},
		{ID: "o2", Kind: "option", StakeholderID: "b", PoolID: "p", ClassID: "common", Quantity: 50_000, IssueDate: "2024-06-01"},
		{ID: "n1", Kind: "safe", StakeholderID: "a", Conv: Convertible{PrincipalC: 1_000}, IssueDate: "2024-01-01"},
	}
	pools := []Pool{{ID: "p", Name: "ESOP 2023", Size: 1_000_000, Adopted: "2023-01-01"}}
	evs := []Event{
		{Kind: "cancel", SecurityID: "o2", Date: "2024-09-01", Quantity: 20_000},
		{Kind: "exercise", SecurityID: "o1", Date: "2025-02-01", Quantity: 10_000},
	}
	tb := Build(holders, map[string]string{"common": "Common"}, pools, secs, evs, "2025-03-01")
	// Outstanding: 10,000,000. Options: o1 100,000 - 10,000 exercised = 90,000; o2 50,000 - 20,000 = 30,000.
	// Pool drawn: o1 100,000 (exercised stays drawn) + o2 30,000 = 130,000 -> unallocated 870,000.
	if tb.Outstanding != 10_000_000 || tb.Pool != 870_000 || tb.FullyDil != 10_000_000+90_000+30_000+870_000 {
		t.Fatalf("totals: out %d pool %d fd %d", tb.Outstanding, tb.Pool, tb.FullyDil)
	}
	if tb.Convertible != 1_000 {
		t.Errorf("convertible %d", tb.Convertible)
	}
	var tot int64
	for _, r := range tb.Rows {
		tot += r.PctFD
	}
	if tot+tb.PoolPctFD != PctScale {
		t.Errorf("fully diluted percentages add to %d", tot+tb.PoolPctFD)
	}
	// Asha: 6,000,000 / 10,000,000 outstanding = 60%.
	if tb.Rows[0].Name != "Asha" || tb.Rows[0].PctOut != 600_000 {
		t.Errorf("%+v", tb.Rows[0])
	}
	// As of 2023-06-01 the options do not exist yet and the pool is untouched.
	early := Build(holders, map[string]string{"common": "Common"}, pools, secs, evs, "2023-06-01")
	if early.Pool != 1_000_000 || early.FullyDil != 11_000_000 {
		t.Errorf("early: pool %d fd %d", early.Pool, early.FullyDil)
	}
	// Vesting of o1 on 2025-03-01: 14 whole months -> floor(100000 x 14/48) = 29,166; 10,000 exercised -> 19,166 exercisable.
	for _, p := range Positions(secs, evs, "2025-03-01") {
		if p.ID == "o1" && (p.Vested != 29_166 || p.Exercisable != 19_166 || p.Outstanding != 90_000) {
			t.Errorf("o1 position %+v", p)
		}
	}
}
