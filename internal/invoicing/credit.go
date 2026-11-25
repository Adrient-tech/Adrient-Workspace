package invoicing

import (
	"context"
	"fmt"
	"strings"
)

// CreditInput creates a credit note against an invoice. With no Lines the whole
// invoice is credited. Lines are the rows being credited (qty, price, tax as on
// the invoice, or a single custom line for a goodwill credit).
type CreditInput struct {
	Lines  []Line `json:"lines"`
	Reason string `json:"reason"`
	Apply  *bool  `json:"apply"` // apply to the invoice balance (default true)
}

// creditedSoFar sums the credit notes already issued against an invoice.
func (r *Repository) creditedSoFar(ctx context.Context, orgID, invoiceID string) (float64, error) {
	var v float64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(total),0)::float8 FROM adrient.invc_documents
		WHERE org_id=$1 AND kind='credit_note' AND credit_for=$2::uuid AND deleted_at IS NULL`, orgID, invoiceID).Scan(&v)
	return v, err
}

// CreateCreditNote issues a credit note for a sent invoice. The credit is
// applied to the invoice balance first; whatever is left over (the invoice was
// already paid) stays on the note and can be refunded with RefundCreditNote.
// A credit note can never exceed what is still creditable on the invoice.
func (r *Repository) CreateCreditNote(ctx context.Context, orgID, userID, invoiceID string, ci CreditInput) (Document, error) {
	inv, err := r.GetDocument(ctx, orgID, invoiceID)
	if err != nil {
		return inv, err
	}
	if inv.Kind != "invoice" || inv.Deleted || inv.Status == "draft" {
		return inv, invalid("a credit note needs a sent invoice")
	}
	lines := ci.Lines
	dk, dv := inv.DiscountKind, inv.DiscountValue
	if len(lines) == 0 {
		lines = inv.Lines
	} else {
		for i := range lines {
			if lines[i].Qty <= 0 {
				return inv, invalid("credit line %d: quantity must be positive", i+1)
			}
		}
		// A fixed document discount is scaled to the credited share so it is not
		// credited twice; a percentage discount carries over unchanged.
		if dk == "amount" {
			var full, part float64
			for _, l := range inv.Lines {
				full += l.Qty * l.Price * (1 - clampPct(l.DiscountPct)/100)
			}
			for _, l := range lines {
				part += l.Qty * l.Price * (1 - clampPct(l.DiscountPct)/100)
			}
			if full > 0 {
				dv = Round2(dv * part / full)
			}
		}
	}
	in := DocInput{Kind: "credit_note", ContactID: inv.ContactID, ClientName: inv.ClientName, ClientEmail: inv.ClientEmail,
		ClientAddress: inv.ClientAddress, ClientGSTIN: inv.ClientGSTIN, PlaceOfSupply: inv.PlaceOfSupply, Currency: inv.Currency,
		Lines: lines, DiscountKind: dk, DiscountValue: dv, Notes: strings.TrimSpace(ci.Reason), CreditFor: inv.ID,
		SourceRef: "invoice:" + inv.Number}
	t := Compute(lines, dk, dv)
	if s, err := r.GetSettings(ctx, orgID); err == nil {
		tt := t
		in2 := in
		in2.IssueDate = today().Format("2006-01-02")
		applyGST(&in2, &tt, s)
		t = tt
	}
	done, err := r.creditedSoFar(ctx, orgID, inv.ID)
	if err != nil {
		return inv, err
	}
	if t.Total <= 0 {
		return inv, invalid("the credit note total must be greater than zero")
	}
	if Round2(done+t.Total) > inv.Total+0.004 {
		return inv, invalid("credit notes would exceed the invoice total (%.2f already credited of %.2f)", done, inv.Total)
	}
	cn, err := r.SaveDocument(ctx, orgID, userID, "", in)
	if err != nil {
		return cn, err
	}
	// Issued immediately: a credit note is never a draft.
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.invc_documents SET status='sent', sent_at=now() WHERE org_id=$1 AND id=$2::uuid`, orgID, cn.ID); err != nil {
		return cn, err
	}
	apply := ci.Apply == nil || *ci.Apply
	if apply {
		if cn, err = r.ApplyCreditNote(ctx, orgID, userID, cn.ID, inv.ID, 0); err != nil {
			return cn, err
		}
	}
	return r.GetDocument(ctx, orgID, cn.ID)
}

