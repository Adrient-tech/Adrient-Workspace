package video

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
)

func captionStreamURL(id string) string { return "/api/v1/videos/captions/" + id + "/content" }

func toCaptionProto(c Caption) *adrientv1.VideoCaption {
	return &adrientv1.VideoCaption{
		Id:        c.ID,
		OrgId:     c.OrgID,
		VideoId:   c.VideoID,
		Lang:      c.Lang,
		Label:     c.Label,
		StreamUrl: captionStreamURL(c.ID),
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Service) ListVideoCaptions(ctx context.Context, req *adrientv1.ListVideoCaptionsRequest) (*adrientv1.ListVideoCaptionsResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	if s.captions == nil {
		return &adrientv1.ListVideoCaptionsResponse{}, nil
	}
	list, err := s.captions.ListCaptions(ctx, orgID, req.GetVideoId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list captions: %v", err)
	}
	resp := &adrientv1.ListVideoCaptionsResponse{Captions: make([]*adrientv1.VideoCaption, 0, len(list))}
	for _, c := range list {
		resp.Captions = append(resp.Captions, toCaptionProto(c))
	}
	return resp, nil
}

func (s *Service) DeleteVideoCaption(ctx context.Context, req *adrientv1.DeleteVideoCaptionRequest) (*adrientv1.DeleteVideoCaptionResponse, error) {
	orgID, err := callerOrg(ctx)
	if err != nil {
		return nil, err
	}
	if s.captions == nil {
		return nil, status.Error(codes.Unimplemented, "captions not configured")
	}
	blobKey, err := s.captions.DeleteCaption(ctx, orgID, req.GetVideoId(), req.GetId())
	if errors.Is(err, ErrCaptionNotFound) {
		return nil, status.Error(codes.NotFound, "caption not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete caption: %v", err)
	}
	if s.blobs != nil && blobKey != "" {
		_ = s.blobs.Delete(ctx, blobKey)
	}
	return &adrientv1.DeleteVideoCaptionResponse{}, nil
}
