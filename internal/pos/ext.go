package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
)

// ---- ext settings ----

// ExtSettings are the wave-3 POS settings, kept as one JSON document.
type ExtSettings struct {
	Tiers              []Tier    `json:"tiers"`
	Stamps             StampRule `json:"stamps"`
	DiscountPinOverPct float64   `json:"discount_pin_over_pct"` // 0 = a discount never needs a PIN
	RefundNeedsPin     bool      `json:"refund_needs_pin"`
	VoidNeedsPin       bool      `json:"void_needs_pin"`
	ReceiptFooter      string    `json:"receipt_footer"`
}

func defaultExt() ExtSettings {
	return ExtSettings{RefundNeedsPin: true, VoidNeedsPin: true, Tiers: []Tier{}, ReceiptFooter: "Thank you!"}
}

// GetExt returns the ext settings (defaults until saved).
func (r *Repository) GetExt(ctx context.Context, orgID string) (ExtSettings, error) {
	return r.getExt(ctx, r.pool, orgID)
}

func (r *Repository) getExt(ctx context.Context, q db, orgID string) (ExtSettings, error) {
	s := defaultExt()
	var raw []byte
	err := q.QueryRow(ctx, `SELECT data FROM adrient.pos_ext_settings WHERE org_id=$1`, orgID).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}
	if len(raw) > 2 {
		_ = json.Unmarshal(raw, &s)
	}
	if s.Tiers == nil {
		s.Tiers = []Tier{}
	}
	return s, nil
}

// SaveExt validates and stores the ext settings.
func (r *Repository) SaveExt(ctx context.Context, orgID string, s ExtSettings) (ExtSettings, error) {
	if len(s.Tiers) > 8 {
		return s, invalid("at most 8 tiers")
	}
	for _, t := range s.Tiers {
		if strings.TrimSpace(t.Name) == "" || t.MinSpend < 0 || t.Multiplier < 1 || t.Multiplier > 10 {
			return s, invalid("a tier needs a name, a spend of 0 or more and a multiplier between 1 and 10")
		}
	}
	if s.Stamps.Enabled && (s.Stamps.Goal < 2 || s.Stamps.Goal > 50 || s.Stamps.RewardCredit <= 0 || s.Stamps.MinSale < 0) {
		return s, invalid("stamps need a goal of 2 to 50 and a reward above zero")
	}
	if s.DiscountPinOverPct < 0 || s.DiscountPinOverPct > 100 {
		return s, invalid("discount threshold must be between 0 and 100")
	}
	b, _ := json.Marshal(s)
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.pos_ext_settings (org_id, data) VALUES ($1,$2)
		ON CONFLICT (org_id) DO UPDATE SET data=EXCLUDED.data, updated_at=now()`, orgID, b)
	return s, err
}

// ---- PIN ----

// PinInfo says whether a user has a manager PIN.
type PinInfo struct {
	Set         bool `json:"set"`
	CanVoid     bool `json:"can_void"`
	CanRefund   bool `json:"can_refund"`
	CanDiscount bool `json:"can_discount"`
}

// GetPinInfo reports the caller's own PIN state (never the PIN).
func (r *Repository) GetPinInfo(ctx context.Context, orgID, userID string) (PinInfo, error) {
	var p PinInfo
	err := r.pool.QueryRow(ctx, `SELECT true, can_void, can_refund, can_discount FROM adrient.pos_pins WHERE org_id=$1 AND user_id=$2::uuid`, orgID, userID).
		Scan(&p.Set, &p.CanVoid, &p.CanRefund, &p.CanDiscount)
	if errors.Is(err, pgx.ErrNoRows) {
		return PinInfo{}, nil
	}
	return p, err
}

// SetPin stores a manager PIN (bcrypt). A PIN already used by another manager
// is refused, because a PIN has to identify who approved.
func (r *Repository) SetPin(ctx context.Context, orgID, userID, pin string, can PinInfo) error {
	if err := ValidatePIN(pin); err != nil {
		return err
	}
	rows, err := r.pool.Query(ctx, `SELECT user_id::text, pin_hash FROM adrient.pos_pins WHERE org_id=$1`, orgID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var uid, h string
		if err := rows.Scan(&uid, &h); err != nil {
			rows.Close()
			return err
		}
		if uid != userID && bcrypt.CompareHashAndPassword([]byte(h), []byte(pin)) == nil {
			rows.Close()
			return invalid("another manager already uses that PIN, choose a different one")
		}
	}
	rows.Close()
	h, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.pos_pins (org_id, user_id, pin_hash, can_void, can_refund, can_discount)
		VALUES ($1,$2::uuid,$3,$4,$5,$6) ON CONFLICT (org_id, user_id) DO UPDATE SET pin_hash=EXCLUDED.pin_hash,
		can_void=EXCLUDED.can_void, can_refund=EXCLUDED.can_refund, can_discount=EXCLUDED.can_discount, updated_at=now()`,
		orgID, userID, string(h), can.CanVoid, can.CanRefund, can.CanDiscount)
	return err
}

