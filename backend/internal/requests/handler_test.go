package requests_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"shelfarr/internal/auth"
	"shelfarr/internal/db"
	"shelfarr/internal/prowlarr"
	"shelfarr/internal/qbit"
	"shelfarr/internal/requests"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func testTokenCfg() auth.TokenConfig {
	return auth.TokenConfig{Secret: []byte("test-secret"), Expiry: time.Hour}
}

// seedUser creates a user in the DB and returns their ID.
func seedUser(t *testing.T, d *db.DB, username, role string) string {
	t.Helper()
	id := "user-" + username
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.CreateUser(context.Background(), id, username, hash, role); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedRequest inserts a request directly into the DB and returns its ID.
func seedRequest(t *testing.T, d *db.DB, userID, title, author string) string {
	t.Helper()
	id := uuid.NewString()
	if err := d.CreateRequest(context.Background(), &db.Request{
		ID:          id,
		UserID:      userID,
		Title:       title,
		Author:      author,
		SearchQuery: title + " " + author,
		Status:      db.StatusDownloading,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// authReq builds an *http.Request with JWT claims injected into the context via
// the Authenticate middleware. The caller receives a request ready to be served
// directly to a handler function.
func authReq(t *testing.T, method, target, body string, cfg auth.TokenConfig, userID, username, role string) *http.Request {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}

	tokenStr, err := auth.NewToken(cfg, userID, username, role)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: tokenStr})

	var out *http.Request
	auth.Authenticate(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		out = r
	})).ServeHTTP(httptest.NewRecorder(), req)
	if out == nil {
		t.Fatal("auth middleware rejected test request")
	}
	return out
}

