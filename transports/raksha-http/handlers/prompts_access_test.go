package handlers

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/raksha/raksha/core/schemas"
	"github.com/raksha/raksha/framework/configstore"
	"github.com/raksha/raksha/framework/configstore/tables"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

type accessTestStore struct {
	configstore.ConfigStore
	db *gorm.DB
}

func (s *accessTestStore) DB() *gorm.DB {
	return s.db
}

func (s *accessTestStore) GetPromptByID(ctx context.Context, id string) (*tables.TablePrompt, error) {
	var p tables.TablePrompt
	if err := s.db.Where("id = ?", id).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *accessTestStore) GetUserByID(ctx context.Context, id string) (*tables.TableUser, error) {
	var u tables.TableUser
	if err := s.db.Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *accessTestStore) GetUserByUsername(ctx context.Context, username string) (*tables.TableUser, error) {
	var u tables.TableUser
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *accessTestStore) CreateFolder(ctx context.Context, folder *tables.TableFolder) error {
	return s.db.Create(folder).Error
}

func (s *accessTestStore) GetFolderByID(ctx context.Context, id string) (*tables.TableFolder, error) {
	var folder tables.TableFolder
	if err := s.db.Where("id = ?", id).First(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *accessTestStore) GetSession(ctx context.Context, token string) (*tables.SessionsTable, error) {
	var sess tables.SessionsTable
	if err := s.db.Where("token = ?", token).First(&sess).Error; err != nil {
		return nil, err
	}
	return &sess, nil
}

func newTestCtx() *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 80}, nil)
	return ctx
}

func TestPromptAccess_GetAndUpdateWorkflow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in memory: %v", err)
	}

	err = db.AutoMigrate(
		&tables.TableFolder{},
		&tables.TablePrompt{},
		&tables.TablePromptVersion{},
		&tables.TablePromptVersionMessage{},
		&tables.TablePromptSession{},
		&tables.TablePromptSessionMessage{},
		&tables.TableCustomer{},
		&tables.TableTeam{},
		&tables.TableUser{},
		&tables.TableTeamMember{},
		&tables.SessionsTable{},
	)
	if err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}

	store := &accessTestStore{db: db}
	h := NewPromptsHandler(store, nil)

	// 1. Create Customer, Team, and Users
	custID := "cust_" + uuid.New().String()[:8]
	err = db.Create(&tables.TableCustomer{
		ID:        custID,
		Name:      "Acme Enterprise",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Error
	if err != nil {
		t.Fatalf("create customer failed: %v", err)
	}

	teamID := "team_" + uuid.New().String()[:8]
	err = db.Create(&tables.TableTeam{
		ID:         teamID,
		Name:       "DevOps Team",
		CustomerID: &custID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}).Error
	if err != nil {
		t.Fatalf("create team failed: %v", err)
	}

	userAlice := &tables.TableUser{
		ID:        "uid_alice_" + uuid.New().String()[:8],
		Username:  "alice",
		Email:     "alice@example.com",
		Role:      "member",
		Status:    "approved",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	userBob := &tables.TableUser{
		ID:        "uid_bob_" + uuid.New().String()[:8],
		Username:  "bob",
		Email:     "bob@example.com",
		Role:      "member",
		Status:    "approved",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	userCharlie := &tables.TableUser{
		ID:        "uid_charlie_" + uuid.New().String()[:8],
		Username:  "charlie",
		Email:     "charlie@example.com",
		Role:      "member",
		Status:    "approved",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.Create(userAlice)
	db.Create(userBob)
	db.Create(userCharlie)

	// Create sessions for alice, bob, charlie
	db.Create(&tables.SessionsTable{
		Token:     "token_alice",
		Username:  userAlice.Username,
		Role:      "member",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	db.Create(&tables.SessionsTable{
		Token:     "token_bob",
		Username:  userBob.Username,
		Role:      "member",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	db.Create(&tables.SessionsTable{
		Token:     "token_charlie",
		Username:  userCharlie.Username,
		Role:      "member",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	// Bob is member of DevOps team
	db.Create(&tables.TableTeamMember{
		TeamID:    teamID,
		UserID:    userBob.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	// 2. Create prompt
	promptID := uuid.New().String()
	p := &tables.TablePrompt{
		ID:        promptID,
		Name:      "Deployment Audit Prompt",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.Create(p)

	// 3. Test GET /access initially (unrestricted)
	{
		reqCtx := newTestCtx()
		reqCtx.SetUserValue("id", promptID)
		reqCtx.Request.Header.SetMethod("GET")

		h.getPromptAccess(reqCtx)

		if reqCtx.Response.StatusCode() != fasthttp.StatusOK {
			t.Fatalf("expected 200, got %d: %s", reqCtx.Response.StatusCode(), reqCtx.Response.Body())
		}

		var resp map[string]interface{}
		json.Unmarshal(reqCtx.Response.Body(), &resp)

		custIDs := resp["customer_ids"].([]interface{})
		if len(custIDs) != 0 {
			t.Fatalf("expected 0 customer_ids, got %v", custIDs)
		}
	}

	// 4. Test PUT /access with Customer, Team, and Alice directly
	{
		payload := map[string]interface{}{
			"customer_ids": []string{custID},
			"team_ids":     []string{teamID},
			"user_ids":     []string{userAlice.ID},
		}
		bodyBytes, _ := json.Marshal(payload)

		reqCtx := newTestCtx()
		reqCtx.SetUserValue("id", promptID)
		reqCtx.Request.Header.SetMethod("PUT")
		reqCtx.Request.Header.SetContentType("application/json")
		reqCtx.Request.SetBody(bodyBytes)

		h.updatePromptAccess(reqCtx)

		if reqCtx.Response.StatusCode() != fasthttp.StatusOK {
			t.Fatalf("expected 200, got %d: %s", reqCtx.Response.StatusCode(), reqCtx.Response.Body())
		}

		var resp map[string]interface{}
		json.Unmarshal(reqCtx.Response.Body(), &resp)

		userList := resp["effective_users"].([]interface{})
		if len(userList) == 0 {
			t.Fatalf("expected effective_users to be populated, got empty")
		}

		foundAlice := false
		for _, u := range userList {
			um := u.(map[string]interface{})
			if um["user_id"] == userAlice.ID {
				foundAlice = true
				if um["origin"] != "direct" {
					t.Errorf("expected alice origin to be direct, got %v", um["origin"])
				}
			}
		}

		if !foundAlice {
			t.Errorf("expected alice in effective users")
		}
	}

	// 5. Test Access Enforcement (h.checkPromptAccess)
	{
		// Alice has direct access
		reqAlice := newTestCtx()
		reqAlice.SetUserValue(schemas.RakshaContextKeySessionToken, "token_alice")
		if !h.checkPromptAccess(reqAlice, promptID) {
			t.Errorf("expected Alice to have access")
		}

		// Charlie (not in team or direct) should NOT have access
		reqCharlie := newTestCtx()
		reqCharlie.SetUserValue(schemas.RakshaContextKeySessionToken, "token_charlie")
		if h.checkPromptAccess(reqCharlie, promptID) {
			t.Errorf("expected Charlie to be denied access")
		}
	}
}
