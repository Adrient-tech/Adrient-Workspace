package email

import (
	"encoding/base64"
	"fmt"
	"mime"
	"strings"
)

// Attachment is a file sent with a message.
type Attachment struct {
	Filename    string
	ContentType string // defaults to application/octet-stream
	Data        []byte
}

func (a Attachment) contentType() string {
	if strings.TrimSpace(a.ContentType) == "" {
		return "application/octet-stream"
	}
	return a.ContentType
}

// resendAttachments renders attachments for the Resend HTTP API (base64 content).
func resendAttachments(list []Attachment) []map[string]string {
	out := make([]map[string]string, 0, len(list))
	for _, a := range list {
		out = append(out, map[string]string{
			"filename":     a.Filename,
			"content":      base64.StdEncoding.EncodeToString(a.Data),
			"content_type": a.contentType(),
		})
	}
	return out
}

// buildMIME renders the message; with attachments it wraps the body in
// multipart/mixed.
func buildMIME(from string, m Message) []byte {
	plain := buildMIMEPlain(from, m)
	if len(m.Attachments) == 0 {
		return plain
	}
	s := string(plain)
	i := strings.Index(s, "Content-Type:")
	if i < 0 {
		return plain
	}
	head, body := s[:i], s[i:]
	const boundary = "adrient-mixed-boundary-3b9e"
	var b strings.Builder
	b.WriteString(head)
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\n%s\r\n", boundary, body)
	for _, a := range m.Attachments {
		name := mime.QEncoding.Encode("utf-8", a.Filename)
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		fmt.Fprintf(&b, "Content-Type: %s; name=%q\r\n", a.contentType(), name)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-Disposition: attachment; filename=%q\r\n\r\n", name)
		enc := base64.StdEncoding.EncodeToString(a.Data)
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return []byte(b.String())
}
