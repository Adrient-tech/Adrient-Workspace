package chat

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Service implements adrientv1.ChatServiceServer.
type Service struct {
	repo *Repository
	hub  *Hub // may be nil; when set, new messages and deletes are broadcast over WS
	gate MemberGate
}

// MemberGate decides whether the caller may start or extend a chat with the
// given members (external-chat control and partner policy). nil allows all.
type MemberGate interface {
	CheckChatMembers(ctx context.Context, memberIDs []string) error
}

// UploadGate is optionally implemented by the MemberGate: it enforces
// chat.file_upload_external when attachments are posted to a conversation that
// includes external people.
type UploadGate interface {
	CheckChatUpload(ctx context.Context, memberIDs []string) error
}

// checkUpload refuses a post with attachments when the gate says so.
func checkUpload(ctx context.Context, g MemberGate, memberIDs []string, nAtt int) error {
	if g == nil || nAtt == 0 {
		return nil
	}
	if ug, ok := g.(UploadGate); ok {
		return ug.CheckChatUpload(ctx, memberIDs)
	}
	return nil
}

// WithGate installs the member gate. Returns s for chaining.
func (s *Service) WithGate(g MemberGate) *Service { s.gate = g; return s }

func gateErr(err error) error {
	if d, ok := authz.AsDenied(err); ok {
		return status.Error(codes.PermissionDenied, d.Reason)
	}
	return status.Error(codes.PermissionDenied, err.Error())
}

// NewService constructs a Service. hub may be nil (no realtime broadcasts).
func NewService(repo *Repository, hub *Hub) *Service { return &Service{repo: repo, hub: hub} }

// attachmentsToProto converts a slice of AttachmentMeta to proto ChatAttachments.
func attachmentsToProto(metas []AttachmentMeta) []*adrientv1.ChatAttachment {
	out := make([]*adrientv1.ChatAttachment, 0, len(metas))
	for _, m := range metas {
		out = append(out, &adrientv1.ChatAttachment{
			Id:       m.ID,
			Name:     m.Name,
			MimeType: m.MimeType,
			Size:     m.Size,
			Url:      "/api/v1/chat/attachments/" + m.ID + "/content",
		})
	}
	return out
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

func callerUser(ctx context.Context) (string, string, error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "no session")
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Internal, "missing org context")
	}
	return u.ID, o.ID, nil
}

func channelToProto(ch Channel) *adrientv1.ChatChannel {
	lastAt := ""
	if ch.LastMessageAt != nil {
		lastAt = ch.LastMessageAt.UTC().Format(time.RFC3339)
	}
	return &adrientv1.ChatChannel{
		Id:            ch.ID,
		OrgId:         ch.OrgID,
		Kind:          ch.Kind,
		Name:          ch.Name,
		MemberIds:     ch.MemberIDs,
		CreatedAt:     ch.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     ch.UpdatedAt.UTC().Format(time.RFC3339),
		LastMessageAt: lastAt,
		UnreadCount:   ch.UnreadCount,
	}
}

func reactionsToProto(rxs []Reaction) []*adrientv1.ChatReaction {
	out := make([]*adrientv1.ChatReaction, 0, len(rxs))
	for _, rx := range rxs {
		out = append(out, &adrientv1.ChatReaction{Emoji: rx.Emoji, Count: rx.Count, Me: rx.Me})
	}
	return out
}

func messageToProto(m Message, atts []*adrientv1.ChatAttachment, rxs []Reaction, nonce string) *adrientv1.ChatMessage {
	return &adrientv1.ChatMessage{
		Id:              m.ID,
		ChannelId:       m.ChannelID,
		OrgId:           m.OrgID,
		SenderId:        m.SenderID,
		SenderName:      m.SenderName,
		Body:            m.Body,
		Reactions:       m.Reactions,
		SentAt:          m.SentAt.UTC().Format(time.RFC3339),
		Attachments:     atts,
		ReactionDetails: reactionsToProto(rxs),
		ParentId:        m.ParentID,
		ReplyCount:      m.ReplyCount,
		ClientNonce:     nonce,
	}
}