// withID injects a chi URL parameter named "id" into the request context,
// matching what chi sets when routing /api/requests/{id}.
func withID(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// fakeQBitServer starts a server that accepts qBittorrent login and
// add-torrent requests successfully.
func fakeQBitServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/app/preferences":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"save_path":"/downloads"}`)
		case "/api/v2/torrents/add":
			fmt.Fprint(w, "Ok.")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// seedProwlarrCache creates a fake Prowlarr search server, runs a single
// search to populate the in-memory GUID cache, and returns the ready client.
func seedProwlarrCache(t *testing.T, guid, downloadURL, torrentTitle string) *prowlarr.Client {
	t.Helper()
	type rawRel struct {
		GUID        string `json:"guid"`
		Title       string `json:"title"`
		Seeders     int    `json:"seeders"`
		DownloadURL string `json:"downloadUrl"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]rawRel{{
			GUID:        guid,
			Title:       torrentTitle,
			Seeders:     10,
			DownloadURL: downloadURL,
		}})
	}))
	t.Cleanup(srv.Close)

	pc := prowlarr.New(srv.URL, "key")
	if _, err := pc.Search(context.Background(), "test"); err != nil {
		t.Fatalf("seed prowlarr cache: %v", err)
	}
	return pc
}

// ── Submit ────────────────────────────────────────────────────────────────────

const (
	testGUID         = "test-guid-abc123"
	testMagnet       = "magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd&dn=Test"
	testExpectedHash = "aabbccddeeff00112233445566778899aabbccdd"
	testTorrentTitle = "Test Book - Author Name"
)

func TestSubmitOK(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")

	pc := seedProwlarrCache(t, testGUID, testMagnet, testTorrentTitle)
	qc := qbit.New(fakeQBitServer(t).URL, "admin", "pass")
	h := requests.New(d, pc, qc, "/downloads")

	req := authReq(t, http.MethodPost, "/api/requests",
		fmt.Sprintf(`{"title":"Test Book","author":"Author Name","torrentGuid":%q}`, testGUID),
		cfg, aliceID, "alice", "user")

	rr := httptest.NewRecorder()
	h.Submit(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", rr.Code, rr.Body)
	}

	var resp struct {
		ID          string  `json:"id"`
		Title       string  `json:"title"`
		Author      string  `json:"author"`
		Status      string  `json:"status"`
		TorrentHash *string `json:"torrentHash"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "downloading" {
		t.Errorf("status=%q want %q", resp.Status, "downloading")
	}
	if resp.TorrentHash == nil || *resp.TorrentHash != testExpectedHash {
		t.Errorf("torrentHash=%v want %q", resp.TorrentHash, testExpectedHash)
	}
	if resp.Title != "Test Book" {
		t.Errorf("title=%q want %q", resp.Title, "Test Book")
	}
	if resp.ID == "" {
		t.Error("expected non-empty id")
	}
}

func TestSubmitBadJSON(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	userID := seedUser(t, d, "alice", "user")
	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")

	req := authReq(t, http.MethodPost, "/api/requests", "not json", cfg, userID, "alice", "user")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSubmitMissingFields(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	userID := seedUser(t, d, "alice", "user")
	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")

	cases := []string{
		`{"author":"A","torrentGuid":"g"}`, // missing title
		`{"title":"T","torrentGuid":"g"}`,  // missing author
		`{"title":"T","author":"A"}`,       // missing torrentGuid
	}
	for _, body := range cases {
		req := authReq(t, http.MethodPost, "/api/requests", body, cfg, userID, "alice", "user")
		rr := httptest.NewRecorder()
		h.Submit(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %q: expected 400, got %d", body, rr.Code)
		}
	}
}

func TestSubmitGUIDNotFound(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	userID := seedUser(t, d, "alice", "user")
	// Client with an empty cache (no search performed).
	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")

	req := authReq(t, http.MethodPost, "/api/requests",
		`{"title":"T","author":"A","torrentGuid":"unknown-guid"}`,
		cfg, userID, "alice", "user")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSubmitQBitDown(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")

	pc := seedProwlarrCache(t, testGUID, testMagnet, testTorrentTitle)
	qc := qbit.New("http://127.0.0.1:1", "admin", "pass") // unreachable
	h := requests.New(d, pc, qc, "/downloads")

	req := authReq(t, http.MethodPost, "/api/requests",
		fmt.Sprintf(`{"title":"Test Book","author":"Author Name","torrentGuid":%q}`, testGUID),
		cfg, aliceID, "alice", "user")
	rr := httptest.NewRecorder()
	h.Submit(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rr.Code)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestListUserSeesOwnOnly(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")
	bobID := seedUser(t, d, "bob", "user")

	seedRequest(t, d, aliceID, "Alice Book 1", "Author A")
	seedRequest(t, d, aliceID, "Alice Book 2", "Author A")
	seedRequest(t, d, bobID, "Bob Book", "Author B")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodGet, "/api/requests", "", cfg, aliceID, "alice", "user")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var results []map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for alice, got %d", len(results))
	}
	for _, r := range results {
		if r["title"] == "Bob Book" {
			t.Error("alice should not see bob's request")
		}
	}
}

func TestListAdminSeesAll(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")
	adminID := seedUser(t, d, "admin", "admin")

	seedRequest(t, d, aliceID, "Alice Book", "Author A")
	seedRequest(t, d, adminID, "Admin Book", "Author B")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodGet, "/api/requests", "", cfg, adminID, "admin", "admin")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var results []map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 total results for admin, got %d", len(results))
	}
	// Admin results must include the "username" field.
	for _, r := range results {
		if _, ok := r["username"]; !ok {
			t.Error("admin list response should include 'username' field")
		}
	}
}

func TestListEmptyForNewUser(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	userID := seedUser(t, d, "new", "user")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodGet, "/api/requests", "", cfg, userID, "new", "user")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var results []any
	if err := json.NewDecoder(rr.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty list for new user, got %d items", len(results))
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGetOwnRequest(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	userID := seedUser(t, d, "alice", "user")
	reqID := seedRequest(t, d, userID, "My Book", "Author")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodGet, "/api/requests/"+reqID, "", cfg, userID, "alice", "user")
	req = withID(req, reqID)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["id"] != reqID {
		t.Errorf("id=%v want %q", resp["id"], reqID)
	}
	if resp["title"] != "My Book" {
		t.Errorf("title=%v want %q", resp["title"], "My Book")
	}
}

func TestGetForbiddenForOtherUser(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")
	bobID := seedUser(t, d, "bob", "user")
	reqID := seedRequest(t, d, aliceID, "Alice Book", "Author")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	// Bob attempts to access Alice's request.
	req := authReq(t, http.MethodGet, "/api/requests/"+reqID, "", cfg, bobID, "bob", "user")
	req = withID(req, reqID)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestGetAdminCanAccessAnyRequest(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")
	adminID := seedUser(t, d, "admin", "admin")
	reqID := seedRequest(t, d, aliceID, "Alice Book", "Author")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodGet, "/api/requests/"+reqID, "", cfg, adminID, "admin", "admin")
	req = withID(req, reqID)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestGetNotFound(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	userID := seedUser(t, d, "alice", "user")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodGet, "/api/requests/doesnotexist", "", cfg, userID, "alice", "user")
	req = withID(req, "doesnotexist")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// ── ListWatchDir ──────────────────────────────────────────────────────────────

func TestListWatchDirNotConfigured(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	adminID := seedUser(t, d, "admin", "admin")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "")
	// no SetImportConfig call → watchDir is empty

	req := authReq(t, http.MethodGet, "/api/watchdir", "", cfg, adminID, "admin", "admin")
	rr := httptest.NewRecorder()
	h.ListWatchDir(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rr.Code)
	}
}

