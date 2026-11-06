package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"signalcare/db"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run isolated PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "signal_test_" + newID()
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
		if err != nil {
			t.Error(err)
		}
	})
	sql, err := db.FS.ReadFile("migrations/00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(sql), "-- +goose Down")[0]
	if _, err = pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	return &Store{Pool: pool}
}

func request(source, key string) CreateRequest {
	return CreateRequest{Source: source, ClientRequestID: key, WaitSeconds: 15}
}

func TestIdempotencyAlsoCoversRequestsReusingAnActiveAlert(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, err := s.CreateAlert(ctx, request("manual", "request-original"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateAlert(ctx, request("manual", "request-reused"))
	if err != nil || b.ID != a.ID {
		t.Fatalf("reused alert: %v, %s", err, b.ID)
	}
	if _, err = s.Transition(ctx, a.ID, "acknowledge", "dispatcher"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Transition(ctx, a.ID, "close", "dispatcher"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"request-original", "request-reused"} {
		retried, err := s.CreateAlert(ctx, request("manual", key))
		if err != nil || retried.ID != a.ID || retried.Status != "closed" {
			t.Fatalf("retry after close created a different alert: %+v, %v", retried, err)
		}
	}
	var count int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM alerts`).Scan(&count)
	if count != 1 {
		t.Fatalf("want 1 alert, got %d", count)
	}
}

func TestConcurrentRequestsCreateOnlyOneActiveAlert(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.CreateAlert(ctx, request("manual", newID())); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var alerts, requests int
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM alerts),(SELECT count(*) FROM alert_requests)`).Scan(&alerts, &requests); err != nil {
		t.Fatal(err)
	}
	if alerts != 1 || requests != 8 {
		t.Fatalf("alerts=%d, requests=%d", alerts, requests)
	}
}

func TestDueTaskSurvivesWorkerReplacementAndOnlyEmitsOnce(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, err := s.CreateAlert(ctx, request("simulation", "persisted-task"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "awaiting_response" {
		t.Fatal(a.Status)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE alerts SET deadline_at=now()-interval '1 second' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	restarted := &Store{Pool: s.Pool}
	for i := 0; i < 2; i++ {
		if err = restarted.ProcessDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	d, err := s.Dashboard(ctx, "user")
	if err != nil {
		t.Fatal(err)
	}
	if d.Alerts[0].Status != "received" || len(d.Alerts[0].Events) != 2 {
		t.Fatalf("unexpected restored task: %+v", d.Alerts[0])
	}
	if d.Alerts[0].Events[1].Kind != "response_timeout" {
		t.Fatal("missing timeout event")
	}
}

func TestManualSOSBypassesSimulationAndKeepsSnapshot(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, err := s.CreateAlert(ctx, request("simulation", "simulation-first"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := readProfile(ctx, s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	original := p.FullName
	p.FullName = "Изменённая карточка"
	if _, err = s.UpdateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateAlert(ctx, request("manual", "manual-override"))
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != a.ID || b.Status != "received" || b.DeadlineAt != nil || b.Profile.FullName != original {
		t.Fatalf("manual override lost state or snapshot: %+v", b)
	}
	if _, err = s.Transition(ctx, a.ID, "close", "dispatcher"); !errors.Is(err, ErrConflict) {
		t.Fatalf("closed unaccepted alert: %v", err)
	}
}

func TestCancelPreventsEscalationAndOnlyOneDispatcherAccepts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, err := s.CreateAlert(ctx, request("simulation", "cancel-me"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Transition(ctx, a.ID, "cancel", "user"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE alerts SET deadline_at=now()-interval '1 second' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ProcessDue(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateAlert(ctx, request("manual", "accept-race"))
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := s.Transition(ctx, b.ID, "acknowledge", "dispatcher"); results <- err }()
	}
	wins, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	d, err := s.Dashboard(ctx, "dispatcher")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range d.Alerts {
		if v.ID == a.ID && v.Status != "cancelled" {
			t.Fatal("cancelled task escalated")
		}
	}
}

func TestSessionsRoleBoundariesCSRFAndEventReplay(t *testing.T) {
	s := testStore(t)
	app := New(s, Config{UserPassword: "user-test", DispatcherPassword: "dispatch-test"})
	h := app.Handler()
	call := func(method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Signal-Client", "web")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/user/dashboard", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	login := call("POST", "/api/auth/login", `{"role":"user","password":"user-test"}`, nil, "")
	if login.Code != 200 || len(login.Result().Cookies()) != 1 {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	if !cookie.HttpOnly {
		t.Fatal("session cookie readable by JavaScript")
	}
	if w := call("GET", "/api/dispatcher/dashboard", "", cookie, ""); w.Code != 401 {
		t.Fatal("user session granted dispatcher access")
	}
	if w := call("POST", "/api/user/alerts", `{"source":"manual","client_request_id":"csrf-reject"}`, cookie, "https://foreign.example"); w.Code != 403 {
		t.Fatal("cross-site write was accepted")
	}
	before, err := s.Dashboard(context.Background(), "user")
	if err != nil {
		t.Fatal(err)
	}
	w := call("POST", "/api/user/alerts", `{"source":"manual","client_request_id":"http-sos-test","wait_seconds":0,"location":null}`, cookie, "")
	if w.Code != 200 {
		t.Fatalf("SOS: %d %s", w.Code, w.Body.String())
	}
	var a Alert
	if err = json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	missed, err := s.EventsAfter(context.Background(), before.Cursor)
	if err != nil || len(missed) != 1 || missed[0].AlertID != a.ID {
		t.Fatalf("cannot replay missed event: %v %+v", err, missed)
	}
	if w = call("POST", "/api/auth/logout", `{"role":"user"}`, cookie, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = call("GET", "/api/user/dashboard", "", cookie, ""); w.Code != 401 {
		t.Fatal("revoked session still works")
	}
}

func TestValidationRejectsUnusableLocationWithoutSaving(t *testing.T) {
	request := CreateRequest{Source: "manual", ClientRequestID: "location-test", Location: &Location{Latitude: 55, Longitude: 37, Accuracy: 20, Captured: time.Now().Add(-time.Hour)}}
	if validateCreate(request) == "" {
		t.Fatal("stale location accepted")
	}
	request.Location.Captured = time.Now()
	request.Location.Latitude = 100
	if validateCreate(request) == "" {
		t.Fatal("invalid latitude accepted")
	}
}