// RemovePin deletes the caller's PIN.
func (r *Repository) RemovePin(ctx context.Context, orgID, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.pos_pins WHERE org_id=$1 AND user_id=$2::uuid`, orgID, userID)
	return err
}

// ErrPinLocked is returned while a terminal user is locked out.
var ErrPinLocked = errors.New("too many wrong PINs, try again in a few minutes")

// ErrBadPin is returned for a PIN that matches no manager allowed to approve.
var ErrBadPin = errors.New("wrong PIN")

// Approver is the manager who approved an action.
type Approver struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

// VerifyPin checks pin against the managers allowed to approve scope (void,
// refund or discount). Wrong tries are counted against the person typing
// (callerID); five of them lock that person for five minutes.
func (r *Repository) VerifyPin(ctx context.Context, orgID, callerID, pin, scope string) (Approver, error) {
	var locked *time.Time
	var failed int
	err := r.pool.QueryRow(ctx, `SELECT failed, locked_until FROM adrient.pos_pin_attempts WHERE org_id=$1 AND user_id=$2::uuid`, orgID, callerID).Scan(&failed, &locked)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Approver{}, err
	}
	now := time.Now()
	if PinLocked(locked, now) {
		return Approver{}, ErrPinLocked
	}
	col := map[string]string{"void": "can_void", "refund": "can_refund", "discount": "can_discount"}[scope]
	if col == "" {
		return Approver{}, invalid("unknown approval scope")
	}
	rows, err := r.pool.Query(ctx, `SELECT p.user_id::text, p.pin_hash, COALESCE(NULLIF(u.display_name,''), u.email) FROM adrient.pos_pins p
		JOIN adrient.users u ON u.id=p.user_id WHERE p.org_id=$1 AND p.`+col, orgID)
	if err != nil {
		return Approver{}, err
	}
	var found *Approver
	for rows.Next() {
		var a Approver
		var h string
		if err := rows.Scan(&a.UserID, &h, &a.Name); err != nil {
			rows.Close()
			return Approver{}, err
		}
		if found == nil && bcrypt.CompareHashAndPassword([]byte(h), []byte(pin)) == nil {
			c := a
			found = &c
		}
	}
	rows.Close()
	if found == nil {
		nf, lu := NextAttemptState(failed, now)
		_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.pos_pin_attempts (org_id, user_id, failed, locked_until) VALUES ($1,$2::uuid,$3,$4)
			ON CONFLICT (org_id, user_id) DO UPDATE SET failed=EXCLUDED.failed, locked_until=EXCLUDED.locked_until`, orgID, callerID, nf, lu)
		if lu != nil {
			return Approver{}, ErrPinLocked
		}
		return Approver{}, ErrBadPin
	}
	_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.pos_pin_attempts WHERE org_id=$1 AND user_id=$2::uuid`, orgID, callerID)
	return *found, nil
}

// ---- sync (offline) ----

// SyncResult is what a client gets back for one idempotency key.
type SyncResult struct {
	Status string             `json:"status"` // done | failed
	Replay bool               `json:"replay"` // true when the key was already processed
	SaleID string             `json:"sale_id"`
	Error  string             `json:"error,omitempty"`
	Sale   *adrientv1.PosSale `json:"-"`
}

// ErrSyncBusy means another request holds the same key right now.
var ErrSyncBusy = errors.New("this sale is being saved by another request, retry shortly")

// CompleteIdempotent completes a sale exactly once per idempotency key. The
// key is claimed first, so two parallel or repeated requests cannot both create
// a sale: the second returns the first one's result. A permanent validation
// failure is remembered too (status failed) so the client can show the reason
// instead of retrying forever; a transient failure releases the key.
func (r *Repository) CompleteIdempotent(ctx context.Context, orgID, userID, key string, in *adrientv1.PosSaleInput, approvalPin string) (SyncResult, error) {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 80 {
		return SyncResult{}, invalid("an idempotency key of 8 to 80 characters is required")
	}
	tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.pos_sync_log (org_id, idem_key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, orgID, key)
	if err != nil {
		return SyncResult{}, err
	}
	if tag.RowsAffected() == 0 {
		var st, sid, msg string
		if err := r.pool.QueryRow(ctx, `SELECT status, COALESCE(sale_id::text,''), error FROM adrient.pos_sync_log WHERE org_id=$1 AND idem_key=$2`, orgID, key).Scan(&st, &sid, &msg); err != nil {
			return SyncResult{}, err
		}
		switch st {
		case "done":
			sale, err := r.GetSale(ctx, orgID, sid)
			return SyncResult{Status: "done", Replay: true, SaleID: sid, Sale: sale}, err
		case "failed":
			return SyncResult{Status: "failed", Replay: true, Error: msg}, nil
		}
		return SyncResult{}, ErrSyncBusy
	}
	release := func() {
		_, _ = r.pool.Exec(context.WithoutCancel(ctx), `DELETE FROM adrient.pos_sync_log WHERE org_id=$1 AND idem_key=$2 AND status='pending'`, orgID, key)
	}
	if err := r.checkDiscountApproval(ctx, orgID, userID, in, approvalPin); err != nil {
		r.markSync(ctx, orgID, key, "failed", "", err.Error())
		return SyncResult{Status: "failed", Error: err.Error()}, nil
	}
	in.Id = "" // a sync always creates the sale; holds are an online feature
	sale, err := r.SaveSale(ctx, orgID, userID, in, true)
	if err != nil {
		if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) {
			r.markSync(ctx, orgID, key, "failed", "", cleanErr(err))
			return SyncResult{Status: "failed", Error: cleanErr(err)}, nil
		}
		release()
		return SyncResult{}, err
	}
	r.markSync(ctx, orgID, key, "done", sale.GetId(), "")
	return SyncResult{Status: "done", SaleID: sale.GetId(), Sale: sale}, nil
}

func cleanErr(err error) string {
	m := err.Error()
	for _, p := range []string{"invalid: ", "conflict: ", "not found"} {
		m = strings.TrimPrefix(m, p)
	}
	if m == "" {
		return "the sale could not be saved"
	}
	return m
}

func (r *Repository) markSync(ctx context.Context, orgID, key, status, saleID, msg string) {
	_, _ = r.pool.Exec(context.WithoutCancel(ctx), `UPDATE adrient.pos_sync_log SET status=$3, sale_id=NULLIF($4,'')::uuid, error=$5, updated_at=now()
		WHERE org_id=$1 AND idem_key=$2`, orgID, key, status, saleID, msg)
}

// checkDiscountApproval enforces "discounts above N% need a manager PIN" for
// sales that arrive through the sync endpoint.
func (r *Repository) checkDiscountApproval(ctx context.Context, orgID, userID string, in *adrientv1.PosSaleInput, pin string) error {
	ext, err := r.GetExt(ctx, orgID)
	if err != nil {
		return err
	}
	if ext.DiscountPinOverPct <= 0 {
		return nil
	}
	lines := make([]CalcLine, 0, len(in.GetLines()))
	for _, l := range in.GetLines() {
		lines = append(lines, CalcLine{Qty: l.GetQty(), UnitPrice: l.GetUnitPrice(), DiscountKind: l.GetDiscountKind(), DiscountValue: l.GetDiscountValue()})
	}
	pct := SaleDiscountPercent(lines, in.GetDiscountKind(), in.GetDiscountValue())
	if pct <= ext.DiscountPinOverPct+1e-9 {
		return nil
	}
	if strings.TrimSpace(pin) == "" {
		return fmt.Errorf("a %.1f%% discount needs a manager PIN (limit %.1f%%)", pct, ext.DiscountPinOverPct)
	}
	if _, err := r.VerifyPin(ctx, orgID, userID, pin, "discount"); err != nil {
		return fmt.Errorf("discount approval failed: %w", err)
	}
	return nil
}

// ---- void ----

// VoidSale cancels a completed, unrefunded sale: every line is refunded with
// restock and the reason and approver are recorded.
func (r *Repository) VoidSale(ctx context.Context, orgID, userID, saleID, registerID, reason, pin string) (*adrientv1.PosSale, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 3 {
		return nil, invalid("give a reason for the void")
	}
	ext, err := r.GetExt(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var approver *Approver
	if ext.VoidNeedsPin {
		a, err := r.VerifyPin(ctx, orgID, userID, pin, "void")
		if err != nil {
			return nil, err
		}
		approver = &a
	}
	sale, err := r.GetSale(ctx, orgID, saleID)
	if err != nil {
		return nil, err
	}
	if sale.GetStatus() != "completed" {
		return nil, conflict("only a completed sale that has no refunds can be voided (this one is %s)", sale.GetStatus())
	}
	method := "cash"
	for _, p := range sale.GetPayments() {
		method = p.GetMethod()
		break
	}
	if method == "loyalty" || method == "store_credit" {
		method = "store_credit"
	}
	if method == "store_credit" && sale.GetCustomerId() == "" {
		method = "other"
	}
	lines := make([]*adrientv1.RefundLineInput, 0, len(sale.GetLines()))
	for _, l := range sale.GetLines() {
		lines = append(lines, &adrientv1.RefundLineInput{LineId: l.GetId(), Qty: l.GetQty()})
	}
	if registerID == "" {
		registerID = sale.GetRegisterId()
	}
	out, err := r.RefundSale(ctx, orgID, userID, &adrientv1.RefundPosSaleRequest{Id: saleID, RegisterId: registerID, Lines: lines, Method: method, Restock: true, Note: "VOID: " + reason})
	if err != nil {
		return nil, err
	}
	var appr any
	if approver != nil {
		appr = approver.UserID
	}
	_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.pos_voids (org_id, sale_id, reason, approved_by, created_by) VALUES ($1,$2::uuid,$3,$4::uuid,NULLIF($5,'')::uuid)
		ON CONFLICT DO NOTHING`, orgID, saleID, reason, appr, userID)
	return out, nil
}

