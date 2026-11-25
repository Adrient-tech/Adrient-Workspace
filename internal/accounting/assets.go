package accounting

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Asset is a fixed asset with its depreciation settings.
type Asset struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Tag              string  `json:"tag"`
	Category         string  `json:"category"`
	PurchaseDate     string  `json:"purchase_date"`
	DepStart         string  `json:"dep_start"`
	Cost             float64 `json:"cost"`
	Salvage          float64 `json:"salvage"`
	LifeMonths       int     `json:"life_months"`
	Method           string  `json:"method"` // slm | wdv
	WDVRate          float64 `json:"wdv_rate"`
	AssetAccountID   string  `json:"asset_account_id"`
	AccumAccountID   string  `json:"accum_account_id"`
	ExpenseAccountID string  `json:"expense_account_id"`
	Class            string  `json:"class"`
	Status           string  `json:"status"`
	DisposedOn       string  `json:"disposed_on"`
	Proceeds         float64 `json:"proceeds"`
	// Derived
	Accumulated float64 `json:"accumulated"`
	BookValue   float64 `json:"book_value"`
	// Input only: post the purchase (Dr asset, Cr this account, usually bank or payables).
	PaidFromID string `json:"paid_from_id,omitempty"`
}

// DepRow is one month of a depreciation schedule (amounts in rupees).
type DepRow struct {
	Period  string  `json:"period"` // YYYY-MM
	Opening float64 `json:"opening"`
	Dep     float64 `json:"depreciation"`
	Accum   float64 `json:"accumulated"`
	Closing float64 `json:"closing"`
	Posted  bool    `json:"posted"`
}

// DepSchedule computes the monthly schedule. SLM spreads (cost - salvage) evenly over the
// life, the last month absorbing the rounding remainder. WDV charges wdv_rate/12 percent of
// the opening book value each month, never taking the book value below salvage and
// finishing at the end of the life. Depreciation stops after the disposal month.
func DepSchedule(cost, salvage float64, lifeMonths int, method string, wdvRate float64, start string, disposedOn string) []DepRow {
	t, err := time.Parse("2006-01-02", start)
	if err != nil || lifeMonths <= 0 {
		return nil
	}
	c, s := paise(cost), paise(salvage)
	if c <= 0 || s < 0 || s >= c {
		return nil
	}
	var stopAt string
	if disposedOn != "" {
		stopAt = disposedOn[:7]
	}
	base := c - s
	out := []DepRow{}
	var accum int64
	book := c
	for i := 0; i < lifeMonths; i++ {
		period := time.Date(t.Year(), t.Month()+time.Month(i), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
		if stopAt != "" && period > stopAt {
			break
		}
		var d int64
		switch method {
		case "wdv":
			d = int64(math.Round(float64(book) * wdvRate / 1200))
			if i == lifeMonths-1 || book-d < s {
				d = book - s
			}
		default:
			d = base / int64(lifeMonths)
			if i == lifeMonths-1 {
				d = base - accum
			}
		}
		if d < 0 {
			d = 0
		}
		if book-d < s {
			d = book - s
		}
		row := DepRow{Period: period, Opening: rupees(book), Dep: rupees(d)}
		accum += d
		book -= d
		row.Accum, row.Closing = rupees(accum), rupees(book)
		out = append(out, row)
		if book <= s {
			break
		}
	}
	return out
}

func validAsset(a *Asset) error {
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return invalid("asset name is required")
	}
	var err error
	if a.PurchaseDate, err = validDate(a.PurchaseDate); err != nil {
		return err
	}
	if a.DepStart == "" {
		a.DepStart = a.PurchaseDate
	}
	if a.DepStart, err = validDate(a.DepStart); err != nil {
		return err
	}
	if paise(a.Cost) <= 0 {
		return invalid("cost must be greater than zero")
	}
	if a.Salvage < 0 || paise(a.Salvage) >= paise(a.Cost) {
		return invalid("salvage value must be below the cost")
	}
	if a.LifeMonths < 1 || a.LifeMonths > 1200 {
		return invalid("useful life must be between 1 and 1200 months")
	}
	switch a.Method {
	case "", "slm":
		a.Method = "slm"
	case "wdv":
		if a.WDVRate <= 0 || a.WDVRate > 100 {
			return invalid("a written-down-value asset needs an annual rate between 0 and 100")
		}
	default:
		return invalid("method must be slm or wdv")
	}
	return nil
}

const assetCols = `id::text, name, tag, category, to_char(purchase_date,'YYYY-MM-DD'), to_char(dep_start,'YYYY-MM-DD'), cost::float8, salvage::float8, life_months, method,
	wdv_rate::float8, asset_account_id::text, accum_account_id::text, expense_account_id::text, class, status, coalesce(to_char(disposed_on,'YYYY-MM-DD'),''), proceeds::float8,
	coalesce((SELECT sum(amount) FROM adrient.acct_asset_dep d WHERE d.asset_id=a.id),0)::float8`

