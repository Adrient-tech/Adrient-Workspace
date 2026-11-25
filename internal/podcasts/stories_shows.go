package podcasts

import (
	"bytes"
	"context"
	"errors"
	"hash/fnv"
	"io"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	xhtml "golang.org/x/net/html"

	"github.com/Adrient-tech/Adrient-Workspace/internal/tts"
)

// ErrNotFound is returned for a missing or invisible story, episode or comment.
// (It is the same value the RSS subscriptions use.)

var langRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)

// Genres offered in the UI; any short text is accepted.
var Genres = []string{"General", "Onboarding", "Safety", "Leadership", "Culture", "Product", "Customers", "Learning", "Fiction", "Updates", "Wellbeing"}

// ShowInput is what an author may set on a show.
type ShowInput struct {
	Title         *string            `json:"title"`
	Description   *string            `json:"description"`
	Genre         *string            `json:"genre"`
	Language      *string            `json:"language"`
	CoverStyle    *string            `json:"cover_style"`
	Audience      *string            `json:"audience"`
	AudienceID    *string            `json:"audience_id"`
	CommentsOn    *bool              `json:"comments_on"`
	AllowDownload *bool              `json:"allow_download"`
	Provider      *string            `json:"provider"`
	Voice         *string            `json:"voice"`
	VoiceMap      *map[string]string `json:"voice_map"`
	Speed         *float64           `json:"speed"`
	Pitch         *float64           `json:"pitch"`
}

