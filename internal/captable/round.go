package captable

import (
	"fmt"
	"math/big"
)

// Convertible is a SAFE or a convertible note. Rates are in basis points (100 bp = 1%) so nothing is a float.
type Convertible struct {
	ID         string
	Name       string
	Kind       string // safe | note
	PrincipalC int64  // cents
	CapC       int64  // valuation cap in cents, 0 = none
	DiscountBp int    // discount to the round price, 2000 = 20%
	PostMoney  bool   // post-money SAFE: the cap divides by the company capitalisation after conversion
	InterestBp int    // yearly simple interest, notes only
	IssueDate  Date
}

// Value is principal plus simple interest accrued to asOf (notes), in cents, rounded half up.
// Interest = principal x rate x days / 365.
func (c Convertible) Value(asOf Date) int64 {
	if c.Kind != "note" || c.InterestBp <= 0 || c.IssueDate == "" {
		return c.PrincipalC
	}
	days := daysBetween(c.IssueDate, asOf)
	if days <= 0 {
		return c.PrincipalC
	}
	num := new(big.Int).Mul(big.NewInt(c.PrincipalC), big.NewInt(int64(c.InterestBp)))
	num.Mul(num, big.NewInt(int64(days)))
	return c.PrincipalC + RoundHalfUp(new(big.Rat).SetFrac(num, big.NewInt(10000*365)))
}

// Holding is a group of existing fully diluted shares (a holder, or the unallocated pool).
type Holding struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Shares int64  `json:"shares"`
	Pool   bool   `json:"pool,omitempty"` // the unallocated option pool
}

// RoundInput describes a priced financing round.
type RoundInput struct {
	PreMoneyC    int64         // pre-money valuation, cents
	InvestmentC  int64         // new money, cents
	PoolTargetBp int           // unallocated pool as a share of the post-round fully diluted shares, basis points (0 = no top-up)
	Existing     []Holding     // everyone and everything in today's fully diluted count, including the unallocated pool
	Convertibles []Convertible // SAFEs and notes converting in this round
	Date         Date
}

// RoundHolder is one row of the post-round cap table. Percentages are in PctScale units.
type RoundHolder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Before    int64  `json:"before"`
	Added     int64  `json:"added"`
	After     int64  `json:"after"`
	PctBefore int64  `json:"pct_before"`
	PctAfter  int64  `json:"pct_after"`
}

// ConvertedShares is the outcome for one convertible.
type ConvertedShares struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ValueC      int64  `json:"value_cents"`
	Shares      int64  `json:"shares"`
	PriceCents  string `json:"price_cents"` // exact conversion price, 6 decimals
	CapBinds    bool   `json:"cap_binds"`
	OwnershipPc int64  `json:"ownership_pct"` // after the round, PctScale units
}

// RoundResult is the modelled round.
type RoundResult struct {
	PricePerShareCents string            `json:"price_per_share_cents"` // exact decimal, 6 places
	PreFD              int64             `json:"pre_fully_diluted"`
	PoolTopUp          int64             `json:"pool_top_up"`
	Converted          []ConvertedShares `json:"converted"`
	NewShares          int64             `json:"new_investor_shares"`
	PostFD             int64             `json:"post_fully_diluted"`
	PostMoneyC         int64             `json:"post_money_cents"`
	NewInvestorPct     int64             `json:"new_investor_pct"`
	Holders            []RoundHolder     `json:"holders"`
	Notes              []string          `json:"notes,omitempty"`
}

const fprec = 256

func bf(v int64) *big.Float { return new(big.Float).SetPrec(fprec).SetInt64(v) }
func bfNew() *big.Float     { return new(big.Float).SetPrec(fprec) }

// floorBF is floor(x + 1e-30): exact integer results survive the last digits of the iteration.
func floorBF(x *big.Float) int64 {
	y := bfNew().Add(x, new(big.Float).SetPrec(fprec).SetFloat64(1e-30))
	i, _ := y.Int(nil)
	return i.Int64()
}