// ---- gift cards ----

// GiftCard is a stored-value card.
type GiftCard struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Initial     float64 `json:"initial"`
	Balance     float64 `json:"balance"`
	Status      string  `json:"status"`
	ExpiresOn   string  `json:"expires_on"`
	HolderName  string  `json:"holder_name"`
	HolderEmail string  `json:"holder_email"`
	CreatedAt   string  `json:"created_at"`
}

const giftCols = `id::text, code, initial::float8, balance::float8, status, COALESCE(to_char(expires_on,'YYYY-MM-DD'),''), holder_name, holder_email, to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`

func scanGift(row pgx.Row) (GiftCard, error) {
	var g GiftCard
	err := row.Scan(&g.ID, &g.Code, &g.Initial, &g.Balance, &g.Status, &g.ExpiresOn, &g.HolderName, &g.HolderEmail, &g.CreatedAt)
	return g, err
}

// IssueGiftCard creates a card with a starting balance.
func (r *Repository) IssueGiftCard(ctx context.Context, orgID, userID string, amount float64, holderName, holderEmail, expires string) (GiftCard, error) {
	amount = round2(amount)
	if amount <= 0 || amount > 1e7 || math.IsNaN(amount) {
		return GiftCard{}, invalid("amount must be above zero")
	}
	if expires != "" {
		if _, err := time.Parse("2006-01-02", expires); err != nil {
			return GiftCard{}, invalid("expiry must be YYYY-MM-DD")
		}
	}
	for try := 0; try < 5; try++ {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return GiftCard{}, err
		}
		code := NewGiftCode()
		g, err := scanGift(tx.QueryRow(ctx, `INSERT INTO adrient.pos_gift_cards (org_id, code, initial, balance, expires_on, holder_name, holder_email, created_by)
			VALUES ($1,$2,$3,$3,NULLIF($4,'')::date,$5,$6,NULLIF($7,'')::uuid) RETURNING `+giftCols, orgID, code, amount, expires, strings.TrimSpace(holderName), strings.TrimSpace(holderEmail), userID))
		if err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			if strings.Contains(err.Error(), "pos_gift_cards_code_uq") {
				continue // astronomically rare: pick another code
			}
			return GiftCard{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.pos_gift_card_log (org_id, card_id, delta, reason) VALUES ($1,$2::uuid,$3,'issue')`, orgID, g.ID, amount); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return GiftCard{}, err
		}
		return g, tx.Commit(ctx)
	}
	return GiftCard{}, errors.New("could not generate a unique code")
}

// ListGiftCards returns the newest cards.
func (r *Repository) ListGiftCards(ctx context.Context, orgID string) ([]GiftCard, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+giftCols+` FROM adrient.pos_gift_cards WHERE org_id=$1 ORDER BY created_at DESC LIMIT 500`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GiftCard{}
	for rows.Next() {
		g, err := scanGift(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGiftCard looks a card up by code.
func (r *Repository) GetGiftCard(ctx context.Context, orgID, code string) (GiftCard, error) {
	g, err := scanGift(r.pool.QueryRow(ctx, `SELECT `+giftCols+` FROM adrient.pos_gift_cards WHERE org_id=$1 AND upper(code)=$2`, orgID, NormalizeGiftCode(code)))
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}

// TopUpGiftCard adds value to an active card.
func (r *Repository) TopUpGiftCard(ctx context.Context, orgID, code string, amount float64) (GiftCard, error) {
	amount = round2(amount)
	if amount <= 0 || amount > 1e7 {
		return GiftCard{}, invalid("amount must be above zero")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return GiftCard{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	g, err := scanGift(tx.QueryRow(ctx, `UPDATE adrient.pos_gift_cards SET balance=balance+$3 WHERE org_id=$1 AND upper(code)=$2 AND status='active' RETURNING `+giftCols, orgID, NormalizeGiftCode(code), amount))
	if errors.Is(err, pgx.ErrNoRows) {
		return g, invalid("card not found or not active")
	}
	if err != nil {
		return g, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.pos_gift_card_log (org_id, card_id, delta, reason) VALUES ($1,$2::uuid,$3,'topup')`, orgID, g.ID, amount); err != nil {
		return g, err
	}
	return g, tx.Commit(ctx)
}

