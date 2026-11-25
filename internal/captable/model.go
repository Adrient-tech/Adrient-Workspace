package captable

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// TableOf returns the cap table on asOf (empty = today).
func (r *Repository) TableOf(ctx context.Context, org, asOf string) (Table, error) {
	if asOf == "" {
		asOf = r.today()
	}
	if !validDate(asOf) {
		return Table{}, knowkit.Invalid("as_of must be YYYY-MM-DD")
	}
	st, err := r.loadState(ctx, org)
	if err != nil {
		return Table{}, err
	}
	return Build(st.holders, st.classes, st.pools, st.secs, st.events, asOf), nil
}

// ---- dashboard ---------------------------------------------------------------------------------------------

// Upcoming is a dated thing to watch: a cliff, a full-vest date, an expiry, a note maturity.
type Upcoming struct {
	Date        string `json:"date"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Stakeholder string `json:"stakeholder"`
	Shares      int64  `json:"shares,omitempty"`
}

// Dashboard is the overview.
type Dashboard struct {
	Company     Company    `json:"company"`
	Table       Table      `json:"table"`
	Raised      int64      `json:"capital_raised"`
	LastRound   *RoundRec  `json:"last_round,omitempty"`
	ImpliedVal  int64      `json:"implied_valuation"` // last round price x fully diluted shares, in minor units
	Holders     int        `json:"holders"`
	Upcoming    []Upcoming `json:"upcoming"`
	Convertible int64      `json:"convertible_outstanding"`
}

// Dashboard builds the overview for today.
func (r *Repository) Dashboard(ctx context.Context, org string) (Dashboard, error) {
	var d Dashboard
	var err error
	if d.Company, err = r.GetCompany(ctx, org); err != nil {
		return d, err
	}
	today := r.today()
	st, err := r.loadState(ctx, org)
	if err != nil {
		return d, err
	}
	d.Table = Build(st.holders, st.classes, st.pools, st.secs, st.events, today)
	d.Holders = len(d.Table.Rows)
	d.Convertible = d.Table.Convertible
	rounds, err := r.Rounds(ctx, org)
	if err != nil {
		return d, err
	}
	for i := range rounds {
		d.Raised += rounds[i].Investment
		if rounds[i].Date <= today {
			x := rounds[i]
			d.LastRound = &x
		}
	}
	if d.LastRound != nil && d.LastRound.PricePerShare != "" {
		p := priceCents(d.LastRound.PricePerShare)
		d.ImpliedVal = RoundHalfUp(new(big.Rat).Mul(p, big.NewRat(d.Table.FullyDil, 1)))
	}
	horizon := addMonths(today, 6)
	for _, p := range Positions(st.secs, st.events, today) {
		if p.Pending || p.Outstanding == 0 && p.Kind != "note" && p.Kind != "safe" {
			continue
		}
		who := st.holderNm[p.StakeholderID]
		add := func(date, kind string, shares int64) {
			if date >= today && date <= horizon {
				d.Upcoming = append(d.Upcoming, Upcoming{Date: date, Kind: kind, Label: p.Label, Stakeholder: who, Shares: shares})
			}
		}
		v := p.Vesting
		if v.Months > 0 && (p.Kind == "option" || p.Kind == "stock") {
			if v.CliffMons > 0 {
				add(addMonths(v.Start, v.CliffMons), "cliff", v.Vested(p.Quantity, addMonths(v.Start, v.CliffMons)))
			}
			add(addMonths(v.Start, v.Months), "fully vested", p.Quantity)
		}
		if (p.Kind == "option" || p.Kind == "warrant") && p.Expires != "" {
			add(p.Expires, "expires", p.Outstanding)
		}
	}
	sort.Slice(d.Upcoming, func(i, j int) bool { return d.Upcoming[i].Date < d.Upcoming[j].Date })
	if d.Upcoming == nil {
		d.Upcoming = []Upcoming{}
	}
	return d, nil
}

// ---- round modeller ----------------------------------------------------------------------------------------

// Investor is a new investor in a round and the money they put in.
type Investor struct {
	StakeholderID string `json:"stakeholder_id"`
	Name          string `json:"name"`
	Amount        int64  `json:"amount"`
}

// RoundModelInput is the request to model (or close) a round. Amounts are minor units.
type RoundModelInput struct {
	Name         string     `json:"name"`
	Date         string     `json:"date"`
	PreMoney     int64      `json:"pre_money"`
	Investment   int64      `json:"investment"`
	PoolTargetBp int        `json:"pool_target_bp"`
	Convert      []string   `json:"convert"` // ids of the SAFEs and notes that convert; nil = all open ones
	Investors    []Investor `json:"investors"`
	ClassID      string     `json:"class_id"`
	Kind         string     `json:"kind"`
}

// InvestorShares is what an investor's money buys.
type InvestorShares struct {
	Investor
	Shares int64  `json:"shares"`
	Pct    string `json:"pct"`
}

// RoundModel is the modelled round with named investors.
type RoundModel struct {
	*RoundResult
	Investors   []InvestorShares `json:"investors"`
	Unallocated int64            `json:"unallocated_shares"` // new shares not bought by a named investor (whole-share rounding)
	Closing     []Closing        `json:"closing,omitempty"`
}

// Closing describes a record that closing the round creates.
type Closing struct {
	Kind   string `json:"kind"`
	Holder string `json:"holder"`
	Shares int64  `json:"shares"`
	Detail string `json:"detail"`
}

func (r *Repository) buildRound(ctx context.Context, org string, in RoundModelInput) (*RoundModel, *state, error) {
	if in.Date == "" {
		in.Date = r.today()
	}
	if !validDate(in.Date) {
		return nil, nil, knowkit.Invalid("the date must be YYYY-MM-DD")
	}
	st, err := r.loadState(ctx, org)
	if err != nil {
		return nil, nil, err
	}
	tb := Build(st.holders, st.classes, st.pools, st.secs, st.events, in.Date)
	var existing []Holding
	for _, row := range tb.Rows {
		if row.FullyDiluted > 0 {
			existing = append(existing, Holding{ID: row.ID, Name: row.Name, Shares: row.FullyDiluted})
		}
	}
	if tb.Pool > 0 {
		existing = append(existing, Holding{ID: "pool", Name: "Option pool (unallocated)", Shares: tb.Pool, Pool: true})
	}
	want := map[string]bool{}
	for _, id := range in.Convert {
		want[id] = true
	}
	var convs []Convertible
	for _, p := range Positions(st.secs, st.events, in.Date) {
		if (p.Kind != "safe" && p.Kind != "note") || p.Converted || p.Pending {
			continue
		}
		if in.Convert != nil && !want[p.ID] {
			continue
		}
		c := p.Conv
		c.ID = p.ID
		c.Name = p.Label + " (" + st.holderNm[p.StakeholderID] + ")"
		convs = append(convs, c)
	}
	res, err := ModelRound(RoundInput{PreMoneyC: in.PreMoney, InvestmentC: in.Investment, PoolTargetBp: in.PoolTargetBp, Existing: existing, Convertibles: convs, Date: in.Date})
	if err != nil {
		return nil, nil, knowkit.Invalid("%v", err)
	}
	m := &RoundModel{RoundResult: res, Investors: []InvestorShares{}}
	preRound := res.PostFD - res.NewShares
	var sold int64
	var sum int64
	for _, iv := range in.Investors {
		if iv.Amount < 0 {
			return nil, nil, knowkit.Invalid("an investment cannot be negative")
		}
		sum += iv.Amount
		n := new(big.Int).Mul(big.NewInt(iv.Amount), big.NewInt(preRound))
		n.Div(n, big.NewInt(in.PreMoney))
		sh := n.Int64()
		sold += sh
		pct := "0"
		if res.PostFD > 0 {
			pct = new(big.Rat).Quo(big.NewRat(sh*100, 1), big.NewRat(res.PostFD, 1)).FloatString(4)
		}
		if iv.Name == "" {
			iv.Name = st.holderNm[iv.StakeholderID]
		}
		m.Investors = append(m.Investors, InvestorShares{Investor: iv, Shares: sh, Pct: pct})
	}
	if len(in.Investors) > 0 && sum != in.Investment {
		return nil, nil, knowkit.Invalid("the investors' amounts add up to %d but the round raises %d", sum, in.Investment)
	}
	if len(in.Investors) > 0 {
		m.Unallocated = res.NewShares - sold
	}
	for i, c := range res.Converted {
		m.Closing = append(m.Closing, Closing{Kind: "conversion", Holder: convs[i].Name, Shares: c.Shares, Detail: "converts at " + c.PriceCents + " cents a share"})
	}
	for _, iv := range m.Investors {
		m.Closing = append(m.Closing, Closing{Kind: "issue", Holder: iv.Name, Shares: iv.Shares, Detail: "new shares for the money invested"})
	}
	if res.PoolTopUp > 0 {
		m.Closing = append(m.Closing, Closing{Kind: "pool", Holder: "Option pool", Shares: res.PoolTopUp, Detail: "a new pool top-up"})
	}
	return m, st, nil
}

// ModelRoundPreview models a round against today's cap table without saving anything.
func (r *Repository) ModelRoundPreview(ctx context.Context, org string, in RoundModelInput) (*RoundModel, error) {
	if len(in.Investors) > 50 {
		return nil, knowkit.Invalid("too many investors")
	}
	m, _, err := r.buildRound(ctx, org, in)
	return m, err
}

// CloseRound applies a modelled round: it records the round, issues shares to the named investors, converts the
// SAFEs and notes into shares of the round's class and adds the pool top-up, all in one transaction.
func (r *Repository) CloseRound(ctx context.Context, a knowkit.Actor, in RoundModelInput) (RoundRec, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || tooLong(in.Name, 100) {
		return RoundRec{}, knowkit.Invalid("name the round")
	}
	if !knowkit.IsUUID(in.ClassID) {
		return RoundRec{}, knowkit.Invalid("choose the share class the new shares belong to")
	}
	if in.Investment > 0 && len(in.Investors) == 0 {
		return RoundRec{}, knowkit.Invalid("name the investors and how much each puts in")
	}
	cls, err := r.class(ctx, a.Org, in.ClassID)
	if err != nil {
		return RoundRec{}, knowkit.Invalid("that share class does not exist")
	}
	m, st, err := r.buildRound(ctx, a.Org, in)
	if err != nil {
		return RoundRec{}, err
	}
	if in.Kind == "" {
		in.Kind = "priced"
	}
	for _, iv := range m.Investors {
		if !knowkit.IsUUID(iv.StakeholderID) {
			return RoundRec{}, knowkit.Invalid("each investor must be a stakeholder; add them first")
		}
		if _, ok := st.holderNm[iv.StakeholderID]; !ok {
			return RoundRec{}, knowkit.Invalid("an investor is not a stakeholder of this company")
		}
	}
	issued := int64(0)
	for _, iv := range m.Investors {
		issued += iv.Shares
	}
	for _, c := range m.Converted {
		issued += c.Shares
	}
	if cls.Authorized > 0 && cls.Issued+issued > cls.Authorized {
		return RoundRec{}, knowkit.Invalid("class %s allows %d shares; closing would issue %d on top of %d", cls.Name, cls.Authorized, issued, cls.Issued)
	}
	ppsC, _ := new(big.Rat).SetString(m.PricePerShareCents)
	pps := new(big.Rat).Quo(ppsC, big.NewRat(100, 1)).FloatString(8)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RoundRec{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var roundID string
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.captable_rounds (org_id, name, kind, round_date, pre_money, investment, price_per_share, class_id, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7::numeric,$8,$9) RETURNING id::text`, a.Org, in.Name, in.Kind, in.Date, in.PreMoney, in.Investment, pps, in.ClassID, "Closed from the round modeller").Scan(&roundID); err != nil {
		return RoundRec{}, err
	}
	if cls.IssuePrice == "0" || cls.IssuePrice == "" {
		if _, err := tx.Exec(ctx, `UPDATE adrient.captable_classes SET issue_price=$2::numeric WHERE id=$1`, in.ClassID, pps); err != nil {
			return RoundRec{}, err
		}
	}
	for _, iv := range m.Investors {
		if iv.Shares <= 0 {
			continue
		}
		if _, err := r.insertSecurity(ctx, tx, a, SecurityInput{Kind: "stock", StakeholderID: iv.StakeholderID, ClassID: in.ClassID, RoundID: roundID, Quantity: iv.Shares, Price: pps, IssueDate: in.Date, VestFreq: "monthly"}, cls.Name, ""); err != nil {
			return RoundRec{}, err
		}
	}
	idx := 0
	for _, p := range Positions(st.secs, st.events, in.Date) {
		if (p.Kind != "safe" && p.Kind != "note") || p.Converted || p.Pending {
			continue
		}
		if idx >= len(m.Converted) || m.Converted[idx].ID != p.ID {
			continue
		}
		c := m.Converted[idx]
		idx++
		cp, _ := new(big.Rat).SetString(c.PriceCents)
		cpps := new(big.Rat).Quo(cp, big.NewRat(100, 1)).FloatString(8)
		nid, err := r.insertSecurity(ctx, tx, a, SecurityInput{Kind: "stock", StakeholderID: p.StakeholderID, ClassID: in.ClassID, RoundID: roundID, Quantity: c.Shares, Price: cpps, IssueDate: in.Date, VestFreq: "monthly"}, cls.Name, p.ID)
		if err != nil {
			return RoundRec{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.captable_events (org_id, kind, security_id, event_date, quantity, new_security_id, price, note, by_user)
			VALUES ($1,'convert',$2,$3,$4,$5,$6::numeric,$7,$8)`, a.Org, p.ID, in.Date, c.Shares, nid, cpps, "Converted in "+in.Name, a.User); err != nil {
			return RoundRec{}, err
		}
	}
	if m.PoolTopUp > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.captable_pools (org_id, name, size, adopted, notes) VALUES ($1,$2,$3,$4,$5)`, a.Org, in.Name+" pool top-up", m.PoolTopUp, in.Date, "Added when "+in.Name+" closed"); err != nil {
			return RoundRec{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RoundRec{}, err
	}
	r.audit(ctx, "round_close", "captable_round", roundID)
	return r.round(ctx, a.Org, roundID)
}

// ---- exit scenarios ---------------------------------------------------------------------------------------------

// ExitInput asks for payouts at one or more exit values.
type ExitInput struct {
	Exits    []int64 `json:"exits"`    // exit values in minor units; up to 12
	AsOf     string  `json:"as_of"`    // the cap table date (default today)
	Unvested string  `json:"unvested"` // participate (default) | forfeit: do unvested options take part?
}

// HolderPayout is what one stakeholder receives.
type HolderPayout struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Payout int64  `json:"payout"`
	Pct    string `json:"pct"` // of the exit value, 4 decimals
}

// ExitOutcome is one exit value's result.
type ExitOutcome struct {
	Exit    int64          `json:"exit"`
	Payees  []Payee        `json:"payees"`
	Holders []HolderPayout `json:"holders"`
	Common  string         `json:"common_per_share_cents"`
	Total   int64          `json:"total"`
	Notes   []string       `json:"notes,omitempty"`
}

// ExitModel runs the liquidation waterfall at each exit value. Notes are repaid first (principal and interest to
// the date), then the money back of SAFEs, then preferences by seniority, then the rest per share. SAFEs and notes
// are treated as debt here; they are not converted at an exit.
func (r *Repository) ExitModel(ctx context.Context, org string, in ExitInput) ([]ExitOutcome, error) {
	if len(in.Exits) == 0 || len(in.Exits) > 12 {
		return nil, knowkit.Invalid("give between 1 and 12 exit values")
	}
	for _, e := range in.Exits {
		if e < 0 || e > 1e15 {
			return nil, knowkit.Invalid("an exit value is out of range")
		}
	}
	if in.AsOf == "" {
		in.AsOf = r.today()
	}
	if !validDate(in.AsOf) {
		return nil, knowkit.Invalid("as_of must be YYYY-MM-DD")
	}
	if in.Unvested == "" {
		in.Unvested = "participate"
	}
	if !knowkit.OneOf(in.Unvested, "participate", "forfeit") {
		return nil, knowkit.Invalid("unvested must be participate or forfeit")
	}
	st, err := r.loadState(ctx, org)
	if err != nil {
		return nil, err
	}
	classes, err := r.Classes(ctx, org)
	if err != nil {
		return nil, err
	}
	pos := Positions(st.secs, st.events, in.AsOf)
	holderShares := map[string]map[string]int64{} // class -> holder -> shares
	invested := map[string]int64{}
	shares := map[string]int64{}
	var claims []Claim
	claimHolder := map[string]string{}
	var opts []OptionPool
	optHolder := map[string]string{}
	for _, p := range pos {
		if p.Pending {
			continue
		}
		switch p.Kind {
		case "stock":
			if p.Outstanding > 0 {
				if holderShares[p.ClassID] == nil {
					holderShares[p.ClassID] = map[string]int64{}
				}
				holderShares[p.ClassID][p.StakeholderID] += p.Outstanding
				shares[p.ClassID] += p.Outstanding
				invested[p.ClassID] += RoundHalfUp(new(big.Rat).Mul(p.PriceC, big.NewRat(p.Outstanding, 1)))
			}
		case "option", "warrant":
			q := p.Outstanding
			if in.Unvested == "forfeit" && q > p.Exercisable {
				q = p.Exercisable
			}
			if q > 0 {
				opts = append(opts, OptionPool{ID: p.ID, Name: p.Label + " (" + st.holderNm[p.StakeholderID] + ")", Shares: q, StrikeC: p.PriceC})
				optHolder[p.ID] = p.StakeholderID
			}
		case "safe":
			if !p.Converted {
				claims = append(claims, Claim{ID: p.ID, Name: p.Label + " (" + st.holderNm[p.StakeholderID] + ") money back", AmountC: p.Conv.PrincipalC, Rank: 1})
				claimHolder[p.ID] = p.StakeholderID
			}
		case "note":
			if !p.Converted {
				claims = append(claims, Claim{ID: p.ID, Name: p.Label + " (" + st.holderNm[p.StakeholderID] + ") principal and interest", AmountC: p.Conv.Value(in.AsOf), Rank: 2})
				claimHolder[p.ID] = p.StakeholderID
			}
		}
	}
	var terms []ClassTerms
	for _, c := range classes {
		if shares[c.ID] == 0 {
			continue
		}
		terms = append(terms, ClassTerms{ID: c.ID, Name: c.Name, Preferred: c.Kind == "preferred", Shares: shares[c.ID], InvestedC: invested[c.ID],
			PrefMultipleMilli: int64(c.PrefMultipleMilli), Participating: c.Participating, Seniority: c.Seniority, ConvertRatioMilli: int64(c.ConvertRatioMilli)})
	}
	var out []ExitOutcome
	for _, exit := range in.Exits {
		w, err := Waterfall(exit, claims, terms, opts)
		if err != nil {
			return nil, knowkit.Invalid("%v", err)
		}
		per := map[string]int64{}
		for _, p := range w.Payees {
			switch p.Kind {
			case "claim":
				per[claimHolder[p.ID]] += p.PayoutC
			case "options":
				per[optHolder[p.ID]] += p.PayoutC
			case "class":
				hs := holderShares[p.ID]
				ids := make([]string, 0, len(hs))
				for id := range hs {
					ids = append(ids, id)
				}
				sort.Strings(ids)
				wt := make([]*big.Rat, len(ids))
				for i, id := range ids {
					wt[i] = big.NewRat(hs[id], 1)
				}
				for i, c := range SplitCents(p.PayoutC, wt) {
					per[ids[i]] += c
				}
			}
		}
		oc := ExitOutcome{Exit: exit, Payees: w.Payees, Common: w.CommonPerShare, Total: w.TotalC, Notes: w.Notes}
		holderKind := map[string]string{}
		for _, h := range st.holders {
			holderKind[h.ID] = h.Kind
		}
		for id, v := range per {
			pct := "0"
			if exit > 0 {
				pct = new(big.Rat).Quo(big.NewRat(v*100, 1), big.NewRat(exit, 1)).FloatString(4)
			}
			oc.Holders = append(oc.Holders, HolderPayout{ID: id, Name: st.holderNm[id], Kind: holderKind[id], Payout: v, Pct: pct})
		}
		sort.Slice(oc.Holders, func(i, j int) bool {
			if oc.Holders[i].Payout != oc.Holders[j].Payout {
				return oc.Holders[i].Payout > oc.Holders[j].Payout
			}
			return oc.Holders[i].Name < oc.Holders[j].Name
		})
		if oc.Holders == nil {
			oc.Holders = []HolderPayout{}
		}
		out = append(out, oc)
	}
	return out, nil
}

// ---- saved scenarios -------------------------------------------------------------------------------------------

// Scenario is a saved round or exit model.
type Scenario struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind"`
	Name    string          `json:"name"`
	Params  json.RawMessage `json:"params"`
	Created time.Time       `json:"created_at"`
}

