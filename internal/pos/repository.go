package pos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
)

var (
	// ErrNotFound is returned when no record matches within the org.
	ErrNotFound = errors.New("not found")
	// ErrInvalid wraps validation failures.
	ErrInvalid = errors.New("invalid")
	// ErrConflict is returned for state conflicts (shift already open, ...).
	ErrConflict = errors.New("conflict")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func conflict(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, a...))
}

// db is satisfied by both *pgxpool.Pool and pgx.Tx.
type db interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Repository stores POS data. Stock moves go through the inventory repository
// inside the same transaction as the sale.
type Repository struct {
	pool *pgxpool.Pool
	inv  *inventory.Repository
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool, inv *inventory.Repository) *Repository {
	return &Repository{pool: pool, inv: inv}
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tsPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return ts(*t)
}

func parseTS(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		if d, err2 := time.Parse("2006-01-02", s); err2 == nil {
			return d, true
		}
		return time.Time{}, false
	}
	return t, true
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- registers

const registerCols = `r.id::text, r.name, coalesce(r.location_id::text,''), r.archived, r.created_at,
	coalesce((SELECT s.id::text FROM adrient.pos_shifts s WHERE s.register_id=r.id AND s.closed_at IS NULL),'')`

func scanRegister(row pgx.Row) (*adrientv1.PosRegister, error) {
	var r adrientv1.PosRegister
	var created time.Time
	if err := row.Scan(&r.Id, &r.Name, &r.LocationId, &r.Archived, &created, &r.OpenShiftId); err != nil {
		return nil, err
	}
	r.CreatedAt = ts(created)
	return &r, nil
}

// ListRegisters returns the active registers.
func (r *Repository) ListRegisters(ctx context.Context, orgID string) ([]*adrientv1.PosRegister, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+registerCols+` FROM adrient.pos_registers r
		WHERE r.org_id=$1 AND NOT r.archived ORDER BY r.created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*adrientv1.PosRegister{}
	for rows.Next() {
		x, err := scanRegister(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) getRegister(ctx context.Context, q db, orgID, id string) (*adrientv1.PosRegister, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	x, err := scanRegister(q.QueryRow(ctx, `SELECT `+registerCols+` FROM adrient.pos_registers r
		WHERE r.org_id=$1 AND r.id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return x, err
}

// SaveRegister creates (id empty) or renames a register.
func (r *Repository) SaveRegister(ctx context.Context, orgID, id, name, locationID string) (*adrientv1.PosRegister, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, invalid("register name is required")
	}
	if locationID != "" && !isUUID(locationID) {
		return nil, invalid("location not found")
	}
	if id == "" {
		var nid string
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.pos_registers (org_id, name, location_id)
			VALUES ($1,$2,NULLIF($3,'')::uuid) RETURNING id::text`, orgID, name, locationID).Scan(&nid)
		if err != nil {
			return nil, err
		}
		return r.getRegister(ctx, r.pool, orgID, nid)
	}
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.pos_registers SET name=$3, location_id=NULLIF($4,'')::uuid
		WHERE org_id=$1 AND id=$2`, orgID, id, name, locationID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.getRegister(ctx, r.pool, orgID, id)
}

