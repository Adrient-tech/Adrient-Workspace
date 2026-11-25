package accounting

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAuditBeforeAfterAttachmentsAndClose(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	bank, capital := idOf(t, repo, org, "bank"), idOf(t, repo, org, "capital")
	j, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-04-02", Memo: "Capital", Lines: []Line{{AccountID: bank, Debit: 100}, {AccountID: capital, Credit: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateManual(ctx, org, user, j.ID, JournalInput{Date: "2026-04-03", Memo: "Capital v2", Lines: []Line{{AccountID: bank, Debit: 250}, {AccountID: capital, Credit: 250}}}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.ListAudit(ctx, org, 50)
	found := false
	for _, a := range rows {
		if a.Action == "update" && a.Entity == "journal" {
			found = true
			if !strings.Contains(string(a.Before), `"Capital"`) || !strings.Contains(string(a.After), `"Capital v2"`) || !strings.Contains(string(a.After), "250") {
				t.Fatalf("before/after missing: %s / %s", a.Before, a.After)
			}
		}
	}
	if !found {
		t.Fatal("no journal update audit row")
	}
	files, err := repo.AddJournalFile(ctx, org, user, j.ID, Attachment{ID: "file-1", Name: "voucher.pdf"})
	if err != nil || len(files) != 1 {
		t.Fatalf("attach: %v %v", err, files)
	}
	_, _ = repo.AddJournalFile(ctx, org, user, j.ID, Attachment{ID: "file-1", Name: "voucher.pdf"})
	got, _ := repo.GetJournal(ctx, org, j.ID)
	if len(got.Attachments) != 1 {
		t.Fatalf("duplicate attach: %v", got.Attachments)
	}
	if _, err := repo.AddJournalFile(ctx, org, user, "00000000-0000-0000-0000-000000000000", Attachment{ID: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("attach to missing: %v", err)
	}
	if files, _ = repo.RemoveJournalFile(ctx, org, user, j.ID, "file-1"); len(files) != 0 {
		t.Fatal("remove")
	}

	// close checklist: a pending bank line blocks closing
	_, err = repo.pool.Exec(ctx, `INSERT INTO adrient.acct_bank_lines (org_id, account_id, line_date, description, amount, hash) VALUES ($1,$2::uuid,'2026-04-05','x',10,'h1')`, org, bank)
	if err != nil {
		t.Fatal(err)
	}
	c, err := repo.CloseChecklist(ctx, org, "2026-04-30")
	if err != nil || c.CanClose {
		t.Fatalf("pending bank line must block: %v %+v", err, c)
	}
	if _, err := repo.ClosePeriod(ctx, org, user, "2026-04-30", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("close blocked: %v", err)
	}
	_, _ = repo.pool.Exec(ctx, `UPDATE adrient.acct_bank_lines SET status='excluded' WHERE org_id=$1`, org)
	c, _ = repo.CloseChecklist(ctx, org, "2026-04-30")
	if !c.CanClose {
		t.Fatalf("should be closable: %+v", c.Items)
	}
	if _, err := repo.ClosePeriod(ctx, org, user, "2026-04-30", true); err != nil {
		t.Fatalf("close: %v", err)
	}
	st, _ := repo.GetSettings(ctx, org)
	if st.ClosingDate != "2026-04-30" {
		t.Fatalf("closing date %s", st.ClosingDate)
	}
	if _, err := repo.ClosePeriod(ctx, org, user, "2026-03-31", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("closing an earlier date: %v", err)
	}
	if _, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-04-20", Memo: "late", Lines: []Line{{AccountID: bank, Debit: 1}, {AccountID: capital, Credit: 1}}}); !errors.Is(err, ErrLocked) {
		t.Fatalf("closed period must lock: %v", err)
	}
}