// Scenarios lists saved scenarios.
func (r *Repository) Scenarios(ctx context.Context, org, kind string) ([]Scenario, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, kind, name, params, created_at FROM adrient.captable_scenarios WHERE org_id=$1 AND ($2='' OR kind=$2) ORDER BY created_at DESC LIMIT 200`, org, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Scenario{}
	for rows.Next() {
		var s Scenario
		if err := rows.Scan(&s.ID, &s.Kind, &s.Name, &s.Params, &s.Created); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveScenario stores a scenario.
func (r *Repository) SaveScenario(ctx context.Context, a knowkit.Actor, kind, name string, params json.RawMessage) (Scenario, error) {
	name = strings.TrimSpace(name)
	if !knowkit.OneOf(kind, "round", "exit") || name == "" || tooLong(name, 100) || len(params) > 64<<10 || !json.Valid(params) {
		return Scenario{}, knowkit.Invalid("a scenario needs a kind (round or exit), a name and valid parameters")
	}
	var s Scenario
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.captable_scenarios (org_id, kind, name, params, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id::text, kind, name, params, created_at`,
		a.Org, kind, name, []byte(params), a.User).Scan(&s.ID, &s.Kind, &s.Name, &s.Params, &s.Created)
	r.audit(ctx, "scenario_save", "captable_scenario", s.ID)
	return s, err
}

