package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	jwtKeyFilename = "jwt.key"
	authPassKey    = "auth.password_hash"
	jwtTTL         = 7 * 24 * time.Hour
	keyChars       = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// 下限与 setup 页的前端校验一致——校验只放在前端等于没放，直接打 API 就能
	// 绕过。上限是 bcrypt 的硬限制：超过 72 字节 GenerateFromPassword 会返回
	// ErrPasswordTooLong，提前挡掉好过让用户收到一句含义不明的「密码加密失败」。
	minPasswordRunes = 8
	maxPasswordBytes = 72
)

type authIdentity struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type authContextKey struct{}

var errIdentityDataSource = errors.New("identity data source unavailable")

const (
	adminUsername  = "yunkun"
	authCookieName = "yunkun_token"
)

type authClaims struct {
	Version string `json:"version"`
	jwt.RegisteredClaims
}

func passwordVersion(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return hex.EncodeToString(sum[:])
}

func signIdentityJWT(key []byte, subject, version string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, authClaims{
		Version: version,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: subject, ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtTTL)),
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}).SignedString(key)
}

func parseIdentityJWT(token string, key []byte) (*authClaims, error) {
	claims := &authClaims{}
	t, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !t.Valid || claims.ExpiresAt == nil {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func (s *Server) identity(token string) (*authIdentity, error) {
	claims, err := parseIdentityJWT(token, s.jwtKey)
	if err != nil {
		return nil, err
	}
	if s.m == nil || s.m.pg == nil {
		return nil, errIdentityDataSource
	}
	if claims.Subject == adminUsername {
		hash, _, err := s.m.pg.GetSetting(authPassKey)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errIdentityDataSource, err)
		}
		// Uninitialised test installations may use a token without a password version.
		// Configured installations require a password-bound token after upgrade.
		if hash != "" && claims.Version != passwordVersion(hash) {
			return nil, fmt.Errorf("admin credentials changed")
		}
		return &authIdentity{Username: adminUsername, Role: "admin"}, nil
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid subject")
	}
	user, err := s.m.pg.AuthUserByID(id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errIdentityDataSource, err)
	}
	if user == nil || user.Disabled || claims.Version != strconv.FormatInt(user.TokenVersion, 10) ||
		(user.Role != "operator" && user.Role != "viewer") {
		return nil, fmt.Errorf("user disabled or credentials changed")
	}
	return &authIdentity{ID: user.ID, Username: user.Username, Role: user.Role}, nil
}

// Non-admin privileges are intentionally narrow. New API routes default to
// administrator-only until reviewed for sensitive data and side effects.
func roleAllows(role, method, path string) bool {
	if role == "admin" {
		return true
	}
	taskPath := strings.TrimPrefix(path, "/api/tasks/")
	taskParts := strings.Split(taskPath, "/")
	taskID := len(taskParts) > 0 && taskParts[0] != ""
	if taskID {
		id, err := strconv.ParseInt(taskParts[0], 10, 64)
		taskID = err == nil && id > 0
	}
	if method != http.MethodGet {
		if role != "operator" {
			return false
		}
		switch {
		case method == http.MethodPost && path == "/api/tasks",
			method == http.MethodPost && path == "/api/tasks/control/batch":
			return true
		case taskID && len(taskParts) == 2 && taskParts[1] == "control":
			return method == http.MethodPost
		case taskID && len(taskParts) == 3 && taskParts[1] == "network" && taskParts[2] == "lease":
			return method == http.MethodPost
		default:
			return false
		}
	}
	if path == "/api/auth/me" || path == "/api/health" || path == "/api/tasks" || path == "/api/proxy-pools" ||
		path == "/api/proxy-settings" ||
		path == "/api/task-categories" || path == "/api/task-templates" {
		return true
	}
	// Proxy node addresses, route events, discovery candidates and source
	// metadata are sensitive egress data. Keep the list-level pool view
	// readable while requiring administrator access to these subresources.
	if strings.HasPrefix(path, "/api/proxy-pools/") || strings.HasPrefix(path, "/api/proxy-discovery/") {
		return false
	}
	if taskID {
		if len(taskParts) == 1 {
			return true
		}
		return len(taskParts) == 2 && (taskParts[1] == "runtime-metrics" || taskParts[1] == "network")
	}
	if path == "/api/exploration/findings" || path == "/api/exploration/activity/history" ||
		(strings.HasPrefix(path, "/api/exploration/findings/") && strings.HasSuffix(path, "/traffic")) {
		return true
	}
	return false
}