func defaultCoverStyle(seed string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	n := h.Sum32()
	hue := int(n % 360)
	return itoa(hue) + "," + itoa((hue+40+int(n>>9)%80)%360) + "," + itoa(120+int(n>>17)%90)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

var coverStyleRe = regexp.MustCompile(`^\d{1,3},\d{1,3},\d{1,3}$`)

func (s *Stories) checkShowInput(ctx context.Context, org string, in *ShowInput) error {
	if in.Title != nil {
		t := cleanText(*in.Title, 120)
		if t == "" {
			return invalid("a title is required")
		}
		in.Title = &t
	}
	if in.Description != nil {
		d := cleanText(*in.Description, 4000)
		in.Description = &d
	}
	if in.Genre != nil {
		g := cleanText(*in.Genre, 40)
		if g == "" {
			g = "General"
		}
		in.Genre = &g
	}
	if in.Language != nil && !langRe.MatchString(*in.Language) {
		return invalid("language must be a code such as en, hi or ta")
	}
	if in.CoverStyle != nil && *in.CoverStyle != "" && !coverStyleRe.MatchString(*in.CoverStyle) {
		return invalid("cover style is not valid")
	}
	if in.Speed != nil && (*in.Speed < 0.5 || *in.Speed > 2) {
		return invalid("speed must be between 0.5 and 2")
	}
	if in.Pitch != nil && (*in.Pitch < 0.5 || *in.Pitch > 1.5) {
		return invalid("pitch must be between 0.5 and 1.5")
	}
	if in.Voice != nil && *in.Voice != "" && !tts.ValidVoice(*in.Voice) {
		return invalid("voice id is not valid")
	}
	if in.VoiceMap != nil {
		if len(*in.VoiceMap) > 12 {
			return invalid("at most 12 character voices")
		}
		for k, v := range *in.VoiceMap {
			if len(k) > 40 || (v != "" && !tts.ValidVoice(v)) {
				return invalid("character voice is not valid")
			}
		}
	}
	if in.Audience != nil {
		switch *in.Audience {
		case "everyone":
			e := ""
			in.AudienceID = &e
		case "group", "department":
			if in.AudienceID == nil || !isUUID(*in.AudienceID) {
				return invalid("choose a %s", *in.Audience)
			}
			table := "adrient.groups"
			if *in.Audience == "department" {
				table = "adrient.departments"
			}
			var n int
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id=$1::uuid AND org_id=$2::uuid`, *in.AudienceID, org).Scan(&n); err != nil || n == 0 {
				return invalid("that %s does not exist", *in.Audience)
			}
		default:
			return invalid("audience must be everyone, group or department")
		}
	}
	return nil
}

// CreateShow adds a show for the viewer.
func (s *Stories) CreateShow(ctx context.Context, v Viewer, in ShowInput) (Show, error) {
	if in.Title == nil {
		return Show{}, invalid("a title is required")
	}
	if err := s.checkShowInput(ctx, v.OrgID, &in); err != nil {
		return Show{}, err
	}
	var id string
	desc, genre, lang := val(in.Description, ""), val(in.Genre, "General"), val(in.Language, "en")
	style := val(in.CoverStyle, "")
	if style == "" {
		style = defaultCoverStyle(*in.Title + v.UserID + s.now().String())
	}
	aud, audID := val(in.Audience, "everyone"), val(in.AudienceID, "")
	var audArg any
	if audID != "" {
		audArg = audID
	}
	prov, err := providerArg(val(in.Provider, "default"))
	if err != nil {
		return Show{}, err
	}
	vm := map[string]string{}
	if in.VoiceMap != nil {
		vm = *in.VoiceMap
	}
	err = s.Pool.QueryRow(ctx, `INSERT INTO adrient.story_shows (org_id, author_id, title, description, genre, language, cover_style, audience, audience_id,
		comments_on, allow_download, voice_provider, voice, voice_map, speed, pitch)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id::text`,
		v.OrgID, v.UserID, *in.Title, desc, genre, lang, style, aud, audArg, valB(in.CommentsOn, true), valB(in.AllowDownload, false),
		prov, val(in.Voice, ""), vm, valF(in.Speed, 1), valF(in.Pitch, 1)).Scan(&id)
	if err != nil {
		return Show{}, err
	}
	return s.GetShow(ctx, v, id)
}

func val(p *string, d string) string {
	if p == nil {
		return d
	}
	return *p
}
func valB(p *bool, d bool) bool {
	if p == nil {
		return d
	}
	return *p
}
func valF(p *float64, d float64) float64 {
	if p == nil {
		return d
	}
	return *p
}

// GetShow returns a show the viewer may open.
func (s *Stories) GetShow(ctx context.Context, v Viewer, id string) (Show, error) {
	if !isUUID(id) {
		return Show{}, ErrNotFound
	}
	sh, err := scanShow(s.Pool.QueryRow(ctx, `SELECT `+showSelect+` `+showFrom+` WHERE s.id=$5::uuid AND `+visible, v.args(id)...), v.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return sh, ErrNotFound
	}
	return sh, err
}

// ShowFilter narrows ListShows.
type ShowFilter struct {
	Genre, Q, Sort string
	Mine           bool
	Following      bool
	Library        bool
	Limit          int
}

// ListShows lists the shows the viewer may open. Other people's shows appear
// once they hold a published episode.
func (s *Stories) ListShows(ctx context.Context, v Viewer, f ShowFilter) ([]Show, error) {
	where := []string{visible}
	args := v.args()
	add := func(sql string, a any) {
		args = append(args, a)
		where = append(where, strings.ReplaceAll(sql, "?", "$"+itoa(len(args))))
	}
	if f.Mine {
		where = append(where, "s.author_id=$2::uuid")
	} else {
		where = append(where, "EXISTS(SELECT 1 FROM adrient.story_episodes e WHERE e.show_id=s.id AND e.status='published')")
	}
	if f.Genre != "" {
		add("s.genre = ?", f.Genre)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		add(`(lower(s.title) LIKE ? ESCAPE '\' OR lower(s.description) LIKE ? ESCAPE '\' OR lower(u.display_name) LIKE ? ESCAPE '\')`, likePattern(q))
	}
	if f.Following {
		where = append(where, `(EXISTS(SELECT 1 FROM adrient.story_follows f WHERE f.user_id=$2::uuid AND f.kind='show' AND f.target_id=s.id)
			OR EXISTS(SELECT 1 FROM adrient.story_follows f WHERE f.user_id=$2::uuid AND f.kind='author' AND f.target_id=s.author_id))`)
	}
	if f.Library {
		where = append(where, `EXISTS(SELECT 1 FROM adrient.story_library b WHERE b.user_id=$2::uuid AND b.kind='show' AND b.target_id=s.id)`)
	}
	order := "s.updated_at DESC"
	switch f.Sort {
	case "title":
		order = "lower(s.title)"
	case "popular":
		order = "(SELECT count(*) FROM adrient.story_plays p JOIN adrient.story_episodes e ON e.id=p.episode_id WHERE e.show_id=s.id) DESC, s.updated_at DESC"
	}
	n := f.Limit
	if n <= 0 || n > 200 {
		n = 100
	}
	// the search term appears three times but is one argument; fix the placeholders
	sql := `SELECT ` + showSelect + ` ` + showFrom + ` WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + order + ` LIMIT ` + itoa(n)
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Show{}
	for rows.Next() {
		sh, err := scanShow(rows, v.UserID)
		if err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

func likePattern(q string) string {
	q = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(q))
	return "%" + q + "%"
}

// authorOnly loads a show and checks the viewer wrote it.
func (s *Stories) authorOnly(ctx context.Context, v Viewer, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	var author string
	err := s.Pool.QueryRow(ctx, `SELECT author_id::text FROM adrient.story_shows WHERE id=$1::uuid AND org_id=$2::uuid`, id, v.OrgID).Scan(&author)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if author != v.UserID {
		return forbidden("only the author can change this story")
	}
	return nil
}

// UpdateShow changes the fields that are set.
func (s *Stories) UpdateShow(ctx context.Context, v Viewer, id string, in ShowInput) (Show, error) {
	if err := s.authorOnly(ctx, v, id); err != nil {
		return Show{}, err
	}
	if err := s.checkShowInput(ctx, v.OrgID, &in); err != nil {
		return Show{}, err
	}
	sets := []string{"updated_at=now()"}
	args := []any{id}
	set := func(col string, a any) {
		args = append(args, a)
		sets = append(sets, col+"=$"+itoa(len(args)))
	}
	if in.Title != nil {
		set("title", *in.Title)
	}
	if in.Description != nil {
		set("description", *in.Description)
	}
	if in.Genre != nil {
		set("genre", *in.Genre)
	}
	if in.Language != nil {
		set("language", *in.Language)
	}
	if in.CoverStyle != nil {
		set("cover_style", *in.CoverStyle)
	}
	if in.Audience != nil {
		set("audience", *in.Audience)
		var a any
		if val(in.AudienceID, "") != "" {
			a = *in.AudienceID
		}
		set("audience_id", a)
	}
	if in.CommentsOn != nil {
		set("comments_on", *in.CommentsOn)
	}
	if in.AllowDownload != nil {
		set("allow_download", *in.AllowDownload)
	}
	if in.Provider != nil {
		p, err := providerArg(*in.Provider)
		if err != nil {
			return Show{}, err
		}
		set("voice_provider", p)
	}
	if in.Voice != nil {
		set("voice", *in.Voice)
	}
	if in.VoiceMap != nil {
		set("voice_map", *in.VoiceMap)
	}
	if in.Speed != nil {
		set("speed", *in.Speed)
	}
	if in.Pitch != nil {
		set("pitch", *in.Pitch)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_shows SET `+strings.Join(sets, ",")+` WHERE id=$1::uuid`, args...); err != nil {
		return Show{}, err
	}
	return s.GetShow(ctx, v, id)
}

// DeleteShow removes a show with its episodes, audio and cover.
func (s *Stories) DeleteShow(ctx context.Context, v Viewer, id string) error {
	if err := s.authorOnly(ctx, v, id); err != nil {
		return err
	}
	var keys []string
	rows, err := s.Pool.Query(ctx, `SELECT audio_blob FROM adrient.story_episodes WHERE show_id=$1::uuid AND audio_blob<>''
		UNION SELECT cover_blob FROM adrient.story_shows WHERE id=$1::uuid AND cover_blob<>''`, id)
	if err == nil {
		for rows.Next() {
			var k string
			if rows.Scan(&k) == nil {
				keys = append(keys, k)
			}
		}
		rows.Close()
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM adrient.story_library WHERE kind='show' AND target_id=$1::uuid`, id); err != nil {
		return err
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM adrient.story_follows WHERE kind='show' AND target_id=$1::uuid`, id); err != nil {
		return err
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM adrient.story_shows WHERE id=$1::uuid AND org_id=$2::uuid`, id, v.OrgID); err != nil {
		return err
	}
	s.dropBlobs(ctx, v.OrgID, keys)
	return nil
}

// dropBlobs deletes stored objects nothing else uses any more.
func (s *Stories) dropBlobs(ctx context.Context, org string, keys []string) {
	if s.Blobs == nil {
		return
	}
	for _, k := range keys {
		var n int
		if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM adrient.story_episodes WHERE audio_blob=$1)+(SELECT count(*) FROM adrient.story_shows WHERE cover_blob=$1)`, k).Scan(&n); err == nil && n == 0 {
			s.log("delete blob", s.Blobs.Delete(ctx, k))
		}
	}
}

// SetCover stores an uploaded cover image (png, jpeg, webp or gif, up to 3 MB).
func (s *Stories) SetCover(ctx context.Context, v Viewer, id string, body io.Reader) (Show, error) {
	if err := s.authorOnly(ctx, v, id); err != nil {
		return Show{}, err
	}
	if s.Blobs == nil {
		return Show{}, invalid("file storage is not available")
	}
	raw, err := io.ReadAll(io.LimitReader(body, 3<<20+1))
	if err != nil {
		return Show{}, err
	}
	if len(raw) > 3<<20 {
		return Show{}, invalid("the cover image is larger than 3 MB")
	}
	mime := sniffImage(raw)
	if mime == "" {
		return Show{}, invalid("the cover must be a PNG, JPEG, WebP or GIF image")
	}
	key := "stories/covers/" + v.OrgID + "/" + id + "-" + itoa(int(s.now().Unix())) + imageExt(mime)
	if err := s.Blobs.Put(ctx, key, mime, int64(len(raw)), bytes.NewReader(raw)); err != nil {
		return Show{}, err
	}
	var old string
	if err := s.Pool.QueryRow(ctx, `SELECT cover_blob FROM adrient.story_shows WHERE id=$1::uuid`, id).Scan(&old); err != nil {
		return Show{}, err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_shows SET cover_blob=$2, cover_mime=$3, updated_at=now() WHERE id=$1::uuid`, id, key, mime); err != nil {
		return Show{}, err
	}
	if old != "" {
		s.dropBlobs(ctx, v.OrgID, []string{old})
	}
	return s.GetShow(ctx, v, id)
}

// ClearCover goes back to the generated gradient cover.
func (s *Stories) ClearCover(ctx context.Context, v Viewer, id string) (Show, error) {
	if err := s.authorOnly(ctx, v, id); err != nil {
		return Show{}, err
	}
	var old string
	_ = s.Pool.QueryRow(ctx, `SELECT cover_blob FROM adrient.story_shows WHERE id=$1::uuid`, id).Scan(&old)
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_shows SET cover_blob='', cover_mime='', updated_at=now() WHERE id=$1::uuid`, id); err != nil {
		return Show{}, err
	}
	if old != "" {
		s.dropBlobs(ctx, v.OrgID, []string{old})
	}
	return s.GetShow(ctx, v, id)
}

// Cover opens a show's cover image for a viewer who may see the show.
func (s *Stories) Cover(ctx context.Context, v Viewer, id string) (io.ReadCloser, string, error) {
	if !isUUID(id) || s.Blobs == nil {
		return nil, "", ErrNotFound
	}
	var key, mime string
	err := s.Pool.QueryRow(ctx, `SELECT s.cover_blob, s.cover_mime FROM adrient.story_shows s WHERE s.id=$5::uuid AND `+visible, v.args(id)...).Scan(&key, &mime)
	if err != nil || key == "" {
		return nil, "", ErrNotFound
	}
	rc, _, _, err := s.Blobs.Get(ctx, key)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return rc, mime, nil
}

func sniffImage(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("GIF8")):
		return "image/gif"
	case len(b) > 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}

