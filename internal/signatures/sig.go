// Signer roles and order, field types and placement as page fractions, consent, decline, audit trail and
// completion certificate. Signing links, the hash-chained audit trail and PDF assembly run on Adrient's Drive
// and pdfcpu.

// Package signatures sends PDF documents for e-signature: signers get a private link, fill the fields placed on
// the pages, and the finished document is stamped, sealed and saved to Drive with a certificate page.
package signatures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Limits.
const (
	MaxPDFBytes = 25 << 20
	MaxSigners  = 20
	MaxFields   = 200
)

// Deps are the other modules' capabilities; each is optional except file access.
type Deps struct {
	Audit knowkit.Audit
	// Mail sends one email (nil: links are shown to the sender instead).
	Mail func(ctx context.Context, to []string, subject, html, text string) error
	// ReadFile returns a Drive file's bytes; the caller must own it (checked by owner).
	ReadFile func(ctx context.Context, org, owner, fileID string) (data []byte, name string, err error)
	// ReadFileAny reads any Drive file of the org (used for the stored original at completion).
	ReadFileAny func(ctx context.Context, org, fileID string) ([]byte, string, error)
	// SaveFile stores bytes in the owner's Drive root and returns the new file id.
	SaveFile func(ctx context.Context, org, owner, name, mime string, data []byte) (string, error)
	// Notify tells an org user something happened (bell notification); optional.
	Notify func(ctx context.Context, org, userID, title, body, link string)
	Now    func() time.Time
}

// Repository is the Postgres store. Every query is org-scoped.
type Repository struct {
	pool *pgxpool.Pool
	Deps Deps
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) now() time.Time {
	return knowkit.Clock(r.Deps.Now).UTC().Truncate(time.Microsecond)
}

func (r *Repository) audit(ctx context.Context, a, t, id string) {
	if r.Deps.Audit != nil {
		r.Deps.Audit(ctx, a, t, id)
	}
}

// Field is a place on a page a signer fills. x, y, w, h are fractions of the page from its top-left corner.
type Field struct {
	ID       string  `json:"id"`
	SignerID string  `json:"signer_id"`
	Type     string  `json:"type"` // signature | initials | date | text | checkbox
	Label    string  `json:"label,omitempty"`
	Page     int     `json:"page"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
	Required bool    `json:"required"`
	Value    string  `json:"value,omitempty"`
}

// Signer is a person who signs (or receives a copy).
type Signer struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Email         string     `json:"email"`
	UserID        string     `json:"user_id,omitempty"`
	ContactID     string     `json:"contact_id,omitempty"`
	Role          string     `json:"role"`
	Order         int        `json:"order"`
	Status        string     `json:"status"`
	ViewedAt      *time.Time `json:"viewed_at,omitempty"`
	SignedAt      *time.Time `json:"signed_at,omitempty"`
	DeclinedAt    *time.Time `json:"declined_at,omitempty"`
	DeclineReason string     `json:"decline_reason,omitempty"`
	SignatureKind string     `json:"signature_kind,omitempty"`
	SignIP        string     `json:"sign_ip,omitempty"`
	Link          string     `json:"link,omitempty"` // only returned to the sender right after sending
}

// Request is a document sent for signature.
type Request struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Message      string     `json:"message"`
	Status       string     `json:"status"`
	SenderID     string     `json:"sender_id"`
	SenderName   string     `json:"sender_name,omitempty"`
	FileID       string     `json:"file_id"`
	FileName     string     `json:"file_name"`
	DocHash      string     `json:"doc_hash,omitempty"`
	SignedFileID string     `json:"signed_file_id,omitempty"`
	SignedHash   string     `json:"signed_hash,omitempty"`
	Sequential   bool       `json:"sequential"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	ContractID   string     `json:"contract_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Signers      []Signer   `json:"signers,omitempty"`
	Fields       []Field    `json:"fields,omitempty"`
	Progress     string     `json:"progress,omitempty"`
	CanManage    bool       `json:"can_manage"`
}

// SignerInput and FieldInput describe a new request.
type SignerInput struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	UserID    string `json:"user_id"`
	ContactID string `json:"contact_id"`
	Role      string `json:"role"`
}

// FieldInput places a field for the signer at SignerIndex (0-based position in Signers).
type FieldInput struct {
	SignerIndex int     `json:"signer_index"`
	Type        string  `json:"type"`
	Label       string  `json:"label"`
	Page        int     `json:"page"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	W           float64 `json:"w"`
	H           float64 `json:"h"`
	Required    *bool   `json:"required"`
}

// Input creates a request.
type Input struct {
	Title      string        `json:"title"`
	Message    string        `json:"message"`
	FileID     string        `json:"file_id"`
	Signers    []SignerInput `json:"signers"`
	Fields     []FieldInput  `json:"fields"`
	Sequential bool          `json:"sequential"`
	ExpireDays int           `json:"expire_days"`
	ContractID string        `json:"contract_id"`
}

