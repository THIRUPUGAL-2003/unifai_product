package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/raksha/raksha/core/schemas"
	"github.com/raksha/raksha/framework/configstore"
	"github.com/raksha/raksha/framework/configstore/tables"
	"github.com/raksha/raksha/framework/encrypt"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

type testMockLogger struct{}

func (l *testMockLogger) Debug(msg string, args ...any)                                {}
func (l *testMockLogger) Info(msg string, args ...any)                                 {}
func (l *testMockLogger) Warn(msg string, args ...any)                                 {}
func (l *testMockLogger) Error(msg string, args ...any)                                {}
func (l *testMockLogger) Fatal(msg string, args ...any)                                {}
func (l *testMockLogger) SetLevel(level schemas.LogLevel)                              {}
func (l *testMockLogger) SetOutputType(outputType schemas.LoggerOutputType)            {}
func (l *testMockLogger) LogHTTPRequest(level schemas.LogLevel, msg string) schemas.LogEventBuilder {
	return schemas.NoopLogEvent
}

type memoryConfigStore struct {
	configstore.ConfigStore // satisfies any uncalled methods of ConfigStore interface
	mu         sync.RWMutex
	authConfig *configstore.AuthConfig
	users      map[string]*tables.TableUser
	sessions   map[string]*tables.SessionsTable
	lockouts   map[string]*tables.TableLoginLockout
	devices    map[string]bool
	smtpConfig *tables.TableSMTPConfig
	otps       map[uint]*tables.TablePasswordResetOTP
	nextOTPID  uint
}

func newMemoryConfigStore() *memoryConfigStore {
	return &memoryConfigStore{
		users:    make(map[string]*tables.TableUser),
		sessions: make(map[string]*tables.SessionsTable),
		lockouts: make(map[string]*tables.TableLoginLockout),
		devices:  make(map[string]bool),
		otps:     make(map[uint]*tables.TablePasswordResetOTP),
	}
}

func (m *memoryConfigStore) DB() *gorm.DB {
	return nil
}

func (m *memoryConfigStore) GetAuthConfig(ctx context.Context) (*configstore.AuthConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.authConfig, nil
}

func (m *memoryConfigStore) UpdateAuthConfig(ctx context.Context, config *configstore.AuthConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authConfig = config
	return nil
}

func (m *memoryConfigStore) GetUserByUsername(ctx context.Context, username string) (*tables.TableUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[strings.ToLower(strings.TrimSpace(username))]
	if !ok {
		return nil, nil
	}
	copied := *u
	return &copied, nil
}

func (m *memoryConfigStore) AddUser(user *tables.TableUser) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *user
	m.users[strings.ToLower(strings.TrimSpace(user.Username))] = &copied
}

func (m *memoryConfigStore) CreateSession(ctx context.Context, session *tables.SessionsTable) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.Token] = session
	return nil
}

func (m *memoryConfigStore) GetSession(ctx context.Context, token string) (*tables.SessionsTable, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[token]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *memoryConfigStore) DeleteSession(ctx context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
	return nil
}

func (m *memoryConfigStore) DeleteSessionsByUsername(ctx context.Context, username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.sessions {
		if v.Username == username {
			delete(m.sessions, k)
		}
	}
	return nil
}

func (m *memoryConfigStore) FlushSessions(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[string]*tables.SessionsTable)
	return nil
}

func (m *memoryConfigStore) GetLoginLockout(ctx context.Context, key string) (*tables.TableLoginLockout, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.lockouts[key]
	if !ok {
		return nil, nil
	}
	// Return a copy to mimic DB behavior
	copied := *l
	return &copied, nil
}

func (m *memoryConfigStore) UpsertLoginLockout(ctx context.Context, row *tables.TableLoginLockout) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *row
	m.lockouts[row.UsernameKey] = &copied
	return nil
}

func (m *memoryConfigStore) ClearLoginLockout(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.lockouts, key)
	return nil
}

func (m *memoryConfigStore) GetSMTPConfig(ctx context.Context) (*tables.TableSMTPConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.smtpConfig, nil
}

func (m *memoryConfigStore) HasLoginDevice(ctx context.Context, key, fp string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.devices[key+":"+fp], nil
}

