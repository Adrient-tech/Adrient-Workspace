package signatures

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
)

// Client is what the signer's browser reports about itself, stored in the audit trail.
type Client struct{ IP, UA, Base string }

type tokenInfo struct {
	org      string
	req      *Request
	signer   Signer
	rawOrder int
}

// byToken finds the signer a link belongs to. Unknown, voided and expired links all look the same to the caller.
func (r *Repository) byToken(ctx context.Context, tok string) (*tokenInfo, error) {
	if len(tok) != 64 {
		return nil, knowkit.ErrNotFound
	}
	var org, reqID, sid string
	if err := r.pool.QueryRow(ctx, `SELECT org_id::text, request_id::text, id::text FROM adrient.sig_signers WHERE token_hash=$1 AND token_hash<>''`, hashToken(tok)).Scan(&org, &reqID, &sid); err != nil {
		return nil, knowkit.ErrNotFound
	}
	q, err := scanReq(r.pool.QueryRow(ctx, `SELECT `+reqCols+` FROM adrient.sig_requests r LEFT JOIN adrient.users u ON u.id=r.sender_id WHERE r.org_id=$1 AND r.id=$2`, org, reqID))
	if err != nil {
		return nil, err
	}
	if err := r.expireIfNeeded(ctx, q); err != nil {
		return nil, err
	}
	if err := r.people(ctx, q); err != nil {
		return nil, err
	}
	for _, s := range q.Signers {
		if s.ID == sid {
			return &tokenInfo{org: org, req: q, signer: s}, nil
		}
	}
	return nil, knowkit.ErrNotFound
}

// PublicDoc is what a signer's page shows.
type PublicDoc struct {
	Title      string   `json:"title"`
	Message    string   `json:"message"`
	Sender     string   `json:"sender"`
	FileName   string   `json:"file_name"`
	Status     string   `json:"status"`
	Signer     Signer   `json:"signer"`
	Fields     []Field  `json:"fields"`
	Waiting    bool     `json:"waiting"`
	Others     []string `json:"others"`
	CanSign    bool     `json:"can_sign"`
	DoneSigned bool     `json:"done_signed"`
	SignerNote string   `json:"note,omitempty"`
}

// PublicView returns the request for a signer and records that they opened it.
func (r *Repository) PublicView(ctx context.Context, tok string, c Client) (*PublicDoc, error) {
	t, err := r.byToken(ctx, tok)
	if err != nil {
		return nil, err
	}
	q := t.req
	d := &PublicDoc{Title: q.Title, Message: q.Message, Sender: q.SenderName, FileName: q.FileName, Status: q.Status, Signer: t.signer, Fields: []Field{}}
	d.Signer.SignIP = ""
	d.Signer.Link = ""
	for _, f := range q.Fields {
		if f.SignerID == t.signer.ID {
			d.Fields = append(d.Fields, f)
		}
	}
	for _, s := range q.Signers {
		if s.Role == "signer" && s.ID != t.signer.ID {
			d.Others = append(d.Others, fmt.Sprintf("%s (%s)", s.Name, s.Status))
		}
	}
	d.DoneSigned = t.signer.Status == "signed"
	d.Waiting = q.Sequential && !r.myTurn(q, t.signer)
	d.CanSign = q.Status == "sent" && t.signer.Role == "signer" && (t.signer.Status == "pending" || t.signer.Status == "viewed") && !d.Waiting
	if q.Status == "sent" && t.signer.Status == "pending" {
		if _, err := r.pool.Exec(ctx, `UPDATE adrient.sig_signers SET status='viewed', viewed_at=$2 WHERE id=$1 AND status='pending'`, t.signer.ID, r.now()); err == nil {
			_ = r.record(ctx, r.pool, t.org, q.ID, Event{Event: "viewed", SignerID: t.signer.ID, Actor: t.signer.Email, IP: c.IP, UA: trunc(c.UA, 300)})
		}
	}
	return d, nil
}

