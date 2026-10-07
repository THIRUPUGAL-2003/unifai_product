package handlers

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

func TestRegistrationReviewToken_AcceptDenyAndGovernanceVisibility(t *testing.T) {
	_ = os.Setenv("PASSWORD_RESET_SECRET", "super-secret-key-that-is-at-least-32-chars-long!")
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	regCtx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
		"username": "review_candidate",
		"email":    "review.candidate@gatewaytech.io",
		"password": "StrongPassword123!",
	}, "192.168.1.40")
	handler.register(regCtx)
	if regCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("register: %d %s", regCtx.Response.StatusCode(), regCtx.Response.Body())
	}
	var regResp map[string]any
	_ = json.Unmarshal(regCtx.Response.Body(), &regResp)
	userID, _ := regResp["id"].(string)
	if userID == "" {
		t.Fatalf("missing user id: %v", regResp)
	}

	adminToken := getAdminSessionToken(t, handler)
	listCtx := makeFastHTTPCtx("GET", "/api/session/users", nil, "192.168.1.1")
	listCtx.Request.Header.Set("Authorization", "Bearer "+adminToken)
	handler.getUsers(listCtx)
	if listCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("list users: %d", listCtx.Response.StatusCode())
	}
	if !strings.Contains(string(listCtx.Response.Body()), "review_candidate") {
		t.Fatalf("pending user missing from governance list: %s", listCtx.Response.Body())
	}

	approveTok, err := issueRegistrationReviewToken(userID, "approve")
	if err != nil {
		t.Fatalf("issue approve token: %v", err)
	}

	revCtx := &fasthttp.RequestCtx{}
	revCtx.Request.Header.SetMethod("GET")
	revCtx.Request.SetRequestURI("/api/session/users/review?token=" + url.QueryEscape(approveTok))
	revCtx.Request.SetHost("gateway.example.com")
	handler.reviewRegistrationByToken(revCtx)
	if revCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("accept link: %d %s", revCtx.Response.StatusCode(), revCtx.Response.Body())
	}
	user, err := store.GetUserByID(nil, userID)
	if err != nil || user.Status != tables.UserStatusApproved {
		t.Fatalf("expected approved, got %+v err=%v", user, err)
	}

	listCtx2 := makeFastHTTPCtx("GET", "/api/session/users", nil, "192.168.1.1")
	listCtx2.Request.Header.Set("Authorization", "Bearer "+adminToken)
	handler.getUsers(listCtx2)
	body := string(listCtx2.Response.Body())
	if !strings.Contains(body, "review_candidate") || !strings.Contains(body, `"status":"approved"`) {
		t.Fatalf("approved user not visible correctly: %s", body)
	}
}

func TestRejectedEmailReclaimedOnNewSignup(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	now := time.Now()
	denied := &tables.TableUser{
		ID:        "denied-email-1",
		Username:  "old_denied",
		Email:     "free.me@example.com",
		Password:  "x",
		Role:      "user",
		Status:    tables.UserStatusRejected,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateUser(nil, denied); err != nil {
		t.Fatal(err)
	}

	regCtx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
		"username": "new_owner",
		"email":    "free.me@example.com",
		"password": "StrongPassword123!",
	}, "192.168.1.41")
	handler.register(regCtx)
	if regCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("expected rejected email to be reclaimable, got %d %s", regCtx.Response.StatusCode(), regCtx.Response.Body())
	}
	got, _ := store.GetUserByID(nil, denied.ID)
	if got != nil {
		t.Fatal("denied row should have been deleted to free the email")
	}
}

func TestRegistrationReuseClearsPriorGrants(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	sections := "dashboard,logs"
	budgetID := "bud-old"
	now := time.Now()
	existing := &tables.TableUser{
		ID:                 "reuse-1",
		Username:           "reuse_me",
		Email:              "reuse@example.com",
		Password:           "x",
		Role:               "user",
		Status:             tables.UserStatusRejected,
		AllowedSections:    sections,
		AllowedPromptRepos: "p1,p2",
		Budget:             99,
		RateLimit:          50,
		BudgetID:           &budgetID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := store.CreateUser(nil, existing); err != nil {
		t.Fatal(err)
	}

	regCtx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
		"username": "reuse_me",
		"email":    "reuse2@example.com",
		"password": "StrongPassword123!",
	}, "192.168.1.42")
	handler.register(regCtx)
	if regCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("reuse register: %d %s", regCtx.Response.StatusCode(), regCtx.Response.Body())
	}
	got, err := store.GetUserByUsername(nil, "reuse_me")
	if err != nil || got == nil {
		t.Fatalf("user missing: %v", err)
	}
	if got.AllowedSections != "" || got.AllowedPromptRepos != "" || got.Budget != 0 || got.RateLimit != 0 || got.BudgetID != nil {
		t.Fatalf("grants should be cleared on reuse: %+v", got)
	}
}

func TestApproveBlockedUntilEmailVerified(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	adminToken := getAdminSessionToken(t, handler)

	now := time.Now()
	u := &tables.TableUser{
		ID:        "unverified-1",
		Username:  "needs_verify",
		Email:     "needs.verify@example.com",
		Password:  "x",
		Role:      "user",
		Status:    tables.UserStatusEmailUnverified,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateUser(nil, u); err != nil {
		t.Fatal(err)
	}

	approveCtx := makeFastHTTPCtx("POST", "/api/session/users/"+u.ID+"/approve", nil, "192.168.1.1")
	approveCtx.SetUserValue("id", u.ID)
	approveCtx.Request.Header.Set("Authorization", "Bearer "+adminToken)
	handler.approveUser(approveCtx)
	if approveCtx.Response.StatusCode() != fasthttp.StatusConflict {
		t.Fatalf("expected 409 for unverified approve, got %d %s", approveCtx.Response.StatusCode(), approveCtx.Response.Body())
	}

	denyCtx := makeFastHTTPCtx("POST", "/api/session/users/"+u.ID+"/reject", nil, "192.168.1.1")
	denyCtx.SetUserValue("id", u.ID)
	denyCtx.Request.Header.Set("Authorization", "Bearer "+adminToken)
	handler.rejectUser(denyCtx)
	if denyCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("deny unverified should work, got %d %s", denyCtx.Response.StatusCode(), denyCtx.Response.Body())
	}
}
