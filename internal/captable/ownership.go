package captable

import (
	"math/big"
	"sort"
)

// Security is one equity record: shares, an option grant, a warrant, or a SAFE or note.
type Security struct {
	ID            string
	Label         string
	Kind          string // stock | option | warrant | safe | note
	StakeholderID string
	ClassID       string
	PoolID        string
	Quantity      int64
	PriceC        *big.Rat // issue price (stock) or strike (option, warrant), in cents per share
	IssueDate     Date
	Vesting       Vesting
	Expires       Date
	Conv          Convertible // safe | note terms
}

// Event changes a security after it was issued.
type Event struct {
	Kind       string // exercise | cancel | transfer | convert
	SecurityID string
	Date       Date
	Quantity   int64
}

// Position is a security's state on one date.
type Position struct {
	Security
	Outstanding int64 // still held: quantity less exercised, cancelled and transferred away
	Vested      int64 // vested of the original quantity
	Exercised   int64
	Cancelled   int64
	Transferred int64
	Exercisable int64 // options and warrants: vested and not yet exercised
	Converted   bool  // a SAFE or note that has turned into shares
	Pending     bool  // issued after the as-of date
}

// Positions computes every security's state on asOf.
func Positions(secs []Security, evs []Event, asOf Date) []Position {
	byID := map[string][]Event{}
	for _, e := range evs {
		if e.Date <= asOf {
			byID[e.SecurityID] = append(byID[e.SecurityID], e)
		}
	}
	out := make([]Position, 0, len(secs))
	for _, s := range secs {
		p := Position{Security: s}
		if s.IssueDate > asOf {
			p.Pending = true
			out = append(out, p)
			continue
		}
		for _, e := range byID[s.ID] {
			switch e.Kind {
			case "exercise":
				p.Exercised += e.Quantity
			case "cancel":
				p.Cancelled += e.Quantity
			case "transfer":
				p.Transferred += e.Quantity
			case "convert":
				p.Converted = true
			}
		}
		if s.Kind == "safe" || s.Kind == "note" {
			if !p.Converted {
				p.Outstanding = 0 // convertibles are counted in money, not shares
			}
			out = append(out, p)
			continue
		}
		p.Outstanding = s.Quantity - p.Exercised - p.Cancelled - p.Transferred
		if p.Outstanding < 0 {
			p.Outstanding = 0
		}
		p.Vested = s.Vesting.Vested(s.Quantity, asOf)
		if s.Kind == "option" || s.Kind == "warrant" {
			room := p.Vested - p.Exercised - p.Transferred
			if room > p.Outstanding {
				room = p.Outstanding
			}
			if room < 0 {
				room = 0
			}
			p.Exercisable = room
		}
		out = append(out, p)
	}
	return out
}

// Pool is an equity incentive reserve.
type Pool struct {
	ID      string
	Name    string
	Size    int64
	Adopted Date
}

// PoolBalance is what a pool has granted and what is left.
type PoolBalance struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Granted     int64  `json:"granted"` // grants drawn from the pool, less cancelled
	Exercised   int64  `json:"exercised"`
	Unallocated int64  `json:"unallocated"`
}

// PoolBalances: an option grant draws its quantity from its pool; cancelling returns shares to the pool, while
// exercised and transferred options stay drawn (they became shares or went to someone else).
func PoolBalances(pools []Pool, pos []Position, asOf Date) []PoolBalance {
	var out []PoolBalance
	for _, p := range pools {
		if p.Adopted > asOf {
			continue
		}
		b := PoolBalance{ID: p.ID, Name: p.Name, Size: p.Size}
		for _, x := range pos {
			if x.PoolID == p.ID && x.Kind == "option" && !x.Pending {
				b.Granted += x.Quantity - x.Cancelled
				b.Exercised += x.Exercised
			}
		}
		b.Unallocated = p.Size - b.Granted
		if b.Unallocated < 0 {
			b.Unallocated = 0
		}
		out = append(out, b)
	}
	return out
}

// Stakeholder is a holder.
type Stakeholder struct {
	ID   string
	Name string
	Kind string
}

// Row is one holder on the cap table.
type Row struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Stock        int64  `json:"stock"`
	Options      int64  `json:"options"`
	Warrants     int64  `json:"warrants"`
	FullyDiluted int64  `json:"fully_diluted"`
	PctOut       int64  `json:"pct_outstanding"` // PctScale units, 1,000,000 = 100%
	PctFD        int64  `json:"pct_fully_diluted"`
	VestedStock  int64  `json:"vested_stock"`
}

