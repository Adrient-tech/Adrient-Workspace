// Vesting schedules with cliff and frequency, fully diluted counts, convertible conversion price, post-money SAFE
// capitalisation fixed point and liquidation waterfall, with integer share counts, money in cents and big.Rat
// arithmetic, so totals are exact and every rounding rule is stated.

// Package captable keeps a company's equity records and works out ownership, vesting, financing rounds and exit
// payouts. Shares are whole numbers, money is in the smallest currency unit (cents) and percentages are shown to
// four decimals (basis points of a percent) with the largest-remainder rule, so a column always adds up exactly.
package captable

import (
	"math/big"
	"sort"
	"time"
)

// Date is YYYY-MM-DD.
type Date = string

const dayLayout = "2006-01-02"

func parseDate(d Date) (time.Time, bool) {
	t, err := time.Parse(dayLayout, d)
	return t, err == nil
}

func addMonths(d Date, n int) Date {
	t, ok := parseDate(d)
	if !ok {
		return ""
	}
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := t.Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC).Format(dayLayout)
}

func daysBetween(a, b Date) int {
	x, _ := parseDate(a)
	y, _ := parseDate(b)
	return int(y.Sub(x).Hours() / 24)
}

// ---- vesting ---------------------------------------------------------------

// Vesting is a time-based schedule.
type Vesting struct {
	Start     Date
	Months    int    // total length, 0 = fully vested at once
	CliffMons int    // nothing vests before this many months
	Frequency string // monthly (default), quarterly, annual
}

func (v Vesting) step() int {
	switch v.Frequency {
	case "quarterly":
		return 3
	case "annual":
		return 12
	}
	return 1
}

// completedMonths is the number of whole months from start to asOf (a month completes on the same day-of-month,
// clamped to the month length), never negative.
func completedMonths(start, asOf Date) int {
	s, ok1 := parseDate(start)
	a, ok2 := parseDate(asOf)
	if !ok1 || !ok2 || a.Before(s) {
		return 0
	}
	m := (a.Year()-s.Year())*12 + int(a.Month()) - int(s.Month())
	if m > 0 && addMonths(start, m) > asOf {
		m--
	}
	return m
}

// Vested is the number of shares vested on asOf: nothing before the cliff, then straight-line by vesting step,
// rounded down, and everything at the end of the schedule (so rounding never leaves a remainder unvested).
// A schedule without months is fully vested.
func (v Vesting) Vested(quantity int64, asOf Date) int64 {
	if quantity <= 0 {
		return 0
	}
	if v.Months <= 0 {
		return quantity
	}
	m := completedMonths(v.Start, asOf)
	if m >= v.Months {
		return quantity
	}
	if m < v.CliffMons || m < 0 {
		return 0
	}
	m -= m % v.step()
	// floor(quantity * m / months) without overflow.
	r := new(big.Int).Mul(big.NewInt(quantity), big.NewInt(int64(m)))
	r.Div(r, big.NewInt(int64(v.Months)))
	return r.Int64()
}

// ---- percentages -----------------------------------------------------------

// Shares are percentages in units of 0.0001 percent ("hundredths of a basis point"): 100% = 1,000,000.
const PctScale = 1_000_000

// Percentages splits total into parts proportional to weights, in PctScale units that add up to exactly PctScale
// (largest remainder method; ties go to the earlier item). All-zero weights give all zeros.
func Percentages(weights []int64) []int64 {
	out := make([]int64, len(weights))
	var sum int64
	for _, w := range weights {
		sum += w
	}
	if sum <= 0 {
		return out
	}
	type rem struct {
		i int
		r *big.Int
	}
	rems := make([]rem, len(weights))
	var given int64
	for i, w := range weights {
		num := new(big.Int).Mul(big.NewInt(w), big.NewInt(PctScale))
		q, r := new(big.Int).QuoRem(num, big.NewInt(sum), new(big.Int))
		out[i] = q.Int64()
		given += out[i]
		rems[i] = rem{i, r}
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r.Cmp(rems[b].r) > 0 })
	for k := int64(0); k < PctScale-given; k++ {
		out[rems[k].i]++
	}
	return out
}

// SplitCents divides total cents into parts proportional to weights so that the parts add up to total exactly.
func SplitCents(total int64, weights []*big.Rat) []int64 {
	out := make([]int64, len(weights))
	sum := new(big.Rat)
	for _, w := range weights {
		sum.Add(sum, w)
	}
	if sum.Sign() <= 0 || total <= 0 {
		return out
	}
	type rem struct {
		i int
		r *big.Rat
	}
	rems := make([]rem, len(weights))
	var given int64
	for i, w := range weights {
		x := new(big.Rat).Mul(w, big.NewRat(total, 1))
		x.Quo(x, sum)
		fl := new(big.Int).Quo(x.Num(), x.Denom())
		out[i] = fl.Int64()
		given += out[i]
		rems[i] = rem{i, new(big.Rat).Sub(x, new(big.Rat).SetInt(fl))}
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r.Cmp(rems[b].r) > 0 })
	for k := int64(0); k < total-given; k++ {
		out[rems[k].i]++
	}
	return out
}

// RoundHalfUp rounds a non-negative rational to the nearest integer, halves up.
func RoundHalfUp(r *big.Rat) int64 {
	x := new(big.Rat).Add(r, big.NewRat(1, 2))
	return new(big.Int).Quo(x.Num(), x.Denom()).Int64()
}
