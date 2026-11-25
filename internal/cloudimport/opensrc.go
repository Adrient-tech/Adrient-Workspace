package cloudimport

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

func asHTTP(err error, out **HTTPError) bool { return errors.As(err, out) }

// StandardSource reads one person's account over open protocols: WebDAV files,
// CalDAV, CardDAV and IMAP. It backs Nextcloud, ownCloud, generic servers and
// Zoho (mail/calendar/contacts). Any URL left empty disables that kind.
type StandardSource struct {
	Unsupported
	FilesURL   string
	CalDAVURL  string
	CardDAVURL string
	IMAP       *IMAPConfig
	DAV        *DAVClient
	// Directory optionally lists users (Zoho org accounts) for mapping only.
	Directory func(ctx context.Context, fn func(RemoteUser) error) error
}

func (s *StandardSource) Capabilities() []Kind {
	var k []Kind
	if s.Directory != nil {
		k = append(k, MigUsers)
	}
	if s.FilesURL != "" {
		k = append(k, MigFiles)
	}
	if s.IMAP != nil && s.IMAP.Addr != "" {
		k = append(k, MigMail)
	}
	if s.CalDAVURL != "" {
		k = append(k, MigCalendar)
	}
	if s.CardDAVURL != "" {
		k = append(k, MigContacts)
	}
	return k
}

func (s *StandardSource) Users(ctx context.Context, fn func(RemoteUser) error) error {
	if s.Directory == nil {
		return ErrUnsupported
	}
	return s.Directory(ctx, fn)
}

func (s *StandardSource) Files(ctx context.Context, _ RemoteUser, fn func(RemoteFile) error) error {
	if s.FilesURL == "" {
		return ErrUnsupported
	}
	return s.DAV.WalkFiles(ctx, s.FilesURL, fn)
}

func (s *StandardSource) Events(ctx context.Context, _ RemoteUser, fn func(RemoteEvent) error) error {
	if s.CalDAVURL == "" {
		return ErrUnsupported
	}
	return s.DAV.Events(ctx, s.CalDAVURL, fn)
}

func (s *StandardSource) Contacts(ctx context.Context, _ RemoteUser, fn func(RemoteContact) error) error {
	if s.CardDAVURL == "" {
		return ErrUnsupported
	}
	return s.DAV.Contacts(ctx, s.CardDAVURL, fn)
}

func (s *StandardSource) Mail(ctx context.Context, _ RemoteUser, fn func(RemoteMail) error) error {
	if s.IMAP == nil || s.IMAP.Addr == "" {
		return ErrUnsupported
	}
	return IMAPMail(ctx, *s.IMAP, fn)
}

// NextcloudURLs derives the standard Nextcloud/ownCloud endpoints from the
// server URL and username.
func NextcloudURLs(server, user string) (files, caldav, carddav string) {
	b := strings.TrimRight(server, "/")
	u := url.PathEscape(user)
	return b + "/remote.php/dav/files/" + u + "/",
		b + "/remote.php/dav/calendars/" + u + "/",
		b + "/remote.php/dav/addressbooks/users/" + u + "/"
}

// OwnCloudURLs: ownCloud 10 uses the same DAV layout as Nextcloud. (ownCloud
// Infinite Scale uses /dav/spaces and has no CalDAV/CardDAV; not covered.)
func OwnCloudURLs(server, user string) (files, caldav, carddav string) {
	return NextcloudURLs(server, user)
}