// ArchiveRegister hides a register (its sales stay). An open shift blocks it.
func (r *Repository) ArchiveRegister(ctx context.Context, orgID, id string) error {
	reg, err := r.getRegister(ctx, r.pool, orgID, id)
	if err != nil {
		return err
	}
	if reg.OpenShiftId != "" {
		return conflict("close the open shift first")
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.pos_registers SET archived=true WHERE org_id=$1 AND id=$2`, orgID, id)
	return err
}

// ------------------------------------------------------------------- shifts

func (r *Repository) shiftStats(ctx context.Context, q db, s *adrientv1.PosShift) error {
	if err := q.QueryRow(ctx, `SELECT
		coalesce((SELECT sum(p.amount) FROM adrient.pos_payments p JOIN adrient.pos_sales x ON x.id=p.sale_id
		          WHERE x.shift_id=$1 AND p.method='cash'),0)::float8,
		coalesce((SELECT sum(total) FROM adrient.pos_refunds WHERE shift_id=$1 AND method='cash'),0)::float8,
		coalesce((SELECT sum(amount) FROM adrient.pos_cash_moves WHERE shift_id=$1 AND amount>0),0)::float8,
		coalesce((SELECT -sum(amount) FROM adrient.pos_cash_moves WHERE shift_id=$1 AND amount<0),0)::float8,
		(SELECT count(*) FROM adrient.pos_sales WHERE shift_id=$1 AND status<>'held'),
		coalesce((SELECT sum(total) FROM adrient.pos_sales WHERE shift_id=$1 AND status<>'held'),0)::float8`,
		s.Id).Scan(&s.CashSales, &s.CashRefunds, &s.CashIn, &s.CashOut, &s.SalesCount, &s.SalesTotal); err != nil {
		return err
	}
	if !s.Closed {
		s.ExpectedCash = round2(s.OpeningCash + s.CashSales - s.CashRefunds + s.CashIn - s.CashOut)
	}
	rows, err := q.Query(ctx, `SELECT id::text, amount::float8, note, created_at FROM adrient.pos_cash_moves
		WHERE shift_id=$1 ORDER BY created_at`, s.Id)
	if err != nil {
		return err
	}
	defer rows.Close()
	s.Entries = []*adrientv1.PosCashEntry{}
	for rows.Next() {
		var e adrientv1.PosCashEntry
		var t time.Time
		if err := rows.Scan(&e.Id, &e.Amount, &e.Note, &t); err != nil {
			return err
		}
		e.CreatedAt = ts(t)
		s.Entries = append(s.Entries, &e)
	}
	return rows.Err()
}

const shiftCols = `s.id::text, s.register_id::text, r.name, coalesce(u.display_name, u.email, ''),
	s.opened_at, s.opening_cash::float8, s.closed_at, s.counted_cash::float8, s.expected_cash::float8, s.note`

func (r *Repository) scanShift(ctx context.Context, q db, row pgx.Row) (*adrientv1.PosShift, error) {
	var s adrientv1.PosShift
	var opened time.Time
	var closed *time.Time
	var counted, expected *float64
	if err := row.Scan(&s.Id, &s.RegisterId, &s.RegisterName, &s.OpenedBy, &opened, &s.OpeningCash,
		&closed, &counted, &expected, &s.Note); err != nil {
		return nil, err
	}
	s.OpenedAt = ts(opened)
	s.ClosedAt = tsPtr(closed)
	s.Closed = closed != nil
	if counted != nil {
		s.CountedCash = *counted
	}
	if expected != nil {
		s.ExpectedCash = *expected
	}
	if err := r.shiftStats(ctx, q, &s); err != nil {
		return nil, err
	}
	if s.Closed {
		s.Variance = round2(s.CountedCash - s.ExpectedCash)
	}
	return &s, nil
}

const shiftFrom = ` FROM adrient.pos_shifts s JOIN adrient.pos_registers r ON r.id=s.register_id
	LEFT JOIN adrient.users u ON u.id=s.opened_by`

// GetShift returns a shift with live drawer totals.
func (r *Repository) GetShift(ctx context.Context, orgID, id string) (*adrientv1.PosShift, error) {
	return r.getShift(ctx, r.pool, orgID, id)
}

func (r *Repository) getShift(ctx context.Context, q db, orgID, id string) (*adrientv1.PosShift, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	s, err := r.scanShift(ctx, q, q.QueryRow(ctx, `SELECT `+shiftCols+shiftFrom+` WHERE s.org_id=$1 AND s.id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

// ListShifts returns recent shifts, newest first.
func (r *Repository) ListShifts(ctx context.Context, orgID, registerID string, limit int) ([]*adrientv1.PosShift, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if registerID != "" && !isUUID(registerID) {
		return []*adrientv1.PosShift{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+shiftCols+shiftFrom+`
		WHERE s.org_id=$1 AND ($2='' OR s.register_id=NULLIF($2,'')::uuid)
		ORDER BY s.opened_at DESC LIMIT $3`, orgID, registerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*adrientv1.PosShift{}
	for rows.Next() {
		var s adrientv1.PosShift
		var opened time.Time
		var closed *time.Time
		var counted, expected *float64
		if err := rows.Scan(&s.Id, &s.RegisterId, &s.RegisterName, &s.OpenedBy, &opened, &s.OpeningCash,
			&closed, &counted, &expected, &s.Note); err != nil {
			return nil, err
		}
		s.OpenedAt = ts(opened)
		s.ClosedAt = tsPtr(closed)
		s.Closed = closed != nil
		if counted != nil {
			s.CountedCash = *counted
		}
		if expected != nil {
			s.ExpectedCash = *expected
		}
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for _, s := range out {
		if err := r.shiftStats(ctx, r.pool, s); err != nil {
			return nil, err
		}
		if s.Closed {
			s.Variance = round2(s.CountedCash - s.ExpectedCash)
		}
		s.Entries = nil
	}
	return out, nil
}

// OpenShift starts a drawer shift on a register.
func (r *Repository) OpenShift(ctx context.Context, orgID, userID, registerID string, opening float64) (*adrientv1.PosShift, error) {
	if opening < 0 {
		return nil, invalid("opening cash cannot be negative")
	}
	reg, err := r.getRegister(ctx, r.pool, orgID, registerID)
	if err != nil {
		return nil, err
	}
	if reg.Archived {
		return nil, invalid("register is archived")
	}
	var id string
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.pos_shifts (org_id, register_id, opened_by, opening_cash)
		VALUES ($1,$2,NULLIF($3,'')::uuid,$4) RETURNING id::text`, orgID, registerID, userID, round2(opening)).Scan(&id)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return nil, conflict("a shift is already open on this register")
		}
		return nil, err
	}
	return r.GetShift(ctx, orgID, id)
}

// CloseShift records the counted cash and the expected cash at close time.
func (r *Repository) CloseShift(ctx context.Context, orgID, userID, id string, counted float64, note string) (*adrientv1.PosShift, error) {
	if counted < 0 {
		return nil, invalid("counted cash cannot be negative")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	s, err := r.getShift(ctx, tx, orgID, id)
	if err != nil {
		return nil, err
	}
	if s.Closed {
		return nil, conflict("shift is already closed")
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.pos_shifts SET closed_at=now(), closed_by=NULLIF($3,'')::uuid,
		counted_cash=$4, expected_cash=$5, note=$6 WHERE org_id=$1 AND id=$2`,
		orgID, id, userID, round2(counted), s.ExpectedCash, strings.TrimSpace(note)); err != nil {
		return nil, err
	}
	out, err := r.getShift(ctx, tx, orgID, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// CashMove records cash paid into (positive) or out of (negative) the drawer.
func (r *Repository) CashMove(ctx context.Context, orgID, id string, amount float64, note string) (*adrientv1.PosShift, error) {
	amount = round2(amount)
	if amount == 0 {
		return nil, invalid("amount is required")
	}
	s, err := r.GetShift(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if s.Closed {
		return nil, conflict("shift is closed")
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.pos_cash_moves (org_id, shift_id, amount, note) VALUES ($1,$2,$3,$4)`,
		orgID, id, amount, strings.TrimSpace(note)); err != nil {
		return nil, err
	}
	return r.GetShift(ctx, orgID, id)
}

// ------------------------------------------------- taxes, presets, settings

func (r *Repository) ListTaxRates(ctx context.Context, orgID string) ([]*adrientv1.PosTaxRate, error) {
	return r.taxRates(ctx, r.pool, orgID)
}

func (r *Repository) taxRates(ctx context.Context, q db, orgID string) ([]*adrientv1.PosTaxRate, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name, rate::float8, inclusive, is_default
		FROM adrient.pos_tax_rates WHERE org_id=$1 ORDER BY is_default DESC, name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*adrientv1.PosTaxRate{}
	for rows.Next() {
		var t adrientv1.PosTaxRate
		if err := rows.Scan(&t.Id, &t.Name, &t.Rate, &t.Inclusive, &t.IsDefault); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (r *Repository) SaveTaxRate(ctx context.Context, orgID string, in *adrientv1.SavePosTaxRateRequest) (*adrientv1.PosTaxRate, error) {
	name := strings.TrimSpace(in.GetName())
	if name == "" {
		return nil, invalid("tax name is required")
	}
	if in.GetRate() < 0 || in.GetRate() > 100 {
		return nil, invalid("tax rate must be between 0 and 100")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if in.GetIsDefault() {
		if _, err := tx.Exec(ctx, `UPDATE adrient.pos_tax_rates SET is_default=false WHERE org_id=$1`, orgID); err != nil {
			return nil, err
		}
	}
	id := in.GetId()
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO adrient.pos_tax_rates (org_id,name,rate,inclusive,is_default)
			VALUES ($1,$2,$3,$4,$5) RETURNING id::text`, orgID, name, in.GetRate(), in.GetInclusive(), in.GetIsDefault()).Scan(&id)
	} else {
		if !isUUID(id) {
			return nil, ErrNotFound
		}
		tag, e := tx.Exec(ctx, `UPDATE adrient.pos_tax_rates SET name=$3, rate=$4, inclusive=$5, is_default=$6
			WHERE org_id=$1 AND id=$2`, orgID, id, name, in.GetRate(), in.GetInclusive(), in.GetIsDefault())
		err = e
		if err == nil && tag.RowsAffected() == 0 {
			return nil, ErrNotFound
		}
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &adrientv1.PosTaxRate{Id: id, Name: name, Rate: in.GetRate(), Inclusive: in.GetInclusive(), IsDefault: in.GetIsDefault()}, nil
}

func (r *Repository) DeleteTaxRate(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.pos_tax_rates WHERE org_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListDiscountPresets(ctx context.Context, orgID string) ([]*adrientv1.PosDiscountPreset, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, kind, value::float8 FROM adrient.pos_discount_presets
		WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*adrientv1.PosDiscountPreset{}
	for rows.Next() {
		var d adrientv1.PosDiscountPreset
		if err := rows.Scan(&d.Id, &d.Name, &d.Kind, &d.Value); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}

func (r *Repository) SaveDiscountPreset(ctx context.Context, orgID string, in *adrientv1.SavePosDiscountPresetRequest) (*adrientv1.PosDiscountPreset, error) {
	name := strings.TrimSpace(in.GetName())
	if name == "" {
		return nil, invalid("discount name is required")
	}
	if in.GetKind() != "percent" && in.GetKind() != "amount" {
		return nil, invalid("discount kind must be percent or amount")
	}
	if in.GetValue() <= 0 || (in.GetKind() == "percent" && in.GetValue() > 100) {
		return nil, invalid("discount value is out of range")
	}
	id := in.GetId()
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.pos_discount_presets (org_id,name,kind,value)
			VALUES ($1,$2,$3,$4) RETURNING id::text`, orgID, name, in.GetKind(), in.GetValue()).Scan(&id)
	} else {
		if !isUUID(id) {
			return nil, ErrNotFound
		}
		tag, e := r.pool.Exec(ctx, `UPDATE adrient.pos_discount_presets SET name=$3, kind=$4, value=$5
			WHERE org_id=$1 AND id=$2`, orgID, id, name, in.GetKind(), in.GetValue())
		err = e
		if err == nil && tag.RowsAffected() == 0 {
			return nil, ErrNotFound
		}
	}
	if err != nil {
		return nil, err
	}
	return &adrientv1.PosDiscountPreset{Id: id, Name: name, Kind: in.GetKind(), Value: in.GetValue()}, nil
}

func (r *Repository) DeleteDiscountPreset(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.pos_discount_presets WHERE org_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) settings(ctx context.Context, q db, orgID string) (*adrientv1.PosSettings, error) {
	s := &adrientv1.PosSettings{ReceiptFooter: "Thank you!", LoyaltyPointValue: 0.01}
	err := q.QueryRow(ctx, `SELECT store_name, currency, receipt_header, receipt_footer,
		loyalty_earn_per_unit::float8, loyalty_point_value::float8 FROM adrient.pos_settings WHERE org_id=$1`, orgID).
		Scan(&s.StoreName, &s.Currency, &s.ReceiptHeader, &s.ReceiptFooter, &s.LoyaltyEarnPerUnit, &s.LoyaltyPointValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	return s, err
}

func (r *Repository) GetSettings(ctx context.Context, orgID string) (*adrientv1.PosSettings, error) {
	return r.settings(ctx, r.pool, orgID)
}

func (r *Repository) SaveSettings(ctx context.Context, orgID string, in *adrientv1.PosSettings) (*adrientv1.PosSettings, error) {
	if in.GetLoyaltyEarnPerUnit() < 0 || in.GetLoyaltyPointValue() < 0 {
		return nil, invalid("loyalty values cannot be negative")
	}
	cur := strings.ToUpper(strings.TrimSpace(in.GetCurrency()))
	if cur != "" && (len(cur) != 3) {
		return nil, invalid("currency must be a 3-letter code such as USD or INR")
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.pos_settings
		(org_id, store_name, currency, receipt_header, receipt_footer, loyalty_earn_per_unit, loyalty_point_value)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (org_id) DO UPDATE SET store_name=$2, currency=$3, receipt_header=$4, receipt_footer=$5,
		loyalty_earn_per_unit=$6, loyalty_point_value=$7`,
		orgID, strings.TrimSpace(in.GetStoreName()), cur, in.GetReceiptHeader(), in.GetReceiptFooter(),
		in.GetLoyaltyEarnPerUnit(), in.GetLoyaltyPointValue())
	if err != nil {
		return nil, err
	}
	return r.GetSettings(ctx, orgID)
}