func (m *memoryConfigStore) GetUserByEmail(ctx context.Context, email string) (*tables.TableUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return nil, nil
}

func (m *memoryConfigStore) CreateUser(ctx context.Context, user *tables.TableUser, tx ...*gorm.DB) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *user
	if copied.ID == "" {
		copied.ID = fmt.Sprintf("usr_%d", len(m.users)+1)
		user.ID = copied.ID
	}
	m.users[strings.ToLower(strings.TrimSpace(copied.Username))] = &copied
	return nil
}

func (m *memoryConfigStore) UpdateUser(ctx context.Context, user *tables.TableUser, tx ...*gorm.DB) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *user
	if copied.Password == "" {
		if existing, ok := m.users[strings.ToLower(strings.TrimSpace(copied.Username))]; ok {
			copied.Password = existing.Password
		}
	}
	m.users[strings.ToLower(strings.TrimSpace(copied.Username))] = &copied
	return nil
}

func (m *memoryConfigStore) GetUsers(ctx context.Context) ([]*tables.TableUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*tables.TableUser, 0, len(m.users))
	for _, u := range m.users {
		copied := *u
		res = append(res, &copied)
	}
	return res, nil
}

func (m *memoryConfigStore) GetUserByID(ctx context.Context, id string) (*tables.TableUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.ID == id || strings.EqualFold(u.Username, id) {
			copied := *u
			return &copied, nil
		}
	}
	return nil, nil
}

func (m *memoryConfigStore) DeleteUser(ctx context.Context, id string, tx ...*gorm.DB) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, u := range m.users {
		if u.ID == id || u.Username == id {
			delete(m.users, k)
			break
		}
	}
	return nil
}

func (m *memoryConfigStore) ExecuteTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return fn(nil)
}

func (m *memoryConfigStore) GetBudget(ctx context.Context, id string, tx ...*gorm.DB) (*tables.TableBudget, error) {
	return nil, nil
}

func (m *memoryConfigStore) CreateBudget(ctx context.Context, budget *tables.TableBudget, tx ...*gorm.DB) error {
	return nil
}

func (m *memoryConfigStore) UpdateBudget(ctx context.Context, budget *tables.TableBudget, tx ...*gorm.DB) error {
	return nil
}

func (m *memoryConfigStore) DeleteBudget(ctx context.Context, id string, tx ...*gorm.DB) error {
	return nil
}

func (m *memoryConfigStore) GetRateLimit(ctx context.Context, id string, tx ...*gorm.DB) (*tables.TableRateLimit, error) {
	return nil, nil
}

func (m *memoryConfigStore) CreateRateLimit(ctx context.Context, rateLimit *tables.TableRateLimit, tx ...*gorm.DB) error {
	return nil
}

func (m *memoryConfigStore) UpdateRateLimit(ctx context.Context, rateLimit *tables.TableRateLimit, tx ...*gorm.DB) error {
	return nil
}

func (m *memoryConfigStore) DeleteRateLimit(ctx context.Context, id string, tx ...*gorm.DB) error {
	return nil
}

func (m *memoryConfigStore) UpdateSessionsRoleByUsername(ctx context.Context, username, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.Username == username {
			s.Role = role
		}
	}
	return nil
}

func (m *memoryConfigStore) UpsertLoginDevice(ctx context.Context, row *tables.TableLoginDevice) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[row.UsernameKey+":"+row.Fingerprint] = true
	return nil
}

func (m *memoryConfigStore) CreatePasswordResetOTP(ctx context.Context, row *tables.TablePasswordResetOTP) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextOTPID++
	row.ID = m.nextOTPID
	copied := *row
	m.otps[row.ID] = &copied
	return nil
}

func (m *memoryConfigStore) GetLatestPasswordResetOTP(ctx context.Context, username string) (*tables.TablePasswordResetOTP, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest *tables.TablePasswordResetOTP
	for _, otp := range m.otps {
		if strings.EqualFold(otp.Username, username) || strings.EqualFold(otp.Email, username) {
			if latest == nil || otp.ID > latest.ID {
				copied := *otp
				latest = &copied
			}
		}
	}
	return latest, nil
}