// SetGiftCardStatus enables or disables a card.
func (r *Repository) SetGiftCardStatus(ctx context.Context, orgID, code, status string) (GiftCard, error) {
	if status != "active" && status != "disabled" {
		return GiftCard{}, invalid("status must be active or disabled")
	}
	g, err := scanGift(r.pool.QueryRow(ctx, `UPDATE adrient.pos_gift_cards SET status=$3 WHERE org_id=$1 AND upper(code)=$2 RETURNING `+giftCols, orgID, NormalizeGiftCode(code), status))
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}

// redeemGift debits a card inside the sale's transaction. The row is locked, so
// two sales cannot spend the same balance, and the unique log entry per
// (card, sale) makes a retried sale spend it once.
func (r *Repository) redeemGift(ctx context.Context, tx db, orgID, code string, amount float64, saleID string, now time.Time) error {
	var id, status string
	var bal float64
	var exp *time.Time
	err := tx.QueryRow(ctx, `SELECT id::text, balance::float8, status, expires_on FROM adrient.pos_gift_cards WHERE org_id=$1 AND upper(code)=$2 FOR UPDATE`, orgID, NormalizeGiftCode(code)).Scan(&id, &bal, &status, &exp)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("gift card %s was not found", code)
	}
	if err != nil {
		return err
	}
	if status != "active" {
		return invalid("gift card %s is disabled", code)
	}
	if exp != nil && now.After(exp.Add(24*time.Hour)) {
		return invalid("gift card %s expired on %s", code, exp.Format("2006-01-02"))
	}
	if round2(amount) > round2(bal)+0.004 {
		return invalid("gift card %s has only %.2f left", code, bal)
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.pos_gift_cards SET balance=balance-$2 WHERE id=$1::uuid`, id, round2(amount)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO adrient.pos_gift_card_log (org_id, card_id, delta, reason, sale_id) VALUES ($1,$2::uuid,$3,'redeem',$4::uuid)`, orgID, id, -round2(amount), saleID)
	return err
}