// DeleteScenario removes a scenario.
func (r *Repository) DeleteScenario(ctx context.Context, a knowkit.Actor, id string) error {
	if !knowkit.IsUUID(id) {
		return knowkit.ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.captable_scenarios WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "scenario_delete", "captable_scenario", id)
	return err
}

// ---- statements and exports ---------------------------------------------------------------------------------------

// Statement is one holder's equity statement.
type Statement struct {
	Holder     StakeholderRec `json:"holder"`
	Securities []SecurityRec  `json:"securities"`
	Stock      int64          `json:"stock"`
	Options    int64          `json:"options"`
	Vested     int64          `json:"vested_options"`
	PctOut     int64          `json:"pct_outstanding"`
	PctFD      int64          `json:"pct_fully_diluted"`
}

// HolderStatement builds a holder's statement (their securities, events and ownership).
func (r *Repository) HolderStatement(ctx context.Context, org, id string) (Statement, error) {
	h, err := r.stakeholder(ctx, org, id)
	if err != nil {
		return Statement{}, err
	}
	s := Statement{Holder: h}
	if s.Securities, err = r.Securities(ctx, org, id, "", ""); err != nil {
		return s, err
	}
	for i := range s.Securities {
		full, err := r.Security(ctx, org, s.Securities[i].ID)
		if err != nil {
			return s, err
		}
		s.Securities[i].Events, s.Securities[i].Schedule = full.Events, full.Schedule
	}
	tb, err := r.TableOf(ctx, org, "")
	if err != nil {
		return s, err
	}
	for _, row := range tb.Rows {
		if row.ID == id {
			s.Stock, s.Options, s.PctOut, s.PctFD = row.Stock, row.Options, row.PctOut, row.PctFD
		}
	}
	for _, sec := range s.Securities {
		if sec.Kind == "option" {
			s.Vested += sec.Exercisable
		}
	}
	return s, nil
}

// Pct formats a PctScale value as a percentage with 4 decimals.
func Pct(v int64) string {
	return fmt.Sprintf("%d.%04d", v/10000, v%10000)
}
