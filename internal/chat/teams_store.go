package chat

// Teams-class chat store: channels (public/private/DM/group DM), sidebar
// sections, threads, mentions, pins, bookmarks, saved messages, reminders,
// drafts, scheduled sends, profiles and search. One Adrient org == one
// workspace. Stored in Postgres.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrConflict is returned for duplicate names.
var ErrConflict = errors.New("conflict")

// ErrInvalid is returned for bad input.
var ErrInvalid = errors.New("invalid")

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID reports whether s is a canonical UUID string.
func IsUUID(s string) bool { return uuidRe.MatchString(s) }

// TChannel is a channel as seen by one user.
type TChannel struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Name          string   `json:"name"`
	Topic         string   `json:"topic"`
	Description   string   `json:"description"`
	Archived      bool     `json:"archived"`
	Announce      bool     `json:"announce"`
	LastReadAt    string   `json:"last_read_at"`
	CreatedBy     string   `json:"created_by"`
	HuddleCode    string   `json:"huddle_code"`
	MemberIDs     []string `json:"member_ids"`
	LastMessageAt string   `json:"last_message_at"`
	CreatedAt     string   `json:"created_at"`
	UnreadCount   int      `json:"unread_count"`
	MentionCount  int      `json:"mention_count"`
	Starred       bool     `json:"starred"`
	Muted         bool     `json:"muted"`
	SectionID     string   `json:"section_id"`
	IsMember      bool     `json:"is_member"`
}

// TReaction is a reaction aggregate for one user.
type TReaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Me    bool   `json:"me"`
}

// TAttachment mirrors the attachment metadata the UI needs.
type TAttachment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
}

// TMessage is a message as seen by one user.
type TMessage struct {
	ID          string        `json:"id"`
	ChannelID   string        `json:"channel_id"`
	SenderID    string        `json:"sender_id"`
	SenderName  string        `json:"sender_name"`
	Body        string        `json:"body"`
	SentAt      string        `json:"sent_at"`
	EditedAt    string        `json:"edited_at"`
	ParentID    string        `json:"parent_id"`
	ReplyCount  int           `json:"reply_count"`
	LastReplyAt string        `json:"last_reply_at"`
	MentionIDs  []string      `json:"mention_ids"`
	Pinned      bool          `json:"pinned"`
	Saved       bool          `json:"saved"`
	Attachments []TAttachment `json:"attachments"`
	Reactions   []TReaction   `json:"reaction_details"`
	ClientNonce string        `json:"client_nonce,omitempty"`
}

// Section is a custom sidebar section.
type Section struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	Collapsed bool   `json:"collapsed"`
}

// Person is an org member with chat profile data.
type Person struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Title       string `json:"title"`
	StatusText  string `json:"status_text"`
	StatusEmoji string `json:"status_emoji"`
}

// Bookmark is a link pinned to a channel header.
type Bookmark struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Draft is an unsent message.
type Draft struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	ParentID  string `json:"parent_id"`
	Body      string `json:"body"`
	UpdatedAt string `json:"updated_at"`
}

// Scheduled is a send-later message.
type Scheduled struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	ParentID  string `json:"parent_id"`
	Body      string `json:"body"`
	SendAt    string `json:"send_at"`
	Status    string `json:"status"`
}

// Reminder is a "remind me about this" entry.
type Reminder struct {
	ID        string    `json:"id"`
	MessageID string    `json:"message_id"`
	ChannelID string    `json:"channel_id"`
	Note      string    `json:"note"`
	RemindAt  string    `json:"remind_at"`
	Fired     bool      `json:"fired"`
	Message   *TMessage `json:"message,omitempty"`
}

// ActivityItem is one entry in the Activity feed.
type ActivityItem struct {
	Kind    string   `json:"kind"` // mention | thread_reply | reminder
	At      string   `json:"at"`
	Message TMessage `json:"message"`
	Note    string   `json:"note,omitempty"`
	Unread  bool     `json:"unread"`
}

// Store is the teams data layer. It wraps the base Repository pool.
type Store struct {
	pool *pgxpool.Pool
	repo *Repository
}

// NewStore builds a Store.
func NewStore(repo *Repository) *Store { return &Store{pool: repo.pool, repo: repo} }