func scanAsset(row pgx.Row) (Asset, error) {
	var a Asset
	err := row.Scan(&a.ID, &a.Name, &a.Tag, &a.Category, &a.PurchaseDate, &a.DepStart, &a.Cost, &a.Salvage, &a.LifeMonths, &a.Method, &a.WDVRate,
		&a.AssetAccountID, &a.AccumAccountID, &a.ExpenseAccountID, &a.Class, &a.Status, &a.DisposedOn, &a.Proceeds, &a.Accumulated)
	a.Accumulated = rupees(paise(a.Accumulated))
	a.BookValue = rupees(paise(a.Cost) - paise(a.Accumulated))
	return a, err
}

func (r *Repository) GetAsset(ctx context.Context, orgID, id string) (Asset, error) {
	if !isUUID(id) {
		return Asset{}, ErrNotFound
	}
	a, err := scanAsset(r.pool.QueryRow(ctx, `SELECT `+assetCols+` FROM adrient.acct_assets a WHERE org_id=$1 AND id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (r *Repository) ListAssets(ctx context.Context, orgID string) ([]Asset, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+assetCols+` FROM adrient.acct_assets a WHERE org_id=$1 ORDER BY purchase_date DESC, name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateAsset registers an asset. When PaidFromID is set the purchase is posted as well.
func (r *Repository) CreateAsset(ctx context.Context, orgID, userID string, a Asset) (Asset, error) {
	if err := validAsset(&a); err != nil {
		return a, err
	}
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		return a, err
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	a.AssetAccountID = orDefault(a.AssetAccountID, keys["fixed_assets"])
	a.AccumAccountID = orDefault(a.AccumAccountID, keys["accum_depreciation"])
	a.ExpenseAccountID = orDefault(a.ExpenseAccountID, keys["depreciation_expense"])
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_assets (org_id, name, tag, category, purchase_date, dep_start, cost, salvage, life_months, method, wdv_rate,
		asset_account_id, accum_account_id, expense_account_id, class) VALUES ($1,$2,$3,$4,$5::date,$6::date,$7,$8,$9,$10,$11,$12::uuid,$13::uuid,$14::uuid,$15) RETURNING id::text`,
		orgID, a.Name, a.Tag, a.Category, a.PurchaseDate, a.DepStart, rupees(paise(a.Cost)), rupees(paise(a.Salvage)), a.LifeMonths, a.Method, a.WDVRate,
		a.AssetAccountID, a.AccumAccountID, a.ExpenseAccountID, strings.TrimSpace(a.Class)).Scan(&id); err != nil {
		return a, invalid("an account does not exist")
	}
	if a.PaidFromID != "" {
		if _, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: a.PurchaseDate, Memo: "Purchase of asset " + a.Name, Kind: "journal", Source: SourceAssetPurchase, SourceID: id,
			SourceHash: hashOf(id, a.Cost), Lines: []Line{{AccountID: a.AssetAccountID, Debit: a.Cost, Class: a.Class}, {AccountID: a.PaidFromID, Credit: a.Cost}}}); err != nil {
			_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.acct_assets WHERE id=$1::uuid`, id)
			return a, err
		}
	}
	r.audit(ctx, orgID, userID, "create", "asset", id, fmt.Sprintf("%s %.2f", a.Name, a.Cost))
	return r.GetAsset(ctx, orgID, id)
}