// ListChatChannels returns all channels the caller belongs to.
func (s *Service) ListChatChannels(ctx context.Context, _ *adrientv1.ListChatChannelsRequest) (*adrientv1.ListChatChannelsResponse, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	channels, err := s.repo.ListChannelsForUser(ctx, orgID, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list channels: %v", err)
	}
	resp := &adrientv1.ListChatChannelsResponse{Channels: make([]*adrientv1.ChatChannel, 0, len(channels))}
	for _, ch := range channels {
		resp.Channels = append(resp.Channels, channelToProto(ch))
	}
	return resp, nil
}

// CreateChatChannel creates a DM or group channel.
func (s *Service) CreateChatChannel(ctx context.Context, req *adrientv1.CreateChatChannelRequest) (*adrientv1.ChatChannel, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	kind := req.GetKind()
	if kind != "dm" && kind != "group" {
		return nil, status.Error(codes.InvalidArgument, "kind must be 'dm' or 'group'")
	}

	// Ensure caller is always a member.
	memberSet := map[string]struct{}{userID: {}}
	for _, id := range req.GetMemberIds() {
		if id != "" {
			memberSet[id] = struct{}{}
		}
	}
	members := make([]string, 0, len(memberSet))
	for id := range memberSet {
		members = append(members, id)
	}
	if s.gate != nil {
		if gerr := s.gate.CheckChatMembers(ctx, members); gerr != nil {
			return nil, gateErr(gerr)
		}
	}

	if kind == "dm" {
		// A DM is either with one other person (2 members) or with yourself —
		// a "Notes to self" channel (just the caller). De-duplicate either way.
		switch len(members) {
		case 1:
			if existing, err := s.repo.FindDMChannel(ctx, orgID, userID, userID); err == nil {
				return channelToProto(existing), nil
			}
		case 2:
			if existing, err := s.repo.FindDMChannel(ctx, orgID, members[0], members[1]); err == nil {
				return channelToProto(existing), nil
			}
		default:
			return nil, status.Error(codes.InvalidArgument, "a DM is with yourself or one other member")
		}
	} else {
		if req.GetName() == "" {
			return nil, status.Error(codes.InvalidArgument, "group channel requires a name")
		}
	}

	ch, err := s.repo.CreateChannel(ctx, orgID, kind, req.GetName(), members)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create channel: %v", err)
	}
	return channelToProto(ch), nil
}

// GetChatChannel returns a single channel the caller belongs to.
func (s *Service) GetChatChannel(ctx context.Context, req *adrientv1.GetChatChannelRequest) (*adrientv1.ChatChannel, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := s.repo.GetChannel(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "channel not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get channel: %v", err)
	}
	// Verify membership.
	isMember := false
	for _, id := range ch.MemberIDs {
		if id == userID {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, status.Error(codes.PermissionDenied, "not a member of this channel")
	}
	return channelToProto(ch), nil
}

// ListChatMessages returns messages in a channel the caller belongs to.
func (s *Service) ListChatMessages(ctx context.Context, req *adrientv1.ListChatMessagesRequest) (*adrientv1.ListChatMessagesResponse, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := s.repo.GetChannel(ctx, orgID, req.GetChannelId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "channel not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get channel: %v", err)
	}
	isMember := false
	for _, id := range ch.MemberIDs {
		if id == userID {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, status.Error(codes.PermissionDenied, "not a member of this channel")
	}

	msgs, err := s.repo.ListMessages(ctx, orgID, req.GetChannelId(), req.GetBeforeId(), req.GetLimit())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list messages: %v", err)
	}
	// Reverse to chronological order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	// Update read cursor.
	_ = s.repo.UpdateReadCursor(ctx, req.GetChannelId(), userID)

	// Fetch reactions for all messages in a single query.
	msgIDs := make([]string, 0, len(msgs))
	for _, m := range msgs {
		msgIDs = append(msgIDs, m.ID)
	}
	rxMap, _ := s.repo.GetReactionsForMessages(ctx, msgIDs, userID)

	resp := &adrientv1.ListChatMessagesResponse{Messages: make([]*adrientv1.ChatMessage, 0, len(msgs))}
	for _, m := range msgs {
		attMetas, _ := s.repo.GetAttachmentsForMessage(ctx, orgID, m.ID)
		resp.Messages = append(resp.Messages, messageToProto(m, attachmentsToProto(attMetas), rxMap[m.ID], ""))
	}
	return resp, nil
}