func TestListWatchDirFiltersKnown(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	adminID := seedUser(t, d, "admin", "admin")

	// Create a temp dir with two entries.
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/Tracked Book", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir+"/Untracked Book", 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed a request whose torrent_name matches the first entry.
	trackedID := seedUser(t, d, "bob", "user")
	if err := d.CreateRequest(context.Background(), &db.Request{
		ID:          uuid.NewString(),
		UserID:      trackedID,
		Title:       "Tracked Book",
		Author:      "Author",
		SearchQuery: "Tracked Book Author",
		TorrentName: sql.NullString{String: "Tracked Book", Valid: true},
		Status:      db.StatusDownloading,
	}); err != nil {
		t.Fatal(err)
	}

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "")
	h.SetImportConfig(context.Background(), dir, nil, nil)

	req := authReq(t, http.MethodGet, "/api/watchdir", "", cfg, adminID, "admin", "admin")
	rr := httptest.NewRecorder()
	h.ListWatchDir(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}
	var results []map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 untracked entry, got %d", len(results))
	}
	if results[0]["name"] != "Untracked Book" {
		t.Errorf("expected 'Untracked Book', got %v", results[0]["name"])
	}
}

// ── Import ────────────────────────────────────────────────────────────────────

func TestImportNotConfigured(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	adminID := seedUser(t, d, "admin", "admin")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "")

	req := authReq(t, http.MethodPost, "/api/import",
		`{"torrentName":"My Book","title":"My Book","author":"Author"}`,
		cfg, adminID, "admin", "admin")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rr.Code)
	}
}

func TestImportCreatesRequestAndRunsPipeline(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	adminID := seedUser(t, d, "admin", "admin")

	var pipelineCalled string // records the torrentName passed to onImport

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "")
	h.SetImportConfig(context.Background(), t.TempDir(),
		func(_ context.Context, _ *db.Request, torrentName string) error {
			pipelineCalled = torrentName
			return nil
		},
		nil,
	)
	// Run synchronously for deterministic assertions.
	requests.SetLaunch(h, func(f func()) { f() })

	req := authReq(t, http.MethodPost, "/api/import",
		`{"torrentName":"My Audiobook","title":"My Book","author":"Great Author"}`,
		cfg, adminID, "admin", "admin")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d — body: %s", rr.Code, rr.Body)
	}
	if pipelineCalled != "My Audiobook" {
		t.Errorf("pipeline torrentName=%q want %q", pipelineCalled, "My Audiobook")
	}

	// After sync pipeline, status should be "done".
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	id, _ := resp["id"].(string)
	saved, err := d.GetRequest(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != db.StatusDone {
		t.Errorf("status=%q want %q", saved.Status, db.StatusDone)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestDeleteOwnRequest(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")
	reqID := seedRequest(t, d, aliceID, "My Book", "Author")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodDelete, "/api/requests/"+reqID, "", cfg, aliceID, "alice", "user")
	req = withID(req, reqID)
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d — body: %s", rr.Code, rr.Body)
	}
	// Row must be gone.
	if _, err := d.GetRequest(context.Background(), reqID); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteForbiddenForOtherUser(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")
	bobID := seedUser(t, d, "bob", "user")
	reqID := seedRequest(t, d, aliceID, "Alice Book", "Author")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodDelete, "/api/requests/"+reqID, "", cfg, bobID, "bob", "user")
	req = withID(req, reqID)
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestDeleteAdminCanDeleteAnyRequest(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	aliceID := seedUser(t, d, "alice", "user")
	adminID := seedUser(t, d, "admin", "admin")
	reqID := seedRequest(t, d, aliceID, "Alice Book", "Author")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodDelete, "/api/requests/"+reqID, "", cfg, adminID, "admin", "admin")
	req = withID(req, reqID)
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d — body: %s", rr.Code, rr.Body)
	}
}

func TestDeleteNotFound(t *testing.T) {
	d := openTestDB(t)
	cfg := testTokenCfg()
	userID := seedUser(t, d, "alice", "user")

	h := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "/dl")
	req := authReq(t, http.MethodDelete, "/api/requests/doesnotexist", "", cfg, userID, "alice", "user")
	req = withID(req, "doesnotexist")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}
