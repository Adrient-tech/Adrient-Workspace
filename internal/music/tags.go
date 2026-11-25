package music

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Tags are the metadata read from an audio file's ID3v2 header.
type Tags struct {
	Title    string
	Artist   string
	Album    string
	Genre    string
	TrackNo  int
	Year     int
	Lyrics   string
	ArtMIME  string
	ArtBytes []byte
}

// ArtDataURL returns the embedded cover as a data URL, or "" when none.
func (t Tags) ArtDataURL() string {
	if len(t.ArtBytes) == 0 || len(t.ArtBytes) > 600_000 {
		return ""
	}
	mime := t.ArtMIME
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = "image/jpeg"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(t.ArtBytes)
}

// ParseID3v2 reads the ID3v2.3/2.4 tag at the start of data. It returns
// ok=false when data has no ID3v2 header. Unknown frames are skipped, and a
// truncated or corrupt tag yields whatever was read before the damage.
func ParseID3v2(data []byte) (Tags, bool) {
	var t Tags
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return t, false
	}
	ver := data[3]
	if ver != 3 && ver != 4 {
		return t, true // v2.2 and others: recognised, not parsed
	}
	flags := data[5]
	size := syncsafe(data[6:10])
	end := 10 + size
	if end > len(data) {
		end = len(data)
	}
	body := data[10:end]
	if flags&0x40 != 0 && len(body) >= 4 { // extended header
		var ext int
		if ver == 4 {
			ext = syncsafe(body[:4])
		} else {
			ext = int(binary.BigEndian.Uint32(body[:4])) + 4
		}
		if ext > len(body) {
			return t, true
		}
		body = body[ext:]
	}
	for len(body) >= 10 {
		id := string(body[:4])
		if body[0] == 0 {
			break // padding
		}
		var fs int
		if ver == 4 {
			fs = syncsafe(body[4:8])
		} else {
			fs = int(binary.BigEndian.Uint32(body[4:8]))
		}
		if fs < 0 || 10+fs > len(body) {
			break
		}
		payload := body[10 : 10+fs]
		body = body[10+fs:]
		switch id {
		case "TIT2":
			t.Title = textFrame(payload)
		case "TPE1":
			t.Artist = textFrame(payload)
		case "TALB":
			t.Album = textFrame(payload)
		case "TCON":
			t.Genre = cleanGenre(textFrame(payload))
		case "TRCK":
			t.TrackNo = leadingInt(textFrame(payload))
		case "TYER", "TDRC":
			if t.Year == 0 {
				t.Year = leadingInt(textFrame(payload))
			}
		case "USLT":
			t.Lyrics = lyricsFrame(payload)
		case "APIC":
			if mime, pic := pictureFrame(payload); len(pic) > 0 && len(t.ArtBytes) == 0 {
				t.ArtMIME, t.ArtBytes = mime, pic
			}
		}
	}
	return t, true
}

func syncsafe(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

func leadingInt(s string) int {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}

func cleanGenre(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "(") { // "(13)Pop" style
		if i := strings.Index(s, ")"); i > 0 {
			rest := strings.TrimSpace(s[i+1:])
			if rest != "" {
				return rest
			}
		}
		return ""
	}
	return s
}

// decodeText decodes bytes in an ID3 text encoding (0 latin1, 1 utf16 with BOM,
// 2 utf16be, 3 utf8) and trims NULs and spaces.
func decodeText(enc byte, b []byte) string {
	switch enc {
	case 1, 2:
		if len(b) >= 2 {
			be := enc == 2
			if b[0] == 0xFF && b[1] == 0xFE {
				b = b[2:]
			} else if b[0] == 0xFE && b[1] == 0xFF {
				b, be = b[2:], true
			}
			u := make([]uint16, 0, len(b)/2)
			for i := 0; i+1 < len(b); i += 2 {
				if be {
					u = append(u, binary.BigEndian.Uint16(b[i:]))
				} else {
					u = append(u, binary.LittleEndian.Uint16(b[i:]))
				}
			}
			return strings.Trim(string(utf16.Decode(u)), "\x00 ")
		}
		return ""
	case 0:
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		return strings.Trim(string(r), "\x00 ")
	default:
		return strings.Trim(string(b), "\x00 ")
	}
}

func textFrame(p []byte) string {
	if len(p) < 2 {
		return ""
	}
	s := decodeText(p[0], p[1:])
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return s
}

// terminator returns the length of the text terminator for enc.
func terminatorLen(enc byte) int {
	if enc == 1 || enc == 2 {
		return 2
	}
	return 1
}

// indexTerm finds the end of a terminated string starting at b[0].
func indexTerm(enc byte, b []byte) int {
	if terminatorLen(enc) == 1 {
		return bytes.IndexByte(b, 0)
	}
	for i := 0; i+1 < len(b); i += 2 {
		if b[i] == 0 && b[i+1] == 0 {
			return i
		}
	}
	return -1
}

func lyricsFrame(p []byte) string {
	if len(p) < 5 {
		return ""
	}
	enc := p[0]
	rest := p[4:] // skip language
	i := indexTerm(enc, rest)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(decodeText(enc, rest[i+terminatorLen(enc):]))
}

func pictureFrame(p []byte) (string, []byte) {
	if len(p) < 4 {
		return "", nil
	}
	enc := p[0]
	i := bytes.IndexByte(p[1:], 0)
	if i < 0 {
		return "", nil
	}
	mime := string(p[1 : 1+i])
	rest := p[1+i+1:]
	if len(rest) < 2 {
		return "", nil
	}
	rest = rest[1:] // picture type
	j := indexTerm(enc, rest)
	if j < 0 {
		return "", nil
	}
	return mime, rest[j+terminatorLen(enc):]
}

// ParseLRC reports whether text looks like a timed (.lrc) lyrics file: at least
// one "[mm:ss.xx]" stamp.
func IsLRC(text string) bool {
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(ln)
		if len(ln) > 5 && ln[0] == '[' && ln[1] >= '0' && ln[1] <= '9' && strings.Contains(ln, ":") && strings.Contains(ln, "]") {
			return true
		}
	}
	return false
}
