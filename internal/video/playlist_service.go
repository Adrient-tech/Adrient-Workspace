package video

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

func toVideoPlaylistProto(p VideoPlaylist) *adrientv1.VideoPlaylist {
	return &adrientv1.VideoPlaylist{
		Id:          p.ID,
		OrgId:       p.OrgID,
		OwnerUserId: p.OwnerUserID,
		Name:        p.Name,
		CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339),
		ItemCount:   p.ItemCount,
	}
}

func (s *Service) CreateVideoPlaylist(ctx context.Context, req *adrientv1.CreateVideoPlaylistRequest) (*adrientv1.VideoPlaylist, error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no session")
	}
	if s.playlists == nil {
		return nil, status.Error(codes.Unimplemented, "playlists not configured")
	}
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.playlists.CreatePlaylist(ctx, orgID, u.ID, req.GetName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create playlist: %v", err)
	}
	return toVideoPlaylistProto(p), nil
}

func (s *Service) ListVideoPlaylists(ctx context.Context, _ *adrientv1.ListVideoPlaylistsRequest) (*adrientv1.ListVideoPlaylistsResponse, error) {
	if s.playlists == nil {
		return &adrientv1.ListVideoPlaylistsResponse{}, nil
	}
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.playlists.ListPlaylists(ctx, orgID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list playlists: %v", err)
	}
	resp := &adrientv1.ListVideoPlaylistsResponse{Playlists: make([]*adrientv1.VideoPlaylist, 0, len(list))}
	for _, p := range list {
		resp.Playlists = append(resp.Playlists, toVideoPlaylistProto(p))
	}
	return resp, nil
}

func (s *Service) UpdateVideoPlaylist(ctx context.Context, req *adrientv1.UpdateVideoPlaylistRequest) (*adrientv1.VideoPlaylist, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.playlists.UpdatePlaylist(ctx, orgID, req.GetId(), req.GetName())
	if errors.Is(err, ErrPlaylistNotFound) {
		return nil, status.Error(codes.NotFound, "playlist not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update playlist: %v", err)
	}
	return toVideoPlaylistProto(p), nil
}

func (s *Service) DeleteVideoPlaylist(ctx context.Context, req *adrientv1.DeleteVideoPlaylistRequest) (*adrientv1.DeleteVideoPlaylistResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.playlists.DeletePlaylist(ctx, orgID, req.GetId()); errors.Is(err, ErrPlaylistNotFound) {
		return nil, status.Error(codes.NotFound, "playlist not found")
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "delete playlist: %v", err)
	}
	return &adrientv1.DeleteVideoPlaylistResponse{}, nil
}

func (s *Service) AddToVideoPlaylist(ctx context.Context, req *adrientv1.AddToVideoPlaylistRequest) (*adrientv1.AddToVideoPlaylistResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.playlists.AddToPlaylist(ctx, orgID, req.GetPlaylistId(), req.GetVideoId()); err != nil {
		return nil, status.Errorf(codes.Internal, "add to playlist: %v", err)
	}
	return &adrientv1.AddToVideoPlaylistResponse{
		Item: &adrientv1.VideoPlaylistItem{PlaylistId: req.GetPlaylistId(), VideoId: req.GetVideoId()},
	}, nil
}

func (s *Service) RemoveFromVideoPlaylist(ctx context.Context, req *adrientv1.RemoveFromVideoPlaylistRequest) (*adrientv1.RemoveFromVideoPlaylistResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.playlists.RemoveFromPlaylist(ctx, orgID, req.GetPlaylistId(), req.GetVideoId()); errors.Is(err, ErrPlaylistNotFound) {
		return nil, status.Error(codes.NotFound, "item not found")
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "remove from playlist: %v", err)
	}
	return &adrientv1.RemoveFromVideoPlaylistResponse{}, nil
}

func (s *Service) ReorderVideoPlaylist(ctx context.Context, req *adrientv1.ReorderVideoPlaylistRequest) (*adrientv1.ReorderVideoPlaylistResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.playlists.ReorderPlaylist(ctx, orgID, req.GetPlaylistId(), req.GetVideoIds()); errors.Is(err, ErrPlaylistNotFound) {
		return nil, status.Error(codes.NotFound, "playlist not found")
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "reorder playlist: %v", err)
	}
	return &adrientv1.ReorderVideoPlaylistResponse{}, nil
}

func (s *Service) ListVideoPlaylistVideos(ctx context.Context, req *adrientv1.ListVideoPlaylistVideosRequest) (*adrientv1.ListVideoPlaylistVideosResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	vids, err := s.playlists.ListPlaylistVideos(ctx, orgID, req.GetPlaylistId())
	if errors.Is(err, ErrPlaylistNotFound) {
		return nil, status.Error(codes.NotFound, "playlist not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list playlist videos: %v", err)
	}
	resp := &adrientv1.ListVideoPlaylistVideosResponse{Videos: make([]*adrientv1.Video, 0, len(vids))}
	for _, v := range vids {
		resp.Videos = append(resp.Videos, toProto(v))
	}
	return resp, nil
}