func (m *memoryConfigStore) GetPasswordResetOTPByID(ctx context.Context, id uint) (*tables.TablePasswordResetOTP, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if row, ok := m.otps[id]; ok {
		copied := *row
		return &copied, nil
	}
	return nil, nil
}

func (m *memoryConfigStore) MarkPasswordResetOTPUsed(ctx context.Context, id uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.otps[id]; ok {
		row.Used = true
	}
	return nil
}

func (m *memoryConfigStore) BurnPasswordResetOTPCode(ctx context.Context, id uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.otps[id]; ok {
		row.OTPHash = "burned"
	}
	return nil
}

func (m *memoryConfigStore) IncrementPasswordResetOTPFailures(ctx context.Context, id uint) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.otps[id]; ok {
		row.FailedAttempts++
		return row.FailedAttempts, nil
	}
	return 0, nil
}

func setupTestStore(t *testing.T) *memoryConfigStore {
	t.Helper()
	SetLogger(&testMockLogger{})

	store := newMemoryConfigStore()

	adminPassHash, err := encrypt.Hash("AdminPass123!")
	if err != nil {
		t.Fatalf("Failed to hash admin password: %v", err)
	}

	authConfig := &configstore.AuthConfig{
		IsEnabled:     true,
		AdminUserName: schemas.NewSecretVar("admin"),
		AdminPassword: schemas.NewSecretVar(adminPassHash),
	}
	_ = store.UpdateAuthConfig(context.Background(), authConfig)

	return store
}

func makeFastHTTPCtx(method, uri string, body any, clientIP string) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	if clientIP != "" {
		if ip := net.ParseIP(clientIP); ip != nil {
			ctx.Init(&fasthttp.Request{}, &net.TCPAddr{IP: ip, Port: 12345}, nil)
		}
	}
	ctx.Request.Header.SetMethod(method)
	ctx.Request.SetRequestURI(uri)
	if body != nil {
		switch v := body.(type) {
		case string:
			ctx.Request.SetBodyString(v)
		case []byte:
			ctx.Request.SetBody(v)
		default:
			bytes, _ := json.Marshal(v)
			ctx.Request.SetBody(bytes)
		}
		ctx.Request.Header.SetContentType("application/json")
	}
	return ctx
}

func extractResponseCookie(ctx *fasthttp.RequestCtx, key string) (string, *fasthttp.Cookie) {
	c := fasthttp.AcquireCookie()
	c.SetKey(key)
	if ctx.Response.Header.Cookie(c) {
		return string(c.Value()), c
	}
	raw := string(ctx.Response.Header.Peek("Set-Cookie"))
	if strings.Contains(raw, key+"=") {
		parts := strings.Split(raw, ";")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, key+"=") {
				return strings.TrimPrefix(p, key+"="), nil
			}
		}
	}
	return "", nil
}

// 1. Successful Admin Login
func TestLogin_Admin_Success(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "192.168.1.100")

	handler.login(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
	if resp["role"] != "admin" {
		t.Errorf("Expected role 'admin', got %v", resp["role"])
	}

	token, cookieObj := extractResponseCookie(ctx, "token")
	if token == "" {
		t.Fatal("Expected 'token' Set-Cookie in response, but none found")
	}
	if cookieObj != nil && !cookieObj.HTTPOnly() {
		t.Errorf("Cookie missing HttpOnly flag")
	}

	session, err := store.GetSession(context.Background(), token)
	if err != nil || session == nil {
		t.Fatalf("Session not saved in store: %v", err)
	}
	if session.Username != "admin" || session.Role != "admin" {
		t.Errorf("Unexpected session in DB: %+v", session)
	}
	if session.ExpiresAt.Before(time.Now()) {
		t.Errorf("Session expiration is in past: %v", session.ExpiresAt)
	}
}

