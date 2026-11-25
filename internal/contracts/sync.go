package contracts

import (
	"context"
	"fmt"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

type want struct {
	kind, ref, app string
	owner          string
	title, desc    string
	day            Date
	done           bool
}

// desired lists the Calendar events and Tasks a contract should have right now.
func (r *Repository) desired(ctx context.Context, org string, f *Full) ([]want, error) {
	var out []want
	if f.DeletedOrOff() {
		return out, nil
	}
	today := r.today()
	info := f.Contract.RenewalInfo(today, 60)
	if info.Date != "" && DaysBetween(today, info.Date) >= 0 {
		verb := "ends"
		if f.AutoRenew && !(f.Decision != nil && endingDecision(f.Decision.Kind) && f.Decision.ForTermEnd == info.Date) {
			verb = "renews"
		}
		out = append(out, want{kind: "term_end", ref: info.Date, app: "calendar", owner: f.OwnerID, day: info.Date,
			title: fmt.Sprintf("Contract %s: %s", verb, f.Title), desc: fmt.Sprintf("%s with %s. Open Contracts: /contracts?contract=%s", f.Title, f.PartyName, f.ID)})
		if info.NoticeDate != "" && DaysBetween(today, info.NoticeDate) >= 0 && !info.Handled {
			out = append(out, want{kind: "notice", ref: info.NoticeDate, app: "calendar", owner: f.OwnerID, day: info.NoticeDate,
				title: fmt.Sprintf("Notice deadline: %s", f.Title), desc: fmt.Sprintf("Cancel or renegotiate %s by this date (%d days before the term ends). Open Contracts: /contracts?contract=%s", f.Title, f.NoticeDays, f.ID)})
			out = append(out, want{kind: "notice", ref: info.NoticeDate, app: "tasks", owner: f.OwnerID, day: info.NoticeDate,
				title: fmt.Sprintf("Decide on renewal: %s", f.Title), desc: fmt.Sprintf("Notice deadline for %s. Record your decision in Contracts: /contracts?contract=%s", f.Title, f.ID)})
		}
	}
	obs, err := r.obligations(ctx, org, f.ID)
	if err != nil {
		return nil, err
	}
	for _, o := range obs {
		if o.Done || o.Due == "" || o.Owner == "them" {
			continue
		}
		who := o.AssigneeID
		if who == "" {
			who = f.OwnerID
		}
		out = append(out, want{kind: "obligation", ref: o.ID, app: "tasks", owner: who, day: o.Due,
			title: fmt.Sprintf("%s (%s)", o.Title, f.Title), desc: fmt.Sprintf("Contract obligation. Open Contracts: /contracts?contract=%s", f.ID)})
	}
	return out, nil
}

// DeletedOrOff reports whether the contract should have no reminders.
func (f *Full) DeletedOrOff() bool { return f.Status != "active" || !f.Remind }

type link struct{ kind, ref, app, ext string }

// SyncDeadlines makes the contract's Calendar events and Tasks match its dates and open obligations. It is
// idempotent: items are matched through contracts_links, moved when a date changes and removed when no longer needed.
func (r *Repository) SyncDeadlines(ctx context.Context, a knowkit.Actor, contractID string) error {
	if r.Deps.Calendar == nil && r.Deps.Task == nil {
		return nil
	}
	var f *Full
	var gone bool
	row, err := scan(r.pool.QueryRow(ctx, `SELECT `+cols+` FROM adrient.contracts_contracts WHERE org_id=$1 AND id=$2`, a.Org, contractID))
	if err != nil {
		return err
	}
	f = row
	var deleted bool
	_ = r.pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM adrient.contracts_contracts WHERE id=$1`, contractID).Scan(&deleted)
	gone = deleted
	var wants []want
	if !gone {
		if wants, err = r.desired(ctx, a.Org, f); err != nil {
			return err
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT kind, ref, app, external_id::text FROM adrient.contracts_links WHERE contract_id=$1`, contractID)
	if err != nil {
		return err
	}
	have := map[string]link{}
	for rows.Next() {
		var l link
		if err := rows.Scan(&l.kind, &l.ref, &l.app, &l.ext); err != nil {
			rows.Close()
			return err
		}
		have[l.kind+"|"+l.ref+"|"+l.app] = l
	}
	rows.Close()
	keep := map[string]bool{}
	var firstErr error
	for _, w := range wants {
		key := w.kind + "|" + w.ref + "|" + w.app
		keep[key] = true
		cur, ok := have[key]
		id := ""
		if ok {
			id = cur.ext
		}
		var ext string
		var e error
		switch w.app {
		case "calendar":
			if r.Deps.Calendar == nil {
				continue
			}
			ext, e = r.Deps.Calendar(ctx, a.Org, w.owner, id, w.title, w.desc, w.day)
		case "tasks":
			if r.Deps.Task == nil {
				continue
			}
			ext, e = r.Deps.Task(ctx, a.Org, w.owner, id, w.title, w.desc, w.day, false)
		}
		if e != nil {
			if firstErr == nil {
				firstErr = e
			}
			continue
		}
		if _, e := r.pool.Exec(ctx, `INSERT INTO adrient.contracts_links (org_id, contract_id, kind, ref, app, external_id, due) VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (contract_id, kind, ref, app) DO UPDATE SET external_id=EXCLUDED.external_id, due=EXCLUDED.due`, a.Org, contractID, w.kind, w.ref, w.app, ext, w.day); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	for key, l := range have {
		if keep[key] {
			continue
		}
		var e error
		switch l.app {
		case "calendar":
			if r.Deps.DeleteEvent != nil {
				e = r.Deps.DeleteEvent(ctx, a.Org, l.ext)
			}
		case "tasks":
			if r.Deps.DeleteTask != nil {
				e = r.Deps.DeleteTask(ctx, a.Org, l.ext)
			}
		}
		_ = e // the Calendar event or task may already have been deleted by its owner
		if _, e := r.pool.Exec(ctx, `DELETE FROM adrient.contracts_links WHERE contract_id=$1 AND kind=$2 AND ref=$3 AND app=$4`, contractID, l.kind, l.ref, l.app); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	return firstErr
}
