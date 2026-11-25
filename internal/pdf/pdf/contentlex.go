package pdf

import (
	"bytes"
	"fmt"
	"strconv"
)

// A small lexer for PDF page content streams. It splits a stream into
// operations (operands followed by an operator) and keeps the byte range of
// every operation so untouched operations can be copied out verbatim.

type tokKind int

const (
	tokNumber tokKind = iota
	tokName
	tokString // literal or hex string, decoded bytes in str
	tokArray  // elements in arr
	tokDict   // raw only
	tokBool
	tokNull
	tokOp
)

type token struct {
	kind tokKind
	num  float64
	str  []byte // decoded string bytes or name / operator text
	arr  []token
}

type contentOp struct {
	op       string
	operands []token
	start    int // byte offset of the first operand (or the operator)
	end      int // byte offset just past the operator
	inline   []byte
}

type lexer struct {
	b   []byte
	pos int
}

func isWhite(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		if isWhite(c) {
			l.pos++
			continue
		}
		if c == '%' {
			for l.pos < len(l.b) && l.b[l.pos] != '\n' && l.b[l.pos] != '\r' {
				l.pos++
			}
			continue
		}
		return
	}
}

// next reads one token. ok is false at end of input.
func (l *lexer) next() (t token, ok bool, err error) {
	l.skipSpace()
	if l.pos >= len(l.b) {
		return t, false, nil
	}
	c := l.b[l.pos]
	switch {
	case c == '/':
		l.pos++
		s := l.pos
		for l.pos < len(l.b) && !isWhite(l.b[l.pos]) && !isDelim(l.b[l.pos]) {
			l.pos++
		}
		return token{kind: tokName, str: l.b[s:l.pos]}, true, nil
	case c == '(':
		s, err := l.literalString()
		return token{kind: tokString, str: s}, err == nil, err
	case c == '<' && l.pos+1 < len(l.b) && l.b[l.pos+1] == '<':
		// dictionary (only appears as an operand of BDC/DP and inline images)
		depth := 0
		for l.pos < len(l.b)-1 {
			if l.b[l.pos] == '<' && l.b[l.pos+1] == '<' {
				depth++
				l.pos += 2
				continue
			}
			if l.b[l.pos] == '>' && l.b[l.pos+1] == '>' {
				depth--
				l.pos += 2
				if depth == 0 {
					return token{kind: tokDict}, true, nil
				}
				continue
			}
			if l.b[l.pos] == '(' {
				if _, err := l.literalString(); err != nil {
					return t, false, err
				}
				continue
			}
			l.pos++
		}
		return t, false, fmt.Errorf("unterminated dictionary")
	case c == '<':
		l.pos++
		var hex []byte
		for l.pos < len(l.b) && l.b[l.pos] != '>' {
			if !isWhite(l.b[l.pos]) {
				hex = append(hex, l.b[l.pos])
			}
			l.pos++
		}
		l.pos++
		if len(hex)%2 == 1 {
			hex = append(hex, '0')
		}
		out := make([]byte, len(hex)/2)
		for i := range out {
			v, err := strconv.ParseUint(string(hex[2*i:2*i+2]), 16, 8)
			if err != nil {
				return t, false, fmt.Errorf("bad hex string")
			}
			out[i] = byte(v)
		}
		return token{kind: tokString, str: out}, true, nil
	case c == '[':
		l.pos++
		var arr []token
		for {
			l.skipSpace()
			if l.pos >= len(l.b) {
				return t, false, fmt.Errorf("unterminated array")
			}
			if l.b[l.pos] == ']' {
				l.pos++
				return token{kind: tokArray, arr: arr}, true, nil
			}
			e, ok, err := l.next()
			if err != nil || !ok {
				return t, false, fmt.Errorf("bad array: %v", err)
			}
			arr = append(arr, e)
		}
	case c == ']' || c == ')' || c == '>' || c == '{' || c == '}':
		l.pos++
		return token{kind: tokOp, str: []byte{c}}, true, nil
	}
	s := l.pos
	for l.pos < len(l.b) && !isWhite(l.b[l.pos]) && !isDelim(l.b[l.pos]) {
		l.pos++
	}
	word := l.b[s:l.pos]
	if f, err := strconv.ParseFloat(string(word), 64); err == nil {
		return token{kind: tokNumber, num: f}, true, nil
	}
	switch string(word) {
	case "true", "false":
		return token{kind: tokBool, str: word}, true, nil
	case "null":
		return token{kind: tokNull}, true, nil
	}
	return token{kind: tokOp, str: word}, true, nil
}

func (l *lexer) literalString() ([]byte, error) {
	l.pos++ // (
	depth := 1
	var out []byte
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++
		switch c {
		case '\\':
			if l.pos >= len(l.b) {
				return nil, fmt.Errorf("bad escape")
			}
			e := l.b[l.pos]
			l.pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if l.pos < len(l.b) && l.b[l.pos] == '\n' {
					l.pos++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for k := 0; k < 2 && l.pos < len(l.b) && l.b[l.pos] >= '0' && l.b[l.pos] <= '7'; k++ {
						v = v*8 + int(l.b[l.pos]-'0')
						l.pos++
					}
					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return out, nil
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return nil, fmt.Errorf("unterminated string")
}

// parseContent splits a content stream into operations.
func parseContent(b []byte) ([]contentOp, error) {
	l := &lexer{b: b}
	var ops []contentOp
	var operands []token
	start := -1
	for {
		l.skipSpace()
		pos := l.pos
		t, ok, err := l.next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		if start < 0 {
			start = pos
		}
		if t.kind != tokOp {
			operands = append(operands, t)
			continue
		}
		op := contentOp{op: string(t.str), operands: operands, start: start}
		if op.op == "BI" {
			// Inline image: skip to "ID", then binary data up to whitespace+"EI"+whitespace.
			idx := bytes.Index(b[l.pos:], []byte("ID"))
			if idx < 0 {
				return nil, fmt.Errorf("inline image without ID")
			}
			dataStart := l.pos + idx + 3
			op.inline = b[l.pos : l.pos+idx]
			k := dataStart
			for {
				j := bytes.Index(b[k:], []byte("EI"))
				if j < 0 {
					return nil, fmt.Errorf("inline image without EI")
				}
				e := k + j
				if e > 0 && isWhite(b[e-1]) && (e+2 == len(b) || isWhite(b[e+2])) {
					l.pos = e + 2
					break
				}
				k = e + 2
			}
		}
		op.end = l.pos
		ops = append(ops, op)
		operands = nil
		start = -1
	}
	return ops, nil
}

func formatNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func hexString(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, 2*len(b)+2)
	out = append(out, '<')
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&15])
	}
	return string(append(out, '>'))
}
