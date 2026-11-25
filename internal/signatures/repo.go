package signatures

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
)

const reqCols = `r.id::text, r.title, r.message, r.status, r.sender_id::text, COALESCE(NULLIF(u.display_name,''),u.email,''), r.file_id::text, r.file_name, r.doc_hash,
	COALESCE(r.signed_file_id::text,''), r.signed_hash, r.sequential, r.expires_at, COALESCE(r.contract_id::text,''), r.created_at, r.sent_at, r.completed_at`

func scanReq(row pgx.Row) (*Request, error) {
	q := &Request{}
	err := row.Scan(&q.ID, &q.Title, &q.Message, &q.Status, &q.SenderID, &q.SenderName, &q.FileID, &q.FileName, &q.DocHash, &q.SignedFileID, &q.SignedHash,
		&q.Sequential, &q.ExpiresAt, &q.ContractID, &q.CreatedAt, &q.SentAt, &q.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, knowkit.ErrNotFound
	}
	return q, err
}

func (r *Repository) canManage(a knowkit.Actor, q *Request) bool {
	return q.SenderID == a.User || a.Admin
}

// load returns a request the caller may see: the sender, an org admin, or a signer who is an org user.
func (r *Repository) load(ctx context.Context, a knowkit.Actor, id string, needManage bool) (*Request, error) {
	if !knowkit.IsUUID(id) {
		return nil, knowkit.ErrNotFound
	}
	q, err := scanReq(r.pool.QueryRow(ctx, `SELECT `+reqCols+` FROM adrient.sig_requests r LEFT JOIN adrient.users u ON u.id=r.sender_id WHERE r.org_id=$1 AND r.id=$2`, a.Org, id))
	if err != nil {
		return nil, err
	}
	q.CanManage = r.canManage(a, q)
	if !q.CanManage {
		var n int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.sig_signers WHERE request_id=$1 AND user_id=$2`, id, a.User).Scan(&n)
		if n == 0 {
			return nil, knowkit.ErrNotFound
		}
		if needManage {
			return nil, knowkit.ErrForbidden
		}
	}
	return q, nil
}

func (r *Repository) people(ctx context.Context, q *Request) error {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, email, COALESCE(user_id::text,''), COALESCE(contact_id::text,''), role, order_idx, status, viewed_at, signed_at, declined_at, decline_reason, signature_kind, sign_ip
		FROM adrient.sig_signers WHERE request_id=$1 ORDER BY order_idx, name`, q.ID)
	if err != nil {
		return err
	}
	q.Signers = []Signer{}
	done, total := 0, 0
	for rows.Next() {
		var s Signer
		if err := rows.Scan(&s.ID, &s.Name, &s.Email, &s.UserID, &s.ContactID, &s.Role, &s.Order, &s.Status, &s.ViewedAt, &s.SignedAt, &s.DeclinedAt, &s.DeclineReason, &s.SignatureKind, &s.SignIP); err != nil {
			rows.Close()
			return err
		}
		if s.Role == "signer" {
			total++
			if s.Status == "signed" {
				done++
			}
		}
		q.Signers = append(q.Signers, s)
	}
	rows.Close()
	q.Progress = fmt.Sprintf("%d of %d signed", done, total)
	frows, err := r.pool.Query(ctx, `SELECT id::text, signer_id::text, type, label, page, x, y, w, h, required, value FROM adrient.sig_fields WHERE request_id=$1 ORDER BY page, y, x`, q.ID)
	if err != nil {
		return err
	}
	defer frows.Close()
	q.Fields = []Field{}
	for frows.Next() {
		var f Field
		if err := frows.Scan(&f.ID, &f.SignerID, &f.Type, &f.Label, &f.Page, &f.X, &f.Y, &f.W, &f.H, &f.Required, &f.Value); err != nil {
			return err
		}
		q.Fields = append(q.Fields, f)
	}
	return frows.Err()
}

