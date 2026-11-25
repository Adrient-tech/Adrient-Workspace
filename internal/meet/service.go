package meet

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Service implements adrientv1.MeetServiceServer over a Repository.
type Service struct {
	adrientv1.UnimplementedMeetServiceServer
	repo *Repository
	// OnClosing runs before a room is deleted so its running meeting can be
	// marked ended (reason room_closed). May be nil.
	OnClosing func(ctx context.Context, roomID, reason string)
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

func toProto(r Room) *adrientv1.MeetRoom {
	return &adrientv1.MeetRoom{
		Id:        r.ID,
		OrgId:     r.OrgID,
		OwnerId:   r.OwnerID,
		Name:      r.Name,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Service) ListMeetRooms(ctx context.Context, _ *adrientv1.ListMeetRoomsRequest) (*adrientv1.ListMeetRoomsResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.repo.List(ctx, orgID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list rooms: %v", err)
	}
	resp := &adrientv1.ListMeetRoomsResponse{Rooms: make([]*adrientv1.MeetRoom, 0, len(list))}
	for _, r := range list {
		resp.Rooms = append(resp.Rooms, toProto(r))
	}
	return resp, nil
}

func (s *Service) CreateMeetRoom(ctx context.Context, req *adrientv1.CreateMeetRoomRequest) (*adrientv1.MeetRoom, error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no session")
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Internal, "missing org context")
	}
	r, err := s.repo.Create(ctx, o.ID, u.ID, req.GetName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create room: %v", err)
	}
	return toProto(r), nil
}

func (s *Service) GetMeetRoom(ctx context.Context, req *adrientv1.GetMeetRoomRequest) (*adrientv1.MeetRoom, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	r, err := s.repo.Get(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get room: %v", err)
	}
	return toProto(r), nil
}

func (s *Service) DeleteMeetRoom(ctx context.Context, req *adrientv1.DeleteMeetRoomRequest) (*adrientv1.DeleteMeetRoomResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	if s.OnClosing != nil {
		if _, gerr := s.repo.Get(ctx, orgID, req.GetId()); gerr == nil {
			s.OnClosing(ctx, req.GetId(), EndedRoomClosed)
		}
	}
	err = s.repo.Delete(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "room not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete room: %v", err)
	}
	return &adrientv1.DeleteMeetRoomResponse{}, nil
}
