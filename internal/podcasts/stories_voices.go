package podcasts

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Adrient-tech/Adrient-Workspace/internal/tts"
)

// ProviderRow is a configured voice provider as stored.
type ProviderRow struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Kind         string      `json:"kind"`
	BaseURL      string      `json:"base_url"`
	KeyEnv       string      `json:"key_env"`
	KeySource    string      `json:"key_source"` // "", "stored" or "env"
	Model        string      `json:"model"`
	Format       string      `json:"format"`
	Command      string      `json:"command"`
	CommandFound bool        `json:"command_found"`
	Args         []string    `json:"args"`
	VoicesDir    string      `json:"voices_dir"`
	Voices       []tts.Voice `json:"voices"`
	DefaultVoice string      `json:"default_voice"`
	Enabled      bool        `json:"enabled"`
	IsDefault    bool        `json:"is_default"`
	UpdatedAt    time.Time   `json:"updated_at"`

	sealed []byte
}

// ProviderInput is what an admin saves. APIKey is write-only ("" keeps the stored key).
type ProviderInput struct {
	Name         string      `json:"name"`
	Kind         string      `json:"kind"`
	BaseURL      string      `json:"base_url"`
	APIKey       string      `json:"api_key"`
	ClearKey     bool        `json:"clear_key"`
	KeyEnv       string      `json:"key_env"`
	Model        string      `json:"model"`
	Format       string      `json:"format"`
	Command      string      `json:"command"`
	Args         []string    `json:"args"`
	VoicesDir    string      `json:"voices_dir"`
	Voices       []tts.Voice `json:"voices"`
	DefaultVoice string      `json:"default_voice"`
	Enabled      *bool       `json:"enabled"`
	IsDefault    *bool       `json:"is_default"`
}

const provCols = `id::text, name, kind, base_url, key_sealed, key_env, model, format, command, args, voices_dir, voices, default_voice, enabled, is_default, updated_at`

