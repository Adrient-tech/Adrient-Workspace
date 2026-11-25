package cloudimport

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
)

// IMAPConfig describes an IMAP source. Security: "tls" (implicit, port 993),
// "starttls", or "insecure" (plain TCP; only accepted for loopback hosts so a
// password is never sent in clear over a network).
type IMAPConfig struct {
	Addr        string // host:port
	Security    string
	Username    string
	Password    string // app password; or empty when AccessToken is set
	AccessToken string // OAuth2 (Gmail, Outlook): XOAUTH2
	// SkipFolders are not imported (e.g. Gmail's virtual "All Mail" which
	// duplicates every message).
	SkipFolders []string
	// GmailLabels: the server is Gmail; folders other than system folders are
	// labels, and "[Gmail]/All Mail" is skipped.
	GmailLabels bool
	BatchSize   int
}

// xoauth2 implements the XOAUTH2 SASL mechanism used by Gmail and Outlook.
type xoauth2 struct{ user, token string }

func (x xoauth2) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + x.user + "\x01auth=Bearer " + x.token + "\x01\x01"), nil
}
func (x xoauth2) Next(ch []byte) ([]byte, error) {
	// The server sends a JSON error as a challenge; reply empty to get the final NO.
	return []byte{}, nil
}

var _ sasl.Client = xoauth2{}

func isLoopback(addr string) bool {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		h = addr
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func (c IMAPConfig) dial() (*imapclient.Client, error) {
	host, _, _ := net.SplitHostPort(c.Addr)
	opts := &imapclient.Options{TLSConfig: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	var cl *imapclient.Client
	var err error
	switch c.Security {
	case "", "tls":
		cl, err = imapclient.DialTLS(c.Addr, opts)
	case "starttls":
		cl, err = imapclient.DialStartTLS(c.Addr, opts)
	case "insecure":
		if !isLoopback(c.Addr) {
			return nil, fmt.Errorf("refusing an unencrypted IMAP connection to %s; use tls or starttls", c.Addr)
		}
		cl, err = imapclient.DialInsecure(c.Addr, opts)
	default:
		return nil, fmt.Errorf("unknown IMAP security %q", c.Security)
	}
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", c.Addr, err)
	}
	if c.AccessToken != "" {
		err = cl.Authenticate(xoauth2{c.Username, c.AccessToken})
	} else {
		err = cl.Login(c.Username, c.Password).Wait()
	}
	if err != nil {
		cl.Close()
		return nil, fmt.Errorf("IMAP sign-in failed for %s: %w", c.Username, err)
	}
	return cl, nil
}

// TestIMAP signs in and lists folders, for the wizard's "test connection".
func TestIMAP(c IMAPConfig) ([]string, error) {
	cl, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	boxes, err := cl.List("", "*", nil).Collect()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, b := range boxes {
		out = append(out, b.Mailbox)
	}
	return out, nil
}

func hasAttr(attrs []imap.MailboxAttr, a imap.MailboxAttr) bool {
	for _, x := range attrs {
		if strings.EqualFold(string(x), string(a)) {
			return true
		}
	}
	return false
}

// IMAPMail streams every message from every selectable folder. Message ids are
// folder|UIDVALIDITY|UID, so a re-run skips everything already imported and a
// server-side UIDVALIDITY reset is caught by the Message-ID de-dup.
func IMAPMail(ctx context.Context, c IMAPConfig, fn func(RemoteMail) error) error {
	cl, err := c.dial()
	if err != nil {
		return err
	}
	defer cl.Close()
	boxes, err := cl.List("", "*", &imap.ListOptions{ReturnSpecialUse: cl.Caps().Has(imap.CapSpecialUse)}).Collect()
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	skip := map[string]bool{}
	for _, s := range c.SkipFolders {
		skip[strings.ToLower(s)] = true
	}
	batch := c.BatchSize
	if batch <= 0 {
		batch = 200
	}
	for _, b := range boxes {
		if hasAttr(b.Attrs, imap.MailboxAttrNoSelect) || hasAttr(b.Attrs, imap.MailboxAttrNonExistent) || skip[strings.ToLower(b.Mailbox)] {
			continue
		}
		if c.GmailLabels && (hasAttr(b.Attrs, imap.MailboxAttrAll) || strings.EqualFold(b.Mailbox, "[Gmail]/All Mail") || hasAttr(b.Attrs, imap.MailboxAttrImportant) || hasAttr(b.Attrs, imap.MailboxAttrFlagged)) {
			continue
		}
		folder := b.Mailbox
		switch {
		case hasAttr(b.Attrs, imap.MailboxAttrSent):
			folder = "Sent"
		case hasAttr(b.Attrs, imap.MailboxAttrDrafts):
			folder = "Drafts"
		case hasAttr(b.Attrs, imap.MailboxAttrTrash):
			folder = "Trash"
		case hasAttr(b.Attrs, imap.MailboxAttrJunk):
			folder = "Spam"
		}
		sel, err := cl.Select(b.Mailbox, &imap.SelectOptions{ReadOnly: true}).Wait()
		if err != nil {
			return fmt.Errorf("select %s: %w", b.Mailbox, err)
		}
		if sel.NumMessages == 0 {
			continue
		}
		uids, err := cl.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
		if err != nil {
			return fmt.Errorf("search %s: %w", b.Mailbox, err)
		}
		all := uids.AllUIDs()
		for start := 0; start < len(all); start += batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := start + batch
			if end > len(all) {
				end = len(all)
			}
			set := imap.UIDSetNum(all[start:end]...)
			section := &imap.FetchItemBodySection{Peek: true}
			fc := cl.Fetch(set, &imap.FetchOptions{UID: true, Flags: true, BodySection: []*imap.FetchItemBodySection{section}})
			for {
				md := fc.Next()
				if md == nil {
					break
				}
				buf, err := md.Collect()
				if err != nil {
					fc.Close()
					return err
				}
				raw := buf.FindBodySection(section)
				rm := RemoteMail{
					ID:     fmt.Sprintf("%s|%d|%d", b.Mailbox, sel.UIDValidity, buf.UID),
					Folder: folder,
					Raw:    raw,
				}
				for _, f := range buf.Flags {
					switch f {
					case imap.FlagSeen:
						rm.Read = true
					case imap.FlagFlagged:
						rm.Starred = true
					}
				}
				if err := fn(rm); err != nil {
					fc.Close()
					return err
				}
			}
			if err := fc.Close(); err != nil {
				return fmt.Errorf("fetch %s: %w", b.Mailbox, err)
			}
		}
	}
	_ = cl.Logout().Wait()
	return nil
}

// readAllLimit is used where a vendor hands us a body we must buffer.
func readAllLimit(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("item larger than %d bytes", max)
	}
	return b, nil
}

var _ = time.Second
