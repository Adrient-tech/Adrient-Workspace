package cloudimport

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// memState is an in-memory State.
type memState struct {
	mu    sync.Mutex
	m     map[string][2]string
	users map[string]struct {
		local string
		skip  bool
	}
	progress map[string]*Counter
	errs     []string
}

func newMemState() *memState {
	return &memState{m: map[string][2]string{}, users: map[string]struct {
		local string
		skip  bool
	}{}}
}

func (s *memState) GetMap(_ context.Context, c, k, r string) (string, string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[c+"/"+k+"/"+r]
	return v[0], v[1], ok, nil
}
func (s *memState) PutMap(_ context.Context, c, k, r, l, fp string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[c+"/"+k+"/"+r] = [2]string{l, fp}
	return nil
}
func (s *memState) UserMapping(_ context.Context, c, r string) (string, bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.users[r]
	return v.local, v.skip, ok, nil
}
func (s *memState) SaveRemoteUser(_ context.Context, c string, u RemoteUser, l string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.users[u.ID]; ok && v.local != "" {
		return nil
	}
	s.users[u.ID] = struct {
		local string
		skip  bool
	}{l, false}
	return nil
}
func (s *memState) SaveProgress(_ context.Context, _ string, c map[string]*Counter) error {
	s.mu.Lock()
	s.progress = c
	s.mu.Unlock()
	return nil
}
func (s *memState) AddError(_ context.Context, _, k, r, l, m string) error {
	s.mu.Lock()
	s.errs = append(s.errs, k+":"+l+":"+m)
	s.mu.Unlock()
	return nil
}

// memSink records writes.
type memSink struct {
	mu       sync.Mutex
	users    map[string]string // email -> id
	folders  map[string]string // owner|parent|name -> id
	files    map[string][]byte // owner|path -> content
	paths    map[string]string // id -> path
	shares   []string
	contacts []ContactFields
	events   []EventFields
	mails    []MailFields
	mailIDs  []int
	chats    []RemoteChat
	groups   []string
	n        int
	failName string
}

func newMemSink(users map[string]string) *memSink {
	return &memSink{users: users, folders: map[string]string{}, files: map[string][]byte{}, paths: map[string]string{"": ""}}
}

func (s *memSink) id() string { s.n++; return fmt.Sprintf("L%d", s.n) }

func (s *memSink) FindUserByEmail(_ context.Context, _, e string) (string, bool, error) {
	id, ok := s.users[strings.ToLower(e)]
	return id, ok, nil
}
func (s *memSink) EnsureFolder(_ context.Context, _, owner, parent, name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := owner + "|" + parent + "|" + name
	if id, ok := s.folders[k]; ok {
		return id, nil
	}
	id := s.id()
	s.folders[k] = id
	s.paths[id] = s.paths[parent] + "/" + name
	return id, nil
}
func (s *memSink) PutFile(_ context.Context, _, owner, parent, name, _ string, _ int64, r io.Reader) (string, error) {
	if name == s.failName {
		return "", fmt.Errorf("simulated storage failure")
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id()
	s.paths[id] = s.paths[parent] + "/" + name
	s.files[owner+"|"+s.paths[id]] = b
	return id, nil
}
func (s *memSink) ShareFile(_ context.Context, _, owner, file, grantee, role string) error {
	s.mu.Lock()
	s.shares = append(s.shares, s.paths[file]+"->"+grantee+":"+role)
	s.mu.Unlock()
	return nil
}
func (s *memSink) AddContact(_ context.Context, _, _ string, f ContactFields) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contacts = append(s.contacts, f)
	return s.id(), nil
}
func (s *memSink) AddEvent(_ context.Context, _, _ string, f EventFields) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, f)
	return s.id(), nil
}
func (s *memSink) AddMail(_ context.Context, _, _ string, m MailFields) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mails = append(s.mails, m)
	id := s.id()
	s.mailIDs = append(s.mailIDs, s.n)
	return id, nil
}
func (s *memSink) LabelMail(_ context.Context, _, _, id string, labels []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "L"))
	for i := range s.mails {
		if s.mailIDs[i] == n {
			s.mails[i].Labels = append(s.mails[i].Labels, labels...)
		}
	}
	return nil
}
func (s *memSink) AddChat(_ context.Context, _, _ string, c RemoteChat) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chats = append(s.chats, c)
	return s.id(), nil
}
func (s *memSink) EnsureGroup(_ context.Context, _, _, name, email string, members []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups = append(s.groups, fmt.Sprintf("%s<%s>%v", name, email, members))
	return s.id(), nil
}

func (s *memSink) filePaths() []string {
	var out []string
	for k := range s.files {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
