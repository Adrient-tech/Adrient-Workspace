package tts

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
)

// Sentence is one spoken (and highlighted) unit of an episode.
type Sentence struct {
	Text    string `json:"text"`
	Speaker string `json:"speaker,omitempty"` // "" = narrator
	Para    int    `json:"para"`              // paragraph index, for read-along layout
}

var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"blockquote": true, "ul": true, "ol": true, "hr": true, "pre": true, "tr": true,
}

// HTMLToText turns editor HTML into plain text with one paragraph per line.
func HTMLToText(src string) string {
	if !strings.ContainsAny(src, "<&") {
		return normalizeText(src)
	}
	z := xhtml.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	skip := 0
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return normalizeText(b.String())
		case xhtml.TextToken:
			if skip == 0 {
				b.WriteString(string(z.Text()))
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, _ := z.TagName()
			n := string(name)
			if n == "script" || n == "style" {
				skip++
			}
			if blockTags[n] {
				b.WriteString("\n")
			}
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			n := string(name)
			if n == "script" || n == "style" {
				if skip > 0 {
					skip--
				}
			}
			if blockTags[n] {
				b.WriteString("\n")
			}
		}
	}
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(html.UnescapeString(s), "\r", "")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.Join(strings.Fields(l), " ")
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

var speakerRe = regexp.MustCompile(`^\[([^\[\]\n]{1,40})\]\s*`)

// Prepare splits plain text into sentences. A paragraph that starts with a
// [Name] marker switches the speaker for that paragraph and the ones after it;
// [Narrator] (or [/]) switches back. The marker itself is not spoken.
func Prepare(text string) []Sentence {
	var out []Sentence
	speaker := ""
	para := 0
	for _, line := range strings.Split(normalizeText(text), "\n") {
		if m := speakerRe.FindStringSubmatch(line); m != nil {
			name := strings.TrimSpace(m[1])
			if strings.EqualFold(name, "narrator") || name == "/" {
				speaker = ""
			} else {
				speaker = name
			}
			line = line[len(m[0]):]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		for _, s := range SplitSentences(line) {
			out = append(out, Sentence{Text: s, Speaker: speaker, Para: para})
		}
		para++
	}
	return out
}

// SplitSentences splits one paragraph after . ! ? and the Devanagari danda, but
// not after common abbreviations or inside numbers such as 3.5.
func SplitSentences(p string) []string {
	var out []string
	rs := []rune(strings.TrimSpace(p))
	start := 0
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r != '.' && r != '!' && r != '?' && r != '।' && r != '…' {
			continue
		}
		// swallow runs like "?!" or "..." and closing quotes
		j := i
		for j+1 < len(rs) && strings.ContainsRune(".!?।…\"'”’)", rs[j+1]) {
			j++
		}
		if j+1 < len(rs) && rs[j+1] != ' ' {
			i = j
			continue // 3.5, a.b, e.g.x
		}
		if r == '.' && abbreviation(rs[start:i]) {
			i = j
			continue
		}
		s := strings.TrimSpace(string(rs[start : j+1]))
		if s != "" {
			out = append(out, s)
		}
		start = j + 1
		i = j
	}
	if tail := strings.TrimSpace(string(rs[start:])); tail != "" {
		out = append(out, tail)
	}
	return out
}

var abbrevs = map[string]bool{"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true, "sr": true, "jr": true, "st": true, "vs": true, "etc": true, "inc": true, "ltd": true, "no": true}

func abbreviation(before []rune) bool {
	k := len(before)
	for k > 0 && before[k-1] != ' ' {
		k--
	}
	w := strings.ToLower(string(before[k:]))
	return abbrevs[w] || (len(w) == 1 && w[0] >= 'a' && w[0] <= 'z')
}

// Chunk is a run of sentences by one speaker, sent to the voice engine in one request.
type Chunk struct {
	Speaker string
	Text    string
	First   int // index of the first sentence
	Count   int
}

// Chunks groups sentences into requests of at most maxChars characters.
// A single sentence longer than maxChars is cut at word boundaries and stays
// one logical sentence (it becomes several chunks with Count 0 on the extras).
func Chunks(sents []Sentence, maxChars int) []Chunk {
	if maxChars < 200 {
		maxChars = 200
	}
	var out []Chunk
	var cur *Chunk
	flush := func() {
		if cur != nil && cur.Text != "" {
			out = append(out, *cur)
		}
		cur = nil
	}
	for i, s := range sents {
		if utf8.RuneCountInString(s.Text) > maxChars {
			flush()
			parts := cutWords(s.Text, maxChars)
			for k, p := range parts {
				c := Chunk{Speaker: s.Speaker, Text: p, First: i}
				if k == 0 {
					c.Count = 1
				}
				out = append(out, c)
			}
			continue
		}
		if cur != nil && (cur.Speaker != s.Speaker || utf8.RuneCountInString(cur.Text)+1+utf8.RuneCountInString(s.Text) > maxChars) {
			flush()
		}
		if cur == nil {
			cur = &Chunk{Speaker: s.Speaker, First: i}
		} else {
			cur.Text += " "
		}
		cur.Text += s.Text
		cur.Count++
	}
	flush()
	return out
}

func cutWords(s string, n int) []string {
	var out []string
	cur := ""
	for _, w := range strings.Fields(s) {
		if cur != "" && utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) > n {
			out = append(out, cur)
			cur = ""
		}
		if cur != "" {
			cur += " "
		}
		cur += w
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// Weight is the share of a chunk's duration a sentence is assumed to take:
// its length in characters plus a pause for the closing punctuation.
func Weight(s string) float64 {
	w := float64(utf8.RuneCountInString(s))
	switch {
	case strings.HasSuffix(s, "?"), strings.HasSuffix(s, "!"), strings.HasSuffix(s, "."), strings.HasSuffix(s, "।"):
		w += 6
	case strings.HasSuffix(s, ","), strings.HasSuffix(s, ";"), strings.HasSuffix(s, ":"):
		w += 3
	}
	return w
}

// EstimateMs is how long a text takes to read at a given speed (words per
// minute 160 at speed 1). Used when there is no audio yet.
func EstimateMs(sents []Sentence, speed float64) int {
	if speed <= 0 {
		speed = 1
	}
	words := 0
	for _, s := range sents {
		words += len(strings.Fields(s.Text))
	}
	return int(float64(words) / (160 * speed) * 60000)
}