// 2. Successful Database User Login with RBAC and Allowed Sections
func TestLogin_DatabaseUser_Success(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	userPassHash, err := encrypt.Hash("SecretUser123!")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	store.AddUser(&tables.TableUser{
		Username:        "john_developer",
		Password:        userPassHash,
		Email:           "john@example.com",
		Role:            "developer",
		Status:          tables.UserStatusApproved,
		AllowedSections: "playground,prompt-repo",
	})

	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "john_developer",
		"password": "SecretUser123!",
	}, "192.168.1.101")

	handler.login(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp map[string]any
	_ = json.Unmarshal(ctx.Response.Body(), &resp)
	if resp["role"] != "developer" {
		t.Errorf("Expected role 'developer', got %v", resp["role"])
	}
	if resp["allowed_sections"] != "playground,prompt-repo" {
		t.Errorf("Expected allowed_sections 'playground,prompt-repo', got %v", resp["allowed_sections"])
	}

	token, _ := extractResponseCookie(ctx, "token")
	if token == "" {
		t.Fatal("Expected session token cookie")
	}
	sess, _ := store.GetSession(context.Background(), token)
	if sess == nil || sess.Username != "john_developer" {
		t.Errorf("Expected session for 'john_developer', got %+v", sess)
	}
}

// 3. Failed Login with Invalid Password
func TestLogin_InvalidPassword(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "WrongPassword999!",
	}, "192.168.1.102")

	handler.login(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized, got %d", ctx.Response.StatusCode())
	}

	token, _ := extractResponseCookie(ctx, "token")
	if token != "" {
		t.Errorf("Did not expect session token cookie on failed login")
	}

	// Verify failure count recorded in store
	lockout, _ := store.GetLoginLockout(context.Background(), loginUsernameKey("admin"))
	if lockout == nil || lockout.FailedCount != 1 {
		t.Errorf("Expected lockout failed count 1, got %+v", lockout)
	}
}

// 4. Failed Login with Non-Existent User (Timing Pad & Generic Message)
func TestLogin_NonExistentUser(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "ghost_user",
		"password": "AnyPassword123!",
	}, "192.168.1.103")

	handler.login(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized, got %d", ctx.Response.StatusCode())
	}

	bodyStr := string(ctx.Response.Body())
	if !strings.Contains(bodyStr, "Invalid username or password") {
		t.Errorf("Expected generic error message in body, got %s", bodyStr)
	}
}

// 5. Failed Login with Unapproved / Inactive User
func TestLogin_UnapprovedUser_States(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	passHash, _ := encrypt.Hash("Secret123!")

	// 5a. Pending user with correct password -> 403 "Your registration is waiting for admin approval"
	store.AddUser(&tables.TableUser{
		Username: "pending_user",
		Password: passHash,
		Email:    "pending@example.com",
		Role:     "user",
		Status:   tables.UserStatusPending,
	})

	ctxPending := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "pending_user",
		"password": "Secret123!",
	}, "192.168.1.104")
	handler.login(ctxPending)

	if ctxPending.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for pending user, got %d", ctxPending.Response.StatusCode())
	}
	if !strings.Contains(string(ctxPending.Response.Body()), "waiting for admin approval") {
		t.Errorf("Expected waiting approval message, got %s", string(ctxPending.Response.Body()))
	}

	// 5b. Rejected user with correct password -> 403 "Admin has not accepted your request"
	store.AddUser(&tables.TableUser{
		Username: "rejected_user",
		Password: passHash,
		Email:    "rejected@example.com",
		Role:     "user",
		Status:   tables.UserStatusRejected,
	})
	ctxRejected := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "rejected_user",
		"password": "Secret123!",
	}, "192.168.1.104")
	handler.login(ctxRejected)

	if ctxRejected.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for rejected user, got %d", ctxRejected.Response.StatusCode())
	}
	if !strings.Contains(string(ctxRejected.Response.Body()), "not accepted your request") {
		t.Errorf("Expected rejected message, got %s", string(ctxRejected.Response.Body()))
	}

	// 5c. Pending user with WRONG password -> 401 "Invalid username or password" (does not leak state if password wrong)
	ctxWrong := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "pending_user",
		"password": "WrongPassword!",
	}, "192.168.1.104")
	handler.login(ctxWrong)
	if ctxWrong.Response.StatusCode() != fasthttp.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for wrong password on pending user, got %d", ctxWrong.Response.StatusCode())
	}
}

