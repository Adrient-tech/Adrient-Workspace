package music

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Service implements adrientv1.MusicServiceServer over a Repository.
type Service struct {
	repo  *Repository
	blobs BlobStore
}

// NewService constructs a Service. blobs is used to delete the underlying bytes
// when a track is removed; it may be nil (delete then only drops metadata).
func NewService(repo *Repository, blobs BlobStore) *Service {
	return &Service{repo: repo, blobs: blobs}
}

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

// streamURL is the relative HTTP path the player/download uses to fetch bytes.
func streamURL(id string) string { return "/api/v1/music/" + id + "/content" }

func trackToProto(t Track) *adrientv1.Track {
	return trackToProtoLiked(t, false)
}

func trackToProtoLiked(t Track, liked bool) *adrientv1.Track {
	return &adrientv1.Track{
		Id:              t.ID,
		OrgId:           t.OrgID,
		OwnerId:         t.OwnerID,
		Title:           t.Title,
		Artist:          t.Artist,
		Album:           t.Album,
		ContentType:     t.ContentType,
		Size:            t.Size,
		DurationSeconds: t.DurationSeconds,
		ArtworkDataUrl:  t.ArtworkDataURL,
		StreamUrl:       streamURL(t.ID),
		CreatedAt:       t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       t.UpdatedAt.UTC().Format(time.RFC3339),
		Liked:           liked,
	}
}

func playlistToProto(p Playlist) *adrientv1.Playlist {
	tracks := make([]*adrientv1.Track, 0, len(p.Tracks))
	for _, t := range p.Tracks {
		tracks = append(tracks, trackToProto(t))
	}
	return &adrientv1.Playlist{
		Id:          p.ID,
		OrgId:       p.OrgID,
		OwnerId:     p.OwnerID,
		Name:        p.Name,
		Description: p.Description,
		Tracks:      tracks,
		TrackCount:  int32(p.TrackCount),
		CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Service) ListTracks(ctx context.Context, _ *adrientv1.ListTracksRequest) (*adrientv1.ListTracksResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	u, _ := auth.UserFromContext(ctx)
	list, err := s.repo.ListTracks(ctx, orgID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list tracks: %v", err)
	}
	// Annotate liked status in bulk.
	ids := make([]string, len(list))
	for i, t := range list {
		ids[i] = t.ID
	}
	liked, err := s.repo.LikedSet(ctx, u.ID, ids)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "liked set: %v", err)
	}
	resp := &adrientv1.ListTracksResponse{Tracks: make([]*adrientv1.Track, 0, len(list))}
	for _, t := range list {
		resp.Tracks = append(resp.Tracks, trackToProtoLiked(t, liked[t.ID]))
	}
	return resp, nil
}

func (s *Service) GetTrack(ctx context.Context, req *adrientv1.GetTrackRequest) (*adrientv1.Track, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.GetTrack(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "track not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get track: %v", err)
	}
	return trackToProto(t), nil
}

func (s *Service) UpdateTrack(ctx context.Context, req *adrientv1.UpdateTrackRequest) (*adrientv1.Track, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.UpdateTrack(ctx, orgID, req.GetId(), TrackFields{
		Title:          req.GetTitle(),
		Artist:         req.GetArtist(),
		Album:          req.GetAlbum(),
		ArtworkDataURL: req.GetArtworkDataUrl(),
	})
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "track not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update track: %v", err)
	}
	return trackToProto(t), nil
}

func (s *Service) DeleteTrack(ctx context.Context, req *adrientv1.DeleteTrackRequest) (*adrientv1.DeleteTrackResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	blobKey, err := s.repo.TrashTrack(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "track not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete track: %v", err)
	}
	// Best-effort blob cleanup; metadata is already soft-deleted.
	if s.blobs != nil && blobKey != "" {
		_ = s.blobs.Delete(ctx, blobKey)
	}
	return &adrientv1.DeleteTrackResponse{}, nil
}

func (s *Service) ListPlaylists(ctx context.Context, _ *adrientv1.ListPlaylistsRequest) (*adrientv1.ListPlaylistsResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.repo.ListPlaylists(ctx, orgID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list playlists: %v", err)
	}
	if u, ok := auth.UserFromContext(ctx); ok {
		list = s.repo.filterVisible(ctx, orgID, u.ID, list)
	}
	resp := &adrientv1.ListPlaylistsResponse{Playlists: make([]*adrientv1.Playlist, 0, len(list))}
	for _, p := range list {
		resp.Playlists = append(resp.Playlists, playlistToProto(p))
	}
	return resp, nil
}

