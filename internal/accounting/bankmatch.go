// and src/accounting/src/lib/banking.js (scoring of statement lines against transactions already recorded).

package accounting

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// MatchCandidate is a journal already in the books that moved a bank account.
type MatchCandidate struct {
	JournalID string  `json:"journal_id"`
	Date      string  `json:"date"`
	Memo      string  `json:"memo"`
	Contact   string  `json:"contact"`
	Amount    float64 `json:"amount"` // net debit-credit on the bank account: + money in, - money out
}

// BankMatch pairs one pending statement line with one journal.
type BankMatch struct {
	LineID  string   `json:"line_id"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
	Days    int      `json:"days"`
	MatchCandidate
}

// DefaultMatchDays is how many days a statement may lag the book date.
const DefaultMatchDays = 5

func matchTokens(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
		if len(w) >= 3 {
			out = append(out, w)
		}
	}
	return out
}

func daysBetween(a, b string) (int, bool) {
	ta, e1 := time.Parse("2006-01-02", a)
	tb, e2 := time.Parse("2006-01-02", b)
	if e1 != nil || e2 != nil {
		return 0, false
	}
	return int(tb.Sub(ta).Hours() / 24), true
}

// ScoreBankMatch rates a statement line against a journal. The amount must be exactly equal
// (to the paisa) and in the same direction; the statement may be 1 day earlier or tolDays later.
func ScoreBankMatch(l BankLine, c MatchCandidate, tolDays int) (score int, reasons []string, days int, ok bool) {
	if paise(l.Amount) != paise(c.Amount) || paise(l.Amount) == 0 {
		return 0, nil, 0, false
	}
	days, okd := daysBetween(c.Date, l.Date)
	if !okd || days < -1 || days > tolDays {
		return 0, nil, 0, false
	}
	score = 80
	reasons = []string{"Exact amount"}
	score += max(0, 15-absInt(days)*4)
	if days == 0 {
		reasons = append(reasons, "Same day")
	} else {
		reasons = append(reasons, plural(absInt(days), "day")+" apart")
	}
	lt := map[string]bool{}
	for _, t := range matchTokens(l.Description) {
		lt[t] = true
	}
	shared := 0
	for _, t := range matchTokens(c.Memo + " " + c.Contact) {
		if lt[t] {
			shared++
		}
	}
	if shared > 0 {
		score += min(5, shared*2)
		reasons = append(reasons, "Description matches")
	}
	return min(score, 100), reasons, days, true
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return strings.TrimSpace(strings.Join([]string{itoa(n), w + "s"}, " "))
}

// SuggestBankMatches pairs lines and journals one to one, strongest first.
func SuggestBankMatches(lines []BankLine, cands []MatchCandidate, tolDays int) []BankMatch {
	var all []BankMatch
	for _, l := range lines {
		if l.Status != "pending" {
			continue
		}
		for _, c := range cands {
			if s, r, d, ok := ScoreBankMatch(l, c, tolDays); ok {
				all = append(all, BankMatch{LineID: l.ID, Score: s, Reasons: r, Days: d, MatchCandidate: c})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		if absInt(all[i].Days) != absInt(all[j].Days) {
			return absInt(all[i].Days) < absInt(all[j].Days)
		}
		return all[i].JournalID < all[j].JournalID
	})
	usedL, usedJ := map[string]bool{}, map[string]bool{}
	out := []BankMatch{}
	for _, m := range all {
		if usedL[m.LineID] || usedJ[m.JournalID] {
			continue
		}
		usedL[m.LineID], usedJ[m.JournalID] = true, true
		out = append(out, m)
	}
	return out
}

// MatchCandidates lists journals that moved the bank account, are not voided, were not created
// from a statement line and are not already claimed by one.
func (r *Repository) MatchCandidates(ctx context.Context, orgID, accountID string) ([]MatchCandidate, error) {
	if !isUUID(accountID) {
		return nil, invalid("choose a bank account")
	}
	rows, err := r.pool.Query(ctx, `SELECT j.id::text, to_char(j.entry_date,'YYYY-MM-DD'), j.memo, j.contact_name,
		round(sum(l.debit - l.credit)*100)::bigint
		FROM adrient.acct_journals j JOIN adrient.acct_lines l ON l.journal_id=j.id
		WHERE j.org_id=$1 AND l.account_id=$2::uuid AND NOT j.voided AND j.source <> 'bank_line'
		  AND NOT EXISTS (SELECT 1 FROM adrient.acct_bank_lines b WHERE b.org_id=j.org_id AND b.journal_id=j.id)
		GROUP BY j.id, j.entry_date, j.memo, j.contact_name
		HAVING round(sum(l.debit - l.credit)*100) <> 0
		ORDER BY j.entry_date DESC LIMIT 3000`, orgID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MatchCandidate{}
	for rows.Next() {
		var c MatchCandidate
		var p int64
		if err := rows.Scan(&c.JournalID, &c.Date, &c.Memo, &c.Contact, &p); err != nil {
			return nil, err
		}
		c.Amount = rupees(p)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SuggestMatches proposes matches for every pending line of one bank account.
func (r *Repository) SuggestMatches(ctx context.Context, orgID, accountID string) ([]BankMatch, error) {
	if !isUUID(accountID) {
		return nil, invalid("choose a bank account")
	}
	lines, err := r.ListBankLines(ctx, orgID, accountID, "pending")
	if err != nil {
		return nil, err
	}
	cands, err := r.MatchCandidates(ctx, orgID, accountID)
	if err != nil {
		return nil, err
	}
	return SuggestBankMatches(lines, cands, DefaultMatchDays), nil
}

// MatchBankLine links a pending line to a journal that is already in the books. The journal's
// net movement on the bank account must equal the line amount exactly. No journal is created.
func (r *Repository) MatchBankLine(ctx context.Context, orgID, userID, lineID, journalID string) (BankLine, error) {
	if !isUUID(lineID) || !isUUID(journalID) {
		return BankLine{}, ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BankLine{}, err
	}
	defer tx.Rollback(ctx)
	var acct, status string
	var amt int64
	err = tx.QueryRow(ctx, `SELECT account_id::text, status, round(amount*100)::bigint FROM adrient.acct_bank_lines
		WHERE org_id=$1 AND id=$2::uuid FOR UPDATE`, orgID, lineID).Scan(&acct, &status, &amt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BankLine{}, ErrNotFound
	}
	if err != nil {
		return BankLine{}, err
	}
	if status != "pending" {
		return BankLine{}, invalid("only pending lines can be matched")
	}
	var net int64
	var voided bool
	var source string
	err = tx.QueryRow(ctx, `SELECT j.voided, j.source, coalesce(round(sum(l.debit - l.credit)*100),0)::bigint
		FROM adrient.acct_journals j LEFT JOIN adrient.acct_lines l ON l.journal_id=j.id AND l.account_id=$3::uuid
		WHERE j.org_id=$1 AND j.id=$2::uuid GROUP BY j.voided, j.source`, orgID, journalID, acct).Scan(&voided, &source, &net)
	if errors.Is(err, pgx.ErrNoRows) {
		return BankLine{}, ErrNotFound
	}
	if err != nil {
		return BankLine{}, err
	}
	if voided {
		return BankLine{}, invalid("that transaction is void")
	}
	if source == "bank_line" {
		return BankLine{}, invalid("that transaction was created from a statement line")
	}
	if net != amt {
		return BankLine{}, invalid("the amounts differ: statement %.2f, books %.2f", rupees(amt), rupees(net))
	}
	var taken int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.acct_bank_lines WHERE org_id=$1 AND journal_id=$2::uuid`, orgID, journalID).Scan(&taken); err != nil {
		return BankLine{}, err
	}
	if taken > 0 {
		return BankLine{}, invalid("that transaction is already matched to another statement line")
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.acct_bank_lines SET status='matched', journal_id=$3::uuid WHERE org_id=$1 AND id=$2::uuid`, orgID, lineID, journalID); err != nil {
		return BankLine{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BankLine{}, err
	}
	r.audit(ctx, orgID, userID, "match", "bank", lineID, "journal "+journalID)
	return r.getBankLine(ctx, orgID, lineID)
}