// ---- loyalty tiers and stamps ----

// LoyaltyInfo is a customer's tier and stamp card.
type LoyaltyInfo struct {
	Spend      float64 `json:"lifetime_spend"`
	Tier       string  `json:"tier"`
	Multiplier float64 `json:"multiplier"`
	Stamps     int     `json:"stamps"`
	Goal       int     `json:"stamp_goal"`
	Rewards    int     `json:"rewards"`
}

func (r *Repository) lifetimeSpend(ctx context.Context, q db, orgID, contactID string) (float64, error) {
	var v float64
	err := q.QueryRow(ctx, `SELECT COALESCE(SUM(total) - COALESCE((SELECT SUM(rf.total) FROM adrient.pos_refunds rf JOIN adrient.pos_sales s2 ON s2.id=rf.sale_id
		WHERE s2.org_id=$1 AND s2.customer_id=$2),0),0)::float8 FROM adrient.pos_sales WHERE org_id=$1 AND customer_id=$2 AND status IN ('completed','partially_refunded','refunded')`, orgID, contactID).Scan(&v)
	return v, err
}

// Loyalty returns a customer's tier and stamps.
func (r *Repository) Loyalty(ctx context.Context, orgID, contactID string) (LoyaltyInfo, error) {
	ext, err := r.GetExt(ctx, orgID)
	if err != nil {
		return LoyaltyInfo{}, err
	}
	spend, err := r.lifetimeSpend(ctx, r.pool, orgID, contactID)
	if err != nil {
		return LoyaltyInfo{}, err
	}
	li := LoyaltyInfo{Spend: round2(spend), Multiplier: 1, Goal: ext.Stamps.Goal}
	if t, ok := TierFor(ext.Tiers, spend); ok {
		li.Tier, li.Multiplier = t.Name, t.Multiplier
	}
	_ = r.pool.QueryRow(ctx, `SELECT stamps, rewards FROM adrient.pos_loyalty_ext WHERE org_id=$1 AND contact_id=$2::uuid`, orgID, contactID).Scan(&li.Stamps, &li.Rewards)
	return li, nil
}

// applyLoyaltyExt runs inside the sale transaction after the base points were
// computed: it returns the tier-adjusted points and pays stamp rewards as store credit.
func (r *Repository) tierPoints(ctx context.Context, tx db, orgID, custID string, total, perUnit float64) (float64, error) {
	ext, err := r.getExt(ctx, tx, orgID)
	if err != nil {
		return 0, err
	}
	spend, err := r.lifetimeSpend(ctx, tx, orgID, custID)
	if err != nil {
		return 0, err
	}
	tier, ok := TierFor(ext.Tiers, spend)
	return EarnPoints(total, perUnit, tier, ok), nil
}

