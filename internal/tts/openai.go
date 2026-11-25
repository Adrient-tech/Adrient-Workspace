package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// openAI talks to any server that implements POST {base}/audio/speech.
type openAI struct {
	base   string
	key    string
	model  string
	format string
	fixed  []Voice
	def    string
	client *http.Client
}

// builtinOpenAIVoices are the voices of the hosted OpenAI service.
var builtinOpenAIVoices = []string{"alloy", "ash", "ballad", "coral", "echo", "fable", "nova", "onyx", "sage", "shimmer"}

func newOpenAI(c Config) (Provider, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(c.BaseURL), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("base URL must be an http(s) address such as http://127.0.0.1:8880/v1")
	}
	f := strings.ToLower(c.Format)
	if f == "" {
		f = "wav"
	}
	if f != "wav" && f != "mp3" {
		return nil, errors.New("format must be wav or mp3")
	}
	model := c.Model
	if model == "" {
		model = "tts-1"
	}
	return &openAI{base: u.String(), key: c.APIKey, model: model, format: f, fixed: c.Voices, def: c.DefaultVoice,
		client: &http.Client{Timeout: c.Timeout}}, nil
}

func (o *openAI) Kind() string { return KindOpenAI }

func (o *openAI) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, o.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if o.key != "" {
		req.Header.Set("Authorization", "Bearer "+o.key)
	}
	return o.client.Do(req)
}

// Voices uses the configured list; failing that it asks the server
// (GET /audio/voices, offered by several local servers), then falls back to the
// hosted service's voice names.
func (o *openAI) Voices(ctx context.Context) ([]Voice, error) {
	if len(o.fixed) > 0 {
		return o.fixed, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if resp, err := o.do(cctx, http.MethodGet, "/audio/voices", nil); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if vs := parseVoiceList(raw); len(vs) > 0 {
				return vs, nil
			}
		}
	}
	if strings.Contains(o.base, "api.openai.com") {
		out := make([]Voice, 0, len(builtinOpenAIVoices))
		for _, v := range builtinOpenAIVoices {
			out = append(out, Voice{ID: v, Name: strings.ToUpper(v[:1]) + v[1:]})
		}
		return out, nil
	}
	if o.def != "" {
		return []Voice{{ID: o.def, Name: o.def}}, nil
	}
	return nil, nil
}

// parseVoiceList accepts {"voices":["a","b"]}, {"voices":[{"id":..,"name":..}]} or a bare array.
func parseVoiceList(raw []byte) []Voice {
	var wrapped struct {
		Voices json.RawMessage `json:"voices"`
		Data   json.RawMessage `json:"data"`
	}
	list := json.RawMessage(raw)
	if json.Unmarshal(raw, &wrapped) == nil {
		switch {
		case len(wrapped.Voices) > 0:
			list = wrapped.Voices
		case len(wrapped.Data) > 0:
			list = wrapped.Data
		}
	}
	var names []string
	if json.Unmarshal(list, &names) == nil {
		out := make([]Voice, 0, len(names))
		for _, n := range names {
			if ValidVoice(n) {
				out = append(out, Voice{ID: n, Name: n})
			}
		}
		return out
	}
	// field names vary between servers; decode loosely
	var loose []map[string]any
	if json.Unmarshal(list, &loose) == nil {
		out := make([]Voice, 0, len(loose))
		for _, m := range loose {
			id := firstString(m, "id", "voice_id", "name", "voice")
			if !ValidVoice(id) {
				continue
			}
			name := firstString(m, "name", "display_name", "id")
			out = append(out, Voice{ID: id, Name: name, Language: firstString(m, "language", "lang", "locale")})
		}
		return out
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func (o *openAI) Synthesize(ctx context.Context, r Request) (Result, error) {
	voice := r.Voice
	if voice == "" {
		voice = o.def
	}
	if voice == "" {
		voice = "alloy"
	}
	if !ValidVoice(voice) {
		return Result{}, fmt.Errorf("voice %q is not a valid voice id", voice)
	}
	body, _ := json.Marshal(map[string]any{
		"model": o.model, "input": r.Text, "voice": voice,
		"response_format": o.format, "speed": Clamp(r.Speed, 0.25, 4, 1),
	})
	resp, err := o.do(ctx, http.MethodPost, "/audio/speech", body)
	if err != nil {
		return Result{}, fmt.Errorf("voice server unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return Result{}, fmt.Errorf("reading audio: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("voice server answered %d: %s", resp.StatusCode, clip(string(raw), 200))
	}
	mime := Sniff(raw)
	if mime == "" {
		return Result{}, errors.New("voice server returned something that is not WAV or MP3 audio")
	}
	return Result{Audio: raw, Mime: mime}, nil
}
