package captable

import (
	"fmt"
	"math/big"
	"sort"
)

// ClassTerms describes one class of shares at an exit.
type ClassTerms struct {
	ID                string
	Name              string
	Preferred         bool
	Shares            int64
	InvestedC         int64 // money paid in for the class (preferred), cents
	PrefMultipleMilli int64 // 1000 = 1x: the money back first; 0 means 1000
	Participating     bool  // also shares in what is left with the common (no cap)
	Seniority         int   // higher is paid first; equal is paid side by side
	ConvertRatioMilli int64 // common shares per preferred share x 1000; 0 means 1000
}

// OptionPool is a group of options or warrants (one grant) that exercises when in the money.
type OptionPool struct {
	ID      string
	Name    string
	Shares  int64
	StrikeC *big.Rat // cents per share
}

// Claim is a debt-like amount repaid before any share: a convertible note (principal and interest) or the money
// back of a SAFE. Claims rank by Rank (higher first); equal ranks share a short pot pro rata to what they are owed.
type Claim struct {
	ID      string
	Name    string
	AmountC int64
	Rank    int
}

// Payee is one line of the payout table.
type Payee struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // class | options | claim
	Shares    int64  `json:"shares"`
	PayoutC   int64  `json:"payout_cents"`
	Converted bool   `json:"converted,omitempty"`
	Exercised bool   `json:"exercised,omitempty"`
	PerShare  string `json:"per_share_cents"`
}

// WaterfallResult is the distribution of an exit.
type WaterfallResult struct {
	ExitC          int64    `json:"exit_cents"`
	Payees         []Payee  `json:"payees"`
	TotalC         int64    `json:"total_cents"`
	CommonPerShare string   `json:"common_per_share_cents"`
	Notes          []string `json:"notes,omitempty"`
}

func milliRat(v int64) *big.Rat {
	if v <= 0 {
		v = 1000
	}
	return big.NewRat(v, 1000)
}

