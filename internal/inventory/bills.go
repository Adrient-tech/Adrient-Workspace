package inventory

// Supplier bills matched against purchase orders and goods receipts (three-way
// match). A posted bill becomes a vendor bill in Accounting.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// BillLine is one line of a supplier bill.
type BillLine struct {
	ID          string  `json:"id"`
	POLineID    string  `json:"po_line_id"`
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	UnitCost    float64 `json:"unit_cost"`
	TaxPct      float64 `json:"tax_pct"`
	LineTotal   float64 `json:"line_total"`
}

// Bill is a supplier bill.
type Bill struct {
	ID              string       `json:"id"`
	Number          string       `json:"number"`
	SupplierID      string       `json:"supplier_id"`
	SupplierName    string       `json:"supplier_name"`
	POID            string       `json:"po_id"`
	PONumber        string       `json:"po_number"`
	BillRef         string       `json:"bill_ref"`
	BillDate        string       `json:"bill_date"`
	DueDate         string       `json:"due_date"`
	State           string       `json:"state"`
	MatchStatus     string       `json:"match_status"`
	MatchDetail     []MatchIssue `json:"match_detail"`
	OverrideReason  string       `json:"override_reason"`
	Subtotal        float64      `json:"subtotal"`
	TaxTotal        float64      `json:"tax_total"`
	Total           float64      `json:"total"`
	AccountingState string       `json:"accounting_state"`
	AccountingError string       `json:"accounting_error"`
	CreatedAt       time.Time    `json:"created_at"`
	Lines           []BillLine   `json:"lines"`
}

// BillInput creates or replaces a draft bill.
type BillInput struct {
	SupplierID string `json:"supplier_id"`
	POID       string `json:"po_id"`
	BillRef    string `json:"bill_ref"`
	BillDate   string `json:"bill_date"`
	DueDate    string `json:"due_date"`
	Lines      []struct {
		POLineID    string  `json:"po_line_id"`
		ProductID   string  `json:"product_id"`
		Description string  `json:"description"`
		Qty         float64 `json:"qty"`
		UnitCost    float64 `json:"unit_cost"`
		TaxPct      float64 `json:"tax_pct"`
	} `json:"lines"`
}

// BillDraftFromPO proposes bill lines for what was received and not billed yet.
func (r *Repository) BillDraftFromPO(ctx context.Context, orgID, poID string) (BillInput, error) {
	po, err := r.GetPO(ctx, orgID, poID)
	if err != nil {
		return BillInput{}, err
	}
	in := BillInput{SupplierID: po.SupplierID, POID: poID, BillDate: time.Now().Format("2006-01-02")}
	for _, l := range po.Lines {
		q := round3(l.QtyReceived - l.QtyReturned - l.QtyBilled)
		if q <= 0 {
			continue
		}
		in.Lines = append(in.Lines, struct {
			POLineID    string  `json:"po_line_id"`
			ProductID   string  `json:"product_id"`
			Description string  `json:"description"`
			Qty         float64 `json:"qty"`
			UnitCost    float64 `json:"unit_cost"`
			TaxPct      float64 `json:"tax_pct"`
		}{POLineID: l.ID, ProductID: l.ProductID, Description: l.ProductName, Qty: q, UnitCost: l.UnitCost, TaxPct: l.TaxPct})
	}
	return in, nil
}