// ApplyCreditNote moves part of a credit note onto an invoice's balance. amount
// 0 means "as much as possible". Both sides get a row (the invoice a "credit",
// the note an "applied") so either document tells the whole story.
func (r *Repository) ApplyCreditNote(ctx context.Context, orgID, userID, cnID, invoiceID string, amount float64) (Document, error) {
	cn, err := r.GetDocument(ctx, orgID, cnID)
	if err != nil {
		return cn, err
	}
	inv, err := r.GetDocument(ctx, orgID, invoiceID)
	if err != nil {
		return cn, err
	}
	if cn.Kind != "credit_note" || inv.Kind != "invoice" || inv.Deleted || inv.Status == "draft" {
		return cn, invalid("apply a credit note to a sent invoice")
	}
	if cn.ContactID != inv.ContactID || (cn.ContactID == "" && !strings.EqualFold(cn.ClientName, inv.ClientName)) {
		return cn, invalid("a credit note can only be applied to an invoice of the same customer")
	}
	if cn.Currency != inv.Currency {
		return cn, invalid("currencies differ (%s and %s)", cn.Currency, inv.Currency)
	}
	max := min2(cn.CreditRemain, inv.Balance)
	if amount <= 0 {
		amount = max
	}
	amount = Round2(amount)
	if amount > max+0.004 {
		return cn, invalid("at most %.2f can be applied (credit left %.2f, invoice balance %.2f)", max, cn.CreditRemain, inv.Balance)
	}
	if amount <= 0 {
		return cn, nil // nothing to apply, e.g. the invoice is already paid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return cn, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, row := range []struct{ doc, kind, ref string }{{inv.ID, "credit", cn.Number}, {cn.ID, "applied", inv.Number}} {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.invc_payments (org_id, document_id, kind, amount, method, reference, note, paid_at, created_by)
			VALUES ($1,$2::uuid,$3,$4,'credit_note',$5,$6,CURRENT_DATE,NULLIF($7,'')::uuid)`,
			orgID, row.doc, row.kind, amount, row.ref, fmt.Sprintf("%s %s", cn.Number, inv.Number), userID); err != nil {
			return cn, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return cn, err
	}
	return r.GetDocument(ctx, orgID, cnID)
}

// RefundCreditNote pays the unused part of a credit note back to the customer.
func (r *Repository) RefundCreditNote(ctx context.Context, orgID, userID, cnID string, amount float64, method, reference string) (Document, error) {
	cn, err := r.GetDocument(ctx, orgID, cnID)
	if err != nil {
		return cn, err
	}
	if cn.Kind != "credit_note" || cn.Deleted {
		return cn, invalid("not a credit note")
	}
	if amount <= 0 {
		amount = cn.CreditRemain
	}
	amount = Round2(amount)
	if amount <= 0 || amount > cn.CreditRemain+0.004 {
		return cn, invalid("refund must be between 0.01 and the credit left (%.2f)", cn.CreditRemain)
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.invc_payments (org_id, document_id, kind, amount, method, reference, note, paid_at, created_by)
		VALUES ($1,$2::uuid,'refund',$3,$4,$5,'Credit note refund',CURRENT_DATE,NULLIF($6,'')::uuid)`,
		orgID, cnID, amount, method, reference, userID); err != nil {
		return cn, err
	}
	return r.GetDocument(ctx, orgID, cnID)
}

func min2(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