// 6. Input Validation: Missing, Empty, and Oversized Inputs
func TestLogin_ValidationErrors(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	cases := []struct {
		name     string
		payload  map[string]string
		rawBody  string
		expected int
	}{
		{
			name:     "empty username",
			payload:  map[string]string{"username": "", "password": "Password123!"},
			expected: fasthttp.StatusBadRequest,
		},
		{
			name:     "empty password",
			payload:  map[string]string{"username": "admin", "password": ""},
			expected: fasthttp.StatusBadRequest,
		},
		{
			name:     "oversized username (>128 chars)",
			payload:  map[string]string{"username": strings.Repeat("a", 130), "password": "Password123!"},
			expected: fasthttp.StatusBadRequest,
		},
		{
			name:     "oversized password (>128 chars)",
			payload:  map[string]string{"username": "admin", "password": strings.Repeat("p", 130)},
			expected: fasthttp.StatusBadRequest,
		},
		{
			name:     "malformed json",
			rawBody:  `{username: broken-json`,
			expected: fasthttp.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ctx *fasthttp.RequestCtx
			if tc.rawBody != "" {
				ctx = makeFastHTTPCtx("POST", "/api/session/login", tc.rawBody, "192.168.1.105")
			} else {
				ctx = makeFastHTTPCtx("POST", "/api/session/login", tc.payload, "192.168.1.105")
			}
			handler.login(ctx)
			if ctx.Response.StatusCode() != tc.expected {
				t.Errorf("Expected status %d, got %d", tc.expected, ctx.Response.StatusCode())
			}
		})
	}
}

// 7. Auth Disabled Behavior
func TestLogin_AuthDisabled(t *testing.T) {
	store := newMemoryConfigStore()
	store.UpdateAuthConfig(context.Background(), &configstore.AuthConfig{
		IsEnabled: false,
	})
	handler := NewSessionHandler(store, nil)

	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "192.168.1.106")

	handler.login(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when auth is disabled, got %d", ctx.Response.StatusCode())
	}
}

// 8. Account Lockout After 3 Failed Attempts
func TestLogin_AccountLockout(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// loginMaxFailedAttempts = 3
	for i := 1; i <= 3; i++ {
		ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
			"username": "admin",
			"password": fmt.Sprintf("WrongPass%d!", i),
		}, "192.168.1.107")
		handler.login(ctx)

		if i < 3 {
			if ctx.Response.StatusCode() != fasthttp.StatusUnauthorized {
				t.Fatalf("Attempt %d: expected 401, got %d", i, ctx.Response.StatusCode())
			}
		} else {
			// On 3rd attempt, account becomes locked
			if ctx.Response.StatusCode() != fasthttp.StatusTooManyRequests {
				t.Fatalf("Attempt 3: expected 429 Too Many Requests (Lockout), got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
			}
		}
	}

	// 4th attempt: Even with correct password, account must remain locked!
	ctx4 := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!", // correct password
	}, "192.168.1.107")
	handler.login(ctx4)

	if ctx4.Response.StatusCode() != fasthttp.StatusTooManyRequests {
		t.Fatalf("Attempt 4 with correct password: expected 429 Too Many Requests, got %d", ctx4.Response.StatusCode())
	}
}

// 9. Single Session Fixation / Hijacking Mitigation
func TestLogin_SingleSessionFixationMitigation(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// First login
	ctx1 := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "192.168.1.108")
	handler.login(ctx1)
	token1, _ := extractResponseCookie(ctx1, "token")
	if token1 == "" {
		t.Fatal("First login did not receive token")
	}

	// Verify token1 is in store
	s1, _ := store.GetSession(context.Background(), token1)
	if s1 == nil {
		t.Fatal("token1 should be stored")
	}

	// Second login by same user (e.g., from another browser or attacker session fixation)
	ctx2 := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "192.168.1.109")
	handler.login(ctx2)
	token2, _ := extractResponseCookie(ctx2, "token")
	if token2 == "" {
		t.Fatal("Second login did not receive token")
	}

	// token2 must be different from token1
	if token1 == token2 {
		t.Errorf("New login must generate a new UUID token")
	}

	// token1 must have been purged from store (mitigating concurrent stolen sessions)
	s1Old, _ := store.GetSession(context.Background(), token1)
	if s1Old != nil {
		t.Errorf("Prior session token1 should have been invalidated on new login")
	}

	// token2 must be active
	s2, _ := store.GetSession(context.Background(), token2)
	if s2 == nil {
		t.Errorf("token2 should be active in store")
	}
}