func scanProvider(row pgx.Row) (ProviderRow, error) {
	var p ProviderRow
	var args, voices []byte
	err := row.Scan(&p.ID, &p.Name, &p.Kind, &p.BaseURL, &p.sealed, &p.KeyEnv, &p.Model, &p.Format, &p.Command, &args, &p.VoicesDir, &voices, &p.DefaultVoice, &p.Enabled, &p.IsDefault, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal(args, &p.Args)
	_ = json.Unmarshal(voices, &p.Voices)
	if p.Args == nil {
		p.Args = []string{}
	}
	if p.Voices == nil {
		p.Voices = []tts.Voice{}
	}
	switch {
	case len(p.sealed) > 0:
		p.KeySource = "stored"
	case p.KeyEnv != "" && os.Getenv(p.KeyEnv) != "":
		p.KeySource = "env"
	}
	if p.Kind == tts.KindCommand && p.Command != "" {
		_, err := exec.LookPath(p.Command)
		p.CommandFound = err == nil
	}
	return p, nil
}

func (s *Stories) sealKey(org, id, key string) ([]byte, error) {
	if s.Secret == "" {
		return nil, invalid("the server has no secret key (ADRIENT_SECRETS_KEY) to store an API key; name an environment variable instead")
	}
	g, err := voiceAEAD(s.Secret)
	if err != nil {
		return nil, err
	}
	n := make([]byte, g.NonceSize())
	if _, err := rand.Read(n); err != nil {
		return nil, err
	}
	return g.Seal(n, n, []byte(key), []byte("story-voice|"+org+"|"+id)), nil
}

func (s *Stories) openKey(org, id string, blob []byte) (string, error) {
	g, err := voiceAEAD(s.Secret)
	if err != nil {
		return "", err
	}
	if len(blob) < g.NonceSize() {
		return "", errors.New("stored key is corrupt")
	}
	out, err := g.Open(nil, blob[:g.NonceSize()], blob[g.NonceSize():], []byte("story-voice|"+org+"|"+id))
	if err != nil {
		return "", errors.New("cannot decrypt the stored key (the server secret key changed?)")
	}
	return string(out), nil
}

func voiceAEAD(secret string) (cipher.AEAD, error) {
	if secret == "" {
		return nil, errors.New("no server secret key")
	}
	k := sha256.Sum256([]byte("adrient-story-voice-v1|" + secret))
	b, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// ListProviders returns every provider of the org (admin view; keys are never returned).
func (s *Stories) ListProviders(ctx context.Context, org string) ([]ProviderRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+provCols+` FROM adrient.story_voice_providers WHERE org_id=$1::uuid ORDER BY created_at`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderRow{}
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Provider loads one provider.
func (s *Stories) Provider(ctx context.Context, org, id string) (ProviderRow, error) {
	if !isUUID(id) {
		return ProviderRow{}, ErrNotFound
	}
	p, err := scanProvider(s.Pool.QueryRow(ctx, `SELECT `+provCols+` FROM adrient.story_voice_providers WHERE id=$1::uuid AND org_id=$2::uuid`, id, org))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// DefaultProvider is the org's default enabled provider (ErrNotFound when the org uses the browser voice).
func (s *Stories) DefaultProvider(ctx context.Context, org string) (ProviderRow, error) {
	p, err := scanProvider(s.Pool.QueryRow(ctx, `SELECT `+provCols+` FROM adrient.story_voice_providers WHERE org_id=$1::uuid AND enabled
		ORDER BY is_default DESC, created_at LIMIT 1`, org))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Stories) checkProviderInput(in *ProviderInput) error {
	in.Name = cleanText(in.Name, 60)
	if in.Name == "" {
		return invalid("a name is required")
	}
	switch in.Kind {
	case tts.KindOpenAI:
		if _, err := tts.New(tts.Config{Kind: tts.KindOpenAI, BaseURL: in.BaseURL, Format: in.Format}); err != nil {
			return invalid("%s", err.Error())
		}
		if in.Format == "" {
			in.Format = "wav"
		}
	case tts.KindCommand:
		in.Command = strings.TrimSpace(in.Command)
		if in.Command == "" {
			return invalid("a command is required")
		}
		if len(in.Args) > 24 {
			return invalid("too many arguments")
		}
		for _, a := range in.Args {
			if len(a) > 300 || strings.ContainsRune(a, 0) {
				return invalid("an argument is too long")
			}
		}
	default:
		return invalid("kind must be openai or command")
	}
	if in.KeyEnv != "" && !envNameRe.MatchString(in.KeyEnv) {
		return invalid("environment variable names use capital letters, digits and underscores")
	}
	for _, v := range in.Voices {
		if !tts.ValidVoice(v.ID) {
			return invalid("voice id %q is not valid", v.ID)
		}
	}
	if in.DefaultVoice != "" && !tts.ValidVoice(in.DefaultVoice) {
		return invalid("default voice id is not valid")
	}
	return nil
}

// SaveProvider creates (id "") or updates a provider.
func (s *Stories) SaveProvider(ctx context.Context, org, id string, in ProviderInput) (ProviderRow, error) {
	if err := s.checkProviderInput(&in); err != nil {
		return ProviderRow{}, err
	}
	if in.Args == nil {
		in.Args = []string{}
	}
	if in.Voices == nil {
		in.Voices = []tts.Voice{}
	}
	enabled := in.Enabled == nil || *in.Enabled
	if id == "" {
		err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.story_voice_providers (org_id, name, kind, base_url, key_env, model, format, command, args, voices_dir, voices, default_voice, enabled)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text`,
			org, in.Name, in.Kind, in.BaseURL, in.KeyEnv, in.Model, in.Format, in.Command, in.Args, in.VoicesDir, in.Voices, in.DefaultVoice, enabled).Scan(&id)
		if err != nil {
			return ProviderRow{}, err
		}
	} else {
		if _, err := s.Provider(ctx, org, id); err != nil {
			return ProviderRow{}, err
		}
		_, err := s.Pool.Exec(ctx, `UPDATE adrient.story_voice_providers SET name=$3, kind=$4, base_url=$5, key_env=$6, model=$7, format=$8, command=$9, args=$10,
			voices_dir=$11, voices=$12, default_voice=$13, enabled=$14, updated_at=now() WHERE id=$1::uuid AND org_id=$2::uuid`,
			id, org, in.Name, in.Kind, in.BaseURL, in.KeyEnv, in.Model, in.Format, in.Command, in.Args, in.VoicesDir, in.Voices, in.DefaultVoice, enabled)
		if err != nil {
			return ProviderRow{}, err
		}
	}
	if in.ClearKey {
		if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_voice_providers SET key_sealed=NULL WHERE id=$1::uuid`, id); err != nil {
			return ProviderRow{}, err
		}
	}
	if in.APIKey != "" {
		sealed, err := s.sealKey(org, id, in.APIKey)
		if err != nil {
			return ProviderRow{}, err
		}
		if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_voice_providers SET key_sealed=$2 WHERE id=$1::uuid`, id, sealed); err != nil {
			return ProviderRow{}, err
		}
	}
	if in.IsDefault != nil && *in.IsDefault {
		if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_voice_providers SET is_default = (id=$1::uuid) WHERE org_id=$2::uuid`, id, org); err != nil {
			return ProviderRow{}, err
		}
	} else if in.IsDefault != nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_voice_providers SET is_default=false WHERE id=$1::uuid`, id)
	}
	return s.Provider(ctx, org, id)
}

// DeleteProvider removes a provider; shows that named it fall back to the default.
func (s *Stories) DeleteProvider(ctx context.Context, org, id string) error {
	if _, err := s.Provider(ctx, org, id); err != nil {
		return err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_shows SET voice_provider=NULL, voice='' WHERE org_id=$1::uuid AND voice_provider=$2::uuid`, org, id)
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET voice_provider=NULL, voice='' WHERE org_id=$1::uuid AND voice_provider=$2::uuid`, org, id)
	_, err := s.Pool.Exec(ctx, `DELETE FROM adrient.story_voice_providers WHERE id=$1::uuid AND org_id=$2::uuid`, id, org)
	return err
}

var envNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// build turns a stored provider into a working one.
func (s *Stories) build(org string, p ProviderRow) (tts.Provider, error) {
	cfg := tts.Config{Kind: p.Kind, BaseURL: p.BaseURL, Model: p.Model, Format: p.Format, Command: p.Command, Args: p.Args,
		VoicesDir: p.VoicesDir, Voices: p.Voices, DefaultVoice: p.DefaultVoice, Timeout: 3 * time.Minute}
	switch {
	case len(p.sealed) > 0:
		k, err := s.openKey(org, p.ID, p.sealed)
		if err != nil {
			return nil, err
		}
		cfg.APIKey = k
	case p.KeyEnv != "":
		cfg.APIKey = os.Getenv(p.KeyEnv)
	}
	return tts.New(cfg)
}

// VoiceList is the set of voices one provider offers.
type VoiceList struct {
	ProviderID string      `json:"provider_id"`
	Name       string      `json:"name"`
	Kind       string      `json:"kind"`
	Default    string      `json:"default_voice"`
	IsDefault  bool        `json:"is_default"`
	Voices     []tts.Voice `json:"voices"`
	Error      string      `json:"error,omitempty"`
}

var voiceCache sync.Map // providerID+updated -> cached

type cachedVoices struct {
	at time.Time
	v  []tts.Voice
}

// Voices lists the voices of every enabled provider (what the story editor offers).
func (s *Stories) Voices(ctx context.Context, org string) ([]VoiceList, error) {
	ps, err := s.ListProviders(ctx, org)
	if err != nil {
		return nil, err
	}
	out := []VoiceList{}
	for _, p := range ps {
		if !p.Enabled {
			continue
		}
		vl := VoiceList{ProviderID: p.ID, Name: p.Name, Kind: p.Kind, Default: p.DefaultVoice, IsDefault: p.IsDefault, Voices: []tts.Voice{}}
		ck := p.ID + "|" + p.UpdatedAt.String()
		if c, ok := voiceCache.Load(ck); ok && time.Since(c.(cachedVoices).at) < time.Minute {
			vl.Voices = c.(cachedVoices).v
		} else if prov, err := s.build(org, p); err != nil {
			vl.Error = err.Error()
		} else {
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			vs, err := prov.Voices(cctx)
			cancel()
			if err != nil {
				vl.Error = "could not list voices: " + err.Error()
			} else if vs != nil {
				vl.Voices = vs
				voiceCache.Store(ck, cachedVoices{at: time.Now(), v: vs})
			}
		}
		out = append(out, vl)
	}
	return out, nil
}

// TestResult is the answer to an admin's "test this voice" click.
type TestResult struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	DurationMs int    `json:"duration_ms"`
	Voices     int    `json:"voices"`
	LatencyMs  int    `json:"latency_ms"`
}

// TestProvider speaks one short sentence and lists the voices.
func (s *Stories) TestProvider(ctx context.Context, org, id, voice string) TestResult {
	p, err := s.Provider(ctx, org, id)
	if err != nil {
		return TestResult{Error: "provider not found"}
	}
	prov, err := s.build(org, p)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	t0 := time.Now()
	res, err := prov.Synthesize(cctx, tts.Request{Text: "This is a test of the story voice.", Voice: voice, Language: "en"})
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	d, err := tts.Duration(res.Audio)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	vs, _ := prov.Voices(cctx)
	return TestResult{OK: true, DurationMs: d, Voices: len(vs), LatencyMs: int(time.Since(t0).Milliseconds())}
}

// Settings are the org-wide limits.
type Settings struct {
	DailyChars    int  `json:"daily_chars"`
	PerMinuteJobs int  `json:"per_minute_jobs"`
	MaxChars      int  `json:"max_chars"`
	AutoGenerate  bool `json:"auto_generate"`
}

// GetSettings returns the org's limits (defaults when never saved).
func (s *Stories) GetSettings(ctx context.Context, org string) Settings {
	st := Settings{DailyChars: 200000, PerMinuteJobs: 6, MaxChars: 60000, AutoGenerate: true}
	_ = s.Pool.QueryRow(ctx, `SELECT daily_chars, per_minute_jobs, max_chars, auto_generate FROM adrient.story_settings WHERE org_id=$1::uuid`, org).
		Scan(&st.DailyChars, &st.PerMinuteJobs, &st.MaxChars, &st.AutoGenerate)
	return st
}

// SaveSettings validates and stores the limits.
func (s *Stories) SaveSettings(ctx context.Context, org string, st Settings) (Settings, error) {
	if st.DailyChars < 0 || st.DailyChars > 50_000_000 || st.PerMinuteJobs < 1 || st.PerMinuteJobs > 120 || st.MaxChars < 500 || st.MaxChars > 500_000 {
		return st, invalid("limits are out of range")
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.story_settings (org_id, daily_chars, per_minute_jobs, max_chars, auto_generate) VALUES ($1::uuid,$2,$3,$4,$5)
		ON CONFLICT (org_id) DO UPDATE SET daily_chars=$2, per_minute_jobs=$3, max_chars=$4, auto_generate=$5, updated_at=now()`,
		org, st.DailyChars, st.PerMinuteJobs, st.MaxChars, st.AutoGenerate)
	return st, err
}