// PublicPDF returns the PDF a signer sees: the sent original, or the signed copy once the request is complete.
func (r *Repository) PublicPDF(ctx context.Context, tok string) ([]byte, string, error) {
	t, err := r.byToken(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if t.req.Status == "completed" && t.req.SignedFileID != "" && r.Deps.ReadFileAny != nil {
		if b, _, err := r.Deps.ReadFileAny(ctx, t.org, t.req.SignedFileID); err == nil {
			return b, t.req.FileName, nil
		}
	}
	var data []byte
	if err := r.pool.QueryRow(ctx, `SELECT data FROM adrient.sig_documents WHERE request_id=$1`, t.req.ID).Scan(&data); err != nil {
		return nil, "", knowkit.ErrNotFound
	}
	return data, t.req.FileName, nil
}

// SignInput is a signer's answer.
type SignInput struct {
	Consent       bool              `json:"consent"`
	SignatureKind string            `json:"signature_kind"` // typed | drawn
	SignatureData string            `json:"signature_data"` // the typed name, or a PNG data URL
	Values        map[string]string `json:"values"`         // field id to value (text and checkbox fields)
}

// PublicSign records a signer's signature. When the last required person signs, the document is completed.
func (r *Repository) PublicSign(ctx context.Context, tok string, in SignInput, c Client) (*PublicDoc, error) {
	t, err := r.byToken(ctx, tok)
	if err != nil {
		return nil, err
	}
	if t.req.Status != "sent" {
		return nil, knowkit.Invalid("this request is %s and can no longer be signed", t.req.Status)
	}
	if t.signer.Role != "signer" || t.signer.Status == "signed" || t.signer.Status == "declined" {
		return nil, knowkit.Invalid("you have already responded to this request")
	}
	if t.req.Sequential && !r.myTurn(t.req, t.signer) {
		return nil, knowkit.Invalid("it is not your turn yet; you will be asked when the earlier signers are done")
	}
	if !in.Consent {
		return nil, knowkit.Invalid("please agree to sign electronically")
	}
	switch in.SignatureKind {
	case "typed":
		in.SignatureData = strings.TrimSpace(in.SignatureData)
		if in.SignatureData == "" || utf8.RuneCountInString(in.SignatureData) > 80 {
			return nil, knowkit.Invalid("type your name (up to 80 characters)")
		}
	case "drawn":
		if _, err := decodeSignatureImage(in.SignatureData); err != nil {
			return nil, knowkit.Invalid("%v", err)
		}
	default:
		return nil, knowkit.Invalid("choose a typed or drawn signature")
	}
	today := r.now().Format("2006-01-02")
	type upd struct{ id, val string }
	var ups []upd
	for _, f := range t.req.Fields {
		if f.SignerID != t.signer.ID {
			continue
		}
		v := strings.TrimSpace(in.Values[f.ID])
		switch f.Type {
		case "signature", "initials":
			v = "signed"
		case "date":
			v = today
		case "checkbox":
			if v != "true" {
				v = "false"
			}
			if f.Required && v != "true" {
				return nil, knowkit.Invalid("please tick “%s”", firstNonEmpty(f.Label, "the required box"))
			}
		case "text":
			if f.Required && v == "" {
				return nil, knowkit.Invalid("please fill in “%s”", firstNonEmpty(f.Label, "the required field"))
			}
			if utf8.RuneCountInString(v) > 500 {
				return nil, knowkit.Invalid("a text field is too long")
			}
		}
		ups = append(ups, upd{f.ID, v})
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var st string
	if err := tx.QueryRow(ctx, `SELECT status FROM adrient.sig_requests WHERE id=$1 FOR UPDATE`, t.req.ID).Scan(&st); err != nil || st != "sent" {
		return nil, knowkit.Invalid("this request can no longer be signed")
	}
	var sst string
	if err := tx.QueryRow(ctx, `SELECT status FROM adrient.sig_signers WHERE id=$1 FOR UPDATE`, t.signer.ID).Scan(&sst); err != nil || sst == "signed" || sst == "declined" {
		return nil, knowkit.Invalid("you have already responded to this request")
	}
	now := r.now()
	for _, u := range ups {
		if _, err := tx.Exec(ctx, `UPDATE adrient.sig_fields SET value=$2, filled_at=$3 WHERE id=$1`, u.id, u.val, now); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_signers SET status='signed', signed_at=$2, signature_kind=$3, signature_data=$4, sign_ip=$5, sign_ua=$6, consent_at=$2 WHERE id=$1`,
		t.signer.ID, now, in.SignatureKind, in.SignatureData, c.IP, trunc(c.UA, 300)); err != nil {
		return nil, err
	}
	if err := r.record(ctx, tx, t.org, t.req.ID, Event{Event: "signed", SignerID: t.signer.ID, Actor: t.signer.Email, IP: c.IP, UA: trunc(c.UA, 300), Detail: in.SignatureKind + " signature, consent given"}); err != nil {
		return nil, err
	}
	var left int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.sig_signers WHERE request_id=$1 AND role='signer' AND status<>'signed'`, t.req.ID).Scan(&left); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if left == 0 {
		if err := r.complete(ctx, t.org, t.req.ID); err != nil {
			return nil, fmt.Errorf("signed, but the final document could not be built: %w", err)
		}
	} else if t.req.Sequential {
		r.inviteNext(ctx, t.org, t.req.ID, t.signer.Order, c.Base)
	}
	return r.PublicView(ctx, tok, c)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// inviteNext mails the next person of a sequential request.
func (r *Repository) inviteNext(ctx context.Context, org, reqID string, afterOrder int, base string) {
	q, err := scanReq(r.pool.QueryRow(ctx, `SELECT `+reqCols+` FROM adrient.sig_requests r LEFT JOIN adrient.users u ON u.id=r.sender_id WHERE r.org_id=$1 AND r.id=$2`, org, reqID))
	if err != nil || r.people(ctx, q) != nil {
		return
	}
	for _, s := range q.Signers {
		if s.Role != "signer" || s.Status == "signed" || !r.myTurn(q, s) || s.Order <= afterOrder {
			continue
		}
		tok, err := r.newToken(ctx, s.ID)
		if err == nil {
			r.mailInvite(ctx, q, s, strings.TrimRight(base, "/")+"/sign/"+tok)
		}
		return
	}
}

// PublicDecline records that a signer declined; the request stops.
func (r *Repository) PublicDecline(ctx context.Context, tok, reason string, c Client) error {
	t, err := r.byToken(ctx, tok)
	if err != nil {
		return err
	}
	if t.req.Status != "sent" || t.signer.Role != "signer" || t.signer.Status == "signed" || t.signer.Status == "declined" {
		return knowkit.Invalid("this request can no longer be declined")
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 500 {
		return knowkit.Invalid("the reason is too long")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var st string
	if err := tx.QueryRow(ctx, `SELECT status FROM adrient.sig_requests WHERE id=$1 FOR UPDATE`, t.req.ID).Scan(&st); err != nil || st != "sent" {
		return knowkit.Invalid("this request can no longer be declined")
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_signers SET status='declined', declined_at=$2, decline_reason=$3, sign_ip=$4, sign_ua=$5 WHERE id=$1`, t.signer.ID, r.now(), reason, c.IP, trunc(c.UA, 300)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_requests SET status='declined' WHERE id=$1`, t.req.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_signers SET token_hash='' WHERE request_id=$1 AND status<>'signed'`, t.req.ID); err != nil {
		return err
	}
	if err := r.record(ctx, tx, t.org, t.req.ID, Event{Event: "declined", SignerID: t.signer.ID, Actor: t.signer.Email, IP: c.IP, UA: trunc(c.UA, 300), Detail: reason}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if r.Deps.Notify != nil {
		r.Deps.Notify(ctx, t.org, t.req.SenderID, t.signer.Name+" declined to sign "+t.req.Title, reason, "/signatures?request="+t.req.ID)
	}
	return nil
}

// newToken replaces a signer's link token and returns the new one.
func (r *Repository) newToken(ctx context.Context, signerID string) (string, error) {
	tok := knowkit.NewKey() + knowkit.NewKey()
	_, err := r.pool.Exec(ctx, `UPDATE adrient.sig_signers SET token_hash=$2 WHERE id=$1`, signerID, hashToken(tok))
	return tok, err
}

// complete stamps the fields onto the original, appends the certificate page, seals and saves the result in the sender's Drive.
func (r *Repository) complete(ctx context.Context, org, reqID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var st string
	if err := tx.QueryRow(ctx, `SELECT status FROM adrient.sig_requests WHERE id=$1 FOR UPDATE`, reqID).Scan(&st); err != nil {
		return err
	}
	if st != "sent" {
		return nil // someone else completed it
	}
	q, err := scanReq(tx.QueryRow(ctx, `SELECT `+reqCols+` FROM adrient.sig_requests r LEFT JOIN adrient.users u ON u.id=r.sender_id WHERE r.id=$1`, reqID))
	if err != nil {
		return err
	}
	if err := r.people(ctx, q); err != nil {
		return err
	}
	var orig []byte
	if err := tx.QueryRow(ctx, `SELECT data FROM adrient.sig_documents WHERE request_id=$1`, reqID).Scan(&orig); err != nil {
		return err
	}
	if sha(orig) != q.DocHash {
		return errors.New("the stored original does not match its hash")
	}
	sigData := map[string][2]string{}
	rows, err := tx.Query(ctx, `SELECT id::text, signature_kind, signature_data FROM adrient.sig_signers WHERE request_id=$1`, reqID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, k, d string
		_ = rows.Scan(&id, &k, &d)
		sigData[id] = [2]string{k, d}
	}
	rows.Close()
	var items []stampItem
	for _, f := range q.Fields {
		sd := sigData[f.SignerID]
		it := stampItem{page: f.Page, x: f.X, y: f.Y, w: f.W, h: f.H}
		switch f.Type {
		case "signature", "initials":
			if sd[0] == "drawn" {
				img, err := decodeSignatureImage(sd[1])
				if err != nil {
					return err
				}
				it.imagePNG = img
			} else {
				it.text, it.italic = sd[1], true
				if f.Type == "initials" {
					it.text = initials(sd[1])
				}
			}
		case "date", "text":
			it.text = f.Value
		case "checkbox":
			if f.Value == "true" {
				it.text = "X"
			}
		}
		if it.text != "" || it.imagePNG != nil {
			items = append(items, it)
		}
	}
	stamped, err := Stamp(orig, items)
	if err != nil {
		return err
	}
	now := r.now()
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_requests SET completed_at=$2 WHERE id=$1`, reqID, now); err != nil {
		return err
	}
	if err := r.record(ctx, tx, org, reqID, Event{Event: "completed", Actor: "system", Detail: "all signers signed"}); err != nil {
		return err
	}
	trailRows, err := tx.Query(ctx, `SELECT id, COALESCE(signer_id::text,''), event, actor, ip, ua, detail, at, prev_hash, hash FROM adrient.sig_events WHERE request_id=$1 ORDER BY id`, reqID)
	if err != nil {
		return err
	}
	var evs []Event
	for trailRows.Next() {
		var e Event
		_ = trailRows.Scan(&e.ID, &e.SignerID, &e.Event, &e.Actor, &e.IP, &e.UA, &e.Detail, &e.At, &e.PrevHash, &e.Hash)
		evs = append(evs, e)
	}
	trailRows.Close()
	head := ""
	if len(evs) > 0 {
		head = evs[len(evs)-1].Hash
	}
	full, err := appendPage(stamped, certificatePDF(q.Title, q.DocHash, q.Signers, evs, head))
	if err != nil {
		return err
	}
	sealed, err := Seal(full, org, "Completed e-signature request: "+q.Title, now)
	if err != nil {
		sealed = full // the seal is an extra; the certificate page and audit trail stay
		_ = r.record(ctx, tx, org, reqID, Event{Event: "seal_skipped", Actor: "system", Detail: err.Error()})
	}
	name := strings.TrimSuffix(q.FileName, ".pdf") + " (signed).pdf"
	if r.Deps.SaveFile == nil {
		return errors.New("Drive is not connected")
	}
	fileID, err := r.Deps.SaveFile(ctx, org, q.SenderID, name, "application/pdf", sealed)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_requests SET status='completed', signed_file_id=$2, signed_hash=$3 WHERE id=$1`, reqID, fileID, sha(sealed)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sig_signers SET signature_data='' WHERE request_id=$1`, reqID); err != nil { // drawn images are in the PDF now
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if r.Deps.Notify != nil {
		r.Deps.Notify(ctx, org, q.SenderID, q.Title+" is fully signed", "The signed copy is in your Drive.", "/signatures?request="+reqID)
	}
	if r.Deps.Mail != nil {
		for _, s := range q.Signers {
			body := fmt.Sprintf("<p>Hello %s,</p><p><b>%s</b> has been signed by everyone. The sender has the signed copy.</p>", s.Name, q.Title)
			_ = r.Deps.Mail(ctx, []string{s.Email}, "Completed: "+q.Title, body, "Everyone has signed "+q.Title+".")
		}
	}
	r.audit(ctx, "signature.complete", "sig_request", reqID)
	return nil
}

func initials(name string) string {
	var b strings.Builder
	for _, p := range strings.Fields(name) {
		r, _ := utf8.DecodeRuneInString(p)
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}

// Download returns the signed (or original) PDF of a request to its sender or an admin.
func (r *Repository) Download(ctx context.Context, a knowkit.Actor, id string, original bool) ([]byte, string, error) {
	q, err := r.load(ctx, a, id, false)
	if err != nil {
		return nil, "", err
	}
	if !original && q.Status == "completed" && q.SignedFileID != "" && r.Deps.ReadFileAny != nil {
		b, _, err := r.Deps.ReadFileAny(ctx, a.Org, q.SignedFileID)
		return b, strings.TrimSuffix(q.FileName, ".pdf") + " (signed).pdf", err
	}
	var data []byte
	if err := r.pool.QueryRow(ctx, `SELECT data FROM adrient.sig_documents WHERE request_id=$1`, id).Scan(&data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", knowkit.ErrNotFound
		}
		return nil, "", err
	}
	return data, q.FileName, nil
}

var _ = bytes.MinRead
var _ = time.Second
