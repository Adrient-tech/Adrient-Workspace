package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned for a missing provider.
var ErrNotFound = errors.New("not found")

// UsageRow is one model call in the usage log. It never holds message content.
type UsageRow struct {
	OrgID        string
	UserID       string
	App          string
	ProviderID   string
	Model        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	Status       string
}

// UsageAgg is a usage summary row for the admin page.
type UsageAgg struct {
	Key          string  `json:"key"` // app, model or user id
	Label        string  `json:"label,omitempty"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// Store persists AI settings, providers and usage.
type Store interface {
	GetSettings(ctx context.Context, orgID string) (Settings, error)
	SaveSettings(ctx context.Context, orgID, actorID string, s Settings) error
	ListProviders(ctx context.Context, orgID string) ([]Provider, error)
	GetProvider(ctx context.Context, orgID, id string) (Provider, error)
	// PutProvider inserts or updates. sealed nil keeps the stored key; empty slice clears it.
	PutProvider(ctx context.Context, orgID string, p Provider, sealed []byte) error
	DeleteProvider(ctx context.Context, orgID, id string) error
	LogUsage(ctx context.Context, u UsageRow) error
	// TokensSince sums input+output tokens since t; userID "" sums the org.
	TokensSince(ctx context.Context, orgID, userID string, t time.Time) (int64, error)
	UsageSummary(ctx context.Context, orgID, by string, t time.Time) ([]UsageAgg, error)
}

// PGStore is the Postgres Store.
type PGStore struct{ Pool *pgxpool.Pool }

// NewPGStore constructs a Postgres store.
func NewPGStore(pool *pgxpool.Pool) *PGStore { return &PGStore{Pool: pool} }

func (s *PGStore) GetSettings(ctx context.Context, orgID string) (Settings, error) {
	var raw []byte
	err := s.Pool.QueryRow(ctx, `SELECT settings FROM adrient.ai_settings WHERE org_id=$1`, orgID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var st Settings
	err = json.Unmarshal(raw, &st)
	return st, err
}

func (s *PGStore) SaveSettings(ctx context.Context, orgID, actorID string, st Settings) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO adrient.ai_settings (org_id, settings, updated_by, updated_at)
		VALUES ($1,$2,NULLIF($3,'')::uuid,now())
		ON CONFLICT (org_id) DO UPDATE SET settings=EXCLUDED.settings, updated_by=EXCLUDED.updated_by, updated_at=now()`,
		orgID, raw, actorID)
	return err
}

type providerConfig struct {
	APIVersion string  `json:"api_version,omitempty"`
	Local      bool    `json:"local"`
	Enabled    bool    `json:"enabled"`
	PriceIn    float64 `json:"price_in,omitempty"`
	PriceOut   float64 `json:"price_out,omitempty"`
}

const providerCols = `id::text, name, kind, base_url, models, key_env, key_sealed, config`

func scanProvider(row pgx.Row) (Provider, error) {
	var p Provider
	var models, cfg []byte
	if err := row.Scan(&p.ID, &p.Name, &p.Kind, &p.BaseURL, &models, &p.KeyEnv, &p.sealedKey, &cfg); err != nil {
		return p, err
	}
	_ = json.Unmarshal(models, &p.Models)
	var c providerConfig
	_ = json.Unmarshal(cfg, &c)
	p.APIVersion, p.Local, p.Enabled, p.PriceIn, p.PriceOut = c.APIVersion, c.Local, c.Enabled, c.PriceIn, c.PriceOut
	return p, nil
}

func (s *PGStore) ListProviders(ctx context.Context, orgID string) ([]Provider, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+providerCols+` FROM adrient.ai_providers WHERE org_id=$1 ORDER BY created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PGStore) GetProvider(ctx context.Context, orgID, id string) (Provider, error) {
	p, err := scanProvider(s.Pool.QueryRow(ctx, `SELECT `+providerCols+` FROM adrient.ai_providers WHERE org_id=$1 AND id=$2::uuid`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *PGStore) PutProvider(ctx context.Context, orgID string, p Provider, sealed []byte) error {
	models, _ := json.Marshal(p.Models)
	cfg, _ := json.Marshal(providerConfig{APIVersion: p.APIVersion, Local: p.Local, Enabled: p.Enabled, PriceIn: p.PriceIn, PriceOut: p.PriceOut})
	keep := sealed == nil
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.ai_providers (id, org_id, name, kind, base_url, models, key_env, key_sealed, config)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,NULLIF($8::bytea,''::bytea),$9)
		ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, kind=EXCLUDED.kind, base_url=EXCLUDED.base_url,
		  models=EXCLUDED.models, key_env=EXCLUDED.key_env, config=EXCLUDED.config, updated_at=now(),
		  key_sealed=CASE WHEN $10 THEN adrient.ai_providers.key_sealed ELSE EXCLUDED.key_sealed END
		WHERE adrient.ai_providers.org_id=EXCLUDED.org_id`,
		p.ID, orgID, p.Name, p.Kind, p.BaseURL, models, p.KeyEnv, sealed, cfg, keep)
	return err
}

func (s *PGStore) DeleteProvider(ctx context.Context, orgID, id string) error {
	t, err := s.Pool.Exec(ctx, `DELETE FROM adrient.ai_providers WHERE org_id=$1 AND id=$2::uuid`, orgID, id)
	if err == nil && t.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *PGStore) LogUsage(ctx context.Context, u UsageRow) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.ai_usage (org_id, user_id, app, provider_id, model, input_tokens, output_tokens, cost_usd, status)
		VALUES ($1, NULLIF($2,'')::uuid, $3, NULLIF($4,'')::uuid, $5, $6, $7, $8, $9)`,
		u.OrgID, u.UserID, u.App, u.ProviderID, u.Model, u.InputTokens, u.OutputTokens, u.CostUSD, u.Status)
	return err
}

func (s *PGStore) TokensSince(ctx context.Context, orgID, userID string, t time.Time) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(input_tokens+output_tokens),0) FROM adrient.ai_usage
		WHERE org_id=$1 AND created_at >= $2 AND ($3='' OR user_id=NULLIF($3,'')::uuid)`, orgID, t, userID).Scan(&n)
	return n, err
}

func (s *PGStore) UsageSummary(ctx context.Context, orgID, by string, t time.Time) ([]UsageAgg, error) {
	key := "u.app"
	label := "''"
	join := ""
	switch by {
	case "model":
		key = "u.model"
	case "user":
		key = "COALESCE(u.user_id::text,'')"
		label = "COALESCE(MAX(us.display_name),MAX(us.email),'')"
		join = " LEFT JOIN adrient.users us ON us.id=u.user_id"
	}
	// key/label/join are fixed strings chosen above, never user input.
	rows, err := s.Pool.Query(ctx, `SELECT `+key+`, `+label+`, COUNT(*), COALESCE(SUM(u.input_tokens),0), COALESCE(SUM(u.output_tokens),0), COALESCE(SUM(u.cost_usd),0)::float8
		FROM adrient.ai_usage u`+join+` WHERE u.org_id=$1 AND u.created_at >= $2 GROUP BY 1 ORDER BY 3 DESC LIMIT 100`, orgID, t)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageAgg{}
	for rows.Next() {
		var a UsageAgg
		if err := rows.Scan(&a.Key, &a.Label, &a.Requests, &a.InputTokens, &a.OutputTokens, &a.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
