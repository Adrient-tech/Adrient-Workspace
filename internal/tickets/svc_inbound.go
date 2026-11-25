package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InboundMail is one received message, independent of where it came from.
type InboundMail struct {
	Key       string    `json:"key"`         // unique id of the message (Message-ID or a source id)
	InReplyTo string    `json:"in_reply_to"` // key of the message this answers, if known
	From      string    `json:"from"`        // "Name <a@b>" or "a@b"
	To        []string  `json:"to"`
	Cc        []string  `json:"cc"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Date      time.Time `json:"date"`
	AutoReply bool      `json:"auto_reply"` // Auto-Submitted / bulk / out-of-office
	OrgID     string    `json:"-"`
}

// MailSource hands over new inbound mail for the given addresses. Implementations: the local
// inbox table (LocalInboxSource) and, in tests, FakeMailSource. Messages are marked as seen by the
// ledger in ticket_inbound, so a source may return the same message again without harm.
type MailSource interface {
	Poll(ctx context.Context, aliases []string) ([]InboundMail, error)
}

// FakeMailSource is an in-memory source for tests and local trials.
type FakeMailSource struct{ Queue []InboundMail }

// Poll returns and clears the queue.
func (f *FakeMailSource) Poll(_ context.Context, _ []string) ([]InboundMail, error) {
	q := f.Queue
	f.Queue = nil
	return q, nil
}

// LocalInboxSource reads mail delivered to workspace mailboxes: any inbox message of an organisation
// whose To or Cc contains a team address. It needs a mailbox (user) that owns that address, or
// delivery of the address into one; nothing is fetched from outside the workspace.
type LocalInboxSource struct{ Pool *pgxpool.Pool }

// Poll returns recent, not yet processed inbox messages addressed to one of the aliases.
func (s LocalInboxSource) Poll(ctx context.Context, aliases []string) ([]InboundMail, error) {
	if s.Pool == nil || len(aliases) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `SELECT m.org_id::text, m.id::text, m.from_addr, m.from_name, m.to_addrs, m.cc_addrs, m.subject, m.body, m.sent_at
		FROM adrient.mail_messages m
		WHERE m.folder='inbox' AND m.sent_at > now() - interval '7 days'
		  AND NOT EXISTS (SELECT 1 FROM adrient.ticket_inbound i WHERE i.org_id=m.org_id AND i.message_key='local:'||m.id::text)
		  AND (EXISTS (SELECT 1 FROM jsonb_array_elements_text(m.to_addrs) a WHERE lower(a) = ANY($1))
		    OR EXISTS (SELECT 1 FROM jsonb_array_elements_text(m.cc_addrs) a WHERE lower(a) = ANY($1)))
		ORDER BY m.sent_at LIMIT 200`, lowerAll(aliases))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InboundMail
	for rows.Next() {
		var m InboundMail
		var fromAddr, fromName string
		var to, cc []byte
		if err := rows.Scan(&m.OrgID, &m.Key, &fromAddr, &fromName, &to, &cc, &m.Subject, &m.Body, &m.Date); err != nil {
			return nil, err
		}
		m.Key = "local:" + m.Key
		m.From = fromAddr
		if fromName != "" {
			m.From = fromName + " <" + fromAddr + ">"
		}
		_ = json.Unmarshal(to, &m.To)
		_ = json.Unmarshal(cc, &m.Cc)
		out = append(out, m)
	}
	return out, rows.Err()
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(strings.TrimSpace(s))
	}
	return out
}

// ---- parsing helpers ---------------------------------------------------------

// ParseAddress splits "Name <a@b>" into name and lower-case address. ok is false for invalid input.
func ParseAddress(s string) (name, addr string, ok bool) {
	a, err := mail.ParseAddress(strings.TrimSpace(s))
	if err != nil || !strings.Contains(a.Address, "@") {
		return "", "", false
	}
	return strings.TrimSpace(a.Name), strings.ToLower(a.Address), true
}

var refRe = regexp.MustCompile(`\[([A-Za-z][A-Za-z0-9]{0,9})-(\d{1,9})\]`)

// TicketRefIn finds a "[KEY-123]" reference in a subject.
func TicketRefIn(subject string) (key string, number int64, ok bool) {
	m := refRe.FindStringSubmatch(subject)
	if m == nil {
		return "", 0, false
	}
	var n int64
	_, _ = fmt.Sscanf(m[2], "%d", &n)
	return strings.ToUpper(m[1]), n, n > 0
}

var quoteHeadRe = regexp.MustCompile(`(?i)^(on .{5,200} wrote:|-{2,}\s*original message\s*-{2,}|from:\s.+|_{5,})$`)

// StripQuoted removes the quoted earlier conversation and a "-- " signature from a reply.
func StripQuoted(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var out []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "--" || t == "-- " || quoteHeadRe.MatchString(t) {
			break
		}
		if strings.HasPrefix(t, ">") {
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

var noReplyRe = regexp.MustCompile(`(?i)^(no-?reply|do-?not-?reply|mailer-daemon|postmaster|bounce)`)

// IsAutomated says whether a message should never create a ticket: auto replies, bounces, bulk senders.
func IsAutomated(m InboundMail, fromAddr string) bool {
	if m.AutoReply {
		return true
	}
	local := fromAddr
	if i := strings.IndexByte(fromAddr, '@'); i > 0 {
		local = fromAddr[:i]
	}
	if noReplyRe.MatchString(local) {
		return true
	}
	s := strings.ToLower(m.Subject)
	return strings.HasPrefix(s, "automatic reply") || strings.HasPrefix(s, "out of office") || strings.HasPrefix(s, "undeliverable")
}

// ---- ingestion ----------------------------------------------------------------

// InboundResult says what a message turned into.
type InboundResult struct {
	Outcome  string `json:"outcome"` // ticket | comment | ignored | duplicate
	TicketID string `json:"ticket_id,omitempty"`
	Ref      string `json:"ref,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

const maxInboundPerSenderPerHour = 20

type aliasTarget struct{ groupID, projectID, orgID, alias string }

// aliasTargets lists the teams that have an email address, for an organisation ("" = all).
func (r *Repository) aliasTargets(ctx context.Context, org string) (map[string]aliasTarget, error) {
	rows, err := r.pool.Query(ctx, `SELECT g.id::text, g.project_id::text, g.org_id::text, lower(g.email_alias) FROM adrient.ticket_groups g
		WHERE g.email_alias <> '' AND ($1 = '' OR g.org_id::text = $1)`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]aliasTarget{}
	for rows.Next() {
		var t aliasTarget
		if err := rows.Scan(&t.groupID, &t.projectID, &t.orgID, &t.alias); err != nil {
			return nil, err
		}
		if _, addr, ok := ParseAddress(t.alias); ok {
			out[addr] = t
		}
	}
	return out, rows.Err()
}

// IngestMail turns one received message into a new ticket or a comment on an existing one. m.OrgID
// scopes it; when empty the organisation of the matching team address is used.
func (r *Repository) IngestMail(ctx context.Context, m InboundMail) (InboundResult, error) {
	m.Key = strings.TrimSpace(m.Key)
	if m.Key == "" || len(m.Key) > 300 {
		return InboundResult{}, fmt.Errorf("%w: message key is required", ErrInvalid)
	}
	targets, err := r.aliasTargets(ctx, m.OrgID)
	if err != nil {
		return InboundResult{}, err
	}
	var target *aliasTarget
	for _, list := range [][]string{m.To, m.Cc} {
		for _, a := range list {
			if _, addr, ok := ParseAddress(a); ok {
				if t, found := targets[addr]; found {
					tt := t
					target = &tt
					break
				}
			}
		}
		if target != nil {
			break
		}
	}
	if target == nil {
		return InboundResult{Outcome: "ignored", Reason: "no team address matches"}, nil
	}
	org := target.orgID
	ledger := func(outcome, ticketID, sender string) {
		_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.ticket_inbound (org_id, message_key, ticket_id, outcome, sender) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
			org, m.Key, nullUUID(ticketID), outcome, sender)
	}
	var seen int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_inbound WHERE org_id=$1 AND message_key=$2`, org, m.Key).Scan(&seen)
	if seen > 0 {
		return InboundResult{Outcome: "duplicate"}, nil
	}
	name, from, ok := ParseAddress(m.From)
	if !ok {
		ledger("ignored", "", "")
		return InboundResult{Outcome: "ignored", Reason: "sender address is not valid"}, nil
	}
	if from == target.alias || IsAutomated(m, from) {
		ledger("ignored", "", from)
		return InboundResult{Outcome: "ignored", Reason: "automatic message"}, nil
	}
	var recent int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_inbound WHERE org_id=$1 AND sender=$2 AND created_at > now() - interval '1 hour'`, org, from).Scan(&recent)
	if recent >= maxInboundPerSenderPerHour {
		return InboundResult{Outcome: "ignored", Reason: "too many messages from this sender"}, nil
	}
	body := strings.TrimSpace(m.Body)
	if len(body) > 50000 {
		body = body[:50000]
	}
	subject := strings.TrimSpace(m.Subject)
	if len(subject) > 200 {
		subject = subject[:200]
	}

	// replies: by reference in the subject, or by the message this one answers
	ticketID := ""
	if key, n, ok := TicketRefIn(subject); ok {
		_ = r.pool.QueryRow(ctx, `SELECT t.id::text FROM adrient.tickets t JOIN adrient.ticket_projects p ON p.id=t.project_id
			WHERE t.org_id=$1 AND upper(p.key)=$2 AND t.number=$3`, org, key, n).Scan(&ticketID)
	}
	if ticketID == "" && m.InReplyTo != "" {
		_ = r.pool.QueryRow(ctx, `SELECT COALESCE(ticket_id::text,'') FROM adrient.ticket_inbound WHERE org_id=$1 AND message_key=$2`, org, m.InReplyTo).Scan(&ticketID)
	}
	ctx = WithActor(ctx, Actor{Name: firstNonEmpty(name, from)})
	if ticketID != "" {
		var requester, merged string
		var requesterUser *string
		err := r.pool.QueryRow(ctx, `SELECT lower(requester_email), COALESCE(merged_into::text,''), requester_user_id::text FROM adrient.tickets WHERE id=$1 AND org_id=$2`, ticketID, org).Scan(&requester, &merged, &requesterUser)
		if err == nil && requester == from {
			reply := StripQuoted(body)
			if reply == "" {
				reply = body
			}
			target := ticketID
			if merged != "" {
				target = merged
			}
			if _, err := r.AddComment(ctx, org, target, nil, firstNonEmpty(name, from), reply, false); err != nil {
				return InboundResult{}, err
			}
			ledger("comment", target, from)
			return InboundResult{Outcome: "comment", TicketID: target}, nil
		}
		// A reference from someone who is not the requester never writes into that ticket.
	}
	title := cleanSubject(subject)
	if title == "" {
		title = "(no subject)"
	}
	t, err := r.createFromMail(ctx, *target, title, body, name, from)
	if err != nil {
		return InboundResult{}, err
	}
	ledger("ticket", t.ID, from)
	ref := fmt.Sprintf("%s-%d", t.ProjectKey, t.Number)
	r.acknowledge(ctx, from, ref, title)
	return InboundResult{Outcome: "ticket", TicketID: t.ID, Ref: ref}, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