// Waterfall distributes exitC among claims, classes and option pools.
//
//  1. Claims (notes, SAFE money back) are repaid first, by rank; a short pot is shared pro rata within a rank.
//  2. Preferred classes that do not convert take invested x multiple, most senior first; equal seniority shares a
//     short pot pro rata to what it is owed.
//  3. What is left goes per share to common, converted preferred (times the conversion ratio), participating
//     preferred (as converted) and exercised options. Options exercise only when the per-share value beats the
//     strike; the exercise money joins the pot and is deducted from the option payout.
//  4. A non-participating preferred class converts when that pays strictly more than its preference. The choices
//     are found by repeating until nobody changes their mind.
//
// Everything is big.Rat arithmetic; the final cents are assigned by the largest-remainder rule so the payouts add
// up to exactly the exit value (minus claims that could not be paid in full are already inside the pot logic).
func Waterfall(exitC int64, claims []Claim, classes []ClassTerms, options []OptionPool) (*WaterfallResult, error) {
	if exitC < 0 {
		return nil, fmt.Errorf("the exit value cannot be negative")
	}
	for _, c := range classes {
		if c.Shares < 0 || c.InvestedC < 0 {
			return nil, fmt.Errorf("class %q has negative numbers", c.Name)
		}
	}
	for _, c := range claims {
		if c.AmountC < 0 {
			return nil, fmt.Errorf("claim %q is negative", c.Name)
		}
	}
	conv := make([]bool, len(classes))
	exer := make([]bool, len(options))
	type outcome struct {
		claims []*big.Rat
		cls    []*big.Rat // gross payout per class
		opt    []*big.Rat // gross payout per option pool (before paying the strike)
		price  *big.Rat   // per common-equivalent share
	}
	compute := func() outcome {
		o := outcome{claims: make([]*big.Rat, len(claims)), cls: make([]*big.Rat, len(classes)), opt: make([]*big.Rat, len(options))}
		for i := range o.claims {
			o.claims[i] = new(big.Rat)
		}
		for i := range o.cls {
			o.cls[i] = new(big.Rat)
		}
		for i := range o.opt {
			o.opt[i] = new(big.Rat)
		}
		pot := big.NewRat(exitC, 1)
		for i, op := range options {
			if exer[i] {
				pot.Add(pot, new(big.Rat).Mul(op.StrikeC, big.NewRat(op.Shares, 1)))
			}
		}
		pay := func(idx []int, owed func(int) *big.Rat, into []*big.Rat) {
			total := new(big.Rat)
			for _, i := range idx {
				total.Add(total, owed(i))
			}
			if total.Sign() == 0 {
				return
			}
			if pot.Cmp(total) >= 0 {
				for _, i := range idx {
					into[i].Add(into[i], owed(i))
				}
				pot.Sub(pot, total)
				return
			}
			for _, i := range idx {
				into[i].Add(into[i], new(big.Rat).Quo(new(big.Rat).Mul(pot, owed(i)), total))
			}
			pot = new(big.Rat)
		}
		// Claims by rank.
		byRank := map[int][]int{}
		var ranks []int
		for i, c := range claims {
			if _, ok := byRank[c.Rank]; !ok {
				ranks = append(ranks, c.Rank)
			}
			byRank[c.Rank] = append(byRank[c.Rank], i)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(ranks)))
		for _, r := range ranks {
			pay(byRank[r], func(i int) *big.Rat { return big.NewRat(claims[i].AmountC, 1) }, o.claims)
		}
		// Preferences by seniority.
		bySen := map[int][]int{}
		var sens []int
		for i, c := range classes {
			if c.Preferred && !conv[i] && c.InvestedC > 0 {
				if _, ok := bySen[c.Seniority]; !ok {
					sens = append(sens, c.Seniority)
				}
				bySen[c.Seniority] = append(bySen[c.Seniority], i)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(sens)))
		for _, s := range sens {
			pay(bySen[s], func(i int) *big.Rat {
				return new(big.Rat).Mul(big.NewRat(classes[i].InvestedC, 1), milliRat(classes[i].PrefMultipleMilli))
			}, o.cls)
		}
		// The rest, per common-equivalent share.
		eqOf := func(i int) *big.Rat {
			c := classes[i]
			if !c.Preferred {
				return big.NewRat(c.Shares, 1)
			}
			if conv[i] || c.Participating {
				return new(big.Rat).Mul(big.NewRat(c.Shares, 1), milliRat(c.ConvertRatioMilli))
			}
			return new(big.Rat)
		}
		total := new(big.Rat)
		for i := range classes {
			total.Add(total, eqOf(i))
		}
		for i, op := range options {
			if exer[i] {
				total.Add(total, big.NewRat(op.Shares, 1))
			}
		}
		o.price = new(big.Rat)
		if total.Sign() > 0 {
			o.price = new(big.Rat).Quo(pot, total)
		}
		for i := range classes {
			o.cls[i].Add(o.cls[i], new(big.Rat).Mul(o.price, eqOf(i)))
		}
		for i, op := range options {
			if exer[i] {
				o.opt[i] = new(big.Rat).Mul(o.price, big.NewRat(op.Shares, 1))
			}
		}
		return o
	}
	var out outcome
	for iter := 0; iter < 200; iter++ {
		out = compute()
		changed := false
		for i, c := range classes {
			if !c.Preferred || c.Participating || c.Shares == 0 {
				continue
			}
			conv[i] = !conv[i]
			alt := compute()
			conv[i] = !conv[i]
			// Switch when the other choice pays strictly more.
			if alt.cls[i].Cmp(out.cls[i]) > 0 {
				conv[i] = !conv[i]
				changed = true
				break
			}
		}
		if !changed {
			for i, op := range options {
				want := op.Shares > 0 && out.price.Cmp(op.StrikeC) > 0
				if want != exer[i] {
					exer[i] = want
					changed = true
					break
				}
			}
		}
		if !changed {
			break
		}
	}
	out = compute()
	var payees []Payee
	var weights []*big.Rat
	for i, c := range claims {
		if c.AmountC == 0 {
			continue
		}
		payees = append(payees, Payee{ID: c.ID, Name: c.Name, Kind: "claim"})
		weights = append(weights, out.claims[i])
	}
	for i, c := range classes {
		if c.Shares == 0 && out.cls[i].Sign() == 0 {
			continue
		}
		payees = append(payees, Payee{ID: c.ID, Name: c.Name, Kind: "class", Shares: c.Shares, Converted: c.Preferred && conv[i]})
		weights = append(weights, out.cls[i])
	}
	for i, op := range options {
		if op.Shares == 0 {
			continue
		}
		net := new(big.Rat)
		if exer[i] {
			net.Sub(out.opt[i], new(big.Rat).Mul(op.StrikeC, big.NewRat(op.Shares, 1)))
		}
		payees = append(payees, Payee{ID: op.ID, Name: op.Name, Kind: "options", Shares: op.Shares, Exercised: exer[i]})
		weights = append(weights, net)
	}
	res := &WaterfallResult{ExitC: exitC}
	res.CommonPerShare = out.price.FloatString(6)
	// If everything was paid in full the rationals add up to exitC exactly, so the largest-remainder split
	// of exitC is exact. If nobody holds shares the leftover has no owner.
	sum := new(big.Rat)
	for _, w := range weights {
		sum.Add(sum, w)
	}
	cents := SplitCents(RoundHalfUp(sum), weights)
	for i := range payees {
		payees[i].PayoutC = cents[i]
		res.TotalC += cents[i]
		if payees[i].Shares > 0 {
			payees[i].PerShare = new(big.Rat).Quo(big.NewRat(cents[i], 1), big.NewRat(payees[i].Shares, 1)).FloatString(4)
		}
	}
	if payees == nil {
		payees = []Payee{}
	}
	res.Payees = payees
	if exitC > 0 && res.TotalC != exitC {
		res.Notes = append(res.Notes, fmt.Sprintf("%d of the exit value has no owner: nothing is left for any share class to take it.", exitC-res.TotalC))
	}
	return res, nil
}