func (r *Repository) addStamp(ctx context.Context, tx db, orgID, custID, saleID string, number int64, total float64) error {
	ext, err := r.getExt(ctx, tx, orgID)
	if err != nil || !ext.Stamps.Enabled {
		return err
	}
	var stamps int
	err = tx.QueryRow(ctx, `INSERT INTO adrient.pos_loyalty_ext (org_id, contact_id) VALUES ($1,$2::uuid) ON CONFLICT (org_id, contact_id) DO UPDATE SET stamps=adrient.pos_loyalty_ext.stamps
		RETURNING stamps`, orgID, custID).Scan(&stamps)
	if err != nil {
		return err
	}
	ns, rewards := AddStamp(ext.Stamps, stamps, total)
	if ns == stamps && rewards == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.pos_loyalty_ext SET stamps=$3, rewards=rewards+$4 WHERE org_id=$1 AND contact_id=$2::uuid`, orgID, custID, ns, rewards); err != nil {
		return err
	}
	if rewards > 0 {
		return r.moveBalance(ctx, tx, orgID, custID, "store_credit", ext.Stamps.RewardCredit*float64(rewards), "stamp reward", saleID, fmt.Sprintf("Stamp card reward, sale #%d", number), false)
	}
	return nil
}

// ---- modifiers ----

// ModOption is one choice in a modifier group.
type ModOption struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

// ModGroup is a set of options a product can offer (size, extras, milk type).
type ModGroup struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Required bool        `json:"required"`
	Multi    bool        `json:"multi"`
	Options  []ModOption `json:"options"`
}

// ListModGroups returns all groups.
func (r *Repository) ListModGroups(ctx context.Context, orgID string) ([]ModGroup, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, required, multi, options FROM adrient.pos_modifier_groups WHERE org_id=$1 ORDER BY lower(name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModGroup{}
	for rows.Next() {
		var g ModGroup
		var ob []byte
		if err := rows.Scan(&g.ID, &g.Name, &g.Required, &g.Multi, &ob); err != nil {
			return nil, err
		}
		g.Options = []ModOption{}
		_ = json.Unmarshal(ob, &g.Options)
		out = append(out, g)
	}
	return out, rows.Err()
}

// SaveModGroup creates (id empty) or updates a group.
func (r *Repository) SaveModGroup(ctx context.Context, orgID, id string, g ModGroup) (ModGroup, error) {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" {
		return g, invalid("name is required")
	}
	if len(g.Options) == 0 || len(g.Options) > 40 {
		return g, invalid("a group needs 1 to 40 options")
	}
	for i := range g.Options {
		g.Options[i].Name = strings.TrimSpace(g.Options[i].Name)
		if g.Options[i].Name == "" || g.Options[i].Price < -1e6 || g.Options[i].Price > 1e6 {
			return g, invalid("every option needs a name and a sensible price")
		}
		if g.Options[i].ID == "" {
			g.Options[i].ID = fmt.Sprintf("o%d", i+1)
		}
	}
	ob, _ := json.Marshal(g.Options)
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.pos_modifier_groups (org_id,name,required,multi,options) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`, orgID, g.Name, g.Required, g.Multi, ob).Scan(&g.ID)
	} else {
		var n int
		err = r.pool.QueryRow(ctx, `WITH u AS (UPDATE adrient.pos_modifier_groups SET name=$3,required=$4,multi=$5,options=$6 WHERE org_id=$1 AND id=$2::uuid RETURNING 1) SELECT count(*) FROM u`, orgID, id, g.Name, g.Required, g.Multi, ob).Scan(&n)
		if err == nil && n == 0 {
			err = ErrNotFound
		}
		g.ID = id
	}
	return g, err
}

// DeleteModGroup removes a group.
func (r *Repository) DeleteModGroup(ctx context.Context, orgID, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.pos_modifier_groups WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	return err
}

// AssignModGroups sets which groups a product offers.
func (r *Repository) AssignModGroups(ctx context.Context, orgID, productID string, groupIDs []string) error {
	if !isUUID(productID) {
		return invalid("unknown product")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.pos_product_modifiers WHERE org_id=$1 AND product_id=$2::uuid`, orgID, productID); err != nil {
		return err
	}
	for _, g := range groupIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.pos_product_modifiers (org_id, product_id, group_id)
			SELECT $1,$2::uuid,id FROM adrient.pos_modifier_groups WHERE org_id=$1 AND id=$3::uuid ON CONFLICT DO NOTHING`, orgID, productID, g); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ProductModMap returns product id -> group ids.
func (r *Repository) ProductModMap(ctx context.Context, orgID string) (map[string][]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT product_id::text, group_id::text FROM adrient.pos_product_modifiers WHERE org_id=$1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var p, g string
		if err := rows.Scan(&p, &g); err != nil {
			return nil, err
		}
		out[p] = append(out[p], g)
	}
	return out, rows.Err()
}

// ---- favourites ----

// Favourite is one tile on the favourites grid.
type Favourite struct {
	ProductID  string `json:"product_id"`
	Label      string `json:"label"`
	Color      string `json:"color"`
	RegisterID string `json:"register_id"`
}