var rePrefix = regexp.MustCompile(`(?i)^\s*((re|fw|fwd|aw)\s*:\s*)+`)

func cleanSubject(s string) string {
	s = refRe.ReplaceAllString(s, "")
	return strings.TrimSpace(rePrefix.ReplaceAllString(s, ""))
}

func (r *Repository) createFromMail(ctx context.Context, t aliasTarget, title, body, name, from string) (Ticket, error) {
	ctx = WithCreateOpts(ctx, CreateOpts{GroupID: t.groupID})
	tk, err := r.CreateTicket(ctx, t.orgID, t.projectID, title, body, "normal", nil, firstNonEmpty(name, from), from, "email")
	if err != nil {
		return tk, err
	}
	return tk, nil
}

// acknowledge sends the receipt mail with the reference in the subject so replies thread. It is
// skipped when mail is not configured.
func (r *Repository) acknowledge(ctx context.Context, to, ref, title string) {
	if r.Deps.Mail == nil {
		return
	}
	subj := fmt.Sprintf("[%s] We received your request: %s", ref, title)
	text := fmt.Sprintf("Thank you. Your request was received and is tracked as %s.\nReply to this message to add information.\n", ref)
	htm := fmt.Sprintf("<p>Thank you. Your request was received and is tracked as <b>%s</b>.</p><p>Reply to this message to add information.</p>", html.EscapeString(ref))
	if err := r.Deps.Mail(ctx, []string{to}, subj, htm, text); err != nil {
		slog.Warn("tickets: acknowledgement mail failed", "err", err)
	}
}