// ModelRound computes the price per share, the shares every convertible turns into, the pool top-up and the
// post-round table.
//
// Rules (all exact; the only approximation is the 256-bit fixed-point loop, whose result is rounded down to whole
// shares with a 1e-30 guard so an exact integer is never lost to the last digit):
//   - price per share = pre-money / (pre-round fully diluted + converting shares + pool top-up). The pre-money
//     therefore includes the converting instruments and the top-up, which dilute existing holders only.
//   - a convertible converts at min(round price, cap price, round price x (1 - discount)), cap price = cap / basis.
//     The basis is the pre-round fully diluted count (pre-money SAFEs and notes) or the company capitalisation
//     after conversion (post-money SAFEs), which depends on the shares the SAFEs themselves receive, so it is found by
//     iterating.
//   - the pool top-up makes the unallocated pool the target share of the post-round fully diluted count.
//   - convertible shares are rounded down, the top-up to nearest, new investor shares down.
func ModelRound(in RoundInput) (*RoundResult, error) {
	if in.PreMoneyC <= 0 {
		return nil, fmt.Errorf("the pre-money valuation must be above zero")
	}
	if in.InvestmentC < 0 {
		return nil, fmt.Errorf("the investment cannot be negative")
	}
	if in.PoolTargetBp < 0 || in.PoolTargetBp > 6000 {
		return nil, fmt.Errorf("the pool target must be between 0 and 60 percent")
	}
	var fdPre, poolUnalloc int64
	for _, h := range in.Existing {
		if h.Shares < 0 {
			return nil, fmt.Errorf("share counts cannot be negative")
		}
		fdPre += h.Shares
		if h.Pool {
			poolUnalloc += h.Shares
		}
	}
	if fdPre <= 0 {
		return nil, fmt.Errorf("the company has no shares yet; add a founder issuance first")
	}
	pre, inv := bf(in.PreMoneyC), bf(in.InvestmentC)
	k := bfNew().Quo(bfNew().Add(pre, inv), pre)
	t := bfNew().Quo(bf(int64(in.PoolTargetBp)), bf(10000))
	tk := bfNew().Mul(t, k)
	if tk.Cmp(new(big.Float).SetFloat64(0.99)) >= 0 {
		return nil, fmt.Errorf("the pool target is too large for this round")
	}
	// A capped post-money SAFE promises principal/cap of the company; together they must leave room.
	claim := new(big.Rat)
	for _, c := range in.Convertibles {
		if c.PostMoney && c.CapC > 0 {
			claim.Add(claim, big.NewRat(c.Value(in.Date), c.CapC))
		}
	}
	if claim.Cmp(big.NewRat(95, 100)) >= 0 {
		return nil, fmt.Errorf("the capped post-money SAFEs claim %s%% of the company, which has no sensible solution; check the amounts and caps", new(big.Rat).Mul(claim, big.NewRat(100, 1)).FloatString(0))
	}
	fd := bf(fdPre)
	sc, pool := bf(0), bf(0)
	n := len(in.Convertibles)
	shares := make([]*big.Float, n)
	prices := make([]*big.Float, n)
	eps := new(big.Float).SetPrec(fprec).SetFloat64(1e-40)
	oneMinus := func(bp int) *big.Float { return bfNew().Quo(bf(int64(10000-bp)), bf(10000)) }
	for iter := 0; iter < 4000; iter++ {
		pps := bfNew().Quo(pre, bfNew().Add(bfNew().Add(fd, sc), pool))
		next := bf(0)
		companyCap := bfNew().Add(fd, sc)
		for i, c := range in.Convertibles {
			p := pps
			basis := fd
			if c.PostMoney {
				basis = companyCap
			}
			if c.CapC > 0 && basis.Sign() > 0 {
				if cp := bfNew().Quo(bf(c.CapC), basis); cp.Cmp(p) < 0 {
					p = cp
				}
			}
			if c.DiscountBp > 0 {
				if dp := bfNew().Mul(pps, oneMinus(c.DiscountBp)); dp.Cmp(p) < 0 {
					p = dp
				}
			}
			prices[i] = p
			shares[i] = bfNew().Quo(bf(c.Value(in.Date)), p)
			next.Add(next, shares[i])
		}
		np := bf(0)
		if in.PoolTargetBp > 0 {
			a := bfNew().Add(fd, next)
			num := bfNew().Sub(bfNew().Mul(tk, a), bf(poolUnalloc))
			np = bfNew().Quo(num, bfNew().Sub(bf(1), tk))
			if np.Sign() < 0 {
				np = bf(0)
			}
		}
		d1 := bfNew().Abs(bfNew().Sub(next, sc))
		d2 := bfNew().Abs(bfNew().Sub(np, pool))
		sc, pool = next, np
		if d1.Cmp(eps) < 0 && d2.Cmp(eps) < 0 {
			break
		}
	}
	res := &RoundResult{PreFD: fdPre, Converted: []ConvertedShares{}}
	var conv int64
	pps0 := bfNew().Quo(pre, bfNew().Add(bfNew().Add(fd, sc), pool))
	for i, c := range in.Convertibles {
		cs := floorBF(shares[i])
		conv += cs
		// The cap binds when its price is at or below the discount price.
		capBinds := false
		if c.CapC > 0 {
			basis := fd
			if c.PostMoney {
				basis = bfNew().Add(fd, sc)
			}
			capP := bfNew().Quo(bf(c.CapC), basis)
			dp := bfNew().Mul(pps0, oneMinus(c.DiscountBp))
			capBinds = capP.Cmp(dp) <= 0 && capP.Cmp(pps0) <= 0
		}
		res.Converted = append(res.Converted, ConvertedShares{ID: c.ID, Name: c.Name, ValueC: c.Value(in.Date), Shares: cs, PriceCents: prices[i].Text('f', 6), CapBinds: capBinds})
	}
	res.PoolTopUp = floorBF(bfNew().Add(pool, new(big.Float).SetPrec(fprec).SetFloat64(0.5)))
	preRound := fdPre + conv + res.PoolTopUp
	ppsRat := new(big.Rat).SetFrac64(in.PreMoneyC, preRound) // cents per share, exact
	res.PricePerShareCents = ppsRat.FloatString(6)
	if in.InvestmentC > 0 {
		// floor(investment / price) = floor(investment x preRound / preMoney), in integers.
		num := new(big.Int).Mul(big.NewInt(in.InvestmentC), big.NewInt(preRound))
		res.NewShares = num.Div(num, big.NewInt(in.PreMoneyC)).Int64()
	}
	res.PostFD = preRound + res.NewShares
	res.PostMoneyC = RoundHalfUp(new(big.Rat).Mul(ppsRat, big.NewRat(res.PostFD, 1)))
	// Table.
	var rows []RoundHolder
	var wBefore, wAfter []int64
	for _, h := range in.Existing {
		added := int64(0)
		if h.Pool {
			added = res.PoolTopUp
		}
		rows = append(rows, RoundHolder{ID: h.ID, Name: h.Name, Before: h.Shares, Added: added, After: h.Shares + added})
		wBefore = append(wBefore, h.Shares)
		wAfter = append(wAfter, h.Shares+added)
	}
	poolRow := -1
	for i, h := range in.Existing {
		if h.Pool {
			poolRow = i
		}
	}
	if res.PoolTopUp > 0 && poolRow < 0 {
		rows = append(rows, RoundHolder{ID: "pool", Name: "Option pool (unallocated)", Added: res.PoolTopUp, After: res.PoolTopUp})
		wBefore = append(wBefore, 0)
		wAfter = append(wAfter, res.PoolTopUp)
	}
	convStart := len(rows)
	for i, c := range in.Convertibles {
		nm := c.Name
		if nm == "" {
			nm = c.ID
		}
		rows = append(rows, RoundHolder{ID: c.ID, Name: "Converting: " + nm, Added: res.Converted[i].Shares, After: res.Converted[i].Shares})
		wBefore = append(wBefore, 0)
		wAfter = append(wAfter, res.Converted[i].Shares)
	}
	if res.NewShares > 0 {
		rows = append(rows, RoundHolder{ID: "new", Name: "New investors", Added: res.NewShares, After: res.NewShares})
		wBefore = append(wBefore, 0)
		wAfter = append(wAfter, res.NewShares)
	}
	pb, pa := Percentages(wBefore), Percentages(wAfter)
	for i := range rows {
		rows[i].PctBefore, rows[i].PctAfter = pb[i], pa[i]
	}
	res.Holders = rows
	for i := range res.Converted {
		res.Converted[i].OwnershipPc = pa[convStart+i]
	}
	if res.NewShares > 0 {
		res.NewInvestorPct = pa[len(pa)-1]
	}
	if res.PoolTopUp > 0 {
		res.Notes = append(res.Notes, "The pool top-up is added before the round, so it dilutes existing holders and not the new investors.")
	}
	return res, nil
}