// Get returns one request with its people and fields.
func (r *Repository) Get(ctx context.Context, a knowkit.Actor, id string) (*Request, error) {
	q, err := r.load(ctx, a, id, false)
	if err != nil {
		return nil, err
	}
	if err := r.expireIfNeeded(ctx, q); err != nil {
		return nil, err
	}
	if err := r.people(ctx, q); err != nil {
		return nil, err
	}
	if !q.CanManage { // signers see only their own details through their link; here they see names and progress
		for i := range q.Signers {
			q.Signers[i].SignIP = ""
		}
	}
	return q, nil
}

// List returns requests the caller sent (admins: all) and those they were asked to sign.
func (r *Repository) List(ctx context.Context, a knowkit.Actor, status string) ([]*Request, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+reqCols+` FROM adrient.sig_requests r LEFT JOIN adrient.users u ON u.id=r.sender_id
		WHERE r.org_id=$1 AND ($2='' OR r.status=$2) AND ($3 OR r.sender_id=$4 OR EXISTS(SELECT 1 FROM adrient.sig_signers s WHERE s.request_id=r.id AND s.user_id=$4))
		ORDER BY r.created_at DESC LIMIT 300`, a.Org, status, a.Admin, a.User)
	if err != nil {
		return nil, err
	}
	var out []*Request
	for rows.Next() {
		q, err := scanReq(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		q.CanManage = r.canManage(a, q)
		out = append(out, q)
	}
	rows.Close()
	for _, q := range out {
		_ = r.expireIfNeeded(ctx, q)
		if err := r.people(ctx, q); err != nil {
			return nil, err
		}
		q.Fields = nil
	}
	if out == nil {
		out = []*Request{}
	}
	return out, nil
}

