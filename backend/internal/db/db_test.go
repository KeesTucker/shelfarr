package db_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"shelfarr/internal/db"
)

// openTestDB returns an in-memory SQLite DB with migrations applied.
func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestMigrate(t *testing.T) {
	// Simply opening the DB must not error; idempotency means a second open
	// against the same in-memory DB (different handle) also works.
	openTestDB(t)
}

func TestUserCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	n, err := d.CountUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 users, got %d", n)
	}

	if err := d.CreateUser(ctx, "u1", "alice", "hash", "admin"); err != nil {
		t.Fatal(err)
	}

	u, err := d.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != "admin" || u.ID != "u1" {
		t.Fatalf("unexpected user: %+v", u)
	}

	u2, err := d.GetUserByID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if u2.Username != "alice" {
		t.Fatalf("GetUserByID returned wrong user: %+v", u2)
	}

	_, err = d.GetUserByUsername(ctx, "nonexistent")
	if err != db.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	n, err = d.CountUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 user, got %d", n)
	}
}

func TestRequestCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.CreateUser(ctx, "u1", "alice", "hash", "user"); err != nil {
		t.Fatal(err)
	}

	req := &db.Request{
		ID:          "r1",
		UserID:      "u1",
		Title:       "Mistborn",
		Author:      "Brandon Sanderson",
		SearchQuery: "Brandon Sanderson Mistborn",
		Status:      db.StatusDownloading,
	}
	if err := d.CreateRequest(ctx, req); err != nil {
		t.Fatal(err)
	}

	// Set a torrent hash on creation via UpdateRequestStatus.
	if err := d.UpdateRequestStatus(ctx, "r1", db.StatusDownloading,
		db.WithTorrentHash("abc123"),
		db.WithTorrentName("Mistborn.m4b"),
	); err != nil {
		t.Fatal(err)
	}

	got, err := d.GetRequest(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != db.StatusDownloading {
		t.Fatalf("expected downloading, got %s", got.Status)
	}
	if got.TorrentHash.String != "abc123" {
		t.Fatalf("expected hash abc123, got %q", got.TorrentHash.String)
	}

	// Lookup by hash — used by the watcher goroutine.
	byHash, err := d.GetRequestByHash(ctx, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if byHash.ID != "r1" {
		t.Fatalf("GetRequestByHash returned wrong request: %+v", byHash)
	}

	// Transition to done.
	if err := d.UpdateRequestStatus(ctx, "r1", db.StatusDone,
		db.WithFinalPath("/audiobooks/Brandon Sanderson/Mistborn (2006)/"),
		db.WithMetadata(`{"year":2006}`),
	); err != nil {
		t.Fatal(err)
	}

	done, err := d.GetRequest(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != db.StatusDone {
		t.Fatalf("expected done, got %s", done.Status)
	}
	if done.FinalPath.String != "/audiobooks/Brandon Sanderson/Mistborn (2006)/" {
		t.Fatalf("unexpected final path: %q", done.FinalPath.String)
	}

	// ListRequestsByUser.
	list, err := d.ListRequestsByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 request, got %d", len(list))
	}

	// ListAllRequestsWithUser.
	all, err := d.ListAllRequestsWithUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Username != "alice" {
		t.Fatalf("unexpected admin list result: %+v", all)
	}

	// Active downloads — none since we moved to done.
	active, err := d.ListActiveDownloads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active downloads, got %d", len(active))
	}
}

func TestFailStuckImportingRequests(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.CreateUser(ctx, "u1", "alice", "hash", "user"); err != nil {
		t.Fatal(err)
	}

	statuses := []db.RequestStatus{
		db.StatusPending, db.StatusDownloading, db.StatusImporting, db.StatusImporting, db.StatusDone, db.StatusFailed,
	}
	for i, s := range statuses {
		id := fmt.Sprintf("r%d", i)
		if err := d.CreateRequest(ctx, &db.Request{
			ID:          id,
			UserID:      "u1",
			Title:       "Book",
			Author:      "Author",
			SearchQuery: "Book Author",
			Status:      s,
		}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := d.FailStuckImportingRequests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 rows affected, got %d", n)
	}

	// Both importing requests must now be failed with the expected error text.
	for _, id := range []string{"r2", "r3"} {
		req, err := d.GetRequest(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if req.Status != db.StatusFailed {
			t.Errorf("%s: status=%q want %q", id, req.Status, db.StatusFailed)
		}
		if !req.Error.Valid || req.Error.String == "" {
			t.Errorf("%s: expected non-empty error message", id)
		}
	}

	// All other statuses must be unchanged.
	unchanged := map[string]db.RequestStatus{
		"r0": db.StatusPending,
		"r1": db.StatusDownloading,
		"r4": db.StatusDone,
		"r5": db.StatusFailed,
	}
	for id, want := range unchanged {
		req, err := d.GetRequest(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if req.Status != want {
			t.Errorf("%s: status=%q want %q", id, req.Status, want)
		}
	}
}

func TestDeleteRequest(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.CreateUser(ctx, "u1", "alice", "hash", "user"); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateRequest(ctx, &db.Request{
		ID:          "r1",
		UserID:      "u1",
		Title:       "Book",
		Author:      "Author",
		SearchQuery: "Book Author",
		Status:      db.StatusDone,
	}); err != nil {
		t.Fatal(err)
	}

	// Non-existent ID returns ErrNotFound.
	if err := d.DeleteRequest(ctx, "nope"); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing id, got %v", err)
	}

	// Delete the real row.
	if err := d.DeleteRequest(ctx, "r1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Row is gone.
	if _, err := d.GetRequest(ctx, "r1"); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
