package tts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// Audio formats the layer can join and measure.
const (
	MimeWAV = "audio/wav"
	MimeMP3 = "audio/mpeg"
)

// Sniff returns the mime type of audio bytes ("" when unknown).
func Sniff(b []byte) string {
	switch {
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return MimeWAV
	case len(b) >= 3 && string(b[0:3]) == "ID3":
		return MimeMP3
	case len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0:
		return MimeMP3
	}
	return ""
}

// wavInfo is the PCM format of a WAV file.
type wavInfo struct {
	format, channels, bits uint16
	rate                   uint32
}

// parseWAV returns the format and the PCM data of a RIFF/WAVE file. A data
// chunk whose declared size is 0 or larger than the file (streamed output)
// takes the rest of the file.
func parseWAV(b []byte) (wavInfo, []byte, error) {
	var wi wavInfo
	if Sniff(b) != MimeWAV {
		return wi, nil, errors.New("not a WAV file")
	}
	p := 12
	haveFmt := false
	for p+8 <= len(b) {
		id := string(b[p : p+4])
		size := int(binary.LittleEndian.Uint32(b[p+4 : p+8]))
		body := p + 8
		switch id {
		case "fmt ":
			if body+16 > len(b) {
				return wi, nil, errors.New("short fmt chunk")
			}
			wi.format = binary.LittleEndian.Uint16(b[body:])
			wi.channels = binary.LittleEndian.Uint16(b[body+2:])
			wi.rate = binary.LittleEndian.Uint32(b[body+4:])
			wi.bits = binary.LittleEndian.Uint16(b[body+14:])
			haveFmt = true
		case "data":
			if !haveFmt {
				return wi, nil, errors.New("data before fmt")
			}
			end := body + size
			if size == 0 || end > len(b) || size == 0xFFFFFFFF {
				end = len(b)
			}
			return wi, b[body:end], nil
		}
		p = body + size + size%2
	}
	return wi, nil, errors.New("no data chunk")
}

func buildWAV(wi wavInfo, pcm []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, wi.format)
	_ = binary.Write(&buf, binary.LittleEndian, wi.channels)
	_ = binary.Write(&buf, binary.LittleEndian, wi.rate)
	_ = binary.Write(&buf, binary.LittleEndian, wi.rate*uint32(wi.channels)*uint32(wi.bits)/8)
	_ = binary.Write(&buf, binary.LittleEndian, wi.channels*wi.bits/8)
	_ = binary.Write(&buf, binary.LittleEndian, wi.bits)
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

// MakeWAV builds a 16-bit mono PCM WAV file (used by tests and the fake server).
func MakeWAV(rate int, pcm []byte) []byte {
	return buildWAV(wavInfo{format: 1, channels: 1, rate: uint32(rate), bits: 16}, pcm)
}

// Duration returns the length of audio in milliseconds.
func Duration(b []byte) (int, error) {
	switch Sniff(b) {
	case MimeWAV:
		wi, pcm, err := parseWAV(b)
		if err != nil {
			return 0, err
		}
		bps := int(wi.rate) * int(wi.channels) * int(wi.bits) / 8
		if bps == 0 {
			return 0, errors.New("bad WAV format")
		}
		return len(pcm) * 1000 / bps, nil
	case MimeMP3:
		return mp3Duration(b)
	}
	return 0, errors.New("unknown audio format")
}

// Join concatenates audio parts of the same format into one file.
func Join(parts [][]byte) ([]byte, string, error) {
	if len(parts) == 0 {
		return nil, "", errors.New("no audio")
	}
	mime := Sniff(parts[0])
	if mime == "" {
		return nil, "", errors.New("the voice engine did not return WAV or MP3 audio")
	}
	for _, p := range parts[1:] {
		if Sniff(p) != mime {
			return nil, "", errors.New("the voice engine returned mixed audio formats")
		}
	}
	if len(parts) == 1 {
		return parts[0], mime, nil
	}
	if mime == MimeMP3 {
		var buf bytes.Buffer
		for i, p := range parts {
			if i > 0 {
				p = p[id3Len(p):]
			}
			buf.Write(p)
		}
		return buf.Bytes(), mime, nil
	}
	var wi wavInfo
	var pcm bytes.Buffer
	for i, p := range parts {
		w, d, err := parseWAV(p)
		if err != nil {
			return nil, "", fmt.Errorf("part %d: %w", i+1, err)
		}
		if i == 0 {
			wi = w
		} else if w != wi {
			return nil, "", errors.New("the voice engine returned WAV parts with different formats")
		}
		pcm.Write(d)
	}
	return buildWAV(wi, pcm.Bytes()), mime, nil
}

func id3Len(b []byte) int {
	if len(b) >= 10 && string(b[0:3]) == "ID3" {
		n := int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f)
		if 10+n <= len(b) {
			return 10 + n
		}
	}
	return 0
}

var (
	mpegBitrates = map[[2]int][15]int{ // {version(1|2), layer} -> kbps by index
		{1, 1}: {0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448},
		{1, 2}: {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
		{1, 3}: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
		{2, 1}: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},
		{2, 2}: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
		{2, 3}: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
	}
	mpegRates = map[int][3]int{3: {44100, 48000, 32000}, 2: {22050, 24000, 16000}, 0: {11025, 12000, 8000}}
)

// mp3Duration walks the MPEG audio frames and sums their lengths.
func mp3Duration(b []byte) (int, error) {
	p := id3Len(b)
	var samples, rate float64
	frames := 0
	for p+4 <= len(b) {
		if b[p] != 0xFF || b[p+1]&0xE0 != 0xE0 {
			p++ // resync
			continue
		}
		ver := int(b[p+1]>>3) & 3 // 3=MPEG1, 2=MPEG2, 0=MPEG2.5
		layer := 4 - int(b[p+1]>>1)&3
		bi := int(b[p+2] >> 4)
		si := int(b[p+2]>>2) & 3
		pad := int(b[p+2]>>1) & 1
		if ver == 1 || layer == 4 || bi == 0 || bi == 15 || si == 3 {
			p++
			continue
		}
		v := 2
		if ver == 3 {
			v = 1
		}
		br := mpegBitrates[[2]int{v, layer}][bi] * 1000
		sr := mpegRates[ver][si]
		var spf, flen int
		switch layer {
		case 1:
			spf = 384
			flen = (12*br/sr + pad) * 4
		case 2:
			spf = 1152
			flen = 144*br/sr + pad
		default:
			spf = 1152
			if v == 2 {
				spf = 576
				flen = 72*br/sr + pad
			} else {
				flen = 144*br/sr + pad
			}
		}
		if flen < 4 {
			p++
			continue
		}
		samples += float64(spf)
		rate = float64(sr)
		frames++
		p += flen
	}
	if frames == 0 || rate == 0 {
		return 0, errors.New("no MP3 frames found")
	}
	return int(samples / rate * 1000), nil
}