// PollInbound fetches new mail from the configured source and ingests it. It returns how many
// tickets and comments were created.
func (r *Repository) PollInbound(ctx context.Context) (tickets, comments int, err error) {
	if r.Inbound == nil {
		return 0, 0, nil
	}
	targets, err := r.aliasTargets(ctx, "")
	if err != nil || len(targets) == 0 {
		return 0, 0, err
	}
	var aliases []string
	for a := range targets {
		aliases = append(aliases, a)
	}
	list, err := r.Inbound.Poll(ctx, aliases)
	if err != nil {
		return 0, 0, err
	}
	for _, m := range list {
		res, err := r.IngestMail(ctx, m)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			slog.Warn("tickets: inbound message failed", "key", m.Key, "err", err)
			continue
		}
		switch res.Outcome {
		case "ticket":
			tickets++
		case "comment":
			comments++
		}
	}
	return tickets, comments, nil
}

// mailRequesterReply sends an agent's public reply to the requester of an e-mailed ticket.
func (r *Repository) mailRequesterReply(ctx context.Context, t DeskTicket, author, body string) {
	if r.Deps.Mail == nil || t.Source != "email" || t.RequesterEmail == "" {
		return
	}
	subj := fmt.Sprintf("Re: %s [%s]", t.Title, t.Ref)
	text := body + "\n\n--\n" + author + " (" + t.Ref + ")\nReply to this message to answer.\n"
	htm := "<p>" + strings.ReplaceAll(html.EscapeString(body), "\n", "<br>") + "</p><p style=\"color:#666\">" + html.EscapeString(author) + " (" + html.EscapeString(t.Ref) + "). Reply to this message to answer.</p>"
	if err := r.Deps.Mail(ctx, []string{t.RequesterEmail}, subj, htm, text); err != nil {
		slog.Warn("tickets: reply mail failed", "err", err)
	}
}
