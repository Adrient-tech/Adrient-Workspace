// Package tts turns story text into speech on the server. Providers are
// pluggable: an OpenAI-compatible /v1/audio/speech endpoint (OpenAI itself, or a
// local server that speaks the same API) and a local command that reads text on
// stdin and writes WAV or MP3. The third option, reading aloud with the browser's
// own voice, needs no server code and is handled by the web app.
package tts

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Kinds of server-side provider.
const (
	KindOpenAI  = "openai"
	KindCommand = "command"
)

// Voice is one selectable voice.
type Voice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language,omitempty"`
}

// Request is one piece of text to speak.
type Request struct {
	Text     string
	Voice    string
	Language string
	Speed    float64 // 1 = normal; clamped to 0.5..2 by callers
	Pitch    float64 // 1 = normal; only command providers can use it
}

// Result is synthesized audio (WAV or MP3).
type Result struct {
	Audio []byte
	Mime  string
}

// Provider synthesizes speech.
type Provider interface {
	Kind() string
	Voices(ctx context.Context) ([]Voice, error)
	Synthesize(ctx context.Context, r Request) (Result, error)
}

// Config describes a provider; it is what an admin saves.
type Config struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"-"`
	Model   string `json:"model,omitempty"`
	Format  string `json:"format,omitempty"` // openai: wav or mp3
	// Command provider.
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	VoicesDir string   `json:"voices_dir,omitempty"`
	// Voices is a fixed list used when the engine cannot list its voices.
	Voices       []Voice       `json:"voices,omitempty"`
	DefaultVoice string        `json:"default_voice,omitempty"`
	Timeout      time.Duration `json:"-"`
	// MaxChars is the largest text sent in one request (default 3000).
	MaxChars int `json:"max_chars,omitempty"`
}

// ErrNotConfigured is returned for an unknown or unusable provider kind.
var ErrNotConfigured = errors.New("voice provider is not configured")

// New builds a provider from its configuration.
func New(c Config) (Provider, error) {
	if c.Timeout <= 0 {
		c.Timeout = 120 * time.Second
	}
	switch c.Kind {
	case KindOpenAI:
		return newOpenAI(c)
	case KindCommand:
		return newCommand(c)
	}
	return nil, ErrNotConfigured
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:+\-]{0,79}$`)

// ValidVoice reports whether a voice or language id is safe to hand to an engine
// (letters, digits and _ . : + - only, no leading dash, so it cannot become an option).
func ValidVoice(s string) bool { return idRe.MatchString(s) }

// Clamp keeps v inside [lo, hi], using def for zero or NaN.
func Clamp(v, lo, hi, def float64) float64 {
	if v != v || v == 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Synthesizer runs a whole episode: it chunks the sentences, calls the provider
// once per chunk, joins the audio and works out when each sentence is spoken.
type Synthesizer struct {
	Provider Provider
	// Voices maps a speaker name to a voice id; "" is the narrator.
	Voices   map[string]string
	Language string
	Speed    float64
	Pitch    float64
	MaxChars int
	// Progress is called after each chunk with (done, total).
	Progress func(done, total int)
}

// Timing is when a sentence is spoken, in milliseconds from the start.
type Timing struct {
	I     int `json:"i"`
	Start int `json:"s"`
	End   int `json:"e"`
}

// Output is a finished episode narration.
type Output struct {
	Audio      []byte
	Mime       string
	DurationMs int
	Timings    []Timing
}

// Run synthesizes the sentences.
func (s *Synthesizer) Run(ctx context.Context, sents []Sentence) (Output, error) {
	if len(sents) == 0 {
		return Output{}, errors.New("the story has no text to read")
	}
	max := s.MaxChars
	if max <= 0 {
		max = 3000
	}
	chunks := Chunks(sents, max)
	parts := make([][]byte, 0, len(chunks))
	durs := make([]int, 0, len(chunks))
	for i, c := range chunks {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		voice := s.Voices[c.Speaker]
		if voice == "" {
			voice = s.Voices[""]
		}
		res, err := s.Provider.Synthesize(ctx, Request{Text: c.Text, Voice: voice, Language: s.Language, Speed: s.Speed, Pitch: s.Pitch})
		if err != nil {
			return Output{}, fmt.Errorf("part %d of %d: %w", i+1, len(chunks), err)
		}
		d, err := Duration(res.Audio)
		if err != nil {
			return Output{}, fmt.Errorf("part %d of %d: %w", i+1, len(chunks), err)
		}
		parts = append(parts, res.Audio)
		durs = append(durs, d)
		if s.Progress != nil {
			s.Progress(i+1, len(chunks))
		}
	}
	audio, mime, err := Join(parts)
	if err != nil {
		return Output{}, err
	}
	// Sentence timings: each chunk's real duration is shared among its sentences by weight.
	timings := make([]Timing, len(sents))
	for i := range timings {
		timings[i].I = i
	}
	t := 0
	for ci, c := range chunks {
		if c.Count > 0 {
			var total float64
			for k := c.First; k < c.First+c.Count && k < len(sents); k++ {
				total += Weight(sents[k].Text)
			}
			at := float64(t)
			for k := c.First; k < c.First+c.Count && k < len(sents); k++ {
				share := float64(durs[ci]) * Weight(sents[k].Text) / total
				timings[k].Start = int(at)
				at += share
				timings[k].End = int(at)
			}
		} else if c.First < len(sents) {
			// continuation of a sentence that was cut into several requests
			timings[c.First].End = t + durs[ci]
		}
		t += durs[ci]
	}
	return Output{Audio: audio, Mime: mime, DurationMs: t, Timings: timings}, nil
}

// Clean trims and limits an error message shown to users.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