// 10. Logout Flow
func TestLogout_Success(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// Login first
	loginCtx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "192.168.1.110")
	handler.login(loginCtx)
	token, _ := extractResponseCookie(loginCtx, "token")
	if token == "" {
		t.Fatal("Login failed")
	}

	// Logout with Bearer token
	logoutCtx := makeFastHTTPCtx("POST", "/api/session/logout", nil, "192.168.1.110")
	logoutCtx.Request.Header.Set("Authorization", "Bearer "+token)
	handler.logout(logoutCtx)

	if logoutCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK on logout, got %d", logoutCtx.Response.StatusCode())
	}

	// Verify session deleted from database
	sess, _ := store.GetSession(context.Background(), token)
	if sess != nil {
		t.Errorf("Session should have been deleted from store on logout")
	}

	// Verify cookie cleared
	clearedCookie, cObj := extractResponseCookie(logoutCtx, "token")
	if cObj != nil {
		if cObj.MaxAge() > 0 {
			t.Errorf("Logout cookie MaxAge should be <= 0")
		}
	} else if clearedCookie != "" {
		t.Errorf("Logout cookie should be empty, got %q", clearedCookie)
	}
}

// 11. IsAuthEnabled Endpoint
func TestIsAuthEnabled_Endpoint(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// Without token
	ctx1 := makeFastHTTPCtx("GET", "/api/session/is-auth-enabled", nil, "192.168.1.111")
	handler.isAuthEnabled(ctx1)

	var resp1 map[string]any
	_ = json.Unmarshal(ctx1.Response.Body(), &resp1)
	if resp1["is_auth_enabled"] != true {
		t.Errorf("Expected is_auth_enabled=true, got %v", resp1["is_auth_enabled"])
	}
	if resp1["has_valid_token"] != false {
		t.Errorf("Expected has_valid_token=false without token, got %v", resp1["has_valid_token"])
	}

	// Now login to get token
	loginCtx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "192.168.1.111")
	handler.login(loginCtx)
	token, _ := extractResponseCookie(loginCtx, "token")

	// Call with Authorization header
	ctx2 := makeFastHTTPCtx("GET", "/api/session/is-auth-enabled", nil, "192.168.1.111")
	ctx2.Request.Header.Set("Authorization", "Bearer "+token)
	handler.isAuthEnabled(ctx2)

	var resp2 map[string]any
	_ = json.Unmarshal(ctx2.Response.Body(), &resp2)
	if resp2["has_valid_token"] != true {
		t.Errorf("Expected has_valid_token=true with valid token, got %v", resp2["has_valid_token"])
	}
	if resp2["role"] != "admin" {
		t.Errorf("Expected role 'admin', got %v", resp2["role"])
	}
}

// 12. SMTP Independence: Login functions 100% properly without SMTP configured
func TestLogin_SMTPResilience(t *testing.T) {
	store := setupTestStore(t)
	// Explicitly leave SMTP nil/disabled
	store.smtpConfig = &tables.TableSMTPConfig{
		Enabled: false,
	}
	handler := NewSessionHandler(store, nil)

	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "192.168.1.112")

	handler.login(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Login must succeed even without SMTP configured, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}
}

// 13. Username Leading/Trailing Whitespace Trimming
func TestLogin_WhitespaceTrimming(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "   admin   ", // with leading and trailing spaces
		"password": "AdminPass123!",
	}, "192.168.1.113")

	handler.login(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK with trimmed username, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}
}

