package cloudimport

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoSecretKey means secrets cannot be stored because the server has no key.
var ErrNoSecretKey = errors.New("server secret key is not configured (set ADRIENT_SECRETS_KEY)")

// sealer encrypts connection secrets with AES-256-GCM bound to the row id.
type sealer struct{ key string }

func (s sealer) aead() (cipher.AEAD, error) {
	if s.key == "" {
		return nil, ErrNoSecretKey
	}
	k := sha256.Sum256([]byte("adrient-import-v1|" + s.key))
	b, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func (s sealer) seal(v any, aad string) ([]byte, error) {
	g, err := s.aead()
	if err != nil {
		return nil, err
	}
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	n := make([]byte, g.NonceSize())
	if _, err := rand.Read(n); err != nil {
		return nil, err
	}
	return g.Seal(n, n, plain, []byte(aad)), nil
}

func (s sealer) open(blob []byte, aad string, out any) error {
	if len(blob) == 0 {
		return nil
	}
	g, err := s.aead()
	if err != nil {
		return err
	}
	if len(blob) < g.NonceSize() {
		return errors.New("sealed value too short")
	}
	plain, err := g.Open(nil, blob[:g.NonceSize()], blob[g.NonceSize():], []byte(aad))
	if err != nil {
		return errors.New("cannot decrypt stored credentials (server secret key changed?)")
	}
	return json.Unmarshal(plain, out)
}

// ProviderSettings is the admin-entered OAuth app for one provider.
type ProviderSettings struct {
	Provider     string            `json:"provider"`
	ClientID     string            `json:"client_id"`
	HasSecret    bool              `json:"has_secret"`
	Extra        map[string]string `json:"extra"`
	UpdatedAt    time.Time         `json:"updated_at"`
	clientSecret providerSecret
}

// providerSecret is the sealed part of ProviderSettings.
type providerSecret struct {
	ClientSecret       string `json:"client_secret,omitempty"`
	ServiceAccountJSON string `json:"service_account_json,omitempty"`
}

// Connection is one connected source account.
type Connection struct {
	ID        string            `json:"id"`
	OrgID     string            `json:"-"`
	CreatedBy string            `json:"created_by"`
	Provider  string            `json:"provider"`
	Label     string            `json:"label"`
	Scope     string            `json:"scope"`
	Config    map[string]string `json:"config"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	secret    connSecret
}

// connSecret is the sealed part of a Connection.
type connSecret struct {
	Password     string    `json:"password,omitempty"`
	IMAPPassword string    `json:"imap_password,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	OAuthState   string    `json:"oauth_state,omitempty"` // sha256 of the pending state
}

// Run is one execution.
type Run struct {
	ID           string              `json:"id"`
	ConnectionID string              `json:"connection_id"`
	StartedBy    string              `json:"started_by"`
	Kinds        []string            `json:"kinds"`
	DryRun       bool                `json:"dry_run"`
	Status       string              `json:"status"`
	Counters     map[string]*Counter `json:"counters"`
	Message      string              `json:"message"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	FinishedAt   *time.Time          `json:"finished_at,omitempty"`
	ErrorCount   int                 `json:"error_count"`
}

// UserMapRow is one row of the mapping table.
type UserMapRow struct {
	RemoteID    string `json:"remote_id"`
	RemoteEmail string `json:"remote_email"`
	RemoteName  string `json:"remote_name"`
	LocalUserID string `json:"local_user_id"`
	LocalEmail  string `json:"local_email"`
	Skip        bool   `json:"skip"`
}

// ImportError is one row of the error report.
type ImportError struct {
	Kind     string    `json:"kind"`
	RemoteID string    `json:"remote_id"`
	Label    string    `json:"label"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
}

// WizardStore persists the migration wizard's state.
type WizardStore struct {
	pool *pgxpool.Pool
	seal sealer
}

// NewWizardStore constructs the store. secretKey seals credentials at rest.
func NewWizardStore(pool *pgxpool.Pool, secretKey string) *WizardStore {
	return &WizardStore{pool: pool, seal: sealer{secretKey}}
}

// ---- provider settings -----------------------------------------------------

func (s *WizardStore) GetSettings(ctx context.Context, orgID, provider string) (ProviderSettings, bool, error) {
	var ps ProviderSettings
	var extra []byte
	var sec []byte
	err := s.pool.QueryRow(ctx, `SELECT provider, client_id, client_secret_enc, extra, updated_at
		FROM adrient.import_provider_settings WHERE org_id=$1 AND provider=$2`, orgID, provider).
		Scan(&ps.Provider, &ps.ClientID, &sec, &extra, &ps.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderSettings{Provider: provider, Extra: map[string]string{}}, false, nil
	}
	if err != nil {
		return ps, false, err
	}
	_ = json.Unmarshal(extra, &ps.Extra)
	if ps.Extra == nil {
		ps.Extra = map[string]string{}
	}
	if err := s.seal.open(sec, orgID+"|"+provider, &ps.clientSecret); err != nil {
		return ps, true, err
	}
	ps.HasSecret = ps.clientSecret.ClientSecret != "" || ps.clientSecret.ServiceAccountJSON != ""
	return ps, true, nil
}

// SaveSettings stores settings. Empty secret fields keep the stored value, so
// the UI never needs to read a secret back.
func (s *WizardStore) SaveSettings(ctx context.Context, orgID, provider, clientID, clientSecret, serviceAccountJSON string, extra map[string]string) error {
	cur, _, err := s.GetSettings(ctx, orgID, provider)
	if err != nil && !errors.Is(err, ErrNoSecretKey) {
		return err
	}
	sec := cur.clientSecret
	if clientSecret != "" {
		sec.ClientSecret = clientSecret
	}
	if serviceAccountJSON != "" {
		sec.ServiceAccountJSON = serviceAccountJSON
	}
	var blob []byte
	if sec.ClientSecret != "" || sec.ServiceAccountJSON != "" {
		if blob, err = s.seal.seal(sec, orgID+"|"+provider); err != nil {
			return err
		}
	}
	ex, _ := json.Marshal(extra)
	_, err = s.pool.Exec(ctx, `INSERT INTO adrient.import_provider_settings (org_id, provider, client_id, client_secret_enc, extra, updated_at)
		VALUES ($1,$2,$3,$4,$5,now())
		ON CONFLICT (org_id, provider) DO UPDATE SET client_id=EXCLUDED.client_id, client_secret_enc=EXCLUDED.client_secret_enc, extra=EXCLUDED.extra, updated_at=now()`,
		orgID, provider, clientID, blob, ex)
	return err
}

// ---- connections -----------------------------------------------------------

const connCols = `id::text, org_id::text, created_by::text, provider, label, scope, config, secret_enc, status, created_at`

func (s *WizardStore) scanConn(row pgx.Row) (Connection, error) {
	var c Connection
	var cfg, sec []byte
	if err := row.Scan(&c.ID, &c.OrgID, &c.CreatedBy, &c.Provider, &c.Label, &c.Scope, &cfg, &sec, &c.Status, &c.CreatedAt); err != nil {
		return c, err
	}
	_ = json.Unmarshal(cfg, &c.Config)
	if c.Config == nil {
		c.Config = map[string]string{}
	}
	if err := s.seal.open(sec, c.ID, &c.secret); err != nil {
		c.Status = "error"
	}
	return c, nil
}

func (s *WizardStore) CreateConnection(ctx context.Context, c Connection) (Connection, error) {
	cfg, _ := json.Marshal(c.Config)
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO adrient.import_connections (org_id, created_by, provider, label, scope, config, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, c.OrgID, c.CreatedBy, c.Provider, c.Label, c.Scope, cfg, c.Status).Scan(&id); err != nil {
		return c, fmt.Errorf("create connection: %w", err)
	}
	c.ID = id
	if err := s.SaveSecret(ctx, c.OrgID, id, c.secret); err != nil {
		_, _ = s.pool.Exec(ctx, `DELETE FROM adrient.import_connections WHERE id=$1`, id)
		return c, err
	}
	return s.GetConnection(ctx, c.OrgID, id)
}

func (s *WizardStore) SaveSecret(ctx context.Context, orgID, id string, sec connSecret) error {
	var blob []byte
	if sec != (connSecret{}) {
		var err error
		if blob, err = s.seal.seal(sec, id); err != nil {
			return err
		}
	}
	_, err := s.pool.Exec(ctx, `UPDATE adrient.import_connections SET secret_enc=$3, updated_at=now() WHERE org_id=$1 AND id=$2`, orgID, id, blob)
	return err
}

func (s *WizardStore) SetConnStatus(ctx context.Context, orgID, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE adrient.import_connections SET status=$3, updated_at=now() WHERE org_id=$1 AND id=$2`, orgID, id, status)
	return err
}

func (s *WizardStore) GetConnection(ctx context.Context, orgID, id string) (Connection, error) {
	c, err := s.scanConn(s.pool.QueryRow(ctx, `SELECT `+connCols+` FROM adrient.import_connections WHERE org_id=$1 AND id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// ListConnections returns connections the user created, plus all org-scope
// connections when admin is true.
func (s *WizardStore) ListConnections(ctx context.Context, orgID, userID string, admin bool) ([]Connection, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+connCols+` FROM adrient.import_connections
		WHERE org_id=$1 AND (created_by=$2 OR ($3 AND scope='org')) ORDER BY created_at DESC`, orgID, userID, admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		c, err := s.scanConn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteConnection removes a connection, its runs, mappings and stored
// credentials. Data already imported into Adrient is kept.
func (s *WizardStore) DeleteConnection(ctx context.Context, orgID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM adrient.import_connections WHERE org_id=$1 AND id=$2`, orgID, id)
	return err
}

// ---- runs ------------------------------------------------------------------

const runCols = `r.id::text, r.connection_id::text, r.started_by::text, r.kinds, r.dry_run, r.status, r.counters, r.message, r.created_at, r.updated_at, r.finished_at,
	(SELECT count(*) FROM adrient.import_errors e WHERE e.run_id=r.id)`

func scanRun(row pgx.Row) (Run, error) {
	var r Run
	var ctr []byte
	err := row.Scan(&r.ID, &r.ConnectionID, &r.StartedBy, &r.Kinds, &r.DryRun, &r.Status, &ctr, &r.Message, &r.CreatedAt, &r.UpdatedAt, &r.FinishedAt, &r.ErrorCount)
	if err == nil {
		_ = json.Unmarshal(ctr, &r.Counters)
	}
	return r, err
}

func (s *WizardStore) CreateRun(ctx context.Context, orgID, connID, userID string, kinds []string, dry bool) (Run, error) {
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO adrient.import_runs (org_id, connection_id, started_by, kinds, dry_run, status)
		VALUES ($1,$2,$3,$4,$5,'queued') RETURNING id::text`, orgID, connID, userID, kinds, dry).Scan(&id); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, orgID, id)
}

func (s *WizardStore) GetRun(ctx context.Context, orgID, id string) (Run, error) {
	r, err := scanRun(s.pool.QueryRow(ctx, `SELECT `+runCols+` FROM adrient.import_runs r WHERE r.org_id=$1 AND r.id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *WizardStore) ListRuns(ctx context.Context, orgID, connID string) ([]Run, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+runCols+` FROM adrient.import_runs r WHERE r.org_id=$1 AND r.connection_id=$2 ORDER BY r.created_at DESC LIMIT 50`, orgID, connID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ActiveRun returns a queued/running run on the connection, if any.
func (s *WizardStore) ActiveRun(ctx context.Context, orgID, connID string) (string, bool, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id::text FROM adrient.import_runs WHERE org_id=$1 AND connection_id=$2 AND status IN ('queued','running') LIMIT 1`, orgID, connID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

func (s *WizardStore) SetRunStatus(ctx context.Context, id, status, msg string) error {
	fin := status == "done" || status == "failed" || status == "cancelled" || status == "interrupted"
	_, err := s.pool.Exec(ctx, `UPDATE adrient.import_runs SET status=$2, message=$3, updated_at=now(),
		finished_at = CASE WHEN $4 THEN now() ELSE finished_at END WHERE id=$1`, id, status, msg, fin)
	return err
}

// MarkInterrupted flags runs left "running" by a previous process (crash or
// restart). They can be resumed: a new run skips everything already imported.
func (s *WizardStore) MarkInterrupted(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE adrient.import_runs SET status='interrupted', message='server restarted during the run; start it again to resume where it stopped', finished_at=now(), updated_at=now()
		WHERE status IN ('queued','running')`)
	return tag.RowsAffected(), err
}

// ---- State implementation (used by the Engine) ------------------------------

func (s *WizardStore) GetMap(ctx context.Context, connID, kind, remoteID string) (string, string, bool, error) {
	var l, fp string
	err := s.pool.QueryRow(ctx, `SELECT local_id, fingerprint FROM adrient.import_map WHERE connection_id=$1 AND kind=$2 AND remote_id=$3`, connID, kind, remoteID).Scan(&l, &fp)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	return l, fp, err == nil, err
}

func (s *WizardStore) PutMap(ctx context.Context, connID, kind, remoteID, local, fp string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO adrient.import_map (connection_id, kind, remote_id, local_id, fingerprint) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (connection_id, kind, remote_id) DO UPDATE SET local_id=EXCLUDED.local_id, fingerprint=EXCLUDED.fingerprint`, connID, kind, remoteID, local, fp)
	return err
}

func (s *WizardStore) UserMapping(ctx context.Context, connID, remoteID string) (string, bool, bool, error) {
	var id *string
	var skip bool
	err := s.pool.QueryRow(ctx, `SELECT local_user_id::text, skip FROM adrient.import_user_map WHERE connection_id=$1 AND remote_id=$2`, connID, remoteID).Scan(&id, &skip)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, err
	}
	if id == nil {
		return "", skip, true, nil
	}
	return *id, skip, true, nil
}

func (s *WizardStore) SaveRemoteUser(ctx context.Context, connID string, u RemoteUser, localID string) error {
	var lid any
	if localID != "" {
		lid = localID
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO adrient.import_user_map (connection_id, remote_id, remote_email, remote_name, local_user_id)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (connection_id, remote_id) DO UPDATE SET remote_email=EXCLUDED.remote_email, remote_name=EXCLUDED.remote_name,
		  local_user_id=COALESCE(adrient.import_user_map.local_user_id, EXCLUDED.local_user_id)`, connID, u.ID, u.Email, u.Name, lid)
	return err
}

func (s *WizardStore) SaveProgress(ctx context.Context, runID string, c map[string]*Counter) error {
	b, _ := json.Marshal(c)
	_, err := s.pool.Exec(ctx, `UPDATE adrient.import_runs SET counters=$2, updated_at=now() WHERE id=$1`, runID, b)
	return err
}

func (s *WizardStore) AddError(ctx context.Context, runID, kind, remoteID, label, msg string) error {
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO adrient.import_errors (run_id, kind, remote_id, label, message) VALUES ($1,$2,$3,$4,$5)`, runID, kind, remoteID, label, msg)
	return err
}

// ---- mapping UI ------------------------------------------------------------

func (s *WizardStore) ListUserMap(ctx context.Context, orgID, connID string) ([]UserMapRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT m.remote_id, m.remote_email, m.remote_name, COALESCE(m.local_user_id::text,''), COALESCE(u.email,''), m.skip
		FROM adrient.import_user_map m
		JOIN adrient.import_connections c ON c.id=m.connection_id AND c.org_id=$1
		LEFT JOIN adrient.users u ON u.id=m.local_user_id
		WHERE m.connection_id=$2 ORDER BY m.remote_email`, orgID, connID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserMapRow
	for rows.Next() {
		var r UserMapRow
		if err := rows.Scan(&r.RemoteID, &r.RemoteEmail, &r.RemoteName, &r.LocalUserID, &r.LocalEmail, &r.Skip); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetUserMap changes one mapping. localUserID must belong to the org.
func (s *WizardStore) SetUserMap(ctx context.Context, orgID, connID, remoteID, localUserID string, skip bool) error {
	var lid any
	if localUserID != "" {
		var ok bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id::text=$1 AND org_id=$2)`, localUserID, orgID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return errors.New("that user is not in this organisation")
		}
		lid = localUserID
	}
	tag, err := s.pool.Exec(ctx, `UPDATE adrient.import_user_map m SET local_user_id=$4, skip=$5
		FROM adrient.import_connections c WHERE c.id=m.connection_id AND c.org_id=$1 AND m.connection_id=$2 AND m.remote_id=$3`, orgID, connID, remoteID, lid, skip)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *WizardStore) ListErrors(ctx context.Context, orgID, runID string, limit int) ([]ImportError, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.kind, e.remote_id, e.label, e.message, e.at FROM adrient.import_errors e
		JOIN adrient.import_runs r ON r.id=e.run_id AND r.org_id=$1 WHERE e.run_id=$2 ORDER BY e.id LIMIT $3`, orgID, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImportError
	for rows.Next() {
		var e ImportError
		if err := rows.Scan(&e.Kind, &e.RemoteID, &e.Label, &e.Message, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetConfig replaces a connection's non-secret config.
func (s *WizardStore) SetConfig(ctx context.Context, orgID, id string, cfg map[string]string) error {
	b, _ := json.Marshal(cfg)
	_, err := s.pool.Exec(ctx, `UPDATE adrient.import_connections SET config=$3, updated_at=now() WHERE org_id=$1 AND id=$2`, orgID, id, b)
	return err
}