// errDataSourceUnavailable 是密码相关读操作失败时统一的回复。这些 handler 绝不能
// 把"读不到"当成"没有设置"：authInit 曾因此在数据库报错时放行，让未认证请求覆盖
// 掉已有的管理员密码。
const errDataSourceUnavailable = "数据源暂时不可用，请稍后重试"

// validatePassword 返回空串表示通过，否则返回可直接展示给用户的中文原因。
func validatePassword(pw string) string {
	if utf8.RuneCountInString(pw) < minPasswordRunes {
		return fmt.Sprintf("密码长度至少 %d 位", minPasswordRunes)
	}
	if len(pw) > maxPasswordBytes {
		return fmt.Sprintf("密码长度不能超过 %d 字节", maxPasswordBytes)
	}
	return ""
}

// loadOrCreateJWTKey reads the 32-byte signing key from keyDir/jwt.key. keyDir is
// the project base dir (next to the executable), NOT the browsable workspace root
// (dataDir) — the signing key must never be listable/downloadable via the file
// manager. Legacy installs kept it at dataDir/jwt.key; if present there and not yet
// at the new location, it is migrated (key preserved, so sessions stay valid) and
// the old file removed so it disappears from the workspace. On first run a random
// key is generated and persisted.
func loadOrCreateJWTKey(keyDir, dataDir string) ([]byte, error) {
	path := filepath.Join(keyDir, jwtKeyFilename)
	// one-time migration out of the old in-workspace location.
	if legacy := filepath.Join(dataDir, jwtKeyFilename); legacy != path {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if data, rerr := os.ReadFile(legacy); rerr == nil {
				if werr := os.WriteFile(path, data, 0o600); werr == nil {
					_ = os.Remove(legacy)
					log.Printf("[auth] JWT key 已从 %s 迁移到 %s（移出可浏览工作区）", legacy, path)
				}
			}
		}
	}
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return []byte(strings.TrimSpace(string(data))), nil
	}
	buf := make([]byte, 32)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(keyChars))))
		if err != nil {
			return nil, fmt.Errorf("generate jwt key: %w", err)
		}
		buf[i] = keyChars[n.Int64()]
	}
	if err := os.WriteFile(path, buf, 0600); err != nil {
		return nil, fmt.Errorf("write jwt key: %w", err)
	}
	log.Printf("[auth] 新 JWT key 已写入 %s", path)
	return buf, nil
}

// signJWT issues a 7-day HS256 token for the administrator.
func signJWT(key []byte) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   adminUsername,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}).SignedString(key)
}

// verifyJWT returns true when tokenStr is a valid, non-expired HS256 token.
func verifyJWT(tokenStr string, key []byte) bool {
	t, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return key, nil
	})
	return err == nil && t.Valid
}

// extractToken reads the JWT from Authorization: Bearer header,
// the application cookie, or ?token= for SSE connections.
func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie(authCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return r.URL.Query().Get("token")
}

// requireAuth wraps h with database-backed identity and role validation.
func (s *Server) requireAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/api/auth/status" || p == "/api/auth/init" || p == "/api/auth/login" || p == "/api/health" {
			h.ServeHTTP(w, r)
			return
		}
		tok := extractToken(r)
		if tok == "" {
			writeErr(w, 401, "未授权")
			return
		}
		identity, err := s.identity(tok)
		if err != nil {
			writeIdentityError(w, err)
			return
		}
		if !roleAllows(identity.Role, r.Method, p) {
			writeErr(w, 403, "权限不足")
			return
		}
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, identity)))
	})
}

func writeIdentityError(w http.ResponseWriter, err error) {
	if errors.Is(err, errIdentityDataSource) {
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusServiceUnavailable, errDataSourceUnavailable)
		return
	}
	writeErr(w, http.StatusUnauthorized, "token 无效或已过期")
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	identity, _ := r.Context().Value(authContextKey{}).(*authIdentity)
	if identity == nil {
		writeErr(w, 401, "未授权")
		return
	}
	writeJSON(w, 200, identity)
}

func (s *Server) authUsers(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	users, err := pg.ListAuthUsers()
	if err != nil {
		writeErr(w, 500, "读取用户失败")
		return
	}
	writeJSON(w, 200, users)
}

func (s *Server) authCreateUser(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if decode(r, &req) != nil || req.Username == "" || req.Username == adminUsername ||
		len(req.Username) > 64 || strings.TrimSpace(req.Username) != req.Username ||
		(req.Role != "operator" && req.Role != "viewer") {
		writeErr(w, 400, "用户参数无效")
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "密码加密失败")
		return
	}
	user, err := pg.CreateAuthUser(req.Username, string(hash), req.Role)
	if err != nil {
		writeErr(w, 409, "用户已存在或保存失败")
		return
	}
	writeJSON(w, 201, user)
}