// AssetSchedule returns the full schedule with the posted months marked.
func (r *Repository) AssetSchedule(ctx context.Context, orgID, id string) ([]DepRow, error) {
	a, err := r.GetAsset(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	rows := DepSchedule(a.Cost, a.Salvage, a.LifeMonths, a.Method, a.WDVRate, a.DepStart, a.DisposedOn)
	posted := map[string]bool{}
	pr, err := r.pool.Query(ctx, `SELECT period FROM adrient.acct_asset_dep WHERE asset_id=$1::uuid`, id)
	if err != nil {
		return nil, err
	}
	defer pr.Close()
	for pr.Next() {
		var p string
		if pr.Scan(&p) == nil {
			posted[p] = true
		}
	}
	for i := range rows {
		rows[i].Posted = posted[rows[i].Period]
	}
	return rows, nil
}

// DepResult reports a depreciation run.
type DepResult struct {
	Posted int      `json:"posted"`
	Locked int      `json:"locked"`
	Errors []string `json:"errors"`
}

// RunDepreciation posts every unposted month up to and including upTo (YYYY-MM) for every
// active asset, dated the last day of the month. Idempotent: a month is recorded once.
func (r *Repository) RunDepreciation(ctx context.Context, orgID, userID, upTo string) (DepResult, error) {
	res := DepResult{Errors: []string{}}
	if _, err := time.Parse("2006-01", upTo); err != nil {
		return res, invalid("period must be YYYY-MM")
	}
	assets, err := r.ListAssets(ctx, orgID)
	if err != nil {
		return res, err
	}
	for _, a := range assets {
		if a.Status != "active" {
			continue
		}
		sched, err := r.AssetSchedule(ctx, orgID, a.ID)
		if err != nil {
			return res, err
		}
		for _, row := range sched {
			if row.Posted || row.Period > upTo || paise(row.Dep) == 0 {
				continue
			}
			first, _ := time.Parse("2006-01", row.Period)
			last := first.AddDate(0, 1, -1).Format("2006-01-02")
			sid := "asset:" + a.ID + ":" + row.Period
			var claimed string
			if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.acct_asset_dep (asset_id, org_id, period, amount) VALUES ($1::uuid,$2,$3,$4)
				ON CONFLICT (asset_id, period) DO NOTHING RETURNING period`, a.ID, orgID, row.Period, row.Dep).Scan(&claimed); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return res, err
			}
			_, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: last, Memo: fmt.Sprintf("Depreciation %s %s", a.Name, row.Period), Ref: a.Tag, Kind: "journal",
				Source: SourceDepreciation, SourceID: sid, SourceHash: hashOf(sid, row.Dep),
				Lines: []Line{{AccountID: a.ExpenseAccountID, Debit: row.Dep, Class: a.Class}, {AccountID: a.AccumAccountID, Credit: row.Dep}}})
			if err != nil {
				_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.acct_asset_dep WHERE asset_id=$1::uuid AND period=$2`, a.ID, row.Period)
				if errors.Is(err, ErrLocked) {
					res.Locked++
					continue
				}
				if len(res.Errors) < 10 {
					res.Errors = append(res.Errors, a.Name+" "+row.Period+": "+err.Error())
				}
				continue
			}
			res.Posted++
		}
	}
	if res.Posted > 0 {
		r.audit(ctx, orgID, userID, "depreciate", "asset", "", fmt.Sprintf("%d months to %s", res.Posted, upTo))
	}
	return res, nil
}

// DisposeAsset sells or scraps an asset: depreciation is brought up to the disposal
// month, then Dr accumulated depreciation, Dr bank (proceeds), Cr cost, and the
// difference is a gain or loss on disposal.
func (r *Repository) DisposeAsset(ctx context.Context, orgID, userID, id, date string, proceeds float64, accountID string) (Asset, error) {
	a, err := r.GetAsset(ctx, orgID, id)
	if err != nil {
		return a, err
	}
	d, err := validDate(date)
	if err != nil {
		return a, err
	}
	if a.Status != "active" {
		return a, invalid("this asset is already disposed")
	}
	if d < a.PurchaseDate {
		return a, invalid("disposal cannot be before purchase")
	}
	if proceeds < 0 {
		return a, invalid("proceeds cannot be negative")
	}
	if proceeds > 0 && !isUUID(accountID) {
		return a, invalid("choose the account the proceeds went to")
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_assets SET disposed_on=$3::date WHERE org_id=$1 AND id=$2::uuid`, orgID, id, d); err != nil {
		return a, err
	}
	rollback := func() {
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.acct_assets SET disposed_on=NULL WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	}
	if _, err := r.RunDepreciation(ctx, orgID, userID, d[:7]); err != nil {
		rollback()
		return a, err
	}
	a, _ = r.GetAsset(ctx, orgID, id)
	if err := r.EnsureSystemAccounts(ctx, orgID); err != nil {
		rollback()
		return a, err
	}
	keys, _ := r.accountKeys(ctx, r.pool, orgID)
	gain := paise(proceeds) - paise(a.BookValue)
	lines := []Line{{AccountID: a.AssetAccountID, Credit: a.Cost, Class: a.Class}}
	if paise(a.Accumulated) > 0 {
		lines = append(lines, Line{AccountID: a.AccumAccountID, Debit: a.Accumulated})
	}
	if proceeds > 0 {
		lines = append(lines, Line{AccountID: accountID, Debit: proceeds})
	}
	if gain > 0 {
		lines = append(lines, Line{AccountID: keys["asset_disposal"], Credit: rupees(gain), Memo: "Gain on disposal"})
	} else if gain < 0 {
		lines = append(lines, Line{AccountID: keys["asset_disposal"], Debit: rupees(-gain), Memo: "Loss on disposal"})
	}
	if _, _, err := r.PostJournal(ctx, orgID, userID, JournalInput{Date: d, Memo: "Disposal of asset " + a.Name, Kind: "journal", Source: SourceAssetDisposal, SourceID: id,
		SourceHash: hashOf(id, d, proceeds), Lines: lines}); err != nil {
		rollback()
		return a, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.acct_assets SET status='disposed', proceeds=$3 WHERE org_id=$1 AND id=$2::uuid`, orgID, id, rupees(paise(proceeds))); err != nil {
		return a, err
	}
	r.audit(ctx, orgID, userID, "dispose", "asset", id, fmt.Sprintf("%.2f", proceeds))
	return r.GetAsset(ctx, orgID, id)
}