// PostChatMessage posts a message to a channel.
func (s *Service) PostChatMessage(ctx context.Context, req *adrientv1.PostChatMessageRequest) (*adrientv1.ChatMessage, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetBody() == "" {
		return nil, status.Error(codes.InvalidArgument, "body is required")
	}
	u, _ := auth.UserFromContext(ctx)
	ch, err := s.repo.GetChannel(ctx, orgID, req.GetChannelId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "channel not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get channel: %v", err)
	}
	isMember := false
	for _, id := range ch.MemberIDs {
		if id == userID {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, status.Error(codes.PermissionDenied, "not a member of this channel")
	}

	if gerr := checkUpload(ctx, s.gate, ch.MemberIDs, len(req.GetAttachmentIds())); gerr != nil {
		return nil, gateErr(gerr)
	}
	senderName := u.DisplayName
	if senderName == "" {
		senderName = u.Email
	}
	m, err := s.repo.PostMessage(ctx, req.GetChannelId(), orgID, userID, senderName, req.GetBody(), "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "post message: %v", err)
	}
	// Link any pre-uploaded attachments to this message.
	attIDs := req.GetAttachmentIds()
	if len(attIDs) > 0 {
		if linkErr := s.repo.LinkAttachmentsToMessage(ctx, orgID, m.ID, attIDs); linkErr != nil {
			// Non-fatal: message is posted; log and continue.
			_ = linkErr
		}
	}
	attMetas, _ := s.repo.GetAttachmentsForMessage(ctx, orgID, m.ID)
	proto := messageToProto(m, attachmentsToProto(attMetas), nil, req.GetClientNonce())
	// Broadcast to all WebSocket subscribers of this channel so others see
	// the message in real time. The nonce is included so the sender's own
	// client can reconcile its optimistic copy.
	if s.hub != nil {
		s.hub.BroadcastMessage(m.ChannelID, proto)
		// Also feed the per-user live feed used by the Teams UI.
		s.hub.NotifyUsers(ch.MemberIDs, feedEnvelope{"type": "message", "channel_id": m.ChannelID, "message": proto, "notify": s.legacyNotify(ch, userID)})
	}
	return proto, nil
}

// DeleteChatMessage deletes a message (only sender can delete).
func (s *Service) DeleteChatMessage(ctx context.Context, req *adrientv1.DeleteChatMessageRequest) (*adrientv1.DeleteChatMessageResponse, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	err = s.repo.DeleteMessage(ctx, orgID, req.GetChannelId(), req.GetId(), userID)
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "message not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete message: %v", err)
	}
	if s.hub != nil {
		s.hub.BroadcastDeleted(req.GetChannelId(), req.GetId())
		if ch, gerr := s.repo.GetChannel(ctx, orgID, req.GetChannelId()); gerr == nil {
			s.hub.NotifyUsers(ch.MemberIDs, feedEnvelope{"type": "deleted", "channel_id": req.GetChannelId(), "id": req.GetId()})
		}
	}
	return &adrientv1.DeleteChatMessageResponse{}, nil
}

// ReactToChatMessage toggles an emoji reaction on a message.
func (s *Service) ReactToChatMessage(ctx context.Context, req *adrientv1.ReactToChatMessageRequest) (*adrientv1.ReactToChatMessageResponse, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetEmoji() == "" {
		return nil, status.Error(codes.InvalidArgument, "emoji is required")
	}
	// Verify the caller is a member of the channel.
	ch, err := s.repo.GetChannel(ctx, orgID, req.GetChannelId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "channel not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get channel: %v", err)
	}
	isMember := false
	for _, id := range ch.MemberIDs {
		if id == userID {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, status.Error(codes.PermissionDenied, "not a member of this channel")
	}
	rxs, err := s.repo.ToggleReaction(ctx, orgID, req.GetMessageId(), userID, req.GetEmoji())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "toggle reaction: %v", err)
	}
	return &adrientv1.ReactToChatMessageResponse{ReactionDetails: reactionsToProto(rxs)}, nil
}

