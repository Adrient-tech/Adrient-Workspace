package tts

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// tone makes a 16-bit mono 8 kHz sine of the given length.
func tone(ms int) []byte {
	n := 8000 * ms / 1000
	pcm := make([]byte, n*2)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(8000*math.Sin(float64(i)/8))))
	}
	return MakeWAV(8000, pcm)
}

// fakeSpeech is an OpenAI-compatible speech server: one second of tone per 10 characters.
type fakeSpeech struct {
	mu   sync.Mutex
	reqs []map[string]any
	auth []string
	fail bool
}

func (f *fakeSpeech) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/audio/speech", func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		f.mu.Lock()
		f.reqs = append(f.reqs, m)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		fail := f.fail
		f.mu.Unlock()
		if fail {
			http.Error(w, "model overloaded", 503)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(tone(len(m["input"].(string)) * 100))
	})
	mux.HandleFunc("/v1/audio/voices", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"voices": []string{"af_bella", "hi_priya"}})
	})
	return mux
}

func TestOpenAICompatible(t *testing.T) {
	f := &fakeSpeech{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	p, err := New(Config{Kind: KindOpenAI, BaseURL: srv.URL + "/v1", APIKey: "k123", Model: "kokoro", Format: "wav"})
	if err != nil {
		t.Fatal(err)
	}
	vs, err := p.Voices(context.Background())
	if err != nil || len(vs) != 2 || vs[1].ID != "hi_priya" {
		t.Fatalf("voices %v %v", vs, err)
	}
	sents := Prepare("[Asha] Hello there. How are you?\n\nI am fine. Thanks!")
	s := &Synthesizer{Provider: p, Voices: map[string]string{"": "af_bella", "Asha": "hi_priya"}, Speed: 1.25, MaxChars: 200}
	out, err := s.Run(context.Background(), sents)
	if err != nil {
		t.Fatal(err)
	}
	if len(sents) != 4 || sents[0].Speaker != "Asha" || sents[3].Speaker != "Asha" {
		t.Fatalf("sentences %+v", sents)
	}
	if f.auth[0] != "Bearer k123" || f.reqs[0]["voice"] != "hi_priya" || f.reqs[0]["model"] != "kokoro" || f.reqs[0]["speed"].(float64) != 1.25 {
		t.Fatalf("request %v %v", f.auth, f.reqs[0])
	}
	if strings.Contains(f.reqs[0]["input"].(string), "[Asha]") {
		t.Fatal("speaker marker was spoken")
	}
	if out.Mime != MimeWAV || out.DurationMs < 3000 {
		t.Fatalf("output %s %d", out.Mime, out.DurationMs)
	}
	d, _ := Duration(out.Audio)
	if d < out.DurationMs-5 || d > out.DurationMs+5 {
		t.Fatalf("joined duration %d vs summed %d", d, out.DurationMs)
	}
	// timings are contiguous and cover the whole narration
	if out.Timings[0].Start != 0 || out.Timings[3].End < out.DurationMs-5 {
		t.Fatalf("timings %+v", out.Timings)
	}
	for i := 1; i < len(out.Timings); i++ {
		if out.Timings[i].Start < out.Timings[i-1].End-1 {
			t.Fatalf("overlap %+v", out.Timings)
		}
	}
}

func TestOpenAIErrors(t *testing.T) {
	f := &fakeSpeech{fail: true}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	p, _ := New(Config{Kind: KindOpenAI, BaseURL: srv.URL + "/v1"})
	_, err := (&Synthesizer{Provider: p}).Run(context.Background(), Prepare("One sentence."))
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("want 503 error, got %v", err)
	}
	if _, err := New(Config{Kind: KindOpenAI, BaseURL: "ftp://x"}); err == nil {
		t.Fatal("bad scheme accepted")
	}
	if _, err := p.Synthesize(context.Background(), Request{Text: "x", Voice: "--evil"}); err == nil {
		t.Fatal("bad voice accepted")
	}
}

// TestHelperProcess is the fake command provider: it reads stdin and writes a
// tone whose length depends on the text, to {out} when given, else to stdout.
func TestHelperProcess(t *testing.T) {
	args := os.Args
	for i, a := range args {
		if a != "--" {
			continue
		}
		rest := args[i+1:]
		buf := make([]byte, 1<<16)
		n, _ := os.Stdin.Read(buf)
		wav := tone(n * 50)
		if os.Getenv("X") == "" && len(rest) >= 2 && rest[1] != "" {
			_ = os.WriteFile(rest[1], wav, 0o600)
		} else {
			_, _ = os.Stdout.Write(wav)
		}
		fmt.Fprint(os.Stderr, "")
		os.Exit(0)
	}
}