func (r *Repository) expireIfNeeded(ctx context.Context, q *Request) error {
	if q.Status == "sent" && q.ExpiresAt != nil && r.now().After(*q.ExpiresAt) {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		var st, org string
		if err := tx.QueryRow(ctx, `SELECT status, org_id::text FROM adrient.sig_requests WHERE id=$1 FOR UPDATE`, q.ID).Scan(&st, &org); err != nil || st != "sent" {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.sig_requests SET status='expired' WHERE id=$1`, q.ID); err != nil {
			return err
		}
		if err := r.record(ctx, tx, org, q.ID, Event{Event: "expired", Actor: "system"}); err != nil {
			return err
		}
		q.Status = "expired"
		return tx.Commit(ctx)
	}
	return nil
}

// Create stores a draft request. The PDF must be a file the caller owns in Drive.
func (r *Repository) Create(ctx context.Context, a knowkit.Actor, in Input) (*Request, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	if r.Deps.ReadFile == nil {
		return nil, knowkit.ErrUnavailable
	}
	data, name, err := r.Deps.ReadFile(ctx, a.Org, a.User, in.FileID)
	if err != nil {
		return nil, knowkit.Invalid("that Drive file cannot be used: %v", err)
	}
	if err := checkPDF(data); err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var exp any
	if in.ExpireDays > 0 {
		exp = r.now().AddDate(0, 0, in.ExpireDays)
	}
	var contract any
	if in.ContractID != "" {
		contract = in.ContractID
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.sig_requests (org_id, title, message, sender_id, file_id, file_name, doc_hash, sequential, expires_at, contract_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`,
		a.Org, in.Title, in.Message, a.User, in.FileID, name, sha(data), in.Sequential, exp, contract).Scan(&id); err != nil {
		return nil, err
	}
	ids := make([]string, len(in.Signers))
	for i, s := range in.Signers {
		var uid, cid any
		if s.UserID != "" {
			uid = s.UserID
		}
		if s.ContactID != "" {
			cid = s.ContactID
		}
		order := 0
		if in.Sequential {
			order = i
		}
		if err := tx.QueryRow(ctx, `INSERT INTO adrient.sig_signers (org_id, request_id, name, email, user_id, contact_id, role, order_idx) VALUES ($1,$2,$3,$4,
			(SELECT id FROM adrient.users WHERE id=$5::uuid AND org_id=$1), $6, $7, $8) RETURNING id::text`, a.Org, id, s.Name, s.Email, uid, cid, s.Role, order).Scan(&ids[i]); err != nil {
			return nil, err
		}
	}
	for _, f := range in.Fields {
		req := f.Required == nil || *f.Required
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.sig_fields (org_id, request_id, signer_id, type, label, page, x, y, w, h, required) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			a.Org, id, ids[f.SignerIndex], f.Type, f.Label, f.Page, f.X, f.Y, f.W, f.H, req); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.sig_documents (request_id, org_id, data) VALUES ($1,$2,$3)`, id, a.Org, data); err != nil {
		return nil, err
	}
	if err := r.record(ctx, tx, a.Org, id, Event{Event: "created", Actor: a.User, Detail: name + " sha256:" + sha(data)}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	r.audit(ctx, "signature.create", "sig_request", id)
	return r.Get(ctx, a, id)
}

// Send turns the draft into a live request: every signer gets a private link. Links are returned once to the
// sender (and mailed when mail is configured); only a hash of each token is stored.
func (r *Repository) Send(ctx context.Context, a knowkit.Actor, id, baseURL string) (*Request, error) {
	q, err := r.load(ctx, a, id, true)
	if err != nil {
		return nil, err
	}
	if q.Status != "draft" {
		return nil, knowkit.Invalid("only a draft can be sent")
	}
	if err := r.people(ctx, q); err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var st string
	if err := tx.QueryRow(ctx, `SELECT status FROM adrient.sig_requests WHERE id=$1 FOR UPDATE`, id).Scan(&st); err != nil || st != "draft" {
		return nil, knowkit.Invalid("only a draft can be sent")
	}
	links := map[string]string{}
	for _, s := range q.Signers {
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, err
		}
		tok := hex.EncodeToString(raw[:])
		if _, err := tx.Exec(ctx, `UPDATE adrient.sig_signers SET token_hash=$2 WHERE id=$1`, s.ID, hashToken(tok)); err != nil {
			return nil, err
		}
		links[s.ID] = strings.TrimRight(baseURL, "/") + "/sign/" + tok
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_requests SET status='sent', sent_at=$2 WHERE id=$1`, id, r.now()); err != nil {
		return nil, err
	}
	if err := r.record(ctx, tx, a.Org, id, Event{Event: "sent", Actor: a.User, Detail: fmt.Sprintf("%d people", len(q.Signers))}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	for _, s := range q.Signers {
		if q.Sequential && s.Role == "signer" && s.Order > 0 {
			continue // the next person is asked when the previous one has signed
		}
		r.mailInvite(ctx, q, s, links[s.ID])
	}
	r.audit(ctx, "signature.send", "sig_request", id)
	out, err := r.Get(ctx, a, id)
	if err != nil {
		return nil, err
	}
	for i := range out.Signers {
		out.Signers[i].Link = links[out.Signers[i].ID]
	}
	return out, nil
}

func (r *Repository) mailInvite(ctx context.Context, q *Request, s Signer, link string) {
	if s.UserID != "" && r.Deps.Notify != nil && s.Role == "signer" {
		r.Deps.Notify(ctx, "", s.UserID, "Please sign: "+q.Title, q.SenderName+" asked you to sign "+q.Title, link)
	}
	if r.Deps.Mail == nil {
		return
	}
	verb := "review and sign"
	if s.Role == "cc" {
		verb = "view"
	}
	subj := fmt.Sprintf("%s asks you to %s: %s", q.SenderName, verb, q.Title)
	body := fmt.Sprintf(`<p>Hello %s,</p><p>%s asked you to %s <b>%s</b>.</p>%s<p><a href="%s">Open the document</a></p><p>This link is personal. Do not forward it.</p>`,
		html.EscapeString(s.Name), html.EscapeString(q.SenderName), verb, html.EscapeString(q.Title), msgHTML(q.Message), html.EscapeString(link))
	text := fmt.Sprintf("Hello %s,\n\n%s asked you to %s %s.\n%s\nOpen: %s\n\nThis link is personal. Do not forward it.\n", s.Name, q.SenderName, verb, q.Title, q.Message, link)
	_ = r.Deps.Mail(ctx, []string{s.Email}, subj, body, text)
}

func msgHTML(m string) string {
	if strings.TrimSpace(m) == "" {
		return ""
	}
	return "<blockquote>" + strings.ReplaceAll(html.EscapeString(m), "\n", "<br>") + "</blockquote>"
}

// Void cancels a live request; the links stop working.
func (r *Repository) Void(ctx context.Context, a knowkit.Actor, id, reason string) error {
	q, err := r.load(ctx, a, id, true)
	if err != nil {
		return err
	}
	if q.Status != "sent" && q.Status != "draft" {
		return knowkit.Invalid("a %s request cannot be voided", q.Status)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_requests SET status='voided' WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_signers SET token_hash='' WHERE request_id=$1 AND status NOT IN ('signed')`, id); err != nil {
		return err
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	if err := r.record(ctx, tx, a.Org, id, Event{Event: "voided", Actor: a.User, Detail: reason}); err != nil {
		return err
	}
	r.audit(ctx, "signature.void", "sig_request", id)
	return tx.Commit(ctx)
}

// Delete removes a draft, voided, declined or expired request (completed ones are kept for the record).
func (r *Repository) Delete(ctx context.Context, a knowkit.Actor, id string) error {
	q, err := r.load(ctx, a, id, true)
	if err != nil {
		return err
	}
	if q.Status == "sent" || q.Status == "completed" {
		return knowkit.Invalid("void a live request first; completed requests are kept")
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.sig_requests WHERE org_id=$1 AND id=$2`, a.Org, id)
	return err
}

// Remind re-sends the link to everyone who has not signed yet. The token is replaced, so earlier links stop working.
func (r *Repository) Remind(ctx context.Context, a knowkit.Actor, id, baseURL string) (int, error) {
	q, err := r.load(ctx, a, id, true)
	if err != nil {
		return 0, err
	}
	if q.Status != "sent" {
		return 0, knowkit.Invalid("only a live request can be reminded")
	}
	if err := r.people(ctx, q); err != nil {
		return 0, err
	}
	n := 0
	for _, s := range q.Signers {
		if s.Role != "signer" || s.Status == "signed" || s.Status == "declined" {
			continue
		}
		if q.Sequential && !r.myTurn(q, s) {
			continue
		}
		var raw [32]byte
		_, _ = rand.Read(raw[:])
		tok := hex.EncodeToString(raw[:])
		if _, err := r.pool.Exec(ctx, `UPDATE adrient.sig_signers SET token_hash=$2 WHERE id=$1`, s.ID, hashToken(tok)); err != nil {
			return n, err
		}
		r.mailInvite(ctx, q, s, strings.TrimRight(baseURL, "/")+"/sign/"+tok)
		n++
	}
	_ = r.record(ctx, r.pool, a.Org, id, Event{Event: "reminder", Actor: a.User, Detail: fmt.Sprintf("%d people", n)})
	return n, nil
}

// myTurn reports whether every signer before s (in a sequential request) has signed.
func (r *Repository) myTurn(q *Request, s Signer) bool {
	for _, o := range q.Signers {
		if o.Role == "signer" && o.Order < s.Order && o.Status != "signed" {
			return false
		}
	}
	return true
}

func sha(b []byte) string { return hashBytes(b) }

func checkPDF(b []byte) error {
	if len(b) == 0 {
		return knowkit.Invalid("the file is empty")
	}
	if len(b) > MaxPDFBytes {
		return knowkit.Invalid("the PDF is larger than %d MB", MaxPDFBytes>>20)
	}
	if !bytes.HasPrefix(bytes.TrimLeft(b[:min(len(b), 1024)], "\x00\r\n\t "), []byte("%PDF-")) {
		return knowkit.Invalid("only PDF files can be sent for signature")
	}
	return nil
}

var _ = time.Second
