package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Config struct {
	UserPassword       string
	DispatcherPassword string
	WebDir             string
	SecureCookies      bool
}

type Server struct {
	Store    *Store
	Config   Config
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func New(store *Store, config Config) *Server {
	return &Server{Store: store, Config: config, attempts: make(map[string][]time.Time)}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.Store.Pool.Ping(ctx); err != nil {
			writeError(w, 503, "База данных временно недоступна.")
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ok", "mode": "training", "version": "0.1.0"})
	})
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	for _, role := range []string{"user", "dispatcher"} {
		mux.HandleFunc("GET /api/"+role+"/dashboard", s.requireRole(role, s.dashboard(role)))
		mux.HandleFunc("GET /api/"+role+"/events", s.requireRole(role, s.events))
	}
	mux.HandleFunc("PUT /api/user/profile", s.requireRole("user", s.profile))
	mux.HandleFunc("POST /api/user/alerts", s.requireRole("user", s.createAlert))
	mux.HandleFunc("POST /api/user/alerts/{id}/cancel", s.requireRole("user", s.transition("cancel", "user")))
	mux.HandleFunc("POST /api/dispatcher/alerts/{id}/acknowledge", s.requireRole("dispatcher", s.transition("acknowledge", "dispatcher")))
	mux.HandleFunc("POST /api/dispatcher/alerts/{id}/close", s.requireRole("dispatcher", s.transition("close", "dispatcher")))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, 404, "Такого API-метода нет.")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			writeError(w, http.StatusMethodNotAllowed, "Метод не поддерживается.")
			return
		}
		s.static(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("X-Signal-Client") != "web" {
				writeError(w, 403, "Отсутствует заголовок клиента.")
				return
			}
			origin := r.Header.Get("Origin")
			allowed := origin == "http://"+r.Host || origin == "https://"+r.Host || origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173"
			if origin != "" && !allowed {
				writeError(w, 403, "Запрос с другого сайта запрещён.")
				return
			}
			if origin == "" && r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeError(w, 403, "Запрос с другого сайта запрещён.")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeError(w, 415, "Ожидается JSON.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		writeError(w, 400, "Некорректный JSON или слишком большой запрос.")
		return false
	}
	if err = d.Decode(new(any)); err != io.EOF {
		writeError(w, 400, "Ожидается один JSON-объект.")
		return false
	}
	return true
}

func tokenHash(v string) string     { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }
func cookieName(role string) string { return "signal_session_" + role }

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Role != "user" && req.Role != "dispatcher" {
		writeError(w, 400, "Неизвестная роль.")
		return
	}
	if !s.allowLogin(r.RemoteAddr) {
		writeError(w, 429, "Слишком много попыток. Подождите одну минуту.")
		return
	}
	expected := s.Config.UserPassword
	if req.Role == "dispatcher" {
		expected = s.Config.DispatcherPassword
	}
	gotHash, wantHash := sha256.Sum256([]byte(req.Password)), sha256.Sum256([]byte(expected))
	if subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) != 1 {
		writeError(w, 401, "Неверный пароль учебного аккаунта.")
		return
	}
	token := newID() + newID()
	_, err := s.Store.Pool.Exec(r.Context(), `INSERT INTO sessions(token_hash,role,expires_at) VALUES($1,$2,now()+interval '12 hours')`, tokenHash(token), req.Role)
	if err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName(req.Role), Value: token, Path: "/api", HttpOnly: true, Secure: s.Config.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	writeJSON(w, 200, map[string]string{"role": req.Role, "mode": "training"})
}

