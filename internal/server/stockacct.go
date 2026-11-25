package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
)

// stockPoster hands Inventory/Purchasing entries (stock_cogs, stock_adjustment,
// purchase_bill, purchase_return) to Accounting. Lines name accounts by system
// key ("inventory", "cogs", "ap", "gst_in_cgst" ...). This is the one place to
// change if Accounting's posting API changes: wave 3 Accounting offers
// PostSourceJournal(ctx, org, user, accounting.SourcePost{...}) which takes the
// same keys, and can replace the body of PostEntry.
type stockPoster struct{ repo *accounting.Repository }

func (p stockPoster) PostEntry(ctx context.Context, orgID, userID string, e inventory.JournalEntry) error {
	if err := p.repo.EnsureSeeded(ctx, orgID); err != nil {
		return err
	}
	accts, err := p.repo.ListAccounts(ctx, orgID)
	if err != nil {
		return err
	}
	byKey := map[string]string{}
	for _, a := range accts {
		if a.SystemKey != "" {
			byKey[a.SystemKey] = a.ID
		}
	}
	in := accounting.JournalInput{Date: e.Date, DueDate: e.DueDate, Memo: e.Memo, Ref: e.Ref, Kind: "journal", Source: e.Source, SourceID: e.SourceID, ContactName: e.ContactName}
	if e.Source == "purchase_bill" {
		in.Kind = "bill"
	}
	h := sha1.New()
	for _, l := range e.Lines {
		id, ok := byKey[l.AccountKey]
		if !ok {
			return fmt.Errorf("accounting has no %q account (check the chart of accounts)", l.AccountKey)
		}
		in.Lines = append(in.Lines, accounting.Line{AccountID: id, Debit: l.Debit, Credit: l.Credit, Memo: l.Memo})
		fmt.Fprintf(h, "%s|%.2f|%.2f;", l.AccountKey, l.Debit, l.Credit)
	}
	in.SourceHash = strings.TrimSpace(e.Date + hex.EncodeToString(h.Sum(nil)))
	_, _, err = p.repo.PostJournal(ctx, orgID, userID, in)
	return err
}

// startStockJournalSync wires the poster and flushes pending entries every 30s,
// so sales rung up through POS or Invoicing reach Accounting too.
func startStockJournalSync(ctx context.Context, inv *inventory.Repository, acct *accounting.Repository) {
	inventory.SetJournalPoster(stockPoster{repo: acct})
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				inv.FlushAllJournals(ctx)
			}
		}
	}()
}

// stockReserver lets confirmed sales orders hold stock in Inventory.
type stockReserver struct{ repo *inventory.Repository }

func (s stockReserver) ReserveForOrder(ctx context.Context, orgID, userID, ref, productID string, qty float64) error {
	_, err := s.repo.Reserve(ctx, orgID, userID, inventory.ReserveInput{ProductID: productID, Qty: qty, RefType: "sales_order", Ref: ref, Backorder: true, Note: "Sales order " + ref})
	return err
}

func (s stockReserver) ReleaseOrder(ctx context.Context, orgID, ref string) error {
	_, err := s.repo.ReleaseReservations(ctx, orgID, ref)
	return err
}