func (s *Service) GetPlaylist(ctx context.Context, req *adrientv1.GetPlaylistRequest) (*adrientv1.Playlist, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.GetPlaylist(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "playlist not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get playlist: %v", err)
	}
	if u, ok := auth.UserFromContext(ctx); ok && len(s.repo.filterVisible(ctx, orgID, u.ID, []Playlist{p})) == 0 {
		return nil, status.Error(codes.NotFound, "playlist not found")
	}
	return playlistToProto(p), nil
}

func (s *Service) CreatePlaylist(ctx context.Context, req *adrientv1.CreatePlaylistRequest) (*adrientv1.Playlist, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	u, _ := auth.UserFromContext(ctx)
	p, err := s.repo.CreatePlaylist(ctx, orgID, u.ID, PlaylistFields{
		Name:        req.GetName(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create playlist: %v", err)
	}
	return playlistToProto(p), nil
}

func (s *Service) UpdatePlaylist(ctx context.Context, req *adrientv1.UpdatePlaylistRequest) (*adrientv1.Playlist, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.UpdatePlaylist(ctx, orgID, req.GetId(), PlaylistFields{
		Name:        req.GetName(),
		Description: req.GetDescription(),
	})
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "playlist not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update playlist: %v", err)
	}
	return playlistToProto(p), nil
}

func (s *Service) DeletePlaylist(ctx context.Context, req *adrientv1.DeletePlaylistRequest) (*adrientv1.DeletePlaylistResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	err = s.repo.TrashPlaylist(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "playlist not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete playlist: %v", err)
	}
	return &adrientv1.DeletePlaylistResponse{}, nil
}

func (s *Service) AddTrackToPlaylist(ctx context.Context, req *adrientv1.AddTrackToPlaylistRequest) (*adrientv1.Playlist, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.AddTrackToPlaylist(ctx, orgID, req.GetPlaylistId(), req.GetTrackId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "playlist or track not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "add track to playlist: %v", err)
	}
	return playlistToProto(p), nil
}

func (s *Service) RemoveTrackFromPlaylist(ctx context.Context, req *adrientv1.RemoveTrackFromPlaylistRequest) (*adrientv1.Playlist, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.RemoveTrackFromPlaylist(ctx, orgID, req.GetPlaylistId(), req.GetTrackId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "playlist not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "remove track from playlist: %v", err)
	}
	return playlistToProto(p), nil
}

func (s *Service) ReorderPlaylistTrack(ctx context.Context, req *adrientv1.ReorderPlaylistTrackRequest) (*adrientv1.Playlist, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.ReorderPlaylistTrack(ctx, orgID, req.GetPlaylistId(), req.GetTrackId(), int(req.GetNewPosition()))
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "playlist or track not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "reorder playlist track: %v", err)
	}
	return playlistToProto(p), nil
}

func (s *Service) LikeTrack(ctx context.Context, req *adrientv1.LikeTrackRequest) (*adrientv1.LikeTrackResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	u, _ := auth.UserFromContext(ctx)
	if err := s.repo.LikeTrack(ctx, orgID, u.ID, req.GetTrackId()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "track not found")
		}
		return nil, status.Errorf(codes.Internal, "like track: %v", err)
	}
	return &adrientv1.LikeTrackResponse{}, nil
}

func (s *Service) UnlikeTrack(ctx context.Context, req *adrientv1.UnlikeTrackRequest) (*adrientv1.UnlikeTrackResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	u, _ := auth.UserFromContext(ctx)
	if err := s.repo.UnlikeTrack(ctx, orgID, u.ID, req.GetTrackId()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "track not found")
		}
		return nil, status.Errorf(codes.Internal, "unlike track: %v", err)
	}
	return &adrientv1.UnlikeTrackResponse{}, nil
}

func (s *Service) ListLikedTracks(ctx context.Context, _ *adrientv1.ListLikedTracksRequest) (*adrientv1.ListLikedTracksResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	u, _ := auth.UserFromContext(ctx)
	list, err := s.repo.ListLikedTracks(ctx, orgID, u.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list liked tracks: %v", err)
	}
	resp := &adrientv1.ListLikedTracksResponse{Tracks: make([]*adrientv1.Track, 0, len(list))}
	for _, t := range list {
		resp.Tracks = append(resp.Tracks, trackToProtoLiked(t, true))
	}
	return resp, nil
}
