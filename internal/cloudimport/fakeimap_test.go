package cloudimport

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"
)

type lit struct {
	*bytes.Reader
}

func (l lit) Size() int64 { return l.Reader.Size() }

// fakeIMAP is an in-memory IMAP server that accepts LOGIN user/password and
// XOAUTH2 with a fixed access token, like Gmail and Outlook do.
type fakeIMAP struct {
	Addr  string
	user  *imapmemserver.User
	token string
	name  string
	pass  string
}

type oauthSession struct {
	*imapmemserver.UserSession
	f *fakeIMAP
}

func (s *oauthSession) Login(u, p string) error {
	if u != s.f.name || p != s.f.pass {
		return imapserver.ErrAuthFailed
	}
	s.UserSession = imapmemserver.NewUserSession(s.f.user)
	return nil
}
func (s *oauthSession) Close() error {
	if s.UserSession != nil {
		return s.UserSession.Close()
	}
	return nil
}
func (s *oauthSession) AuthenticateMechanisms() []string { return []string{"XOAUTH2"} }
func (s *oauthSession) Authenticate(mech string) (sasl.Server, error) {
	if mech != "XOAUTH2" {
		return nil, errors.New("unsupported")
	}
	return &xoauthServer{s: s}, nil
}

type xoauthServer struct{ s *oauthSession }

func (x *xoauthServer) Next(resp []byte) ([]byte, bool, error) {
	if resp == nil {
		return nil, false, nil
	}
	parts := strings.Split(string(resp), "\x01")
	user := strings.TrimPrefix(parts[0], "user=")
	tok := ""
	for _, p := range parts {
		if strings.HasPrefix(p, "auth=Bearer ") {
			tok = strings.TrimPrefix(p, "auth=Bearer ")
		}
	}
	if user != x.s.f.name || tok != x.s.f.token {
		return nil, true, imapserver.ErrAuthFailed
	}
	x.s.UserSession = imapmemserver.NewUserSession(x.s.f.user)
	return nil, true, nil
}

func startFakeIMAP(t *testing.T, name, pass, token string, boxes map[string][]string) *fakeIMAP {
	t.Helper()
	f := &fakeIMAP{name: name, pass: pass, token: token, user: imapmemserver.NewUser(name, pass)}
	for box, msgs := range boxes {
		_ = f.user.Create(box, nil)
		for i, m := range msgs {
			var flags []imap.Flag
			if i == 0 {
				flags = append(flags, imap.FlagSeen)
			}
			raw := strings.ReplaceAll(m, "\n", "\r\n")
			if _, err := f.user.Append(box, lit{bytes.NewReader([]byte(raw))}, &imap.AppendOptions{Flags: flags}); err != nil {
				t.Fatal(err)
			}
		}
	}
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &oauthSession{f: f}, nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	f.Addr = ln.Addr().String()
	return f
}

func rawMail(from, to, subject, msgID, body string) string {
	return "From: " + from + "\nTo: " + to + "\nSubject: " + subject + "\nMessage-ID: <" + msgID + ">\nDate: Mon, 02 Jan 2023 10:00:00 +0000\nContent-Type: text/plain; charset=utf-8\n\n" + body + "\n"
}