func imageExt(m string) string {
	switch m {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ".jpg"
}

// ---- HTML -------------------------------------------------------------------

var allowedTags = map[string]bool{"p": true, "br": true, "h1": true, "h2": true, "h3": true, "strong": true, "b": true, "em": true, "i": true, "u": true,
	"s": true, "ul": true, "ol": true, "li": true, "blockquote": true, "hr": true, "a": true, "code": true}

// SanitizeHTML keeps a small set of formatting tags and drops everything else
// (scripts, styles, event attributes, unknown tags). Links keep http, https and mailto only.
func SanitizeHTML(src string) string {
	z := xhtml.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	skip := 0
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			return b.String()
		}
		switch tt {
		case xhtml.TextToken:
			if skip == 0 {
				b.WriteString(xhtmlEscape(string(z.Text())))
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			n := string(name)
			if n == "script" || n == "style" {
				skip++
				continue
			}
			if skip > 0 || !allowedTags[n] {
				continue
			}
			if n == "a" {
				href := ""
				for hasAttr {
					var k, vv []byte
					k, vv, hasAttr = z.TagAttr()
					if string(k) == "href" {
						href = strings.TrimSpace(string(vv))
					}
				}
				low := strings.ToLower(href)
				if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "mailto:") {
					b.WriteString(`<a href="` + xhtmlEscape(href) + `" target="_blank" rel="noopener noreferrer">`)
				} else {
					b.WriteString("<a>")
				}
				continue
			}
			b.WriteString("<" + n + ">")
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			n := string(name)
			if n == "script" || n == "style" {
				if skip > 0 {
					skip--
				}
				continue
			}
			if skip == 0 && allowedTags[n] && n != "br" && n != "hr" {
				b.WriteString("</" + n + ">")
			}
		}
	}
}

func xhtmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
