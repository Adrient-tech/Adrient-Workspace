package tts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// command runs a local text-to-speech program. The text goes to its standard
// input; the audio comes back on standard output, or in the file named by an
// {out} placeholder in the arguments. No shell is involved: arguments are passed
// as a list and the voice id is validated, so text can never become an option.
//
// Placeholders: {voice} {lang} {out} {voices_dir} {speed} {speed_wpm}
// {length_scale} {pitch} {pitch_99}.
type command struct {
	bin       string
	args      []string
	voicesDir string
	fixed     []Voice
	def       string
	timeout   time.Duration
}

// Preset is a ready-made command configuration found on this machine.
type Preset struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Path    string   `json:"path"`
	Args    []string `json:"args"`
	Found   bool     `json:"found"`
	Note    string   `json:"note,omitempty"`
}

// Detect looks for known engines with `which` (exec.LookPath).
func Detect() []Preset {
	list := []Preset{
		{ID: "espeak-ng", Name: "eSpeak NG", Command: "espeak-ng", Args: []string{"--stdout", "-v", "{voice}", "-s", "{speed_wpm}", "-p", "{pitch_99}"},
			Note: "Small and offline, many languages including English, Hindi (hi) and Tamil (ta). Robotic voice. Install: apt install espeak-ng"},
		{ID: "piper", Name: "Piper", Command: "piper", Args: []string{"--model", "{voices_dir}/{voice}.onnx", "--length_scale", "{length_scale}", "--output_file", "{out}"},
			Note: "Natural neural voices, offline. Download voice models (.onnx plus .onnx.json) into a folder and set it as the voices folder."},
	}
	for i := range list {
		if p, err := exec.LookPath(list[i].Command); err == nil {
			list[i].Found = true
			list[i].Path = p
		}
	}
	return list
}

func newCommand(c Config) (Provider, error) {
	if strings.TrimSpace(c.Command) == "" {
		return nil, errors.New("command is required")
	}
	bin, err := exec.LookPath(c.Command)
	if err != nil {
		return nil, fmt.Errorf("command %q was not found on this server", c.Command)
	}
	return &command{bin: bin, args: c.Args, voicesDir: c.VoicesDir, fixed: c.Voices, def: c.DefaultVoice, timeout: c.Timeout}, nil
}

func (c *command) Kind() string { return KindCommand }

// Voices: the configured list, the .onnx files of the voices folder, or the
// output of `espeak-ng --voices`.
func (c *command) Voices(ctx context.Context) ([]Voice, error) {
	if len(c.fixed) > 0 {
		return c.fixed, nil
	}
	if c.voicesDir != "" {
		files, _ := filepath.Glob(filepath.Join(c.voicesDir, "*.onnx"))
		sort.Strings(files)
		out := make([]Voice, 0, len(files))
		for _, f := range files {
			id := strings.TrimSuffix(filepath.Base(f), ".onnx")
			if !ValidVoice(id) {
				continue
			}
			lang := ""
			if i := strings.IndexAny(id, "_-"); i > 0 {
				lang = id[:i]
			}
			out = append(out, Voice{ID: id, Name: id, Language: lang})
		}
		return out, nil
	}
	if strings.HasPrefix(filepath.Base(c.bin), "espeak") {
		cmd := exec.CommandContext(ctx, c.bin, "--voices")
		cmd.Env = cleanEnv()
		raw, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		var out []Voice
		for i, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if i == 0 || len(f) < 4 || !ValidVoice(f[1]) {
				continue
			}
			out = append(out, Voice{ID: f[1], Name: strings.ReplaceAll(f[3], "_", " "), Language: f[1]})
		}
		return out, nil
	}
	if c.def != "" {
		return []Voice{{ID: c.def, Name: c.def}}, nil
	}
	return nil, nil
}

func cleanEnv() []string {
	return []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "HOME=" + os.TempDir()}
}

func (c *command) Synthesize(ctx context.Context, r Request) (Result, error) {
	voice := r.Voice
	if voice == "" {
		voice = c.def
	}
	if voice != "" && !ValidVoice(voice) {
		return Result{}, fmt.Errorf("voice %q is not a valid voice id", voice)
	}
	lang := r.Language
	if lang != "" && !ValidVoice(lang) {
		lang = ""
	}
	if voice == "" {
		voice = lang
	}
	speed := Clamp(r.Speed, 0.5, 2, 1)
	pitch := Clamp(r.Pitch, 0.5, 1.5, 1)
	out := ""
	var tmp string
	for _, a := range c.args {
		if strings.Contains(a, "{out}") {
			f, err := os.CreateTemp("", "story-tts-*.wav")
			if err != nil {
				return Result{}, err
			}
			tmp = f.Name()
			_ = f.Close()
			defer os.Remove(tmp)
			out = tmp
			break
		}
	}
	rep := strings.NewReplacer(
		"{voice}", voice, "{lang}", lang, "{out}", out, "{voices_dir}", c.voicesDir,
		"{speed}", strconv.FormatFloat(speed, 'f', 2, 64),
		"{speed_wpm}", strconv.Itoa(int(175*speed)),
		"{length_scale}", strconv.FormatFloat(1/speed, 'f', 2, 64),
		"{pitch}", strconv.FormatFloat(pitch, 'f', 2, 64),
		"{pitch_99}", strconv.Itoa(int(50*pitch)),
	)
	args := make([]string, len(c.args))
	for i, a := range c.args {
		args[i] = rep.Replace(a)
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Env = cleanEnv()
	cmd.Stdin = strings.NewReader(r.Text)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("%s failed: %v %s", filepath.Base(c.bin), err, clip(stderr.String(), 200))
	}
	raw := stdout.Bytes()
	if tmp != "" {
		b, err := os.ReadFile(tmp)
		if err != nil {
			return Result{}, err
		}
		raw = b
	}
	mime := Sniff(raw)
	if mime == "" {
		return Result{}, fmt.Errorf("%s did not produce WAV or MP3 audio", filepath.Base(c.bin))
	}
	return Result{Audio: raw, Mime: mime}, nil
}
