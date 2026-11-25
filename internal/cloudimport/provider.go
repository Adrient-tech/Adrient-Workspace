package cloudimport

import (
	"context"
	"errors"
	"io"
	"time"
)

// Kind is one category of data a provider can migrate.
type Kind string

const (
	MigUsers    Kind = "users"
	MigGroups   Kind = "groups"
	MigFiles    Kind = "files"
	MigMail     Kind = "mail"
	MigCalendar Kind = "calendar"
	MigContacts Kind = "contacts"
	MigChat     Kind = "chat"
)

// AllKinds is the processing order. Users come first because every other kind
// is written into a mapped local user's account.
var AllKinds = []Kind{MigUsers, MigGroups, MigFiles, MigMail, MigCalendar, MigContacts, MigChat}

// ErrUnsupported is returned by a Source for a kind it cannot migrate.
var ErrUnsupported = errors.New("not supported by this provider")

// RemoteUser is an account on the source system.
type RemoteUser struct {
	ID    string
	Email string
	Name  string
}

// RemoteGroup is a group / distribution list on the source.
type RemoteGroup struct {
	ID      string
	Email   string
	Name    string
	Members []string // member e-mail addresses
}

// RemoteShare is one sharing entry on a remote file.
type RemoteShare struct {
	Email string
	Role  string // viewer | commenter | editor
}

// RemoteFile is a Drive file or folder. Sources MUST emit a folder before any
// of its children.
type RemoteFile struct {
	ID          string
	ParentID    string // "" = root of the user's drive
	Name        string
	IsDir       bool
	Mime        string
	Size        int64
	Fingerprint string // etag / modified+size; unchanged fingerprint = skip on re-run
	Shares      []RemoteShare
	Drive       string // "" = My Drive; otherwise the shared drive / site name
	// Open streams the content. Nil for folders.
	Open func(ctx context.Context) (io.ReadCloser, error)
}

// RemoteContact is a contact with a stable remote id.
type RemoteContact struct {
	ID          string
	Fingerprint string
	Fields      ContactFields
}

// RemoteEvent is a calendar event with a stable remote id.
type RemoteEvent struct {
	ID          string
	Fingerprint string
	Fields      EventFields
}

// RemoteMail is one message. Raw is the RFC 5322 source.
type RemoteMail struct {
	ID      string // stable id: folder + UIDVALIDITY + UID, or the vendor message id
	Folder  string // source folder path as the vendor names it
	Labels  []string
	Read    bool
	Starred bool
	Raw     []byte
}

// RemoteChat is one exported chat message (only where the vendor allows export).
type RemoteChat struct {
	ID, Space, Author, Text string
	At                      time.Time
}

// Source reads from one connected source account. A Source embeds Unsupported
// and overrides only the kinds it can really read.
type Source interface {
	Capabilities() []Kind
	Users(ctx context.Context, fn func(RemoteUser) error) error
	Groups(ctx context.Context, fn func(RemoteGroup) error) error
	Files(ctx context.Context, u RemoteUser, fn func(RemoteFile) error) error
	Mail(ctx context.Context, u RemoteUser, fn func(RemoteMail) error) error
	Events(ctx context.Context, u RemoteUser, fn func(RemoteEvent) error) error
	Contacts(ctx context.Context, u RemoteUser, fn func(RemoteContact) error) error
	Chat(ctx context.Context, u RemoteUser, fn func(RemoteChat) error) error
}

// Unsupported is embedded to provide ErrUnsupported for every kind.
type Unsupported struct{}

func (Unsupported) Users(context.Context, func(RemoteUser) error) error   { return ErrUnsupported }
func (Unsupported) Groups(context.Context, func(RemoteGroup) error) error { return ErrUnsupported }
func (Unsupported) Files(context.Context, RemoteUser, func(RemoteFile) error) error {
	return ErrUnsupported
}
func (Unsupported) Mail(context.Context, RemoteUser, func(RemoteMail) error) error {
	return ErrUnsupported
}
func (Unsupported) Events(context.Context, RemoteUser, func(RemoteEvent) error) error {
	return ErrUnsupported
}
func (Unsupported) Contacts(context.Context, RemoteUser, func(RemoteContact) error) error {
	return ErrUnsupported
}
func (Unsupported) Chat(context.Context, RemoteUser, func(RemoteChat) error) error {
	return ErrUnsupported
}

// MailFields is a parsed message ready to be stored.
type MailFields struct {
	Folder    string // inbox | sent | drafts | trash | spam | archive
	FromAddr  string
	FromName  string
	To, Cc    []string
	Subject   string
	Body      string
	Labels    []string
	Read      bool
	Starred   bool
	SentAt    time.Time
	MessageID string
}

// Sink writes into Adrient. The server wires a concrete implementation over the
// real repositories; tests use an in-memory one.
type Sink interface {
	// FindUserByEmail returns the local user with that e-mail in the org.
	FindUserByEmail(ctx context.Context, orgID, email string) (userID string, ok bool, err error)
	// EnsureFolder returns the id of folder name under parent, creating it if absent.
	EnsureFolder(ctx context.Context, orgID, ownerID, parent, name string) (string, error)
	// PutFile stores a file under parent. When a file of that name exists the
	// implementation must pick a free "name (n)" rather than fail.
	PutFile(ctx context.Context, orgID, ownerID, parent, name, mime string, size int64, r io.Reader) (string, error)
	// ShareFile grants a local user access to a file.
	ShareFile(ctx context.Context, orgID, ownerID, fileID, granteeUserID, role string) error
	AddContact(ctx context.Context, orgID, ownerID string, f ContactFields) (string, error)
	AddEvent(ctx context.Context, orgID, ownerID string, f EventFields) (string, error)
	AddMail(ctx context.Context, orgID, ownerID string, m MailFields) (string, error)
	// LabelMail adds labels to an already-imported message (Gmail shows one
	// message under several labels; Adrient stores it once with all labels).
	LabelMail(ctx context.Context, orgID, ownerID, mailID string, labels []string) error
	// AddChat posts an imported chat message into a channel named after the
	// source space (created on first use, owner as the only member).
	AddChat(ctx context.Context, orgID, ownerID string, c RemoteChat) (string, error)
	// EnsureGroup creates the group if no group of that name exists.
	EnsureGroup(ctx context.Context, orgID, ownerID, name, email string, memberUserIDs []string) (string, error)
}