func (s *Server) authUpdateUser(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "用户 ID 无效")
		return
	}
	var req struct {
		Role     string `json:"role"`
		Disabled bool   `json:"disabled"`
		Password string `json:"password"`
	}
	if decode(r, &req) != nil || (req.Role != "operator" && req.Role != "viewer") {
		writeErr(w, 400, "角色无效")
		return
	}
	if req.Password != "" {
		if msg := validatePassword(req.Password); msg != "" {
			writeErr(w, 400, msg)
			return
		}
	}
	user, err := pg.UpdateAuthUser(id, req.Role, req.Disabled)
	if err != nil {
		writeErr(w, 500, "保存用户失败")
		return
	}
	if user == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil || pg.SetAuthUserPassword(id, string(hash)) != nil {
			writeErr(w, 500, "保存密码失败")
			return
		}
		user, err = pg.AuthUserByID(id)
		if err != nil {
			writeErr(w, 500, "读取用户失败")
			return
		}
	}
	writeJSON(w, 200, user)
}

// GET /api/auth/status — reports whether the admin password has been initialised.
// 读失败必须回 503 而不是 initialized:false：前端在 initialized:false 时会把用户
// 送到 /setup 去设置密码（login/page.tsx），把数据库故障包装成 200 等于把用户往
// 覆盖已有密码的路上推。
func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	hash, _, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]any{"initialized": hash != ""})
}

// POST /api/auth/init — sets the password for the first time; rejected if already set.
func (s *Server) authInit(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	existing, _, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if existing != "" {
		writeErr(w, 403, "密码已设置")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil || req.Password == "" {
		writeErr(w, 400, "密码不能为空")
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "密码加密失败")
		return
	}
	// 用 INSERT ... ON CONFLICT DO NOTHING 而不是 upsert：上面那次 GetSetting 只是
	// 快速失败路径，真正"仅首次可设"的保证落在主键约束上。bcrypt 要跑几十毫秒，
	// 这期间别的请求完全可能先把密码设好，而读检查本身也可能因故障而失效。
	inserted, err := pg.InsertSettingIfAbsent(authPassKey, string(hash))
	if err != nil {
		writeErr(w, 500, "保存失败: "+err.Error())
		return
	}
	if !inserted {
		writeErr(w, 403, "密码已设置")
		return
	}
	tok, err := signIdentityJWT(s.jwtKey, adminUsername, passwordVersion(string(hash)))
	if err != nil {
		writeErr(w, 500, "token 生成失败")
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok})
}

// POST /api/auth/change-password — changes the admin password. Requires a valid
// token (this route is under /api/auth/* which requireAuth exempts, so the token
// is validated here) AND the current password.
func (s *Server) authChangePassword(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	identity, err := s.identity(extractToken(r))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	if identity.Role != "admin" {
		writeErr(w, 403, "权限不足")
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if req.NewPassword == "" {
		writeErr(w, 400, "新密码不能为空")
		return
	}
	if msg := validatePassword(req.NewPassword); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, ok, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if !ok || hash == "" {
		writeErr(w, 403, "密码未初始化，请先设置密码")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)); err != nil {
		writeErr(w, 401, "当前密码错误")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "密码加密失败")
		return
	}
	if err := pg.SetSetting(authPassKey, string(newHash)); err != nil {
		writeErr(w, 500, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/auth/login — validates username/password and returns a JWT.
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if req.Username != adminUsername {
		user, err := pg.AuthUserByName(req.Username)
		if err != nil {
			writeErr(w, 503, errDataSourceUnavailable)
			return
		}
		if user == nil || user.Disabled || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			writeErr(w, 401, "用户名或密码错误")
			return
		}
		tok, err := signIdentityJWT(s.jwtKey, strconv.FormatInt(user.ID, 10), strconv.FormatInt(user.TokenVersion, 10))
		if err != nil {
			writeErr(w, 500, "token 生成失败")
			return
		}
		writeJSON(w, 200, map[string]any{"token": tok})
		return
	}
	hash, ok, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if !ok || hash == "" {
		writeErr(w, 403, "密码未初始化，请先设置密码")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	tok, err := signIdentityJWT(s.jwtKey, adminUsername, passwordVersion(hash))
	if err != nil {
		writeErr(w, 500, "token 生成失败")
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok})
}