func (s *Server) allowLogin(remote string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// All requests are local in this prototype. Keep a bounded attempt window.
	now := time.Now()
	key := strings.Split(remote, ":")[0]
	old := s.attempts[key]
	recent := []time.Time{}
	for _, t := range old {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 20 {
		return false
	}
	s.attempts[key] = append(recent, now)
	return true
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Role != "user" && req.Role != "dispatcher" {
		writeError(w, 400, "Неизвестная роль.")
		return
	}
	if c, err := r.Cookie(cookieName(req.Role)); err == nil {
		if _, err = s.Store.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(c.Value)); err != nil {
			s.fail(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName(req.Role), Path: "/api", HttpOnly: true, Secure: s.Config.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) authorized(r *http.Request, role string) (bool, error) {
	c, err := r.Cookie(cookieName(role))
	if err != nil {
		return false, nil
	}
	var exists bool
	err = s.Store.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND role=$2 AND expires_at>now())`, tokenHash(c.Value), role).Scan(&exists)
	return exists, err
}

func (s *Server) requireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, err := s.authorized(r, role)
		if err != nil {
			s.fail(w, err)
			return
		}
		if !ok {
			writeError(w, 401, "Войдите в учебный аккаунт.")
			return
		}
		next(w, r)
	}
}

func (s *Server) dashboard(role string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, err := s.Store.Dashboard(r.Context(), role)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, 200, d)
	}
}

func validateProfile(p Profile) string {
	if strings.TrimSpace(p.FullName) == "" || strings.TrimSpace(p.Address) == "" {
		return "Укажите имя и учебный адрес."
	}
	fields := []struct {
		v string
		n int
	}{{p.FullName, 120}, {p.Address, 500}, {p.Apartment, 30}, {p.Entrance, 30}, {p.Floor, 30}, {p.AccessNotes, 1000}, {p.ContactName, 120}, {p.ContactPhone, 60}, {p.Conditions, 2000}, {p.Allergies, 2000}}
	for _, f := range fields {
		if utf8.RuneCountInString(f.v) > f.n {
			return "Одно из полей превышает допустимую длину."
		}
	}
	if p.BirthDate != "" {
		date, err := time.Parse("2006-01-02", p.BirthDate)
		if err != nil || date.After(time.Now()) || date.Year() < 1900 {
			return "Проверьте дату рождения."
		}
	}
	return ""
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	var p Profile
	if !decode(w, r, &p) {
		return
	}
	p.FullName = strings.TrimSpace(p.FullName)
	p.Address = strings.TrimSpace(p.Address)
	if message := validateProfile(p); message != "" {
		writeError(w, 422, message)
		return
	}
	saved, err := s.Store.UpdateProfile(r.Context(), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, saved)
}

func validateCreate(req CreateRequest) string {
	if req.Source != "manual" && req.Source != "simulation" {
		return "Неизвестный источник тревоги."
	}
	if len(req.ClientRequestID) < 8 || len(req.ClientRequestID) > 100 {
		return "Не указан корректный идентификатор отправки."
	}
	if req.Source == "simulation" && (req.WaitSeconds < 5 || req.WaitSeconds > 120) {
		return "Учебное ожидание должно быть от 5 до 120 секунд."
	}
	if req.Location != nil {
		l := req.Location
		if math.IsNaN(l.Latitude) || math.IsNaN(l.Longitude) || math.IsNaN(l.Accuracy) || math.IsInf(l.Latitude, 0) || math.IsInf(l.Longitude, 0) || math.IsInf(l.Accuracy, 0) || l.Latitude < -90 || l.Latitude > 90 || l.Longitude < -180 || l.Longitude > 180 || l.Accuracy < 0 || l.Accuracy > 100000 {
			return "Некорректные координаты."
		}
		if l.Captured.IsZero() || time.Since(l.Captured) > 10*time.Minute || time.Until(l.Captured) > time.Minute {
			return "Координаты устарели. Обновите местоположение."
		}
	}
	return ""
}

func (s *Server) createAlert(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if !decode(w, r, &req) {
		return
	}
	if msg := validateCreate(req); msg != "" {
		writeError(w, 422, msg)
		return
	}
	a, err := s.Store.CreateAlert(r.Context(), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) transition(action, role string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		if !decode(w, r, &body) {
			return
		}
		a, err := s.Store.Transition(r.Context(), r.PathValue("id"), action, role)
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "Тревога не найдена.")
			return
		}
		if errors.Is(err, ErrConflict) {
			writeError(w, 409, "Состояние уже изменилось. Обновляем данные — проверьте доступные действия.")
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, 200, a)
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "Поток событий не поддерживается.")
		return
	}
	value := r.Header.Get("Last-Event-ID")
	if value == "" {
		value = r.URL.Query().Get("cursor")
	}
	cursor, _ := strconv.ParseInt(value, 10, 64)
	if cursor < 0 {
		cursor = 0
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	f.Flush()
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		events, err := s.Store.EventsAfter(r.Context(), cursor)
		if err != nil {
			return
		}
		for _, e := range events {
			b, _ := json.Marshal(e)
			if _, err = fmt.Fprintf(w, "id: %d\nevent: change\ndata: %s\n\n", e.ID, b); err != nil {
				return
			}
			cursor = e.ID
		}
		if len(events) > 0 {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		case <-heartbeat.C:
			role := "user"
			if strings.Contains(r.URL.Path, "dispatcher") {
				role = "dispatcher"
			}
			valid, err := s.authorized(r, role)
			if err != nil || !valid {
				return
			}
			if _, err = fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			f.Flush()
		}
	}
}

func (s *Server) Worker(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := s.Store.ProcessDue(ctx); err != nil && ctx.Err() == nil {
			slog.Error("background task", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	clean := filepath.Clean("/" + r.URL.Path)
	path := filepath.Join(s.Config.WebDir, clean)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	if filepath.Ext(clean) != "" {
		http.NotFound(w, r)
		return
	}
	index := filepath.Join(s.Config.WebDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.Error(w, "Сначала выполните npm install и npm run build. Для разработки откройте http://127.0.0.1:5173", 503)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	writeError(w, 503, "Сервис временно недоступен. Подтверждения приёма нет — повторите попытку после восстановления связи.")
}
