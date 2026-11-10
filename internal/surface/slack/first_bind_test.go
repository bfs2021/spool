package slack

import (
	"context"
	"errors"
	"testing"

	"github.com/enes-alatas/spool/internal/store"
)

// roomsListFail is a store whose rooms can be sighted and bound but not
// listed, as when a read meets a busy database.
type roomsListFail struct{ store.Store }

func (failing roomsListFail) Rooms() store.RoomStore {
	return listFailRooms{failing.Store.Rooms()}
}

type listFailRooms struct{ store.RoomStore }

func (listFailRooms) List(context.Context, string) ([]*store.Room, error) {
	return nil, errors.New("database is locked")
}

// A rooms read that fails binds nothing: the fleet channel stays in the
// Slack channel it was bound to, rather than the error reading as "no
// fleet room" and moving the mirror to the channel just heard (#1464).
func TestAFailedRoomsReadMovesNoFleetRoom(t *testing.T) {
	adapter, db, _ := inboundFixture(t)
	ctx := context.Background()
	if _, err := db.Rooms().Bind(ctx, "loop_terra", store.SurfaceSlack, "C0OLD", store.FleetChannel, 1); err != nil {
		t.Fatal(err)
	}
	adapter.store = roomsListFail{db}
	adapter.ingest(ctx, &link{loopID: "loop_terra"}, channelEvent("U0ALICE", "first", "1.1"))
	rooms, err := db.Rooms().List(ctx, "loop_terra")
	if err != nil {
		t.Fatal(err)
	}
	if fleet := store.BoundRoom(rooms, store.SurfaceSlack, store.FleetChannel); fleet == nil || fleet.RoomID != "C0OLD" {
		t.Errorf("the fleet channel's room is %+v after the failed read, want C0OLD", fleet)
	}
}
