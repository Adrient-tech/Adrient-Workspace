package cloudimport

import (
	"bytes"
	"errors"
	"html"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // decode non-UTF-8 bodies
	netmail "github.com/emersion/go-message/mail"
)

// MapMailFolder maps a vendor folder name to an Adrient folder
// (inbox|sent|drafts|trash|spam). Anything else is kept in inbox and the
// original folder becomes a label, so no folder structure is lost.
func MapMailFolder(name string) (folder, label string) {
	l := strings.ToLower(strings.TrimSpace(name))
	l = strings.TrimPrefix(l, "[gmail]/")
	l = strings.TrimPrefix(l, "[google mail]/")
	l = strings.TrimPrefix(l, "inbox.")
	switch l {
	case "inbox":
		return "inbox", ""
	case "sent", "sent mail", "sent items", "sent messages":
		return "sent", ""
	case "drafts", "draft":
		return "drafts", ""
	case "trash", "deleted items", "deleted messages", "bin":
		return "trash", ""
	case "spam", "junk", "junk email", "junk e-mail", "bulk mail":
		return "spam", ""
	}
	return "inbox", strings.TrimSpace(name)
}

var (
	reTags  = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(script|style)>|<[^>]+>`)
	reSpace = regexp.MustCompile(`[ \t\r\f\v]+`)
	reLines = regexp.MustCompile(`\n{3,}`)
)

func htmlToText(s string) string {
	s = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n\n", "</div>", "\n").Replace(s)
	s = reTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(reLines.ReplaceAllString(s, "\n\n"))
}

// ParseMail converts a raw RFC 5322 message into storable fields. Attachments
// are not imported (Adrient stores them as separate blobs); this is reported by
// the caller's documentation, not silently hidden: the message body is intact.
func ParseMail(m RemoteMail) (MailFields, error) {
	mr, err := netmail.CreateReader(bytes.NewReader(m.Raw))
	if err != nil && !message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err) {
		return MailFields{}, err
	}
	if mr == nil {
		return MailFields{}, errors.New("unparseable message")
	}
	h := mr.Header
	f := MailFields{Read: m.Read, Starred: m.Starred}
	var lbl string
	f.Folder, lbl = MapMailFolder(m.Folder)
	f.Labels = append(f.Labels, m.Labels...)
	if lbl != "" {
		f.Labels = append(f.Labels, lbl)
	}
	f.Subject, _ = h.Subject()
	if from, err := h.AddressList("From"); err == nil && len(from) > 0 {
		f.FromAddr, f.FromName = from[0].Address, from[0].Name
	}
	if to, err := h.AddressList("To"); err == nil {
		for _, a := range to {
			f.To = append(f.To, a.Address)
		}
	}
	if cc, err := h.AddressList("Cc"); err == nil {
		for _, a := range cc {
			f.Cc = append(f.Cc, a.Address)
		}
	}
	if d, err := h.Date(); err == nil {
		f.SentAt = d
	} else {
		f.SentAt = time.Time{}
	}
	f.MessageID, _ = h.MessageID()

	var plain, htm string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if message.IsUnknownCharset(err) || message.IsUnknownEncoding(err) {
				continue
			}
			break
		}
		if ih, ok := p.Header.(*netmail.InlineHeader); ok {
			ct, _, _ := ih.ContentType()
			b, _ := io.ReadAll(io.LimitReader(p.Body, 8<<20))
			switch ct {
			case "text/plain":
				if plain == "" {
					plain = string(b)
				}
			case "text/html":
				if htm == "" {
					htm = string(b)
				}
			}
		}
	}
	switch {
	case plain != "":
		f.Body = plain
	case htm != "":
		f.Body = htmlToText(htm)
	}
	return f, nil
}
