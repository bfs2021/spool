package telegram

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/enes-alatas/spool/internal/bus"
	"github.com/enes-alatas/spool/internal/store"
	"github.com/enes-alatas/spool/internal/store/sqlite"
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
// group it was bound to, and the new chat waits unbound, rather than the
// error reading as "no fleet room" and moving the mirror (#1464).
func TestAFailedRoomsReadMovesNoFleetRoom(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Loops().Create(ctx, &store.Loop{ID: "l1", Name: "terra", Status: store.StatusActive,
		WorkspaceMode: store.WorkspaceNone, Pacing: store.PacingFixed, Runtime: store.RuntimeBare}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Rooms().Bind(ctx, "l1", store.SurfaceTelegram, "-100", store.FleetChannel, 1); err != nil {
		t.Fatal(err)
	}
	br := &Bridge{store: roomsListFail{db}, log: slog.New(slog.NewTextHandler(io.Discard, nil)), bus: bus.New()}
	if channel := br.roomChannel(ctx, &poller{loopID: "l1", name: "terra"}, Chat{ID: -200, Title: "Other"}); channel != "" {
		t.Errorf("a chat heard while the rooms couldn't be read carries %q, want nothing", channel)
	}
	rooms, err := db.Rooms().List(ctx, "l1")
	if err != nil {
		t.Fatal(err)
	}
	if fleet := store.BoundRoom(rooms, store.SurfaceTelegram, store.FleetChannel); fleet == nil || fleet.RoomID != "-100" {
		t.Errorf("the fleet channel's room is %+v after the failed read, want -100", fleet)
	}
}