func ts(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func isMember(ids []string, uid string) bool {
	for _, id := range ids {
		if id == uid {
			return true
		}
	}
	return false
}

// ---- channels ---------------------------------------------------------------

const tchanSelect = `SELECT c.id::text, c.kind, c.name, c.topic, c.description, c.archived, c.announce, COALESCE(rc.last_read_at::text,''),
	COALESCE(c.created_by::text,''), c.huddle_code, c.member_ids, c.last_message_at, c.created_at,
	COALESCE((SELECT COUNT(*) FROM adrient.chat_messages m WHERE m.channel_id=c.id AND m.parent_id IS NULL
	   AND m.sender_id <> $2::uuid AND m.sent_at > COALESCE(rc.last_read_at,'-infinity'::timestamptz)),0)::int,
	COALESCE((SELECT COUNT(*) FROM adrient.chat_messages m WHERE m.channel_id=c.id
	   AND m.sender_id <> $2::uuid AND m.mention_ids ? $3::text AND m.sent_at > COALESCE(rc.last_read_at,'-infinity'::timestamptz)),0)::int,
	COALESCE(p.starred,false), COALESCE(p.muted,false), COALESCE(p.section_id::text,'')
	FROM adrient.chat_channels c
	LEFT JOIN adrient.chat_read_cursors rc ON rc.channel_id=c.id AND rc.user_id=$2::uuid
	LEFT JOIN adrient.chat_channel_prefs p ON p.channel_id=c.id AND p.user_id=$2::uuid`

func scanTChannel(row pgx.Row, uid string) (TChannel, error) {
	var c TChannel
	var mj []byte
	var last *time.Time
	var created time.Time
	if err := row.Scan(&c.ID, &c.Kind, &c.Name, &c.Topic, &c.Description, &c.Archived, &c.Announce, &c.LastReadAt, &c.CreatedBy, &c.HuddleCode,
		&mj, &last, &created, &c.UnreadCount, &c.MentionCount, &c.Starred, &c.Muted, &c.SectionID); err != nil {
		return TChannel{}, err
	}
	_ = json.Unmarshal(mj, &c.MemberIDs)
	if c.MemberIDs == nil {
		c.MemberIDs = []string{}
	}
	c.LastMessageAt = ts(last)
	c.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	c.IsMember = isMember(c.MemberIDs, uid)
	return c, nil
}

// GetChannel returns a channel for a user (member or not).
func (s *Store) GetChannel(ctx context.Context, orgID, uid, id string) (TChannel, error) {
	if !IsUUID(id) {
		return TChannel{}, ErrNotFound
	}
	c, err := scanTChannel(s.pool.QueryRow(ctx, tchanSelect+` WHERE c.id=$1::uuid AND c.org_id=$4::uuid`, id, uid, uid, orgID), uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return TChannel{}, ErrNotFound
	}
	return c, err
}

// ListChannels returns channels the user belongs to.
func (s *Store) ListChannels(ctx context.Context, orgID, uid string) ([]TChannel, error) {
	rows, err := s.pool.Query(ctx, tchanSelect+` WHERE c.org_id=$1::uuid AND c.member_ids ? $3::text
	  ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC`, orgID, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TChannel{}
	for rows.Next() {
		c, err := scanTChannel(rows, uid)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// BrowseChannels lists public channels (joined or not).
func (s *Store) BrowseChannels(ctx context.Context, orgID, uid string) ([]TChannel, error) {
	rows, err := s.pool.Query(ctx, tchanSelect+` WHERE c.org_id=$1::uuid AND c.kind='public' AND NOT c.archived
	  ORDER BY lower(c.name)`, orgID, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TChannel{}
	for rows.Next() {
		c, err := scanTChannel(rows, uid)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

var nameRe = regexp.MustCompile(`[^a-z0-9_\-]+`)

// NormalizeChannelName lower-cases and slugifies a public/private channel name.
func NormalizeChannelName(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	n = nameRe.ReplaceAllString(n, "-")
	n = strings.Trim(n, "-")
	if len(n) > 60 {
		n = n[:60]
	}
	return n
}

// OrgMembersAmong returns the subset of ids that are users of orgID.
func (s *Store) OrgMembersAmong(ctx context.Context, orgID string, ids []string) ([]string, error) {
	clean := []string{}
	for _, id := range ids {
		if IsUUID(id) {
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		return []string{}, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1::uuid AND id = ANY($2::uuid[])`, orgID, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CreateTChannel creates (or returns the existing) channel.
func (s *Store) CreateTChannel(ctx context.Context, orgID, uid, kind, name, topic, desc string, memberIDs []string) (TChannel, error) {
	members, err := s.OrgMembersAmong(ctx, orgID, append([]string{uid}, memberIDs...))
	if err != nil {
		return TChannel{}, err
	}
	set := map[string]bool{}
	uniq := []string{}
	for _, m := range members {
		if !set[m] {
			set[m] = true
			uniq = append(uniq, m)
		}
	}
	if !set[uid] {
		uniq = append(uniq, uid)
	}
	switch kind {
	case "dm":
		if len(uniq) < 1 || len(uniq) > 2 {
			return TChannel{}, fmt.Errorf("%w: a DM is with yourself or one other member", ErrInvalid)
		}
		a, b := uniq[0], uniq[0]
		if len(uniq) == 2 {
			b = uniq[1]
		}
		if ex, err := s.repo.FindDMChannel(ctx, orgID, a, b); err == nil {
			return s.GetChannel(ctx, orgID, uid, ex.ID)
		}
		name = ""
	case "group":
		if len(uniq) < 3 || len(uniq) > 9 {
			return TChannel{}, fmt.Errorf("%w: a group DM needs 3 to 9 people", ErrInvalid)
		}
		if strings.TrimSpace(name) == "" {
			var id string
			err := s.pool.QueryRow(ctx, `SELECT id::text FROM adrient.chat_channels WHERE org_id=$1::uuid AND kind='group' AND name=''
			  AND member_ids @> $2::jsonb AND jsonb_array_length(member_ids)=$3 LIMIT 1`, orgID, jsonArr(uniq), len(uniq)).Scan(&id)
			if err == nil {
				return s.GetChannel(ctx, orgID, uid, id)
			}
		}
	case "public", "private":
		name = NormalizeChannelName(name)
		if name == "" {
			return TChannel{}, fmt.Errorf("%w: channel name is required", ErrInvalid)
		}
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.chat_channels WHERE org_id=$1::uuid AND kind IN ('public','private') AND lower(name)=$2 AND NOT archived)`,
			orgID, name).Scan(&exists); err != nil {
			return TChannel{}, err
		}
		if exists {
			return TChannel{}, ErrConflict
		}
	default:
		return TChannel{}, fmt.Errorf("%w: unknown kind", ErrInvalid)
	}
	var id string
	err = s.pool.QueryRow(ctx, `INSERT INTO adrient.chat_channels (org_id, kind, name, topic, description, member_ids, created_by)
	  VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::uuid) RETURNING id::text`,
		orgID, kind, name, topic, desc, jsonArr(uniq), uid).Scan(&id)
	if err != nil {
		return TChannel{}, err
	}
	return s.GetChannel(ctx, orgID, uid, id)
}

// UpdateChannel edits name/topic/description/archived.
func (s *Store) UpdateChannel(ctx context.Context, orgID, id string, name, topic, desc *string, archived *bool, announce ...*bool) error {
	var ann *bool
	if len(announce) > 0 {
		ann = announce[0]
	}
	cur, err := s.GetChannel(ctx, orgID, "00000000-0000-0000-0000-000000000000", id)
	if err != nil {
		return err
	}
	if name != nil && (cur.Kind == "public" || cur.Kind == "private") {
		n := NormalizeChannelName(*name)
		if n == "" {
			return ErrInvalid
		}
		var exists bool
		_ = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.chat_channels WHERE org_id=$1::uuid AND kind IN ('public','private') AND lower(name)=$2 AND id<>$3::uuid AND NOT archived)`,
			orgID, n, id).Scan(&exists)
		if exists {
			return ErrConflict
		}
		name = &n
	} else if name != nil && cur.Kind == "group" {
		n := strings.TrimSpace(*name)
		name = &n
	} else {
		name = nil
	}
	_, err = s.pool.Exec(ctx, `UPDATE adrient.chat_channels SET
	   name = COALESCE($3, name), topic = COALESCE($4, topic), description = COALESCE($5, description),
	   archived = COALESCE($6, archived), announce = COALESCE($7, announce), updated_at = now() WHERE id=$1::uuid AND org_id=$2::uuid`,
		id, orgID, name, topic, desc, archived, ann)
	return err
}

// AddMembers adds users to a channel.
func (s *Store) AddMembers(ctx context.Context, orgID, id string, userIDs []string) error {
	valid, err := s.OrgMembersAmong(ctx, orgID, userIDs)
	if err != nil {
		return err
	}
	for _, u := range valid {
		if _, err := s.pool.Exec(ctx, `UPDATE adrient.chat_channels SET member_ids = member_ids || to_jsonb($3::text), updated_at=now()
		  WHERE id=$1::uuid AND org_id=$2::uuid AND NOT (member_ids ? $3::text)`, id, orgID, u); err != nil {
			return err
		}
	}
	return nil
}

// RemoveMember removes a user from a channel.
func (s *Store) RemoveMember(ctx context.Context, orgID, id, uid string) error {
	_, err := s.pool.Exec(ctx, `UPDATE adrient.chat_channels SET member_ids = member_ids - $3::text, updated_at=now()
	  WHERE id=$1::uuid AND org_id=$2::uuid`, id, orgID, uid)
	return err
}

// SetHuddle records the channel's Meet code if none is set; returns the effective code.
func (s *Store) SetHuddle(ctx context.Context, orgID, id, code string) (string, error) {
	var out string
	err := s.pool.QueryRow(ctx, `UPDATE adrient.chat_channels SET huddle_code = CASE WHEN huddle_code='' THEN $3 ELSE huddle_code END
	  WHERE id=$1::uuid AND org_id=$2::uuid RETURNING huddle_code`, id, orgID, code).Scan(&out)
	return out, err
}

// SetPrefs updates star/mute/section for a user.
func (s *Store) SetPrefs(ctx context.Context, uid, chID string, starred, muted *bool, sectionID *string) error {
	var sec *string
	setSec := false
	if sectionID != nil {
		setSec = true
		if *sectionID != "" {
			if !IsUUID(*sectionID) {
				return ErrInvalid
			}
			sec = sectionID
		}
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO adrient.chat_channel_prefs (channel_id, user_id, starred, muted, section_id)
	  VALUES ($1::uuid,$2::uuid, COALESCE($3,false), COALESCE($4,false), CASE WHEN $6 THEN $5::uuid ELSE NULL END)
	  ON CONFLICT (channel_id,user_id) DO UPDATE SET
	    starred = COALESCE($3, adrient.chat_channel_prefs.starred),
	    muted = COALESCE($4, adrient.chat_channel_prefs.muted),
	    section_id = CASE WHEN $6 THEN $5::uuid ELSE adrient.chat_channel_prefs.section_id END`,
		chID, uid, starred, muted, sec, setSec)
	return err
}

// ---- sections ---------------------------------------------------------------

// ListSections returns a user's custom sections.
func (s *Store) ListSections(ctx context.Context, uid string) ([]Section, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, name, position, collapsed FROM adrient.chat_sections WHERE user_id=$1::uuid ORDER BY position, name`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Section{}
	for rows.Next() {
		var x Section
		if err := rows.Scan(&x.ID, &x.Name, &x.Position, &x.Collapsed); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// CreateSection adds a custom section.
func (s *Store) CreateSection(ctx context.Context, orgID, uid, name string) (Section, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 60 {
		return Section{}, ErrInvalid
	}
	var x Section
	err := s.pool.QueryRow(ctx, `INSERT INTO adrient.chat_sections (org_id,user_id,name,position)
	  VALUES ($1::uuid,$2::uuid,$3,(SELECT COALESCE(MAX(position),0)+1 FROM adrient.chat_sections WHERE user_id=$2::uuid))
	  RETURNING id::text, name, position, collapsed`, orgID, uid, name).Scan(&x.ID, &x.Name, &x.Position, &x.Collapsed)
	return x, err
}

// UpdateSection renames or collapses a section.
func (s *Store) UpdateSection(ctx context.Context, uid, id string, name *string, collapsed *bool) error {
	if !IsUUID(id) {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `UPDATE adrient.chat_sections SET name=COALESCE($3,name), collapsed=COALESCE($4,collapsed)
	  WHERE id=$1::uuid AND user_id=$2::uuid`, id, uid, name, collapsed)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteSection removes a section (channels fall back to the default sections).
func (s *Store) DeleteSection(ctx context.Context, uid, id string) error {
	if !IsUUID(id) {
		return ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM adrient.chat_sections WHERE id=$1::uuid AND user_id=$2::uuid`, id, uid)
	return err
}

// ---- people -----------------------------------------------------------------

// People lists all org users with chat profiles.
func (s *Store) People(ctx context.Context, orgID string) ([]Person, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id::text, u.display_name, u.email,
	   COALESCE(p.title,''), COALESCE(p.status_text,''), COALESCE(p.status_emoji,'')
	   FROM adrient.users u LEFT JOIN adrient.chat_profiles p ON p.user_id=u.id
	   WHERE u.org_id=$1::uuid ORDER BY lower(u.display_name), u.email LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.Title, &p.StatusText, &p.StatusEmoji); err != nil {
			return nil, err
		}
		if p.Name == "" {
			p.Name = p.Email
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetProfile upserts the caller's title and status.
func (s *Store) SetProfile(ctx context.Context, uid, title, statusText, statusEmoji string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO adrient.chat_profiles (user_id,title,status_text,status_emoji) VALUES ($1::uuid,$2,$3,$4)
	  ON CONFLICT (user_id) DO UPDATE SET title=$2, status_text=$3, status_emoji=$4`, uid, title, statusText, statusEmoji)
	return err
}

// ---- messages ---------------------------------------------------------------

const tmsgSelect = `SELECT m.id::text, m.channel_id::text, m.sender_id::text, m.sender_name, m.body, m.sent_at, m.edited_at,
	COALESCE(m.parent_id::text,''), m.mention_ids,
	(SELECT COUNT(*)::int FROM adrient.chat_messages r WHERE r.parent_id=m.id),
	(SELECT MAX(r.sent_at) FROM adrient.chat_messages r WHERE r.parent_id=m.id),
	EXISTS(SELECT 1 FROM adrient.chat_pins pn WHERE pn.message_id=m.id),
	EXISTS(SELECT 1 FROM adrient.chat_saved sv WHERE sv.message_id=m.id AND sv.user_id=$1::uuid)
	FROM adrient.chat_messages m`

func scanTMessage(row pgx.Row) (TMessage, error) {
	var m TMessage
	var sent time.Time
	var edited, lastReply *time.Time
	var mj []byte
	if err := row.Scan(&m.ID, &m.ChannelID, &m.SenderID, &m.SenderName, &m.Body, &sent, &edited, &m.ParentID, &mj,
		&m.ReplyCount, &lastReply, &m.Pinned, &m.Saved); err != nil {
		return TMessage{}, err
	}
	m.SentAt = sent.UTC().Format(time.RFC3339Nano)
	m.EditedAt = ts(edited)
	m.LastReplyAt = ts(lastReply)
	_ = json.Unmarshal(mj, &m.MentionIDs)
	if m.MentionIDs == nil {
		m.MentionIDs = []string{}
	}
	m.Attachments = []TAttachment{}
	m.Reactions = []TReaction{}
	return m, nil
}

func (s *Store) queryMessages(ctx context.Context, uid, sql string, args ...any) ([]TMessage, error) {
	rows, err := s.pool.Query(ctx, sql, append([]any{uid}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TMessage{}
	for rows.Next() {
		m, err := scanTMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.hydrate(ctx, uid, out)
}

// hydrate fills reactions and attachments for a batch of messages.
func (s *Store) hydrate(ctx context.Context, uid string, msgs []TMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]string, len(msgs))
	idx := map[string]int{}
	for i, m := range msgs {
		ids[i] = m.ID
		idx[m.ID] = i
	}
	rx, err := s.repo.GetReactionsForMessages(ctx, ids, uid)
	if err != nil {
		return err
	}
	for id, list := range rx {
		for _, r := range list {
			msgs[idx[id]].Reactions = append(msgs[idx[id]].Reactions, TReaction{Emoji: r.Emoji, Count: int(r.Count), Me: r.Me})
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT message_id::text, id::text, name, mime_type, size_bytes FROM adrient.chat_attachments
	  WHERE message_id = ANY($1::uuid[]) ORDER BY created_at`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var mid string
		var a TAttachment
		if err := rows.Scan(&mid, &a.ID, &a.Name, &a.MimeType, &a.Size); err != nil {
			return err
		}
		a.URL = "/api/v1/chat/attachments/" + a.ID + "/content"
		msgs[idx[mid]].Attachments = append(msgs[idx[mid]].Attachments, a)
	}
	return rows.Err()
}

// GetMessage returns one message (any org message; callers check channel membership).
func (s *Store) GetMessage(ctx context.Context, orgID, uid, id string) (TMessage, error) {
	if !IsUUID(id) {
		return TMessage{}, ErrNotFound
	}
	ms, err := s.queryMessages(ctx, uid, tmsgSelect+` WHERE m.id=$2::uuid AND m.org_id=$3::uuid`, id, orgID)
	if err != nil {
		return TMessage{}, err
	}
	if len(ms) == 0 {
		return TMessage{}, ErrNotFound
	}
	return ms[0], nil
}

// ListChannelMessages returns top-level messages: newest page (before), from a date, or around a message.
func (s *Store) ListChannelMessages(ctx context.Context, orgID, uid, chID, before, around string, from *time.Time, limit int) ([]TMessage, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	base := tmsgSelect + ` WHERE m.channel_id=$2::uuid AND m.org_id=$3::uuid AND m.parent_id IS NULL`
	switch {
	case from != nil:
		ms, err := s.queryMessages(ctx, uid, base+` AND m.sent_at >= $4 ORDER BY m.sent_at ASC LIMIT $5`, chID, orgID, *from, limit)
		return ms, false, err
	case around != "":
		if !IsUUID(around) {
			return nil, false, ErrNotFound
		}
		var at time.Time
		err := s.pool.QueryRow(ctx, `SELECT COALESCE((SELECT p.sent_at FROM adrient.chat_messages p WHERE p.id=m.parent_id), m.sent_at)
		  FROM adrient.chat_messages m WHERE m.id=$1::uuid AND m.channel_id=$2::uuid`, around, chID).Scan(&at)
		if err != nil {
			return nil, false, ErrNotFound
		}
		older, err := s.queryMessages(ctx, uid, base+` AND m.sent_at < $4 ORDER BY m.sent_at DESC LIMIT 25`, chID, orgID, at)
		if err != nil {
			return nil, false, err
		}
		newer, err := s.queryMessages(ctx, uid, base+` AND m.sent_at >= $4 ORDER BY m.sent_at ASC LIMIT 26`, chID, orgID, at)
		if err != nil {
			return nil, false, err
		}
		for i, j := 0, len(older)-1; i < j; i, j = i+1, j-1 {
			older[i], older[j] = older[j], older[i]
		}
		return append(older, newer...), len(older) == 25, nil
	case before != "":
		if !IsUUID(before) {
			return nil, false, ErrNotFound
		}
		ms, err := s.queryMessages(ctx, uid, base+` AND m.sent_at < (SELECT sent_at FROM adrient.chat_messages WHERE id=$4::uuid) ORDER BY m.sent_at DESC LIMIT $5`, chID, orgID, before, limit)
		if err != nil {
			return nil, false, err
		}
		more := len(ms) == limit
		for i, j := 0, len(ms)-1; i < j; i, j = i+1, j-1 {
			ms[i], ms[j] = ms[j], ms[i]
		}
		return ms, more, nil
	}
	ms, err := s.queryMessages(ctx, uid, base+` ORDER BY m.sent_at DESC LIMIT $4`, chID, orgID, limit)
	if err != nil {
		return nil, false, err
	}
	more := len(ms) == limit
	for i, j := 0, len(ms)-1; i < j; i, j = i+1, j-1 {
		ms[i], ms[j] = ms[j], ms[i]
	}
	return ms, more, nil
}

// ListThread returns replies of parentID in order.
func (s *Store) ListThread(ctx context.Context, orgID, uid, parentID string) ([]TMessage, error) {
	return s.queryMessages(ctx, uid, tmsgSelect+` WHERE m.parent_id=$2::uuid AND m.org_id=$3::uuid ORDER BY m.sent_at ASC LIMIT 500`, parentID, orgID)
}

type orgUser struct{ ID, Name, Email string }

func (s *Store) orgUsersAmong(ctx context.Context, orgID string, ids []string) ([]orgUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, display_name, email FROM adrient.users WHERE org_id=$1::uuid AND id = ANY($2::uuid[])`, orgID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []orgUser
	for rows.Next() {
		var u orgUser
		if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ExtractMentions finds @Name / @email / @channel / @here / @everyone tokens
// in body that refer to channel members (never the sender).
func ExtractMentions(body string, members []orgUser, senderID string) []string {
	lb := strings.ToLower(body)
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		if id != senderID && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if hasToken(lb, "@channel") || hasToken(lb, "@here") || hasToken(lb, "@everyone") {
		for _, m := range members {
			add(m.ID)
		}
		return out
	}
	for _, m := range members {
		for _, n := range []string{m.Name, m.Email} {
			n = strings.ToLower(strings.TrimSpace(n))
			if n != "" && hasToken(lb, "@"+n) {
				add(m.ID)
				break
			}
		}
	}
	return out
}

// hasToken reports whether tok occurs in s followed by a non-word character.
func hasToken(s, tok string) bool {
	from := 0
	for {
		i := strings.Index(s[from:], tok)
		if i < 0 {
			return false
		}
		end := from + i + len(tok)
		if end >= len(s) {
			return true
		}
		c := s[end]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return true
		}
		from = end
	}
}

// PostResult is the outcome of posting a message.
type PostResult struct {
	Message TMessage
	Channel TChannel
	Notify  []string // user ids who should be notified
}

// Post stores a message (or thread reply) after validating membership.
func (s *Store) Post(ctx context.Context, orgID, uid, uname, chID, parentID, body string, attIDs []string, nonce string) (PostResult, error) {
	if strings.TrimSpace(body) == "" && len(attIDs) == 0 {
		return PostResult{}, ErrInvalid
	}
	if len(body) > 20000 {
		return PostResult{}, ErrInvalid
	}
	if body == "" {
		body = " "
	}
	ch, err := s.GetChannel(ctx, orgID, uid, chID)
	if err != nil {
		return PostResult{}, err
	}
	if !ch.IsMember {
		return PostResult{}, ErrForbidden
	}
	if ch.Archived {
		return PostResult{}, fmt.Errorf("%w: channel is archived", ErrInvalid)
	}
	if !CanPost(ch, uid, parentID != "") {
		return PostResult{}, fmt.Errorf("%w: only the channel owner can post in an announcement channel", ErrForbidden)
	}
	var parentSender string
	if parentID != "" {
		if !IsUUID(parentID) {
			return PostResult{}, ErrNotFound
		}
		err := s.pool.QueryRow(ctx, `SELECT sender_id::text FROM adrient.chat_messages WHERE id=$1::uuid AND channel_id=$2::uuid AND parent_id IS NULL`, parentID, chID).Scan(&parentSender)
		if err != nil {
			return PostResult{}, ErrNotFound
		}
	}
	members, err := s.orgUsersAmong(ctx, orgID, ch.MemberIDs)
	if err != nil {
		return PostResult{}, err
	}
	mentions := ExtractMentions(body, members, uid)
	m, err := s.repo.PostMessage(ctx, chID, orgID, uid, uname, body, parentID)
	if err != nil {
		return PostResult{}, err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE adrient.chat_messages SET mention_ids=$2 WHERE id=$1::uuid`, m.ID, jsonArr(mentions)); err != nil {
		return PostResult{}, err
	}
	if len(attIDs) > 0 {
		_ = s.repo.LinkAttachmentsToMessage(ctx, orgID, m.ID, attIDs)
	}
	_, _ = s.pool.Exec(ctx, `DELETE FROM adrient.chat_drafts WHERE user_id=$1::uuid AND channel_id=$2::uuid AND parent_id=$3`, uid, chID, parentID)
	// The sender has obviously read the channel up to now.
	_ = s.repo.UpdateReadCursor(ctx, chID, uid)

	notify := map[string]bool{}
	for _, id := range mentions {
		notify[id] = true
	}
	if ch.Kind == "dm" || ch.Kind == "group" {
		for _, id := range ch.MemberIDs {
			notify[id] = true
		}
	}
	if parentID != "" {
		notify[parentSender] = true
		rows, err := s.pool.Query(ctx, `SELECT DISTINCT sender_id::text FROM adrient.chat_messages WHERE parent_id=$1::uuid`, parentID)
		if err == nil {
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					notify[id] = true
				}
			}
			rows.Close()
		}
	}
	delete(notify, uid)
	nl := []string{}
	for id := range notify {
		if isMember(ch.MemberIDs, id) {
			nl = append(nl, id)
		}
	}
	full, err := s.GetMessage(ctx, orgID, uid, m.ID)
	if err != nil {
		return PostResult{}, err
	}
	full.ClientNonce = nonce
	return PostResult{Message: full, Channel: ch, Notify: nl}, nil
}

// EditMessage updates a message's body (sender only).
func (s *Store) EditMessage(ctx context.Context, orgID, uid, id, body string) (TMessage, error) {
	if strings.TrimSpace(body) == "" {
		return TMessage{}, ErrInvalid
	}
	cur, err := s.GetMessage(ctx, orgID, uid, id)
	if err != nil {
		return TMessage{}, err
	}
	if cur.SenderID != uid {
		return TMessage{}, ErrForbidden
	}
	ch, err := s.GetChannel(ctx, orgID, uid, cur.ChannelID)
	if err != nil {
		return TMessage{}, err
	}
	members, _ := s.orgUsersAmong(ctx, orgID, ch.MemberIDs)
	mentions := ExtractMentions(body, members, uid)
	if _, err := s.pool.Exec(ctx, `UPDATE adrient.chat_messages SET body=$2, edited_at=now(), mention_ids=$3 WHERE id=$1::uuid`, id, body, jsonArr(mentions)); err != nil {
		return TMessage{}, err
	}
	return s.GetMessage(ctx, orgID, uid, id)
}

// ReactorLists returns emoji -> user ids for broadcasting a reaction change.
func (s *Store) ReactorLists(ctx context.Context, msgID string) (map[string][]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT emoji, user_id::text FROM adrient.chat_message_reactions WHERE message_id=$1::uuid ORDER BY created_at`, msgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var e, u string
		if err := rows.Scan(&e, &u); err != nil {
			return nil, err
		}
		out[e] = append(out[e], u)
	}
	return out, rows.Err()
}

// TogglePin pins/unpins a message and returns the new state.
func (s *Store) TogglePin(ctx context.Context, uid, chID, msgID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM adrient.chat_pins WHERE message_id=$1::uuid`, msgID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO adrient.chat_pins (message_id, channel_id, pinned_by) VALUES ($1::uuid,$2::uuid,$3::uuid)`, msgID, chID, uid)
	return true, err
}

// ListPins returns pinned messages of a channel, newest pin first.
func (s *Store) ListPins(ctx context.Context, orgID, uid, chID string) ([]TMessage, error) {
	return s.queryMessages(ctx, uid, tmsgSelect+` JOIN adrient.chat_pins pn ON pn.message_id=m.id WHERE pn.channel_id=$2::uuid AND m.org_id=$3::uuid ORDER BY pn.created_at DESC`, chID, orgID)
}

// ListBookmarks returns channel bookmarks.
func (s *Store) ListBookmarks(ctx context.Context, chID string) ([]Bookmark, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, title, url FROM adrient.chat_bookmarks WHERE channel_id=$1::uuid ORDER BY created_at`, chID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bookmark{}
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.Title, &b.URL); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AddBookmark adds an http(s) link.
func (s *Store) AddBookmark(ctx context.Context, uid, chID, title, url string) (Bookmark, error) {
	title, url = strings.TrimSpace(title), strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "/") {
		return Bookmark{}, ErrInvalid
	}
	if title == "" {
		title = url
	}
	var b Bookmark
	err := s.pool.QueryRow(ctx, `INSERT INTO adrient.chat_bookmarks (channel_id,title,url,created_by) VALUES ($1::uuid,$2,$3,$4::uuid) RETURNING id::text,title,url`,
		chID, title, url, uid).Scan(&b.ID, &b.Title, &b.URL)
	return b, err
}

// DeleteBookmark removes a bookmark within a channel.
func (s *Store) DeleteBookmark(ctx context.Context, chID, id string) error {
	if !IsUUID(id) {
		return ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM adrient.chat_bookmarks WHERE id=$1::uuid AND channel_id=$2::uuid`, id, chID)
	return err
}

// ToggleSaved toggles a message in the user's Later list.
func (s *Store) ToggleSaved(ctx context.Context, uid, msgID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM adrient.chat_saved WHERE user_id=$1::uuid AND message_id=$2::uuid`, uid, msgID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO adrient.chat_saved (user_id,message_id) VALUES ($1::uuid,$2::uuid)`, uid, msgID)
	return true, err
}

// ListSaved returns the user's saved messages in channels they still belong to.
func (s *Store) ListSaved(ctx context.Context, orgID, uid string) ([]TMessage, error) {
	return s.queryMessages(ctx, uid, tmsgSelect+` JOIN adrient.chat_saved sv ON sv.message_id=m.id AND sv.user_id=$1::uuid
	  JOIN adrient.chat_channels c ON c.id=m.channel_id
	  WHERE m.org_id=$2::uuid AND c.member_ids ? $3::text ORDER BY sv.created_at DESC LIMIT 200`, orgID, uid)
}

// ---- reminders / drafts / scheduled ----------------------------------------

// AddReminder schedules a reminder for a message (or a free note).
func (s *Store) AddReminder(ctx context.Context, orgID, uid, msgID, note string, at time.Time) (Reminder, error) {
	var mid *string
	if msgID != "" {
		mid = &msgID
	}
	var r Reminder
	var t time.Time
	err := s.pool.QueryRow(ctx, `INSERT INTO adrient.chat_reminders (org_id,user_id,message_id,note,remind_at) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5)
	  RETURNING id::text, COALESCE(message_id::text,''), note, remind_at, fired`, orgID, uid, mid, note, at).Scan(&r.ID, &r.MessageID, &r.Note, &t, &r.Fired)
	r.RemindAt = t.UTC().Format(time.RFC3339)
	return r, err
}

// ListReminders returns the user's reminders (pending first).
func (s *Store) ListReminders(ctx context.Context, orgID, uid string) ([]Reminder, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id::text, COALESCE(r.message_id::text,''), r.note, r.remind_at, r.fired, COALESCE(m.channel_id::text,'')
	  FROM adrient.chat_reminders r LEFT JOIN adrient.chat_messages m ON m.id=r.message_id
	  WHERE r.user_id=$1::uuid AND r.org_id=$2::uuid AND (NOT r.fired OR r.remind_at > now() - interval '7 days')
	  ORDER BY r.fired, r.remind_at LIMIT 100`, uid, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reminder{}
	for rows.Next() {
		var r Reminder
		var t time.Time
		if err := rows.Scan(&r.ID, &r.MessageID, &r.Note, &t, &r.Fired, &r.ChannelID); err != nil {
			return nil, err
		}
		r.RemindAt = t.UTC().Format(time.RFC3339)
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteReminder cancels a reminder.
func (s *Store) DeleteReminder(ctx context.Context, uid, id string) error {
	if !IsUUID(id) {
		return ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM adrient.chat_reminders WHERE id=$1::uuid AND user_id=$2::uuid`, id, uid)
	return err
}

// SaveDraft upserts a draft; an empty body deletes it.
func (s *Store) SaveDraft(ctx context.Context, uid, chID, parentID, body string) error {
	if strings.TrimSpace(body) == "" {
		_, err := s.pool.Exec(ctx, `DELETE FROM adrient.chat_drafts WHERE user_id=$1::uuid AND channel_id=$2::uuid AND parent_id=$3`, uid, chID, parentID)
		return err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO adrient.chat_drafts (user_id,channel_id,parent_id,body) VALUES ($1::uuid,$2::uuid,$3,$4)
	  ON CONFLICT (user_id,channel_id,parent_id) DO UPDATE SET body=$4, updated_at=now()`, uid, chID, parentID, body)
	return err
}

// ListDrafts returns the user's drafts.
func (s *Store) ListDrafts(ctx context.Context, uid string) ([]Draft, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, channel_id::text, parent_id, body, updated_at FROM adrient.chat_drafts WHERE user_id=$1::uuid ORDER BY updated_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Draft{}
	for rows.Next() {
		var d Draft
		var t time.Time
		if err := rows.Scan(&d.ID, &d.ChannelID, &d.ParentID, &d.Body, &t); err != nil {
			return nil, err
		}
		d.UpdatedAt = t.UTC().Format(time.RFC3339)
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDraft removes a draft by id.
func (s *Store) DeleteDraft(ctx context.Context, uid, id string) error {
	if !IsUUID(id) {
		return ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM adrient.chat_drafts WHERE id=$1::uuid AND user_id=$2::uuid`, id, uid)
	return err
}

// Schedule queues a message for later.
func (s *Store) Schedule(ctx context.Context, orgID, uid, chID, parentID, body string, at time.Time) (Scheduled, error) {
	if strings.TrimSpace(body) == "" {
		return Scheduled{}, ErrInvalid
	}
	if !at.After(time.Now().Add(-time.Minute)) {
		return Scheduled{}, fmt.Errorf("%w: send time is in the past", ErrInvalid)
	}
	var x Scheduled
	var t time.Time
	err := s.pool.QueryRow(ctx, `INSERT INTO adrient.chat_scheduled (org_id,user_id,channel_id,parent_id,body,send_at) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6)
	  RETURNING id::text, channel_id::text, parent_id, body, send_at, status`, orgID, uid, chID, parentID, body, at).Scan(&x.ID, &x.ChannelID, &x.ParentID, &x.Body, &t, &x.Status)
	x.SendAt = t.UTC().Format(time.RFC3339)
	return x, err
}

// ListScheduled returns the user's pending scheduled messages.
func (s *Store) ListScheduled(ctx context.Context, uid string) ([]Scheduled, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, channel_id::text, parent_id, body, send_at, status FROM adrient.chat_scheduled
	  WHERE user_id=$1::uuid AND status='pending' ORDER BY send_at`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Scheduled{}
	for rows.Next() {
		var x Scheduled
		var t time.Time
		if err := rows.Scan(&x.ID, &x.ChannelID, &x.ParentID, &x.Body, &t, &x.Status); err != nil {
			return nil, err
		}
		x.SendAt = t.UTC().Format(time.RFC3339)
		out = append(out, x)
	}
	return out, rows.Err()
}

// CancelScheduled cancels a pending scheduled message.
func (s *Store) CancelScheduled(ctx context.Context, uid, id string) error {
	if !IsUUID(id) {
		return ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `UPDATE adrient.chat_scheduled SET status='cancelled' WHERE id=$1::uuid AND user_id=$2::uuid AND status='pending'`, id, uid)
	return err
}

type dueScheduled struct{ ID, OrgID, UserID, ChannelID, ParentID, Body string }

// ClaimDueScheduled atomically claims scheduled messages that are due.
func (s *Store) ClaimDueScheduled(ctx context.Context) ([]dueScheduled, error) {
	rows, err := s.pool.Query(ctx, `UPDATE adrient.chat_scheduled SET status='sent' WHERE id IN (
	  SELECT id FROM adrient.chat_scheduled WHERE status='pending' AND send_at <= now() ORDER BY send_at LIMIT 20 FOR UPDATE SKIP LOCKED)
	  RETURNING id::text, org_id::text, user_id::text, channel_id::text, parent_id, body`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dueScheduled
	for rows.Next() {
		var d dueScheduled
		if err := rows.Scan(&d.ID, &d.OrgID, &d.UserID, &d.ChannelID, &d.ParentID, &d.Body); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type dueReminder struct{ ID, OrgID, UserID, MessageID, Note string }

// ClaimDueReminders marks due reminders fired and returns them.
func (s *Store) ClaimDueReminders(ctx context.Context) ([]dueReminder, error) {
	rows, err := s.pool.Query(ctx, `UPDATE adrient.chat_reminders SET fired=true WHERE id IN (
	  SELECT id FROM adrient.chat_reminders WHERE NOT fired AND remind_at <= now() ORDER BY remind_at LIMIT 50 FOR UPDATE SKIP LOCKED)
	  RETURNING id::text, org_id::text, user_id::text, COALESCE(message_id::text,''), note`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dueReminder
	for rows.Next() {
		var d dueReminder
		if err := rows.Scan(&d.ID, &d.OrgID, &d.UserID, &d.MessageID, &d.Note); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UserName returns the display name (or email) for a user.
func (s *Store) UserName(ctx context.Context, uid string) string {
	var n, e string
	if err := s.pool.QueryRow(ctx, `SELECT display_name, email FROM adrient.users WHERE id=$1::uuid`, uid).Scan(&n, &e); err != nil {
		return ""
	}
	if n == "" {
		return e
	}
	return n
}

// ---- sent / activity / search ----------------------------------------------

// ListSent returns the user's most recent messages.
func (s *Store) ListSent(ctx context.Context, orgID, uid string) ([]TMessage, error) {
	return s.queryMessages(ctx, uid, tmsgSelect+` JOIN adrient.chat_channels c ON c.id=m.channel_id
	  WHERE m.org_id=$2::uuid AND m.sender_id=$1::uuid AND c.member_ids ? $3::text ORDER BY m.sent_at DESC LIMIT 50`, orgID, uid)
}

const activityWhere = ` WHERE m.org_id=$2::uuid AND c.member_ids ? $3::text AND m.sender_id <> $1::uuid
	AND (m.mention_ids ? $3::text OR (m.parent_id IS NOT NULL AND (
	   EXISTS(SELECT 1 FROM adrient.chat_messages p WHERE p.id=m.parent_id AND p.sender_id=$1::uuid)
	   OR EXISTS(SELECT 1 FROM adrient.chat_messages r WHERE r.parent_id=m.parent_id AND r.sender_id=$1::uuid))))`

// Activity returns mentions, thread replies and fired reminders.
func (s *Store) Activity(ctx context.Context, orgID, uid string) ([]ActivityItem, error) {
	var seen time.Time
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE((SELECT activity_seen_at FROM adrient.chat_profiles WHERE user_id=$1::uuid),'-infinity'::timestamptz)`, uid).Scan(&seen)
	ms, err := s.queryMessages(ctx, uid, tmsgSelect+` JOIN adrient.chat_channels c ON c.id=m.channel_id`+activityWhere+` ORDER BY m.sent_at DESC LIMIT 60`, orgID, uid)
	if err != nil {
		return nil, err
	}
	out := []ActivityItem{}
	for _, m := range ms {
		kind := "thread_reply"
		if isMember(m.MentionIDs, uid) {
			kind = "mention"
		}
		t, _ := time.Parse(time.RFC3339Nano, m.SentAt)
		out = append(out, ActivityItem{Kind: kind, At: m.SentAt, Message: m, Unread: t.After(seen)})
	}
	rems, err := s.ListReminders(ctx, orgID, uid)
	if err != nil {
		return nil, err
	}
	for _, r := range rems {
		if !r.Fired {
			continue
		}
		it := ActivityItem{Kind: "reminder", At: r.RemindAt, Note: r.Note}
		if r.MessageID != "" {
			if m, err := s.GetMessage(ctx, orgID, uid, r.MessageID); err == nil {
				it.Message = m
			}
		}
		t, _ := time.Parse(time.RFC3339, r.RemindAt)
		it.Unread = t.After(seen)
		out = append(out, it)
	}
	// newest first
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].At > out[j-1].At; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// ActivityUnread counts unseen activity items.
func (s *Store) ActivityUnread(ctx context.Context, orgID, uid string) int {
	var n int
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM adrient.chat_messages m JOIN adrient.chat_channels c ON c.id=m.channel_id`+activityWhere+
		` AND m.sent_at > COALESCE((SELECT activity_seen_at FROM adrient.chat_profiles WHERE user_id=$1::uuid),'-infinity'::timestamptz)`, uid, orgID, uid).Scan(&n)
	var r int
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM adrient.chat_reminders WHERE user_id=$1::uuid AND fired AND remind_at > COALESCE((SELECT activity_seen_at FROM adrient.chat_profiles WHERE user_id=$1::uuid),'-infinity'::timestamptz)`, uid).Scan(&r)
	return n + r
}

// MarkActivitySeen records that the user opened Activity.
func (s *Store) MarkActivitySeen(ctx context.Context, uid string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO adrient.chat_profiles (user_id, activity_seen_at) VALUES ($1::uuid, now())
	  ON CONFLICT (user_id) DO UPDATE SET activity_seen_at=now()`, uid)
	return err
}

// Counts returns drafts/scheduled/saved totals.
func (s *Store) Counts(ctx context.Context, uid string) (drafts, scheduled, saved int) {
	_ = s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM adrient.chat_drafts WHERE user_id=$1::uuid)::int,
	  (SELECT COUNT(*) FROM adrient.chat_scheduled WHERE user_id=$1::uuid AND status='pending')::int,
	  (SELECT COUNT(*) FROM adrient.chat_saved WHERE user_id=$1::uuid)::int`, uid).Scan(&drafts, &scheduled, &saved)
	return
}

func escapeLike(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(q)
}

// Search finds messages the user can see.
func (s *Store) Search(ctx context.Context, orgID, uid, q, chID, senderID string, from, to *time.Time) ([]TMessage, error) {
	q = strings.TrimSpace(q)
	if q == "" && from == nil && senderID == "" {
		return []TMessage{}, nil
	}
	sql := tmsgSelect + ` JOIN adrient.chat_channels c ON c.id=m.channel_id
	  WHERE m.org_id=$2::uuid AND c.member_ids ? $3::text AND m.body ILIKE $4`
	args := []any{orgID, uid, "%" + escapeLike(q) + "%"}
	if chID != "" && IsUUID(chID) {
		args = append(args, chID)
		sql += fmt.Sprintf(` AND m.channel_id=$%d::uuid`, len(args)+1)
	}
	if senderID != "" && IsUUID(senderID) {
		args = append(args, senderID)
		sql += fmt.Sprintf(` AND m.sender_id=$%d::uuid`, len(args)+1)
	}
	if from != nil {
		args = append(args, *from)
		sql += fmt.Sprintf(` AND m.sent_at >= $%d`, len(args)+1)
	}
	if to != nil {
		args = append(args, *to)
		sql += fmt.Sprintf(` AND m.sent_at < $%d`, len(args)+1)
	}
	sql += ` ORDER BY m.sent_at DESC LIMIT 60`
	return s.queryMessages(ctx, uid, sql, args...)
}

// CanPost reports whether uid may post in ch. In an announcement channel only
// the creator posts top-level messages; anyone in it may still reply in threads.
func CanPost(ch TChannel, uid string, isReply bool) bool {
	if !ch.Announce || isReply {
		return true
	}
	return ch.CreatedBy == "" || ch.CreatedBy == uid
}
