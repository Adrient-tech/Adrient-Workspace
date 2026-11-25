package contacts

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Service implements adrientv1.ContactsServiceServer over a Repository.
type Service struct {
	repo *Repository
}

// NewService constructs a Service.
func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func callerOrg(ctx context.Context) (string, error) {
	if _, ok := auth.UserFromContext(ctx); !ok {
		return "", status.Error(codes.Unauthenticated, "no session")
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return "", status.Error(codes.Internal, "missing org context")
	}
	return o.ID, nil
}

func callerOrgAndUser(ctx context.Context) (orgID, userID string, err error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "no session")
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Internal, "missing org context")
	}
	return o.ID, u.ID, nil
}

func toProto(c Contact) *adrientv1.Contact {
	return &adrientv1.Contact{
		Id:          c.ID,
		OrgId:       c.OrgID,
		OwnerId:     c.OwnerID,
		DisplayName: c.DisplayName,
		FirstName:   c.FirstName,
		LastName:    c.LastName,
		Company:     c.Company,
		JobTitle:    c.JobTitle,
		Emails:      c.Emails,
		Phones:      c.Phones,
		Labels:      c.Labels,
		Notes:       c.Notes,
		Starred:     c.Starred,
		CreatedAt:   c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func groupToProto(g ContactGroup) *adrientv1.ContactGroup {
	return &adrientv1.ContactGroup{
		Id:          g.ID,
		OrgId:       g.OrgID,
		OwnerUserId: g.OwnerUserID,
		Name:        g.Name,
		CreatedAt:   g.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ---- Contact RPCs ----

func (s *Service) ListContacts(ctx context.Context, req *adrientv1.ListContactsRequest) (*adrientv1.ListContactsResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	f := ListFilter{
		GroupID:     req.GetGroupId(),
		StarredOnly: req.GetStarredOnly(),
	}
	list, err := s.repo.List(ctx, orgID, f)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list contacts: %v", err)
	}
	resp := &adrientv1.ListContactsResponse{Contacts: make([]*adrientv1.Contact, 0, len(list))}
	for _, c := range list {
		resp.Contacts = append(resp.Contacts, toProto(c))
	}
	return resp, nil
}

func (s *Service) CreateContact(ctx context.Context, req *adrientv1.CreateContactRequest) (*adrientv1.Contact, error) {
	orgID, userID, err := callerOrgAndUser(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.Create(ctx, orgID, userID, Fields{
		DisplayName: req.GetDisplayName(), FirstName: req.GetFirstName(), LastName: req.GetLastName(),
		Company: req.GetCompany(), JobTitle: req.GetJobTitle(), Emails: req.GetEmails(),
		Phones: req.GetPhones(), Labels: req.GetLabels(), Notes: req.GetNotes(), Starred: req.GetStarred(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create contact: %v", err)
	}
	return toProto(c), nil
}

func (s *Service) GetContact(ctx context.Context, req *adrientv1.GetContactRequest) (*adrientv1.Contact, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.Get(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "contact not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get contact: %v", err)
	}
	return toProto(c), nil
}

func (s *Service) UpdateContact(ctx context.Context, req *adrientv1.UpdateContactRequest) (*adrientv1.Contact, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.Update(ctx, orgID, req.GetId(), Fields{
		DisplayName: req.GetDisplayName(), FirstName: req.GetFirstName(), LastName: req.GetLastName(),
		Company: req.GetCompany(), JobTitle: req.GetJobTitle(), Emails: req.GetEmails(),
		Phones: req.GetPhones(), Labels: req.GetLabels(), Notes: req.GetNotes(), Starred: req.GetStarred(),
	})
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "contact not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update contact: %v", err)
	}
	return toProto(c), nil
}

func (s *Service) TrashContact(ctx context.Context, req *adrientv1.TrashContactRequest) (*adrientv1.TrashContactResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	err = s.repo.Trash(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "contact not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "trash contact: %v", err)
	}
	return &adrientv1.TrashContactResponse{}, nil
}

func (s *Service) StarContact(ctx context.Context, req *adrientv1.StarContactRequest) (*adrientv1.Contact, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.SetStarred(ctx, orgID, req.GetId(), req.GetStarred())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "contact not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "star contact: %v", err)
	}
	return toProto(c), nil
}

// ---- Contact group RPCs ----

func (s *Service) CreateContactGroup(ctx context.Context, req *adrientv1.CreateContactGroupRequest) (*adrientv1.ContactGroup, error) {
	orgID, userID, err := callerOrgAndUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	g, err := s.repo.CreateGroup(ctx, orgID, userID, req.GetName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create group: %v", err)
	}
	return groupToProto(g), nil
}

func (s *Service) ListContactGroups(ctx context.Context, _ *adrientv1.ListContactGroupsRequest) (*adrientv1.ListContactGroupsResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := s.repo.ListGroups(ctx, orgID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list groups: %v", err)
	}
	resp := &adrientv1.ListContactGroupsResponse{Groups: make([]*adrientv1.ContactGroup, 0, len(groups))}
	for _, g := range groups {
		resp.Groups = append(resp.Groups, groupToProto(g))
	}
	return resp, nil
}

func (s *Service) UpdateContactGroup(ctx context.Context, req *adrientv1.UpdateContactGroupRequest) (*adrientv1.ContactGroup, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	g, err := s.repo.UpdateGroup(ctx, orgID, req.GetId(), req.GetName())
	if errors.Is(err, ErrGroupNotFound) {
		return nil, status.Error(codes.NotFound, "group not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update group: %v", err)
	}
	return groupToProto(g), nil
}

func (s *Service) DeleteContactGroup(ctx context.Context, req *adrientv1.DeleteContactGroupRequest) (*adrientv1.DeleteContactGroupResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	err = s.repo.DeleteGroup(ctx, orgID, req.GetId())
	if errors.Is(err, ErrGroupNotFound) {
		return nil, status.Error(codes.NotFound, "group not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete group: %v", err)
	}
	return &adrientv1.DeleteContactGroupResponse{}, nil
}

func (s *Service) AddContactToGroup(ctx context.Context, req *adrientv1.AddContactToGroupRequest) (*adrientv1.AddContactToGroupResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	err = s.repo.AddToGroup(ctx, orgID, req.GetGroupId(), req.GetContactIds())
	if errors.Is(err, ErrGroupNotFound) {
		return nil, status.Error(codes.NotFound, "group not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "add to group: %v", err)
	}
	return &adrientv1.AddContactToGroupResponse{}, nil
}

func (s *Service) RemoveContactFromGroup(ctx context.Context, req *adrientv1.RemoveContactFromGroupRequest) (*adrientv1.RemoveContactFromGroupResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	err = s.repo.RemoveFromGroup(ctx, orgID, req.GetGroupId(), req.GetContactId())
	if errors.Is(err, ErrGroupNotFound) {
		return nil, status.Error(codes.NotFound, "group not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "remove from group: %v", err)
	}
	return &adrientv1.RemoveContactFromGroupResponse{}, nil
}

// ---- vCard RPCs ----

func (s *Service) ImportVCard(ctx context.Context, req *adrientv1.ImportVCardRequest) (*adrientv1.ImportVCardResponse, error) {
	orgID, userID, err := callerOrgAndUser(ctx)
	if err != nil {
		return nil, err
	}
	parsed := ParseVCards(req.GetVcfText())
	var created int32
	for _, p := range parsed {
		if !IsMeaningful(p) {
			continue
		}
		_, err := s.repo.Create(ctx, orgID, userID, Fields{
			DisplayName: p.DisplayName,
			FirstName:   p.FirstName,
			LastName:    p.LastName,
			Company:     p.Company,
			JobTitle:    p.JobTitle,
			Emails:      p.Emails,
			Phones:      p.Phones,
			Labels:      p.Labels,
			Notes:       p.Notes,
		})
		if err != nil {
			// Skip failures; best-effort import.
			continue
		}
		created++
	}
	return &adrientv1.ImportVCardResponse{Created: created}, nil
}

func (s *Service) ExportVCard(ctx context.Context, req *adrientv1.ExportVCardRequest) (*adrientv1.ExportVCardResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}

	var contacts []Contact
	switch {
	case len(req.GetContactIds()) > 0:
		contacts, err = s.repo.GetMany(ctx, orgID, req.GetContactIds())
	case req.GetGroupId() != "":
		contacts, err = s.repo.List(ctx, orgID, ListFilter{GroupID: req.GetGroupId()})
	default:
		contacts, err = s.repo.List(ctx, orgID, ListFilter{})
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "export contacts: %v", err)
	}

	fields := make([]Fields, len(contacts))
	displayNames := make([]string, len(contacts))
	for i, c := range contacts {
		fields[i] = Fields{
			DisplayName: c.DisplayName,
			FirstName:   c.FirstName,
			LastName:    c.LastName,
			Company:     c.Company,
			JobTitle:    c.JobTitle,
			Emails:      c.Emails,
			Phones:      c.Phones,
			Labels:      c.Labels,
			Notes:       c.Notes,
		}
		displayNames[i] = c.DisplayName
	}
	vcf := SerializeVCards(fields, displayNames)
	return &adrientv1.ExportVCardResponse{VcfText: vcf}, nil
}