// PostThreadReply posts a reply in a message's thread.
func (s *Service) PostThreadReply(ctx context.Context, req *adrientv1.PostThreadReplyRequest) (*adrientv1.ChatMessage, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetBody() == "" {
		return nil, status.Error(codes.InvalidArgument, "body is required")
	}
	if req.GetParentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "parent_id is required")
	}
	u, _ := auth.UserFromContext(ctx)
	ch, err := s.repo.GetChannel(ctx, orgID, req.GetChannelId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "channel not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get channel: %v", err)
	}
	isMember := false
	for _, id := range ch.MemberIDs {
		if id == userID {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, status.Error(codes.PermissionDenied, "not a member of this channel")
	}
	if gerr := checkUpload(ctx, s.gate, ch.MemberIDs, len(req.GetAttachmentIds())); gerr != nil {
		return nil, gateErr(gerr)
	}
	senderName := u.DisplayName
	if senderName == "" {
		senderName = u.Email
	}
	m, err := s.repo.PostMessage(ctx, req.GetChannelId(), orgID, userID, senderName, req.GetBody(), req.GetParentId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "post reply: %v", err)
	}
	attIDs := req.GetAttachmentIds()
	if len(attIDs) > 0 {
		_ = s.repo.LinkAttachmentsToMessage(ctx, orgID, m.ID, attIDs)
	}
	attMetas, _ := s.repo.GetAttachmentsForMessage(ctx, orgID, m.ID)
	proto := messageToProto(m, attachmentsToProto(attMetas), nil, "")
	if s.hub != nil {
		s.hub.NotifyUsers(ch.MemberIDs, feedEnvelope{"type": "message", "channel_id": m.ChannelID, "message": proto, "notify": s.legacyNotify(ch, userID)})
	}
	return proto, nil
}

// legacyNotify lists members to notify for a message posted through the
// legacy gRPC API (DM/group members other than the sender).
func (s *Service) legacyNotify(ch Channel, sender string) []string {
	out := []string{}
	if ch.Kind == "dm" || ch.Kind == "group" {
		for _, id := range ch.MemberIDs {
			if id != sender {
				out = append(out, id)
			}
		}
	}
	return out
}

// ListThreadReplies returns all replies to a message.
func (s *Service) ListThreadReplies(ctx context.Context, req *adrientv1.ListThreadRepliesRequest) (*adrientv1.ListThreadRepliesResponse, error) {
	userID, orgID, err := callerUser(ctx)
	if err != nil {
		return nil, err
	}
	ch, err := s.repo.GetChannel(ctx, orgID, req.GetChannelId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "channel not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get channel: %v", err)
	}
	isMember := false
	for _, id := range ch.MemberIDs {
		if id == userID {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, status.Error(codes.PermissionDenied, "not a member of this channel")
	}
	msgs, err := s.repo.ListThreadReplies(ctx, orgID, req.GetChannelId(), req.GetParentId(), req.GetLimit())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list thread replies: %v", err)
	}
	msgIDs := make([]string, 0, len(msgs))
	for _, m := range msgs {
		msgIDs = append(msgIDs, m.ID)
	}
	rxMap, _ := s.repo.GetReactionsForMessages(ctx, msgIDs, userID)
	resp := &adrientv1.ListThreadRepliesResponse{Messages: make([]*adrientv1.ChatMessage, 0, len(msgs))}
	for _, m := range msgs {
		attMetas, _ := s.repo.GetAttachmentsForMessage(ctx, orgID, m.ID)
		resp.Messages = append(resp.Messages, messageToProto(m, attachmentsToProto(attMetas), rxMap[m.ID], ""))
	}
	return resp, nil
}