func (in *Input) clean() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 200 {
		return knowkit.Invalid("a request needs a title of up to 200 characters")
	}
	if utf8.RuneCountInString(in.Message) > 5000 {
		return knowkit.Invalid("the message is too long")
	}
	if !knowkit.IsUUID(in.FileID) {
		return knowkit.Invalid("choose a PDF from Drive")
	}
	if in.ContractID != "" && !knowkit.IsUUID(in.ContractID) {
		return knowkit.Invalid("bad contract id")
	}
	if len(in.Signers) == 0 || len(in.Signers) > MaxSigners {
		return knowkit.Invalid("add between 1 and %d people", MaxSigners)
	}
	seen := map[string]bool{}
	nSign := 0
	for i := range in.Signers {
		s := &in.Signers[i]
		s.Name = strings.TrimSpace(s.Name)
		s.Email = strings.ToLower(strings.TrimSpace(s.Email))
		if s.Role == "" {
			s.Role = "signer"
		}
		if !knowkit.OneOf(s.Role, "signer", "cc") {
			return knowkit.Invalid("role must be signer or cc")
		}
		if s.Name == "" || utf8.RuneCountInString(s.Name) > 120 {
			return knowkit.Invalid("each person needs a name")
		}
		if a, err := mail.ParseAddress(s.Email); err != nil || a.Address != s.Email || len(s.Email) > 200 {
			return knowkit.Invalid("%q is not a valid email address", s.Email)
		}
		if seen[s.Email] {
			return knowkit.Invalid("%s appears twice", s.Email)
		}
		seen[s.Email] = true
		if s.Role == "signer" {
			nSign++
		}
		if s.UserID != "" && !knowkit.IsUUID(s.UserID) || s.ContactID != "" && !knowkit.IsUUID(s.ContactID) {
			return knowkit.Invalid("bad person reference")
		}
	}
	if nSign == 0 {
		return knowkit.Invalid("at least one person must sign")
	}
	if len(in.Fields) > MaxFields {
		return knowkit.Invalid("at most %d fields", MaxFields)
	}
	hasSig := map[int]bool{}
	for i := range in.Fields {
		f := &in.Fields[i]
		if f.SignerIndex < 0 || f.SignerIndex >= len(in.Signers) || in.Signers[f.SignerIndex].Role != "signer" {
			return knowkit.Invalid("field %d belongs to someone who does not sign", i+1)
		}
		if !knowkit.OneOf(f.Type, "signature", "initials", "date", "text", "checkbox") {
			return knowkit.Invalid("field type must be signature, initials, date, text or checkbox")
		}
		if f.Page < 1 || f.Page > 2000 {
			return knowkit.Invalid("field page is out of range")
		}
		if !(f.X >= 0 && f.Y >= 0 && f.W > 0.005 && f.H > 0.005 && f.X+f.W <= 1.0001 && f.Y+f.H <= 1.0001) {
			return knowkit.Invalid("field %d is not inside the page", i+1)
		}
		if f.Type == "signature" {
			hasSig[f.SignerIndex] = true
		}
		if len(f.Label) > 80 {
			return knowkit.Invalid("field label is too long")
		}
	}
	for i, s := range in.Signers {
		if s.Role == "signer" && !hasSig[i] {
			return knowkit.Invalid("%s needs at least one signature field", s.Name)
		}
	}
	if in.ExpireDays < 0 || in.ExpireDays > 365 {
		return knowkit.Invalid("expiry must be 0 to 365 days")
	}
	return nil
}

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

// ---- audit trail -----------------------------------------------------------

// Event is one audit entry.
type Event struct {
	ID       int64     `json:"id"`
	SignerID string    `json:"signer_id,omitempty"`
	Event    string    `json:"event"`
	Actor    string    `json:"actor"`
	IP       string    `json:"ip,omitempty"`
	UA       string    `json:"ua,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	At       time.Time `json:"at"`
	PrevHash string    `json:"prev_hash"`
	Hash     string    `json:"hash"`
}

func eventHash(prev string, e Event) string {
	h := sha256.Sum256([]byte(strings.Join([]string{prev, e.SignerID, e.Event, e.Actor, e.IP, e.UA, e.Detail, e.At.UTC().Format(time.RFC3339Nano)}, "\x1f")))
	return hex.EncodeToString(h[:])
}

type qrow interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// record appends an audit entry chained to the previous one. It runs inside the caller's transaction when given
// one; the request row lock taken by the caller keeps entries of one request in order.
func (r *Repository) record(ctx context.Context, q qrow, org, reqID string, e Event) error {
	var prev string
	if err := q.QueryRow(ctx, `SELECT COALESCE((SELECT hash FROM adrient.sig_events WHERE request_id=$1 ORDER BY id DESC LIMIT 1),'')`, reqID).Scan(&prev); err != nil {
		return err
	}
	e.At = r.now()
	e.PrevHash = prev
	e.Hash = eventHash(prev, e)
	var sid any
	if e.SignerID != "" {
		sid = e.SignerID
	}
	var id int64
	return q.QueryRow(ctx, `INSERT INTO adrient.sig_events (org_id, request_id, signer_id, event, actor, ip, ua, detail, at, prev_hash, hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		org, reqID, sid, e.Event, e.Actor, e.IP, e.UA, e.Detail, e.At, e.PrevHash, e.Hash).Scan(&id)
}

// Trail returns the audit entries of a request and whether the hash chain is intact.
func (r *Repository) Trail(ctx context.Context, a knowkit.Actor, reqID string) ([]Event, bool, error) {
	if _, err := r.load(ctx, a, reqID, false); err != nil {
		return nil, false, err
	}
	return r.trail(ctx, reqID)
}

func (r *Repository) trail(ctx context.Context, reqID string) ([]Event, bool, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, COALESCE(signer_id::text,''), event, actor, ip, ua, detail, at, prev_hash, hash FROM adrient.sig_events WHERE request_id=$1 ORDER BY id`, reqID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Event{}
	ok := true
	prev := ""
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.SignerID, &e.Event, &e.Actor, &e.IP, &e.UA, &e.Detail, &e.At, &e.PrevHash, &e.Hash); err != nil {
			return nil, false, err
		}
		if e.PrevHash != prev || e.Hash != eventHash(prev, e) {
			ok = false
		}
		prev = e.Hash
		out = append(out, e)
	}
	return out, ok, rows.Err()
}

var errGone = errors.New("gone")

func fmtErr(format string, a ...any) error { return fmt.Errorf(format, a...) }
