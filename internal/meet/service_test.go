package meet

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

func authCtx(orgID, userID string) context.Context {
	ctx := auth.WithUser(context.Background(), users.User{ID: userID, OrgID: orgID, Email: "tester@test.me", DisplayName: "Tester"})
	return auth.WithOrg(ctx, orgs.Org{ID: orgID, Slug: "default", DisplayName: "Default"})
}

func TestService_RequiresAuth(t *testing.T) {
	pool, _, _ := setupDB(t)
	svc := NewService(NewRepository(pool))
	if _, err := svc.ListMeetRooms(context.Background(), &adrientv1.ListMeetRoomsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListMeetRooms without session: got %v want Unauthenticated", err)
	}
}

func TestService_RoomCRUDRoundTrip(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	svc := NewService(NewRepository(pool))
	ctx := authCtx(orgID, userID)

	room, err := svc.CreateMeetRoom(ctx, &adrientv1.CreateMeetRoomRequest{Name: "Standup"})
	if err != nil {
		t.Fatalf("CreateMeetRoom: %v", err)
	}
	if room.Id == "" || room.Name != "Standup" {
		t.Fatalf("room: %+v", room)
	}

	got, err := svc.GetMeetRoom(ctx, &adrientv1.GetMeetRoomRequest{Id: room.Id})
	if err != nil || got.Id != room.Id {
		t.Fatalf("GetMeetRoom: %+v err=%v", got, err)
	}

	list, err := svc.ListMeetRooms(ctx, &adrientv1.ListMeetRoomsRequest{})
	if err != nil || len(list.Rooms) != 1 {
		t.Fatalf("ListMeetRooms: got %d err=%v", len(list.GetRooms()), err)
	}

	if _, err := svc.DeleteMeetRoom(ctx, &adrientv1.DeleteMeetRoomRequest{Id: room.Id}); err != nil {
		t.Fatalf("DeleteMeetRoom: %v", err)
	}
	if _, err := svc.GetMeetRoom(ctx, &adrientv1.GetMeetRoomRequest{Id: room.Id}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetMeetRoom after delete: got %v want NotFound", err)
	}
}