func TestCommandProvider(t *testing.T) {
	p, err := New(Config{Kind: KindCommand, Command: os.Args[0], Args: []string{"-test.run=^TestHelperProcess$", "--", "{voice}", "{out}"},
		Voices: []Voice{{ID: "en-us", Name: "English"}}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Synthesize(context.Background(), Request{Text: "Hello from the command provider.", Voice: "en-us"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Duration(res.Audio)
	if err != nil || d < 1000 {
		t.Fatalf("duration %d %v", d, err)
	}
	if _, err := p.Synthesize(context.Background(), Request{Text: "x", Voice: "-rf"}); err == nil {
		t.Fatal("option-looking voice accepted")
	}
	if _, err := New(Config{Kind: KindCommand, Command: "definitely-not-installed-tts"}); err == nil {
		t.Fatal("missing binary accepted")
	}
	// a program that prints text instead of audio is reported, not stored
	q, _ := New(Config{Kind: KindCommand, Command: "echo"})
	if _, err := q.Synthesize(context.Background(), Request{Text: "x"}); err == nil || !strings.Contains(err.Error(), "did not produce") {
		t.Fatalf("got %v", err)
	}
}

func TestSentencesAndChunks(t *testing.T) {
	got := SplitSentences("Dr. Rao paid 3.5 lakh. Is it fine? Yes! नमस्ते। வணக்கம்.")
	want := []string{"Dr. Rao paid 3.5 lakh.", "Is it fine?", "Yes!", "नमस्ते।", "வணக்கம்."}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %q", got)
	}
	s := Prepare(HTMLToText("<p>[Maya] Hi.</p><p>Second line. Third.</p><p>[Narrator]Back.</p>"))
	if len(HTMLToText("<p>a</p><p>b &amp; c</p>")) == 0 || HTMLToText("<p>a</p><p>b &amp; c</p>") != "a\nb & c" {
		t.Fatalf("html: %q", HTMLToText("<p>a</p><p>b &amp; c</p>"))
	}
	if len(s) != 4 || s[0].Speaker != "Maya" || s[2].Speaker != "Maya" || s[3].Speaker != "" {
		t.Fatalf("%+v", s)
	}
	long := Prepare(strings.Repeat("word ", 300) + ".")
	ch := Chunks(long, 200)
	if len(ch) < 7 || ch[0].Count != 1 || ch[1].Count != 0 {
		t.Fatalf("chunks %d %+v", len(ch), ch[:2])
	}
	for _, c := range ch {
		if len([]rune(c.Text)) > 200 {
			t.Fatal("chunk over limit")
		}
	}
}

func TestJoinAndMP3(t *testing.T) {
	a, b := tone(500), tone(250)
	j, mime, err := Join([][]byte{a, b})
	if err != nil || mime != MimeWAV {
		t.Fatal(err)
	}
	if d, _ := Duration(j); d < 745 || d > 755 {
		t.Fatalf("joined %d", d)
	}
	if _, _, err := Join([][]byte{a, []byte("ID3xxxxxxxxxxxx")}); err == nil {
		t.Fatal("mixed formats accepted")
	}
	// 10 MPEG-1 Layer III frames at 128 kbit/s, 44.1 kHz: 417 bytes each, 1152 samples
	frame := make([]byte, 417)
	frame[0], frame[1], frame[2], frame[3] = 0xFF, 0xFB, 0x90, 0x00
	var mp3 []byte
	for i := 0; i < 10; i++ {
		mp3 = append(mp3, frame...)
	}
	d, err := Duration(mp3)
	if err != nil || d < 260 || d > 263 {
		t.Fatalf("mp3 duration %d %v", d, err)
	}
	if mj, mm, err := Join([][]byte{mp3, mp3}); err != nil || mm != MimeMP3 || len(mj) != len(mp3)*2 {
		t.Fatal("mp3 join")
	}
}

func TestDetectDoesNotPanic(t *testing.T) {
	for _, p := range Detect() {
		if p.ID == "" || p.Command == "" {
			t.Fatal("bad preset")
		}
	}
}
