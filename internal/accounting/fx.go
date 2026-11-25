package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNoRate means no exchange rate is on file for a currency on or before a date.
var ErrNoRate = errors.New("no exchange rate")

// Rate is one row of the exchange-rate table: base currency per 1 unit of Currency.
type Rate struct {
	ID       string  `json:"id"`
	Currency string  `json:"currency"`
	Date     string  `json:"date"`
	Rate     float64 `json:"rate"`
	Source   string  `json:"source"`
}

func (r *Repository) ListRates(ctx context.Context, orgID, currency string) ([]Rate, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, currency, to_char(rate_date,'YYYY-MM-DD'), rate::float8, source FROM adrient.acct_fx_rates
		WHERE org_id=$1 AND ($2='' OR currency=$2) ORDER BY rate_date DESC, currency LIMIT 1000`, orgID, strings.ToUpper(currency))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rate{}
	for rows.Next() {
		var x Rate
		if err := rows.Scan(&x.ID, &x.Currency, &x.Date, &x.Rate, &x.Source); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SaveRate stores (or replaces) the rate for a currency on a date.
func (r *Repository) SaveRate(ctx context.Context, orgID, userID string, x Rate) (Rate, error) {
	x.Currency = strings.ToUpper(strings.TrimSpace(x.Currency))
	if len(x.Currency) != 3 {
		return Rate{}, invalid("currency must be a 3-letter code")
	}
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return Rate{}, err
	}
	if x.Currency == st.Currency {
		return Rate{}, invalid("%s is the base currency; its rate is always 1", x.Currency)
	}
	if x.Rate <= 0 || math.IsNaN(x.Rate) || math.IsInf(x.Rate, 0) || x.Rate > 1e9 {
		return Rate{}, invalid("rate must be a positive number")
	}
	d, err := validDate(x.Date)
	if err != nil {
		return Rate{}, err
	}
	if x.Source != "fetched" {
		x.Source = "manual"
	}
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_fx_rates (org_id, currency, rate_date, rate, source) VALUES ($1,$2,$3::date,$4,$5)
		ON CONFLICT (org_id, currency, rate_date) DO UPDATE SET rate=EXCLUDED.rate, source=EXCLUDED.source RETURNING id::text`,
		orgID, x.Currency, d, x.Rate, x.Source).Scan(&x.ID)
	if err != nil {
		return Rate{}, err
	}
	x.Date = d
	r.audit(ctx, orgID, userID, "save", "fx_rate", x.ID, fmt.Sprintf("%s %s %.6f", x.Currency, d, x.Rate))
	return x, nil
}

func (r *Repository) DeleteRate(ctx context.Context, orgID, userID, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.acct_fx_rates WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.audit(ctx, orgID, userID, "delete", "fx_rate", id, "")
	return nil
}

// RateOn returns the rate in force on date: the latest row on or before it.
// The base currency (or an empty one) is always 1.
func (r *Repository) RateOn(ctx context.Context, orgID, currency, date string) (float64, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return 0, err
	}
	if cur == "" || cur == st.Currency {
		return 1, nil
	}
	var rate float64
	err = r.pool.QueryRow(ctx, `SELECT rate::float8 FROM adrient.acct_fx_rates WHERE org_id=$1 AND currency=$2 AND rate_date <= $3::date
		ORDER BY rate_date DESC LIMIT 1`, orgID, cur, date).Scan(&rate)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w for %s on or before %s: add one under Currencies", ErrNoRate, cur, date)
	}
	return rate, err
}

