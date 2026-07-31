package gm

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type authFile struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type session struct {
	User string
	Exp  time.Time
}

type authState struct {
	mu       sync.Mutex
	user     string
	pass     string
	sessions map[string]session
}

func loadAuth(configDir string) *authState {
	a := &authState{
		user:     "admin",
		pass:     "admin",
		sessions: make(map[string]session),
	}
	path := filepath.Join(configDir, "gm_auth.json")
	b, err := os.ReadFile(path)
	if err == nil {
		var f authFile
		if json.Unmarshal(b, &f) == nil {
			if f.Username != "" {
				a.user = f.Username
			}
			if f.Password != "" {
				a.pass = f.Password
			}
		}
	}
	return a
}

func (a *authState) login(user, pass string) (token string, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if user != a.user || pass != a.pass {
		return "", false
	}
	tok := randomToken()
	a.sessions[tok] = session{User: user, Exp: time.Now().Add(12 * time.Hour)}
	return tok, true
}

func (a *authState) logout(tok string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, tok)
}

func (a *authState) check(tok string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[tok]
	if !ok || time.Now().After(s.Exp) {
		if ok {
			delete(a.sessions, tok)
		}
		return "", false
	}
	return s.User, true
}

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const cookieName = "gm_token"

func (s *Server) tokenFrom(r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		return c.Value
	}
	if t := r.Header.Get("X-GM-Token"); t != "" {
		return t
	}
	return r.URL.Query().Get("token")
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := s.tokenFrom(r)
		user, ok := s.auth.check(tok)
		if !ok {
			writeErr(w, 401, "未登录")
			return
		}
		r.Header.Set("X-GM-User", user)
		next(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	tok, ok := s.auth.login(req.Username, req.Password)
	if !ok {
		writeErr(w, 401, "账号或密码错误")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   12 * 3600,
	})
	writeJSON(w, map[string]any{"ok": true, "token": tok, "user": req.Username})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	tok := s.tokenFrom(r)
	s.auth.logout(tok)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	tok := s.tokenFrom(r)
	user, ok := s.auth.check(tok)
	if !ok {
		writeErr(w, 401, "未登录")
		return
	}
	// P5 前：单文件账号视为超级管理员，前端可按权隐藏按钮
	writeJSON(w, map[string]any{
		"ok":   true,
		"user": user,
		"role": "super",
		"perms": []string{
			"dashboard.read", "player.read", "player.ban", "player.password", "player.nickname",
			"player.transfer", "player.delete", "pet.read", "pet.grant", "pet.edit", "pet.delete", "pet.clear",
			"item.read", "item.grant", "item.delete", "currency.edit",
			"online.read", "online.kick", "online.teleport", "server.double",
			"mail.send", "mail.delete", "gm.manage", "audit.read",
		},
	})
}
