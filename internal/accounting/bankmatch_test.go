package accounting

import (
	"context"
	"errors"
	"testing"
)

func TestScoreBankMatchRules(t *testing.T) {
	l := BankLine{ID: "l1", Date: "2026-05-12", Description: "UPI Landlord rent May", Amount: -123.45, Status: "pending"}
	c := MatchCandidate{JournalID: "j1", Date: "2026-05-11", Memo: "rent bill", Contact: "Landlord", Amount: -123.45}
	s, r, d, ok := ScoreBankMatch(l, c, 5)
	if !ok || d != 1 || s < 90 || len(r) != 3 {
		t.Fatalf("score %d %v %d %v", s, r, d, ok)
	}
	for name, mod := range map[string]func(MatchCandidate) MatchCandidate{
		"one paisa off":    func(c MatchCandidate) MatchCandidate { c.Amount = -123.46; return c },
		"wrong direction":  func(c MatchCandidate) MatchCandidate { c.Amount = 123.45; return c },
		"too late":         func(c MatchCandidate) MatchCandidate { c.Date = "2026-05-01"; return c },
		"bank before book": func(c MatchCandidate) MatchCandidate { c.Date = "2026-05-20"; return c },
	} {
		if _, _, _, ok := ScoreBankMatch(l, mod(c), 5); ok {
			t.Fatalf("%s should not match", name)
		}
	}
}

func TestSuggestBankMatchesOneToOne(t *testing.T) {
	lines := []BankLine{
		{ID: "a", Date: "2026-05-10", Description: "ACME", Amount: -100, Status: "pending"},
		{ID: "b", Date: "2026-05-11", Description: "ACME", Amount: -100, Status: "pending"},
		{ID: "c", Date: "2026-05-11", Description: "done", Amount: -100, Status: "added"},
	}
	cands := []MatchCandidate{{JournalID: "j1", Date: "2026-05-10", Memo: "ACME", Amount: -100}}
	got := SuggestBankMatches(lines, cands, 5)
	if len(got) != 1 || got[0].LineID != "a" || got[0].JournalID != "j1" {
		t.Fatalf("closest/same-day line must win, one pair only: %+v", got)
	}
}

func TestMatchBankLineDoesNotDoublePost(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	bank, capital := idOf(t, repo, org, "bank"), idOf(t, repo, org, "capital")
	j, _, err := repo.PostJournal(ctx, org, user, JournalInput{Date: "2026-05-11", Memo: "Capital", Lines: []Line{{AccountID: bank, Debit: 500.25}, {AccountID: capital, Credit: 500.25}}})
	if err != nil {
		t.Fatal(err)
	}
	ins := func(h string, amt float64) string {
		var id string
		if err := repo.pool.QueryRow(ctx, `INSERT INTO adrient.acct_bank_lines (org_id, account_id, line_date, description, amount, hash) VALUES ($1,$2::uuid,'2026-05-12','Capital intro',$3,$4) RETURNING id::text`, org, bank, amt, h).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	good, wrong, second := ins("m1", 500.25), ins("m2", 500.20), ins("m3", 500.25)
	sug, err := repo.SuggestMatches(ctx, org, bank)
	if err != nil || len(sug) != 1 || sug[0].JournalID != j.ID {
		t.Fatalf("suggestions: %v %+v", err, sug)
	}
	if _, err := repo.MatchBankLine(ctx, org, user, wrong, j.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("amount mismatch must be refused: %v", err)
	}
	before, _ := repo.LoadPostings(ctx, org, "")
	l, err := repo.MatchBankLine(ctx, org, user, good, j.ID)
	if err != nil || l.Status != "matched" || l.JournalID != j.ID {
		t.Fatalf("match: %v %+v", err, l)
	}
	after, _ := repo.LoadPostings(ctx, org, "")
	if len(after) != len(before) {
		t.Fatalf("matching posted %d new lines", len(after)-len(before))
	}
	if _, err := repo.MatchBankLine(ctx, org, user, second, j.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a journal can be matched once: %v", err)
	}
	if c, _ := repo.MatchCandidates(ctx, org, bank); len(c) != 0 {
		t.Fatalf("matched journal still offered: %+v", c)
	}
	// undo only releases the link, the journal stays
	if err := repo.UndoBankLine(ctx, org, user, good); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetJournal(ctx, org, j.ID); err != nil {
		t.Fatalf("undo must keep the journal: %v", err)
	}
	if c, _ := repo.MatchCandidates(ctx, org, bank); len(c) != 1 {
		t.Fatalf("journal offered again after undo: %+v", c)
	}
}