func (r *Repository) writeBill(ctx context.Context, tx pgx.Tx, orgID, id string, in BillInput) (sub, tax float64, err error) {
	if len(in.Lines) == 0 {
		return 0, 0, invalid("add at least one line")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.inv_supplier_bill_lines WHERE bill_id=$1`, id); err != nil {
		return 0, 0, err
	}
	for _, l := range in.Lines {
		if l.Qty <= 0 || l.UnitCost < 0 || l.TaxPct < 0 || l.TaxPct > 100 {
			return 0, 0, invalid("quantity must be positive; cost and tax cannot be negative")
		}
		if l.POLineID == "" && l.ProductID == "" && strings.TrimSpace(l.Description) == "" {
			return 0, 0, invalid("each line needs a product or a description")
		}
		if l.POLineID != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT true FROM adrient.inv_purchase_order_lines WHERE org_id=$1 AND id=$2 AND po_id=NULLIF($3,'')::uuid`, orgID, l.POLineID, in.POID).Scan(&ok); err != nil {
				return 0, 0, invalid("a line does not belong to the purchase order")
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO adrient.inv_supplier_bill_lines (bill_id, org_id, po_line_id, product_id, description, qty, unit_cost, tax_pct)
			 VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7,$8)`,
			id, orgID, l.POLineID, l.ProductID, l.Description, round3(l.Qty), round4(l.UnitCost), l.TaxPct); err != nil {
			return 0, 0, err
		}
		line := round2(l.Qty * l.UnitCost)
		sub += line
		tax += round2(line * l.TaxPct / 100)
	}
	return round2(sub), round2(tax), nil
}

// CreateBill saves a draft supplier bill and runs the match.
func (r *Repository) CreateBill(ctx context.Context, orgID, userID string, in BillInput) (Bill, error) {
	bd, err := parseDate(in.BillDate)
	if err != nil {
		return Bill{}, err
	}
	if bd == nil {
		t := time.Now()
		bd = &t
	}
	dd, err := parseDate(in.DueDate)
	if err != nil {
		return Bill{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Bill{}, err
	}
	defer tx.Rollback(ctx)
	supplier := in.SupplierID
	if in.POID != "" {
		if err := tx.QueryRow(ctx, `SELECT supplier_id::text FROM adrient.inv_purchase_orders WHERE org_id=$1 AND id=$2`, orgID, in.POID).Scan(&supplier); err != nil {
			return Bill{}, invalid("purchase order not found")
		}
	}
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT true FROM adrient.inv_suppliers WHERE org_id=$1 AND id=$2`, orgID, supplier).Scan(&ok); err != nil {
		return Bill{}, invalid("pick a supplier")
	}
	if in.BillRef != "" {
		var dup bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.inv_supplier_bills WHERE org_id=$1 AND supplier_id=$2 AND lower(bill_ref)=lower($3) AND state<>'void')`,
			orgID, supplier, in.BillRef).Scan(&dup); err != nil {
			return Bill{}, err
		}
		if dup {
			return Bill{}, invalid("this supplier invoice number was already entered")
		}
	}
	num, err := nextNumber(ctx, tx, orgID, "bill", "BILL")
	if err != nil {
		return Bill{}, err
	}
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO adrient.inv_supplier_bills (org_id, number, supplier_id, po_id, bill_ref, bill_date, due_date, created_by)
		 VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,NULLIF($8,'')::uuid) RETURNING id::text`,
		orgID, num, supplier, in.POID, in.BillRef, bd, dd, userID).Scan(&id); err != nil {
		return Bill{}, err
	}
	sub, tax, err := r.writeBill(ctx, tx, orgID, id, in)
	if err != nil {
		return Bill{}, err
	}
	status, detail, err := r.matchBill(ctx, tx, orgID, id)
	if err != nil {
		return Bill{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.inv_supplier_bills SET subtotal=$2, tax_total=$3, total=$4, match_status=$5, match_detail=$6 WHERE id=$1`,
		id, sub, tax, round2(sub+tax), status, detail); err != nil {
		return Bill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Bill{}, err
	}
	return r.GetBill(ctx, orgID, id)
}

// matchBill compares a bill with its PO and receipts. Lines of the same PO line
// are summed before comparing.
func (r *Repository) matchBill(ctx context.Context, tx pgx.Tx, orgID, billID string) (string, []MatchIssue, error) {
	var poID string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(po_id::text,'') FROM adrient.inv_supplier_bills WHERE id=$1`, billID).Scan(&poID); err != nil {
		return "", nil, err
	}
	st, err := getSettings(ctx, tx, orgID)
	if err != nil {
		return "", nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT COALESCE(b.po_line_id::text,''), COALESCE(p.name, b.description), b.qty::float8, b.unit_cost::float8,
		        COALESCE(l.qty::float8,0), COALESCE((l.qty_received - l.qty_returned)::float8,0), COALESCE(l.qty_billed::float8,0), COALESCE(l.unit_cost::float8,0)
		 FROM adrient.inv_supplier_bill_lines b
		 LEFT JOIN adrient.inv_purchase_order_lines l ON l.id=b.po_line_id
		 LEFT JOIN adrient.inv_products p ON p.id=COALESCE(l.product_id, b.product_id)
		 WHERE b.bill_id=$1 ORDER BY b.id`, billID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var lines []MatchLine
	idx := map[string]int{}
	for rows.Next() {
		var m MatchLine
		if err := rows.Scan(&m.LineID, &m.Name, &m.BillQty, &m.BillCost, &m.OrderedQty, &m.ReceivedQty, &m.BilledBefore, &m.POCost); err != nil {
			return "", nil, err
		}
		if i, ok := idx[m.LineID]; ok && m.LineID != "" { // same PO line twice: add quantity, keep the higher price
			lines[i].BillQty += m.BillQty
			if m.BillCost > lines[i].BillCost {
				lines[i].BillCost = m.BillCost
			}
			continue
		}
		idx[m.LineID] = len(lines)
		lines = append(lines, m)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if poID == "" {
		return "unmatched", []MatchIssue{}, nil
	}
	issues := ThreeWayMatch(lines, st.QtyTolerancePct, st.PriceTolerancePct)
	if issues == nil {
		issues = []MatchIssue{}
	}
	if len(issues) > 0 {
		return "exceptions", issues, nil
	}
	return "matched", issues, nil
}

// GetBill loads one bill.
func (r *Repository) GetBill(ctx context.Context, orgID, id string) (Bill, error) {
	bs, err := r.ListBills(ctx, orgID, id, "", "")
	if err != nil {
		return Bill{}, err
	}
	if len(bs) == 0 {
		return Bill{}, ErrNotFound
	}
	return bs[0], nil
}

// ListBills lists supplier bills.
func (r *Repository) ListBills(ctx context.Context, orgID, onlyID, state, supplierID string) ([]Bill, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT b.id::text, b.number, b.supplier_id::text, s.name, COALESCE(b.po_id::text,''), COALESCE(o.number,''), b.bill_ref,
		        to_char(b.bill_date,'YYYY-MM-DD'), COALESCE(to_char(b.due_date,'YYYY-MM-DD'),''), b.state, b.match_status, b.match_detail, b.override_reason,
		        b.subtotal::float8, b.tax_total::float8, b.total::float8, b.accounting_state, b.accounting_error, b.created_at
		 FROM adrient.inv_supplier_bills b JOIN adrient.inv_suppliers s ON s.id=b.supplier_id LEFT JOIN adrient.inv_purchase_orders o ON o.id=b.po_id
		 WHERE b.org_id=$1 AND ($2='' OR b.id::text=$2) AND ($3='' OR b.state=$3) AND ($4='' OR b.supplier_id::text=$4)
		 ORDER BY b.created_at DESC LIMIT 300`, orgID, onlyID, state, supplierID)
	if err != nil {
		return nil, err
	}
	out := []Bill{}
	idx := map[string]int{}
	var ids []string
	for rows.Next() {
		var b Bill
		var detail []byte
		if err := rows.Scan(&b.ID, &b.Number, &b.SupplierID, &b.SupplierName, &b.POID, &b.PONumber, &b.BillRef, &b.BillDate, &b.DueDate, &b.State, &b.MatchStatus,
			&detail, &b.OverrideReason, &b.Subtotal, &b.TaxTotal, &b.Total, &b.AccountingState, &b.AccountingError, &b.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal(detail, &b.MatchDetail)
		if b.MatchDetail == nil {
			b.MatchDetail = []MatchIssue{}
		}
		b.Lines = []BillLine{}
		idx[b.ID] = len(out)
		out = append(out, b)
		ids = append(ids, b.ID)
	}
	rows.Close()
	lr, err := r.pool.Query(ctx,
		`SELECT l.bill_id::text, l.id::text, COALESCE(l.po_line_id::text,''), COALESCE(l.product_id::text,''), COALESCE(p.name,''), l.description, l.qty::float8, l.unit_cost::float8, l.tax_pct::float8
		 FROM adrient.inv_supplier_bill_lines l LEFT JOIN adrient.inv_products p ON p.id=l.product_id WHERE l.org_id=$1 AND l.bill_id = ANY($2::uuid[]) ORDER BY l.id`, orgID, ids)
	if err != nil {
		return nil, err
	}
	defer lr.Close()
	for lr.Next() {
		var bid string
		var l BillLine
		if err := lr.Scan(&bid, &l.ID, &l.POLineID, &l.ProductID, &l.ProductName, &l.Description, &l.Qty, &l.UnitCost, &l.TaxPct); err != nil {
			return nil, err
		}
		l.LineTotal = round2(l.Qty * l.UnitCost)
		out[idx[bid]].Lines = append(out[idx[bid]].Lines, l)
	}
	return out, lr.Err()
}

// UpdateBill replaces a draft bill and re-runs the match.
func (r *Repository) UpdateBill(ctx context.Context, orgID, id string, in BillInput) (Bill, error) {
	bd, err := parseDate(in.BillDate)
	if err != nil {
		return Bill{}, err
	}
	dd, err := parseDate(in.DueDate)
	if err != nil {
		return Bill{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Bill{}, err
	}
	defer tx.Rollback(ctx)
	var state, poID string
	if err := tx.QueryRow(ctx, `SELECT state, COALESCE(po_id::text,'') FROM adrient.inv_supplier_bills WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&state, &poID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Bill{}, ErrNotFound
		}
		return Bill{}, err
	}
	if state != "draft" {
		return Bill{}, invalid("a posted bill cannot be edited")
	}
	in.POID = poID
	sub, tax, err := r.writeBill(ctx, tx, orgID, id, in)
	if err != nil {
		return Bill{}, err
	}
	status, detail, err := r.matchBill(ctx, tx, orgID, id)
	if err != nil {
		return Bill{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE adrient.inv_supplier_bills SET bill_ref=$3, bill_date=COALESCE($4,bill_date), due_date=$5, subtotal=$6, tax_total=$7, total=$8, match_status=$9, match_detail=$10 WHERE org_id=$1 AND id=$2`,
		orgID, id, in.BillRef, bd, dd, sub, tax, round2(sub+tax), status, detail); err != nil {
		return Bill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Bill{}, err
	}
	return r.GetBill(ctx, orgID, id)
}

// DeleteBill deletes a draft bill.
func (r *Repository) DeleteBill(ctx context.Context, orgID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.inv_supplier_bills WHERE org_id=$1 AND id=$2 AND state='draft'`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return invalid("only a draft bill can be deleted")
	}
	return nil
}

// ErrMatchFailed is returned when posting is blocked by the three-way match.
var ErrMatchFailed = errors.New("three-way match failed")

// PostBill posts a bill. A bill that does not match its PO and receipts is
// refused unless override is given (and recorded). Posting adds to the PO's
// billed quantities and creates the vendor bill in Accounting.
func (r *Repository) PostBill(ctx context.Context, orgID, userID, id, override string) (Bill, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Bill{}, err
	}
	defer tx.Rollback(ctx)
	var state, poID string
	if err := tx.QueryRow(ctx, `SELECT state, COALESCE(po_id::text,'') FROM adrient.inv_supplier_bills WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, id).Scan(&state, &poID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Bill{}, ErrNotFound
		}
		return Bill{}, err
	}
	if state != "draft" {
		return Bill{}, invalid("this bill is already %s", state)
	}
	// Lock PO lines so two bills cannot both use the same received quantity.
	if poID != "" {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM adrient.inv_purchase_order_lines WHERE po_id=$1 FOR UPDATE`, poID); err != nil {
			return Bill{}, err
		}
	}
	status, detail, err := r.matchBill(ctx, tx, orgID, id)
	if err != nil {
		return Bill{}, err
	}
	override = strings.TrimSpace(override)
	if status == "exceptions" && override == "" {
		msgs := []string{}
		for _, d := range detail {
			msgs = append(msgs, d.Name+": "+d.Detail)
		}
		return Bill{}, invalid("%s: %s. Fix the bill, receive the goods, or post with an override reason", ErrMatchFailed.Error(), strings.Join(msgs, "; "))
	}
	if _, err := tx.Exec(ctx,
		`UPDATE adrient.inv_purchase_order_lines l SET qty_billed = l.qty_billed + s.q
		 FROM (SELECT po_line_id, sum(qty) q FROM adrient.inv_supplier_bill_lines WHERE bill_id=$1 AND po_line_id IS NOT NULL GROUP BY po_line_id) s
		 WHERE l.id = s.po_line_id`, id); err != nil {
		return Bill{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE adrient.inv_supplier_bills SET state='posted', posted_at=now(), match_status=$2, match_detail=$3, override_reason=$4, accounting_state='pending' WHERE id=$1`,
		id, status, detail, override); err != nil {
		return Bill{}, err
	}
	if poID != "" { // fully received and fully billed: the order is done
		var open int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM adrient.inv_purchase_order_lines WHERE po_id=$1 AND (qty_received - qty_returned) > qty_billed + 0.0005`, poID).Scan(&open); err != nil {
			return Bill{}, err
		}
		if open == 0 {
			if _, err := tx.Exec(ctx, `UPDATE adrient.inv_purchase_orders SET state='closed', closed_at=now(), updated_at=now() WHERE id=$1 AND state='received'`, poID); err != nil {
				return Bill{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Bill{}, err
	}
	r.postBillJournal(ctx, orgID, userID, id)
	return r.GetBill(ctx, orgID, id)
}

// postBillJournal hands a posted bill to Accounting as a vendor bill (Dr stock
// and input tax, Cr payables). Failures are recorded and retried.
func (r *Repository) postBillJournal(ctx context.Context, orgID, userID, id string) {
	p := currentPoster()
	if p == nil {
		return
	}
	b, err := r.GetBill(ctx, orgID, id)
	if err != nil || b.State != "posted" || b.AccountingState == "posted" {
		return
	}
	st, err := r.GetPurchaseSettings(ctx, orgID)
	if err != nil {
		return
	}
	memo := "Supplier bill " + b.Number
	if b.BillRef != "" {
		memo += " (" + b.BillRef + ")"
	}
	lines := []JournalLine{{AccountKey: "inventory", Debit: b.Subtotal, Memo: memo}}
	lines = append(lines, taxLines(st.TaxSplit, b.TaxTotal, false, memo)...)
	lines = append(lines, JournalLine{AccountKey: "ap", Credit: b.Total, Memo: memo})
	e := JournalEntry{Date: b.BillDate, DueDate: b.DueDate, Memo: memo, Ref: b.BillRef, Source: "purchase_bill", SourceID: b.ID, ContactName: b.SupplierName, Lines: lines}
	if err := p.PostEntry(ctx, orgID, userID, e); err != nil {
		msg := err.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.inv_supplier_bills SET accounting_state='failed', accounting_error=$2 WHERE id=$1`, id, msg)
		return
	}
	_, _ = r.pool.Exec(ctx, `UPDATE adrient.inv_supplier_bills SET accounting_state='posted', accounting_error='' WHERE id=$1`, id)
}

// RetryBillJournals re-posts bills whose hand-over to Accounting failed or is pending.
func (r *Repository) RetryBillJournals(ctx context.Context, orgID string) int {
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM adrient.inv_supplier_bills WHERE org_id=$1 AND state='posted' AND accounting_state IN ('pending','failed')`, orgID)
	if err != nil {
		return 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		r.postBillJournal(ctx, orgID, "", id)
	}
	return len(ids)
}