// 14. IP-level Rate Limiting (30 failed attempts triggers network lockout)
func TestLogin_IPRateLimiting(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	attackerIP := "203.0.113.50"

	// Simulate 30 failed login attempts from the same IP with different usernames to avoid username lockout
	for i := 1; i <= 30; i++ {
		ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
			"username": fmt.Sprintf("random_target_%d", i),
			"password": "WrongPassword!",
		}, attackerIP)
		handler.login(ctx)
	}

	// 31st attempt from the attacker IP: Must be blocked at IP rate limit level
	ctxBlocked := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, attackerIP)
	handler.login(ctxBlocked)

	if ctxBlocked.Response.StatusCode() != fasthttp.StatusTooManyRequests {
		t.Fatalf("Expected 429 Too Many Requests from locked-out IP, got %d", ctxBlocked.Response.StatusCode())
	}
	if !strings.Contains(string(ctxBlocked.Response.Body()), "Too many login attempts from this network") {
		t.Errorf("Expected network lockout message, got %s", string(ctxBlocked.Response.Body()))
	}

	// But a DIFFERENT, legitimate IP should NOT be blocked!
	cleanCtx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "198.51.100.99")
	handler.login(cleanCtx)

	if cleanCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Clean IP should still be able to login, got %d", cleanCtx.Response.StatusCode())
	}
}

// 15. Cookie Security Flags: TLS vs Non-TLS
func TestLogin_CookieSecurityFlags(t *testing.T) {
	// Plain HTTP context -> Secure flag false
	plainCtx := &fasthttp.RequestCtx{}
	if cookieShouldBeSecure(plainCtx) {
		t.Errorf("cookieShouldBeSecure should return false on plain unencrypted HTTP")
	}

	// Set TRUST_PROXY_HEADERS for trusted proxy test
	_ = os.Setenv("TRUST_PROXY_HEADERS", "1")
	defer func() { _ = os.Unsetenv("TRUST_PROXY_HEADERS") }()

	proxyCtx := &fasthttp.RequestCtx{}
	proxyCtx.Init(&fasthttp.Request{}, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 80}, nil)
	proxyCtx.Request.Header.Set("X-Forwarded-Proto", "https")

	if !cookieShouldBeSecure(proxyCtx) {
		t.Errorf("cookieShouldBeSecure should return true when behind trusted proxy forwarding HTTPS")
	}
}

// 16. Full Lifecycle: Register -> Pending Approval -> Approve -> Login Success
func TestLogin_FullLifecycle_RegisterApproveLogin(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// Step 1: User registers
	regCtx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
		"username": "new_engineer",
		"email":    "engineer@rakshatech.io",
		"password": "StrongPassword2026!",
	}, "192.168.1.115")
	handler.register(regCtx)

	if regCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Registration failed: %d - %s", regCtx.Response.StatusCode(), string(regCtx.Response.Body()))
	}

	// Step 2: Try to login immediately -> Must be rejected with 403 (waiting for approval)
	login1Ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "new_engineer",
		"password": "StrongPassword2026!",
	}, "192.168.1.115")
	handler.login(login1Ctx)

	if login1Ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("Unapproved user should get 403 Forbidden, got %d", login1Ctx.Response.StatusCode())
	}
	if !strings.Contains(string(login1Ctx.Response.Body()), "waiting for admin approval") {
		t.Errorf("Expected waiting approval message, got %s", string(login1Ctx.Response.Body()))
	}

	// Step 3: Admin approves the user in the database
	user, err := store.GetUserByUsername(context.Background(), "new_engineer")
	if err != nil || user == nil {
		t.Fatalf("User not found in store: %v", err)
	}
	user.Status = tables.UserStatusApproved
	_ = store.UpdateUser(context.Background(), user)

	// Step 4: Login again after approval -> Must succeed with 200 OK and token cookie
	login2Ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "new_engineer",
		"password": "StrongPassword2026!",
	}, "192.168.1.115")
	handler.login(login2Ctx)

	if login2Ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Approved user login must succeed, got %d: %s", login2Ctx.Response.StatusCode(), string(login2Ctx.Response.Body()))
	}

	token, _ := extractResponseCookie(login2Ctx, "token")
	if token == "" {
		t.Fatal("Approved user did not receive session token")
	}

	sess, _ := store.GetSession(context.Background(), token)
	if sess == nil || sess.Username != "new_engineer" {
		t.Fatalf("Session was not created properly for approved user")
	}
}