// ListFavourites returns the grid in order. registerID "" returns the shared grid.
func (r *Repository) ListFavourites(ctx context.Context, orgID, registerID string) ([]Favourite, error) {
	rows, err := r.pool.Query(ctx, `SELECT product_id::text, label, color, COALESCE(register_id::text,'') FROM adrient.pos_favourites
		WHERE org_id=$1 AND (register_id IS NULL OR register_id=NULLIF($2,'')::uuid) ORDER BY position, id`, orgID, registerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Favourite{}
	for rows.Next() {
		var f Favourite
		if err := rows.Scan(&f.ProductID, &f.Label, &f.Color, &f.RegisterID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SaveFavourites replaces the shared grid with the given order (max 60 tiles).
func (r *Repository) SaveFavourites(ctx context.Context, orgID string, favs []Favourite) error {
	if len(favs) > 60 {
		return invalid("at most 60 favourites")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.pos_favourites WHERE org_id=$1 AND register_id IS NULL`, orgID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, f := range favs {
		if !isUUID(f.ProductID) || seen[f.ProductID] {
			return invalid("favourite %d is not a product or is listed twice", i+1)
		}
		seen[f.ProductID] = true
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.pos_favourites (org_id, product_id, label, color, position) VALUES ($1,$2::uuid,$3,$4,$5)`,
			orgID, f.ProductID, trimTo(f.Label, 40), trimTo(f.Color, 20), i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---- stores ----

// Store is a shop with its own receipt header and registers.
type Store struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Address       string `json:"address"`
	Phone         string `json:"phone"`
	ReceiptHeader string `json:"receipt_header"`
	ReceiptFooter string `json:"receipt_footer"`
	Active        bool   `json:"active"`
}

// ListStores returns the stores.
func (r *Repository) ListStores(ctx context.Context, orgID string) ([]Store, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, address, phone, receipt_header, receipt_footer, active FROM adrient.pos_stores WHERE org_id=$1 ORDER BY lower(name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Store{}
	for rows.Next() {
		var s Store
		if err := rows.Scan(&s.ID, &s.Name, &s.Address, &s.Phone, &s.ReceiptHeader, &s.ReceiptFooter, &s.Active); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveStore creates (id empty) or updates a store.
func (r *Repository) SaveStore(ctx context.Context, orgID, id string, s Store) (Store, error) {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return s, invalid("name is required")
	}
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.pos_stores (org_id,name,address,phone,receipt_header,receipt_footer,active) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
			orgID, s.Name, s.Address, s.Phone, s.ReceiptHeader, s.ReceiptFooter, s.Active).Scan(&s.ID)
	} else {
		var n int
		err = r.pool.QueryRow(ctx, `WITH u AS (UPDATE adrient.pos_stores SET name=$3,address=$4,phone=$5,receipt_header=$6,receipt_footer=$7,active=$8 WHERE org_id=$1 AND id=$2::uuid RETURNING 1) SELECT count(*) FROM u`,
			orgID, id, s.Name, s.Address, s.Phone, s.ReceiptHeader, s.ReceiptFooter, s.Active).Scan(&n)
		if err == nil && n == 0 {
			err = ErrNotFound
		}
		s.ID = id
	}
	return s, err
}

// AssignRegisterStore puts a register in a store (storeID "" removes it).
func (r *Repository) AssignRegisterStore(ctx context.Context, orgID, registerID, storeID string) error {
	if !isUUID(registerID) {
		return invalid("unknown register")
	}
	var ok bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM adrient.pos_registers WHERE org_id=$1 AND id=$2::uuid)`, orgID, registerID).Scan(&ok); err != nil || !ok {
		return invalid("unknown register")
	}
	if storeID == "" {
		_, err := r.pool.Exec(ctx, `DELETE FROM adrient.pos_register_store WHERE register_id=$1::uuid`, registerID)
		return err
	}
	tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.pos_register_store (register_id, store_id) SELECT $2::uuid, id FROM adrient.pos_stores WHERE org_id=$1 AND id=$3::uuid
		ON CONFLICT (register_id) DO UPDATE SET store_id=EXCLUDED.store_id`, orgID, registerID, storeID)
	if err == nil && tag.RowsAffected() == 0 {
		return invalid("unknown store")
	}
	return err
}

// RegisterStoreMap returns register id -> store id.
func (r *Repository) RegisterStoreMap(ctx context.Context, orgID string) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT rs.register_id::text, rs.store_id::text FROM adrient.pos_register_store rs JOIN adrient.pos_stores s ON s.id=rs.store_id WHERE s.org_id=$1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[a] = b
	}
	return out, rows.Err()
}

// StoreForRegister returns the store a register belongs to (zero value if none).
func (r *Repository) StoreForRegister(ctx context.Context, orgID, registerID string) Store {
	var s Store
	if !isUUID(registerID) {
		return s
	}
	_ = r.pool.QueryRow(ctx, `SELECT s.id::text, s.name, s.address, s.phone, s.receipt_header, s.receipt_footer, s.active FROM adrient.pos_register_store rs
		JOIN adrient.pos_stores s ON s.id=rs.store_id WHERE s.org_id=$1 AND rs.register_id=$2::uuid`, orgID, registerID).
		Scan(&s.ID, &s.Name, &s.Address, &s.Phone, &s.ReceiptHeader, &s.ReceiptFooter, &s.Active)
	return s
}

// ---- Z report ----

// ZMethod is a payment method total.
type ZMethod struct {
	Method string  `json:"method"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

// ZReport is the end-of-day summary of one shift.
type ZReport struct {
	ShiftID      string    `json:"shift_id"`
	RegisterID   string    `json:"register_id"`
	Register     string    `json:"register"`
	Store        string    `json:"store"`
	OpenedAt     string    `json:"opened_at"`
	ClosedAt     string    `json:"closed_at"`
	Sales        int       `json:"sales"`
	Gross        float64   `json:"gross"`
	Discounts    float64   `json:"discounts"`
	Tax          float64   `json:"tax"`
	Refunds      float64   `json:"refunds"`
	RefundCount  int       `json:"refund_count"`
	Voids        int       `json:"voids"`
	VoidTotal    float64   `json:"void_total"`
	Net          float64   `json:"net"`
	ByMethod     []ZMethod `json:"by_method"`
	RefundMethod []ZMethod `json:"refund_by_method"`
	Opening      float64   `json:"opening_cash"`
	CashSales    float64   `json:"cash_sales"`
	CashRefunds  float64   `json:"cash_refunds"`
	PaidInOut    float64   `json:"paid_in_out"`
	Expected     float64   `json:"expected_cash"`
	Counted      float64   `json:"counted_cash"`
	Variance     float64   `json:"variance"`
	Closed       bool      `json:"closed"`
}

// ZReport builds the Z report of a shift from its sales, refunds and cash moves.
func (r *Repository) ZReport(ctx context.Context, orgID, shiftID string) (ZReport, error) {
	if !isUUID(shiftID) {
		return ZReport{}, ErrNotFound
	}
	var z ZReport
	var counted *float64
	err := r.pool.QueryRow(ctx, `SELECT s.id::text, s.register_id::text, rg.name, to_char(s.opened_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		COALESCE(to_char(s.closed_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),''), s.opening_cash::float8, s.counted_cash::float8, s.closed_at IS NOT NULL
		FROM adrient.pos_shifts s JOIN adrient.pos_registers rg ON rg.id=s.register_id WHERE s.org_id=$1 AND s.id=$2::uuid`, orgID, shiftID).
		Scan(&z.ShiftID, &z.RegisterID, &z.Register, &z.OpenedAt, &z.ClosedAt, &z.Opening, &counted, &z.Closed)
	if errors.Is(err, pgx.ErrNoRows) {
		return z, ErrNotFound
	}
	if err != nil {
		return z, err
	}
	z.Store = r.StoreForRegister(ctx, orgID, z.RegisterID).Name
	if err := r.pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(total),0)::float8, COALESCE(sum(discount_total),0)::float8, COALESCE(sum(tax_total),0)::float8
		FROM adrient.pos_sales WHERE org_id=$1 AND shift_id=$2::uuid AND number > 0 AND status IN ('completed','partially_refunded','refunded')`, orgID, shiftID).
		Scan(&z.Sales, &z.Gross, &z.Discounts, &z.Tax); err != nil {
		return z, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(total),0)::float8 FROM adrient.pos_refunds WHERE org_id=$1 AND shift_id=$2::uuid`, orgID, shiftID).
		Scan(&z.RefundCount, &z.Refunds); err != nil {
		return z, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(s.total),0)::float8 FROM adrient.pos_voids v JOIN adrient.pos_sales s ON s.id=v.sale_id
		WHERE v.org_id=$1 AND s.shift_id=$2::uuid`, orgID, shiftID).Scan(&z.Voids, &z.VoidTotal); err != nil {
		return z, err
	}
	z.ByMethod, z.RefundMethod = []ZMethod{}, []ZMethod{}
	rows, err := r.pool.Query(ctx, `SELECT p.method, COALESCE(sum(p.amount),0)::float8, count(*) FROM adrient.pos_payments p JOIN adrient.pos_sales s ON s.id=p.sale_id
		WHERE s.org_id=$1 AND s.shift_id=$2::uuid GROUP BY p.method ORDER BY p.method`, orgID, shiftID)
	if err != nil {
		return z, err
	}
	for rows.Next() {
		var m ZMethod
		if err := rows.Scan(&m.Method, &m.Amount, &m.Count); err != nil {
			rows.Close()
			return z, err
		}
		z.ByMethod = append(z.ByMethod, m)
		if m.Method == "cash" {
			z.CashSales = m.Amount
		}
	}
	rows.Close()
	rr, err := r.pool.Query(ctx, `SELECT method, COALESCE(sum(total),0)::float8, count(*) FROM adrient.pos_refunds WHERE org_id=$1 AND shift_id=$2::uuid GROUP BY method ORDER BY method`, orgID, shiftID)
	if err != nil {
		return z, err
	}
	for rr.Next() {
		var m ZMethod
		if err := rr.Scan(&m.Method, &m.Amount, &m.Count); err != nil {
			rr.Close()
			return z, err
		}
		z.RefundMethod = append(z.RefundMethod, m)
		if m.Method == "cash" {
			z.CashRefunds = m.Amount
		}
	}
	rr.Close()
	_ = r.pool.QueryRow(ctx, `SELECT COALESCE(sum(amount),0)::float8 FROM adrient.pos_cash_moves WHERE shift_id=$1::uuid`, shiftID).Scan(&z.PaidInOut)
	z.Net = round2(z.Gross - z.Refunds)
	// Cash taken is net of change given: payments rows already hold the applied amount.
	z.Expected = round2(z.Opening + z.CashSales - z.CashRefunds + z.PaidInOut)
	if counted != nil && z.Closed {
		z.Counted = *counted
		z.Variance = round2(z.Counted - z.Expected)
	}
	return z, nil
}
