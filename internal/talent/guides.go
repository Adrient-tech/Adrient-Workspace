package talent

import (
	"context"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// GuideQuestion is one question of an interview guide; Dimension names the rating it feeds.
type GuideQuestion struct {
	Text      string `json:"text"`
	Dimension string `json:"dimension,omitempty"`
}

// GuideInput creates or replaces a guide.
type GuideInput struct {
	Name      string          `json:"name"`
	Stage     string          `json:"stage"`
	Format    string          `json:"format"`
	Questions []GuideQuestion `json:"questions"`
}

// Answer is one answered guide question, snapshotted onto the scorecard so later guide edits do not rewrite history.
type Answer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

const (
	maxGuideQuestions = 30
	maxAnswerLen      = 3000
)

// Validate trims and checks a guide.
func (g *GuideInput) Validate() error {
	g.Name = clean(g.Name)
	if g.Name == "" || utf8.RuneCountInString(g.Name) > 80 {
		return invalid("a guide needs a name of at most 80 characters")
	}
	if g.Format != "" && !oneOf(g.Format, "phone", "video", "onsite") {
		return invalid("format must be phone, video or onsite")
	}
	g.Stage = clean(g.Stage)
	out := make([]GuideQuestion, 0, len(g.Questions))
	for _, q := range g.Questions {
		q.Text, q.Dimension = clean(q.Text), clean(q.Dimension)
		if q.Text == "" {
			continue
		}
		if utf8.RuneCountInString(q.Text) > 300 || utf8.RuneCountInString(q.Dimension) > 40 {
			return invalid("a guide question is too long")
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		return invalid("add at least one question")
	}
	if len(out) > maxGuideQuestions {
		return invalid("a guide holds at most %d questions", maxGuideQuestions)
	}
	g.Questions = out
	return nil
}

// CleanAnswers trims answers, drops blank ones and enforces limits.
func CleanAnswers(in []Answer) ([]Answer, error) {
	if len(in) > maxGuideQuestions {
		return nil, invalid("too many answers")
	}
	out := make([]Answer, 0, len(in))
	for _, a := range in {
		a.Question, a.Answer = clean(a.Question), clean(a.Answer)
		if a.Question == "" || a.Answer == "" {
			continue
		}
		if utf8.RuneCountInString(a.Answer) > maxAnswerLen || utf8.RuneCountInString(a.Question) > 300 {
			return nil, invalid("an answer is too long")
		}
		out = append(out, a)
	}
	return out, nil
}

// ListGuides returns the org's guides. Guides hold no personal data, so every signed-in member can read them.
func (r *Repository) ListGuides(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT id::text AS id, name, stage, format, questions FROM adrient.talent_guides WHERE org_id=$1 ORDER BY lower(name)`, a.OrgID)
}

// SaveGuide creates (id == "") or replaces a guide. HR only.
func (r *Repository) SaveGuide(ctx context.Context, a Actor, id string, in GuideInput) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	qb, _ := json.Marshal(in.Questions)
	q := `SELECT id::text AS id, name, stage, format, questions FROM adrient.talent_guides WHERE id=$1`
	if id == "" {
		var nid string
		if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_guides (org_id, name, stage, format, questions, created_by) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
			a.OrgID, in.Name, in.Stage, in.Format, qb, a.UserID).Scan(&nid); err != nil {
			return nil, err
		}
		r.audit(ctx, a, "guide.create", "talent_guide", nid)
		return r.row(ctx, q, nid)
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.talent_guides SET name=$3, stage=$4, format=$5, questions=$6, updated_at=now() WHERE org_id=$1 AND id=$2`,
		a.OrgID, id, in.Name, in.Stage, in.Format, qb)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	r.audit(ctx, a, "guide.update", "talent_guide", id)
	return r.row(ctx, q, id)
}

// DeleteGuide removes a guide. Answers already stored on scorecards keep their snapshot.
func (r *Repository) DeleteGuide(ctx context.Context, a Actor, id string) error {
	if !a.IsHR {
		return ErrForbidden
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM adrient.talent_guides WHERE org_id=$1 AND id=$2`, a.OrgID, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.audit(ctx, a, "guide.delete", "talent_guide", id)
	return nil
}

// guideBelongs checks that a guide exists in the actor's org.
func (r *Repository) guideBelongs(ctx context.Context, a Actor, id string) error {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.talent_guides WHERE org_id=$1 AND id=$2`, a.OrgID, id).Scan(&n); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if n == 0 {
		return invalid("unknown guide")
	}
	return nil
}