// FetchRates pulls today's rates for every currency already used by the org
// (from frankfurter.app, free and keyless) and stores them as source "fetched".
// It never overwrites a manual rate for the same day. Returns how many were stored.
func (r *Repository) FetchRates(ctx context.Context, orgID, userID string, client *http.Client) (int, error) {
	st, err := r.GetSettings(ctx, orgID)
	if err != nil {
		return 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT currency FROM (
		SELECT currency FROM adrient.acct_fx_rates WHERE org_id=$1
		UNION SELECT currency FROM adrient.acct_journals WHERE org_id=$1 AND currency<>''
		UNION SELECT currency FROM adrient.acct_bills WHERE org_id=$1 AND currency<>'') c WHERE currency<>$2`, orgID, st.Currency)
	if err != nil {
		return 0, err
	}
	var curs []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err == nil {
			curs = append(curs, c)
		}
	}
	rows.Close()
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	n := 0
	for _, c := range curs {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.frankfurter.app/latest?from="+c+"&to="+st.Currency, nil)
		resp, err := client.Do(req)
		if err != nil {
			return n, fmt.Errorf("fetch %s: %w", c, err)
		}
		var body struct {
			Date  string             `json:"date"`
			Rates map[string]float64 `json:"rates"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil || body.Rates[st.Currency] <= 0 {
			continue
		}
		tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.acct_fx_rates (org_id, currency, rate_date, rate, source) VALUES ($1,$2,$3::date,$4,'fetched')
			ON CONFLICT (org_id, currency, rate_date) DO NOTHING`, orgID, c, body.Date, body.Rates[st.Currency])
		if err == nil && tag.RowsAffected() > 0 {
			n++
		}
	}
	r.audit(ctx, orgID, userID, "fetch", "fx_rate", "", fmt.Sprintf("%d rates", n))
	return n, nil
}

// ---- pure conversion (unit-tested) ----

// toBase converts a journal whose lines are in a foreign currency into base
// currency at rate. Original amounts are kept in FcyDebit/FcyCredit. Rounding
// residue is added to the largest line on the lighter side so the journal balances exactly.
func toBase(lines []Line, rate float64) []Line {
	out := make([]Line, len(lines))
	var d, c int64
	for i, l := range lines {
		l.FcyDebit, l.FcyCredit = l.Debit, l.Credit
		l.Debit = rupees(int64(math.Round(float64(paise(l.Debit)) * rate)))
		l.Credit = rupees(int64(math.Round(float64(paise(l.Credit)) * rate)))
		d += paise(l.Debit)
		c += paise(l.Credit)
		out[i] = l
	}
	diff := d - c
	if diff == 0 {
		return out
	}
	// find the largest line on the side that is short
	best := -1
	for i, l := range out {
		if diff > 0 && paise(l.Credit) > 0 && (best < 0 || paise(l.Credit) > paise(out[best].Credit)) {
			best = i
		}
		if diff < 0 && paise(l.Debit) > 0 && (best < 0 || paise(l.Debit) > paise(out[best].Debit)) {
			best = i
		}
	}
	if best >= 0 {
		if diff > 0 {
			out[best].Credit = rupees(paise(out[best].Credit) + diff)
		} else {
			out[best].Debit = rupees(paise(out[best].Debit) - diff)
		}
	}
	return out
}

// settlementLines builds a payment against a foreign-currency receivable or payable.
// amount is in the document currency; docRate is the rate the document was booked
// at, payRate the rate on the payment date. receipt=true for money received from a
// customer (Dr bank, Cr AR), false for money paid to a vendor (Dr AP, Cr bank).
// The difference between the two base values is the realised exchange gain or loss.
func settlementLines(bankAcct, controlAcct, fxAcct string, amount, docRate, payRate float64, receipt bool, memo string) []Line {
	a := paise(amount)
	ctlBase := int64(math.Round(float64(a) * docRate))
	bankBase := int64(math.Round(float64(a) * payRate))
	mk := func(acct string, debit bool, base, fcy int64) Line {
		l := Line{AccountID: acct, Memo: memo}
		if debit {
			l.Debit, l.FcyDebit = rupees(base), rupees(fcy)
		} else {
			l.Credit, l.FcyCredit = rupees(base), rupees(fcy)
		}
		return l
	}
	var ls []Line
	if receipt {
		ls = []Line{mk(bankAcct, true, bankBase, a), mk(controlAcct, false, ctlBase, a)}
		// bank received more base than the receivable carried => gain (credit)
		if g := bankBase - ctlBase; g > 0 {
			ls = append(ls, Line{AccountID: fxAcct, Credit: rupees(g), Memo: "Realised exchange gain"})
		} else if g < 0 {
			ls = append(ls, Line{AccountID: fxAcct, Debit: rupees(-g), Memo: "Realised exchange loss"})
		}
	} else {
		ls = []Line{mk(controlAcct, true, ctlBase, a), mk(bankAcct, false, bankBase, a)}
		// paid more base than the payable carried => loss (debit)
		if g := bankBase - ctlBase; g > 0 {
			ls = append(ls, Line{AccountID: fxAcct, Debit: rupees(g), Memo: "Realised exchange loss"})
		} else if g < 0 {
			ls = append(ls, Line{AccountID: fxAcct, Credit: rupees(-g), Memo: "Realised exchange gain"})
		}
	}
	return ls
}

// ---- unrealised revaluation ----

// RevalResult lists what a revaluation posted.
type RevalResult struct {
	Date    string       `json:"date"`
	Entries []RevalEntry `json:"entries"`
}

// RevalEntry is the adjustment for one currency and control account.
type RevalEntry struct {
	Currency   string  `json:"currency"`
	Account    string  `json:"account"`
	ForeignBal float64 `json:"foreign_balance"`
	Carrying   float64 `json:"carrying"`
	Revalued   float64 `json:"revalued"`
	Adjustment float64 `json:"adjustment"`
	Rate       float64 `json:"rate"`
}

// Revalue posts unrealised gain/loss entries for open foreign-currency receivables
// and payables as of date, and reverses them the next day so realised gains are
// always measured against the original booking rate. Re-running for a date
// replaces that date's entries (idempotent).
func (r *Repository) Revalue(ctx context.Context, orgID, userID, date string) (RevalResult, error) {
	date, err := validDate(date)
	if err != nil {
		return RevalResult{}, err
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return RevalResult{}, err
	}
	keys, err := r.accountKeys(ctx, r.pool, orgID)
	if err != nil {
		return RevalResult{}, err
	}
	res := RevalResult{Date: date, Entries: []RevalEntry{}}
	next, _ := time.Parse("2006-01-02", date)
	nextDay := next.AddDate(0, 0, 1).Format("2006-01-02")
	for _, ctl := range []string{"ar", "ap"} {
		acct := keys[ctl]
		rows, err := r.pool.Query(ctx, `SELECT j.currency, sum(l.fcy_debit - l.fcy_credit)::float8, sum(l.debit - l.credit)::float8
			FROM adrient.acct_lines l JOIN adrient.acct_journals j ON j.id=l.journal_id
			WHERE l.org_id=$1 AND l.account_id=$2::uuid AND NOT j.voided AND j.currency<>'' AND j.entry_date <= $3::date
			  AND j.source NOT IN ('revaluation')
			GROUP BY j.currency ORDER BY j.currency`, orgID, acct, date)
		if err != nil {
			return res, err
		}
		type agg struct {
			cur        string
			fcy, carry float64
		}
		var list []agg
		for rows.Next() {
			var a agg
			if err := rows.Scan(&a.cur, &a.fcy, &a.carry); err != nil {
				rows.Close()
				return res, err
			}
			list = append(list, a)
		}
		rows.Close()
		sort.Slice(list, func(i, j int) bool { return list[i].cur < list[j].cur })
		for _, a := range list {
			id1 := fmt.Sprintf("reval:%s:%s:%s", date, a.cur, ctl)
			id2 := id1 + ":rev"
			rate, err := r.RateOn(ctx, orgID, a.cur, date)
			if err != nil {
				return res, err
			}
			revalued := rupees(paise(a.fcy * rate))
			adj := rupees(paise(revalued) - paise(a.carry))
			e := RevalEntry{Currency: a.cur, Account: ctl, ForeignBal: rupees(paise(a.fcy)), Carrying: rupees(paise(a.carry)), Revalued: revalued, Adjustment: adj, Rate: rate}
			res.Entries = append(res.Entries, e)
			if paise(adj) == 0 {
				_, _ = r.voidBySource(ctx, orgID, userID, SourceRevaluation, id1)
				_, _ = r.voidBySource(ctx, orgID, userID, SourceRevaluation, id2)
				continue
			}
			// asset (AR): positive adjustment is a gain; liability (AP carried as credit): the sign of carry is already negative
			up, down := Line{AccountID: acct}, Line{AccountID: keys["fx_unrealised"]}
			if adj > 0 {
				up.Debit, down.Credit = adj, adj
			} else {
				up.Credit, down.Debit = -adj, -adj
			}
			memo := fmt.Sprintf("Unrealised exchange revaluation %s %s at %.6f", strings.ToUpper(ctl), a.cur, rate)
			post := func(sid, d string, a1, a2 Line, m string) error {
				_, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: d, Memo: m, Kind: "journal", Source: SourceRevaluation, SourceID: sid,
					SourceHash: hashOf(d, paise(a1.Debit), paise(a1.Credit), a.cur), Currency: a.cur, Rate: rate, Lines: []Line{a1, a2}})
				return err
			}
			if err := post(id1, date, up, down, memo); err != nil {
				return res, err
			}
			rv1, rv2 := up, down
			rv1.Debit, rv1.Credit = up.Credit, up.Debit
			rv2.Debit, rv2.Credit = down.Credit, down.Debit
			if err := post(id2, nextDay, rv1, rv2, "Reversal of "+memo); err != nil {
				return res, err
			}
		}
	}
	r.audit(ctx, orgID, userID, "revalue", "fx", date, fmt.Sprintf("%d entries", len(res.Entries)))
	return res, nil
}