// ClassRow is the shares of one class.
type ClassRow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Shares int64  `json:"shares"`
	PctOut int64  `json:"pct_outstanding"`
}

// Table is the cap table on one date.
type Table struct {
	AsOf        Date          `json:"as_of"`
	Rows        []Row         `json:"rows"`
	Pool        int64         `json:"pool_unallocated"`
	Pools       []PoolBalance `json:"pools"`
	Outstanding int64         `json:"outstanding"`
	FullyDil    int64         `json:"fully_diluted"` // includes the unallocated pool
	PoolPctFD   int64         `json:"pool_pct_fully_diluted"`
	Classes     []ClassRow    `json:"classes"`
	Convertible int64         `json:"convertible_cents"` // principal of SAFEs and notes not yet converted
}

// Build builds the table. Holders with nothing are left out. Fully diluted counts shares, options, warrants and
// the unallocated pool; SAFEs and notes are excluded until they convert. Percentages use the largest-remainder
// rule so each column adds up to exactly 100%.
func Build(holders []Stakeholder, classes map[string]string, pools []Pool, secs []Security, evs []Event, asOf Date) Table {
	pos := Positions(secs, evs, asOf)
	t := Table{AsOf: asOf, Pools: PoolBalances(pools, pos, asOf)}
	rows := map[string]*Row{}
	order := []string{}
	nameOf := map[string]Stakeholder{}
	for _, h := range holders {
		nameOf[h.ID] = h
	}
	get := func(id string) *Row {
		if r, ok := rows[id]; ok {
			return r
		}
		h := nameOf[id]
		r := &Row{ID: id, Name: h.Name, Kind: h.Kind}
		rows[id] = r
		order = append(order, id)
		return r
	}
	byClass := map[string]int64{}
	for _, p := range pos {
		if p.Pending {
			continue
		}
		switch p.Kind {
		case "stock":
			if p.Outstanding > 0 {
				r := get(p.StakeholderID)
				r.Stock += p.Outstanding
				if p.Quantity > 0 {
					// vested share of what is still held
					r.VestedStock += mulDiv(p.Outstanding, min(p.Vested, p.Quantity), p.Quantity)
				}
				byClass[p.ClassID] += p.Outstanding
			}
		case "option":
			if p.Outstanding > 0 {
				get(p.StakeholderID).Options += p.Outstanding
			}
		case "warrant":
			if p.Outstanding > 0 {
				get(p.StakeholderID).Warrants += p.Outstanding
			}
		case "safe", "note":
			if !p.Converted {
				t.Convertible += p.Conv.PrincipalC
			}
		}
	}
	for _, b := range t.Pools {
		t.Pool += b.Unallocated
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := rows[order[i]], rows[order[j]]
		if a.Stock+a.Options+a.Warrants != b.Stock+b.Options+b.Warrants {
			return a.Stock+a.Options+a.Warrants > b.Stock+b.Options+b.Warrants
		}
		return a.Name < b.Name
	})
	var wOut, wFD []int64
	for _, id := range order {
		r := rows[id]
		r.FullyDiluted = r.Stock + r.Options + r.Warrants
		t.Outstanding += r.Stock
		t.FullyDil += r.FullyDiluted
		wOut = append(wOut, r.Stock)
		wFD = append(wFD, r.FullyDiluted)
	}
	t.FullyDil += t.Pool
	wFD = append(wFD, t.Pool)
	pOut, pFD := Percentages(wOut), Percentages(wFD)
	for i, id := range order {
		r := rows[id]
		r.PctOut, r.PctFD = pOut[i], pFD[i]
		t.Rows = append(t.Rows, *r)
	}
	t.PoolPctFD = pFD[len(pFD)-1]
	ids := make([]string, 0, len(byClass))
	for id := range byClass {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var wc []int64
	for _, id := range ids {
		wc = append(wc, byClass[id])
	}
	pc := Percentages(wc)
	for i, id := range ids {
		t.Classes = append(t.Classes, ClassRow{ID: id, Name: classes[id], Shares: byClass[id], PctOut: pc[i]})
	}
	sort.SliceStable(t.Classes, func(i, j int) bool { return t.Classes[i].Shares > t.Classes[j].Shares })
	if t.Rows == nil {
		t.Rows = []Row{}
	}
	return t
}

// mulDiv returns floor(a*b/c) without overflow.
func mulDiv(a, b, c int64) int64 {
	x := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	return x.Div(x, big.NewInt(c)).Int64()
}
