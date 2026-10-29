package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/transports/gateway-http/lib"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// PromptCacheReloader is implemented by the prompts plugin to allow the HTTP handler
// to trigger an in-memory cache refresh after any repository mutation.
type PromptCacheReloader interface {
	Reload(ctx context.Context) error
}

// teamMoveSync refreshes in-memory governance after a team folder is dragged to a new parent.
type teamMoveSync interface {
	ReloadTeam(ctx context.Context, id string) (*tables.TableTeam, error)
	ReloadBusinessUnitTeamIndex(ctx context.Context)
}

// PromptsHandler handles prompt repository endpoints
type PromptsHandler struct {
	store     configstore.ConfigStore
	reloader  PromptCacheReloader // optional; nil when the prompts plugin is not loaded
	lifecycle *PromptLifecycleManager
	moveSync  teamMoveSync
}

// SetTeamMoveSync wires governance memory refresh after a team folder changes parent.
func (h *PromptsHandler) SetTeamMoveSync(sync teamMoveSync) {
	if h != nil {
		h.moveSync = sync
	}
}

// NewPromptsHandler creates a new PromptsHandler.
// reloader may be nil; when set, the in-memory prompt cache is refreshed after mutations.
func NewPromptsHandler(store configstore.ConfigStore, reloader PromptCacheReloader) *PromptsHandler {
	if store == nil {
		return nil
	}
	h := &PromptsHandler{
		store:     store,
		reloader:  reloader,
		lifecycle: NewPromptLifecycleManager(store),
	}
	h.startPromptHistoryRetentionCleanup()
	return h
}

// reloadCache triggers a cache refresh if a reloader is configured.
// Errors are logged but do not fail the originating request.
func (h *PromptsHandler) reloadCache(ctx context.Context) {
	if h.reloader == nil {
		return
	}
	if err := h.reloader.Reload(ctx); err != nil {
		logger.Error("failed to reload prompt cache: %v", err)
	}
}

// createTeamWithDefaultBudget keeps the governance team and its default budget
// in the same transaction so a budget insert failure cannot leave a team without
// the budget that Prompt Repository team creation promises.
func (h *PromptsHandler) createTeamWithDefaultBudget(ctx context.Context, team *tables.TableTeam, folder *tables.TableFolder) error {
	return h.store.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
		if err := h.store.CreateTeam(ctx, team, tx); err != nil {
			return fmt.Errorf("create governance team: %w", err)
		}

		now := time.Now()
		budget := &tables.TableBudget{
			ID:            uuid.NewString(),
			MaxLimit:      500,
			ResetDuration: "1h",
			TeamID:        &team.ID,
			LastReset:     now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := h.store.CreateBudget(ctx, budget, tx); err != nil {
			return fmt.Errorf("create default team budget: %w", err)
		}
		folder.Type = "team"
		folder.EntityID = &team.ID
		if err := tx.WithContext(ctx).Create(folder).Error; err != nil {
			return fmt.Errorf("create team folder: %w", err)
		}
		return nil
	})
}

// RegisterRoutes registers the routes for the PromptsHandler
func (h *PromptsHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.GatewayHTTPMiddleware) {
	// Folders
	r.GET("/api/prompt-repo/folders", lib.ChainMiddlewares(h.getFolders, middlewares...))
	r.GET("/api/prompt-repo/folders/{id}", lib.ChainMiddlewares(h.getFolderByID, middlewares...))
	r.POST("/api/prompt-repo/folders", lib.ChainMiddlewares(h.createFolder, middlewares...))
	r.PUT("/api/prompt-repo/folders/{id}", lib.ChainMiddlewares(h.updateFolder, middlewares...))
	r.DELETE("/api/prompt-repo/folders/{id}", lib.ChainMiddlewares(h.deleteFolder, middlewares...))

	// Prompts
	r.GET("/api/prompt-repo/prompts", lib.ChainMiddlewares(h.getPrompts, middlewares...))
	r.GET("/api/prompt-repo/prompts/{id}", lib.ChainMiddlewares(h.getPromptByID, middlewares...))
	r.GET("/api/prompt-repo/prompts/{id}/access", lib.ChainMiddlewares(h.getPromptAccess, middlewares...))
	r.PUT("/api/prompt-repo/prompts/{id}/access", lib.ChainMiddlewares(h.updatePromptAccess, middlewares...))
	r.POST("/api/prompt-repo/prompts", lib.ChainMiddlewares(h.createPrompt, middlewares...))
	r.PUT("/api/prompt-repo/prompts/{id}", lib.ChainMiddlewares(h.updatePrompt, middlewares...))
	r.DELETE("/api/prompt-repo/prompts/{id}", lib.ChainMiddlewares(h.deletePrompt, middlewares...))

	// Versions
	r.GET("/api/prompt-repo/prompts/{id}/versions", lib.ChainMiddlewares(h.getPromptVersions, middlewares...))
	r.GET("/api/prompt-repo/versions/{id}", lib.ChainMiddlewares(h.getVersionByID, middlewares...))
	r.POST("/api/prompt-repo/prompts/{id}/versions", lib.ChainMiddlewares(h.createVersion, middlewares...))
	r.DELETE("/api/prompt-repo/versions/{id}", lib.ChainMiddlewares(h.deleteVersion, middlewares...))

	// Sessions
	r.GET("/api/prompt-repo/prompts/{id}/sessions", lib.ChainMiddlewares(h.getPromptSessions, middlewares...))
	r.GET("/api/prompt-repo/sessions/{id}", lib.ChainMiddlewares(h.getSessionByID, middlewares...))
	r.POST("/api/prompt-repo/prompts/{id}/sessions", lib.ChainMiddlewares(h.createSession, middlewares...))
	r.PUT("/api/prompt-repo/sessions/{id}", lib.ChainMiddlewares(h.updateSession, middlewares...))
	r.DELETE("/api/prompt-repo/sessions/{id}", lib.ChainMiddlewares(h.deleteSession, middlewares...))
	r.PUT("/api/prompt-repo/sessions/{id}/rename", lib.ChainMiddlewares(h.renameSession, middlewares...))
	r.POST("/api/prompt-repo/sessions/{id}/commit", lib.ChainMiddlewares(h.commitSession, middlewares...))

	// History Auto-delete & Retention Settings (Admin)
	r.GET("/api/prompt-repo/settings", lib.ChainMiddlewares(h.getPromptSettings, middlewares...))
	r.PUT("/api/prompt-repo/settings", lib.ChainMiddlewares(h.updatePromptSettings, middlewares...))
	r.POST("/api/prompt-repo/history/clear", lib.ChainMiddlewares(h.clearAllPromptHistory, middlewares...))
}

// ============================================================================
// Request/Response Types
// ============================================================================

// CreateFolderRequest represents the request body for creating a folder
type CreateFolderRequest struct {
	Name        string  `json:"name"`
	ParentID    *string `json:"parent_id,omitempty"`
	Description *string `json:"description,omitempty"`
}

// UpdateFolderRequest represents the request body for updating a folder
type UpdateFolderRequest struct {
	Name              string  `json:"name"`
	ParentID          *string `json:"parent_id,omitempty"`
	ParentIDExists    bool    `json:"-"`
	Description       *string `json:"description,omitempty"`
	DescriptionExists bool    `json:"-"` // true when description key is present in JSON (even if null)
}

// UnmarshalJSON implements custom unmarshalling to detect presence of description key
func (r *UpdateFolderRequest) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["name"]; ok {
		if err := json.Unmarshal(v, &r.Name); err != nil {
			return err
		}
	}
	if v, ok := raw["parent_id"]; ok {
		if err := json.Unmarshal(v, &r.ParentID); err != nil {
			return err
		}
		r.ParentIDExists = true
	}
	if v, ok := raw["description"]; ok {
		if err := json.Unmarshal(v, &r.Description); err != nil {
			return err
		}
		r.DescriptionExists = true
	}
	return nil
}

// CreatePromptRequest represents the request body for creating a prompt
type CreatePromptRequest struct {
	Name       string  `json:"name"`
	FolderID   *string `json:"folder_id,omitempty"`
	FolderName string  `json:"folder_name,omitempty"`
}

// UpdatePromptRequest represents the request body for updating a prompt
type UpdatePromptRequest struct {
	Name           string  `json:"name"`
	FolderID       *string `json:"folder_id"`
	FolderIDExists bool    `json:"-"` // true when folder_id key is present in JSON (even if null)
}

// CreateVersionRequest represents the request body for creating a version
type CreateVersionRequest struct {
	CommitMessage string                 `json:"commit_message"`
	Messages      []tables.PromptMessage `json:"messages"`
	ModelParams   tables.ModelParams     `json:"model_params"`
	Provider      string                 `json:"provider"`
	Model         string                 `json:"model"`
	Variables     tables.PromptVariables `json:"variables,omitempty"`
}

// CreateSessionRequest represents the request body for creating a session
type CreateSessionRequest struct {
	Name        string                 `json:"name"`
	VersionID   *uint                  `json:"version_id,omitempty"`
	Messages    []tables.PromptMessage `json:"messages,omitempty"`
	ModelParams tables.ModelParams     `json:"model_params"`
	Provider    string                 `json:"provider"`
	Model       string                 `json:"model"`
	Variables   tables.PromptVariables `json:"variables,omitempty"`
}

// UpdateSessionRequest represents the request body for updating a session
type UpdateSessionRequest struct {
	Name        string                 `json:"name"`
	Messages    []tables.PromptMessage `json:"messages"`
	ModelParams tables.ModelParams     `json:"model_params"`
	Provider    string                 `json:"provider"`
	Model       string                 `json:"model"`
	Variables   tables.PromptVariables `json:"variables,omitempty"`
}

// RenameSessionRequest represents the request body for renaming a session
type RenameSessionRequest struct {
	Name string `json:"name"`
}

// CommitSessionRequest represents the request body for committing a session as a version
type CommitSessionRequest struct {
	CommitMessage  string `json:"commit_message"`
	MessageIndices *[]int `json:"message_indices,omitempty"` // optional: indices of messages to include (0-based). If nil/absent, all messages are included.
}

// ============================================================================
// Folder Handlers
// ============================================================================

// getFolders handles GET /api/prompt-repo/folders
func (h *PromptsHandler) getFolders(ctx *fasthttp.RequestCtx) {
	if h.lifecycle != nil {
		_ = h.lifecycle.EnsureAllSystemRoots(ctx)
	}
	folders, err := h.store.GetFolders(ctx)
	if err != nil {
		logger.Error("failed to get folders: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"folders": folders,
	})
}

// getFolderByID handles GET /api/prompt-repo/folders/{id}
func (h *PromptsHandler) getFolderByID(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "folder ID is required")
		return
	}
	id, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid folder ID")
		return
	}

	folder, err := h.store.GetFolderByID(ctx, id)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "folder not found")
			return
		}
		logger.Error("failed to get folder: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"folder": folder,
	})
}

// createFolder handles POST /api/prompt-repo/folders
func (h *PromptsHandler) createFolder(ctx *fasthttp.RequestCtx) {
	var req CreateFolderRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name is required")
		return
	}

	folder := &tables.TableFolder{
		ID:          uuid.New().String(),
		Name:        req.Name,
		ParentID:    req.ParentID,
		Description: req.Description,
		Type:        "custom",
	}
	folderPersisted := false

	// Bidirectional Sync: If creating under Teams/Customers/Business Units, auto-create in Governance!
	if req.ParentID != nil && *req.ParentID != "" {
		parent, pErr := h.store.GetFolderByID(ctx, *req.ParentID)
		if pErr != nil {
			if errors.Is(pErr, configstore.ErrNotFound) {
				SendError(ctx, fasthttp.StatusBadRequest, "parent folder not found")
				return
			}
			logger.Error("failed to load parent folder for create: %v", pErr)
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load parent folder: "+pErr.Error())
			return
		}
		if parent != nil {
			isTeamsRoot := parent.Type == "system_teams_root" || (parent.Name == "Teams" && (parent.ParentID == nil || *parent.ParentID == ""))
			isCustomersRoot := parent.Type == "system_customers_root" || (parent.Name == "Customers" && (parent.ParentID == nil || *parent.ParentID == ""))
			isBURoot := parent.Type == "system_business_units_root" || (parent.Name == "Business Units" && (parent.ParentID == nil || *parent.ParentID == ""))

			if isTeamsRoot {
				// Standalone team created under Teams/
				teamID := uuid.NewString()
				team := &tables.TableTeam{
					ID:        teamID,
					Name:      req.Name,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if err := h.createTeamWithDefaultBudget(ctx, team, folder); err != nil {
					logger.Error("failed to create team and default budget: %v", err)
					SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
					return
				}
				folderPersisted = true
			} else if parent.Type == "customer" && parent.EntityID != nil {
				// Team created under a Customer
				teamID := uuid.NewString()
				team := &tables.TableTeam{
					ID:         teamID,
					Name:       req.Name,
					CustomerID: parent.EntityID,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}
				if err := h.createTeamWithDefaultBudget(ctx, team, folder); err != nil {
					logger.Error("failed to create team and default budget: %v", err)
					SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
					return
				}
				folderPersisted = true
			} else if parent.Type == "business_unit" && parent.EntityID != nil {
				// Team created under a Business Unit
				teamID := uuid.NewString()
				team := &tables.TableTeam{
					ID:             teamID,
					Name:           req.Name,
					BusinessUnitID: parent.EntityID,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}
				if err := h.createTeamWithDefaultBudget(ctx, team, folder); err != nil {
					logger.Error("failed to create team and default budget: %v", err)
					SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
					return
				}
				folderPersisted = true
			} else if isCustomersRoot {
				// Customer created under Customers/
				custID := uuid.NewString()
				customer := &tables.TableCustomer{
					ID:        custID,
					Name:      req.Name,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				folder.Type = "customer"
				folder.EntityID = &custID
				if err := h.store.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
					if err := h.store.CreateCustomer(ctx, customer, tx); err != nil {
						return fmt.Errorf("create customer: %w", err)
					}
					if err := tx.WithContext(ctx).Create(folder).Error; err != nil {
						return fmt.Errorf("create customer folder: %w", err)
					}
					return nil
				}); err != nil {
					logger.Error("failed to create customer and folder: %v", err)
					SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
					return
				}
				folderPersisted = true
			} else if isBURoot {
				// Business Unit created under Business Units/
				buID := uuid.NewString()
				bu := &tables.TableBusinessUnit{
					ID:        buID,
					Name:      req.Name,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if h.store.DB() == nil {
					SendError(ctx, fasthttp.StatusServiceUnavailable, "Database is unavailable")
					return
				}
				folder.Type = "business_unit"
				folder.EntityID = &buID
				if err := h.store.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
					if err := tx.WithContext(ctx).Create(bu).Error; err != nil {
						return fmt.Errorf("create business unit: %w", err)
					}
					if err := tx.WithContext(ctx).Create(folder).Error; err != nil {
						return fmt.Errorf("create business-unit folder: %w", err)
					}
					return nil
				}); err != nil {
					logger.Error("failed to create business unit and folder: %v", err)
					SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
					return
				}
				folderPersisted = true
			}
		}
	}

	if !folderPersisted {
		if err := h.store.CreateFolder(ctx, folder); err != nil {
			logger.Error("failed to create folder: %v", err)
			SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
			return
		}
	}

	SendJSON(ctx, map[string]any{
		"folder": folder,
	})
}

// updateFolder handles PUT /api/prompt-repo/folders/{id}
func (h *PromptsHandler) updateFolder(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "folder ID is required")
		return
	}
	id, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid folder ID")
		return
	}

	var req UpdateFolderRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	folder, err := h.store.GetFolderByID(ctx, id)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "folder not found")
			return
		}
		logger.Error("failed to get folder: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	var renameEntity func(*gorm.DB) error
	if req.Name != "" {
		if folder.EntityID != nil && *folder.EntityID != "" {
			switch folder.Type {
			case "team":
				team, err := h.store.GetTeam(ctx, *folder.EntityID)
				if err != nil {
					logger.Error("failed to get team for folder rename: %v", err)
					SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
					return
				}
				if team == nil {
					SendError(ctx, fasthttp.StatusInternalServerError, "team linked to folder was not found")
					return
				}
				team.Name = req.Name
				renameEntity = func(tx *gorm.DB) error {
					if err := h.store.UpdateTeam(ctx, team, tx); err != nil {
						return fmt.Errorf("update team name from folder rename: %w", err)
					}
					return nil
				}
			case "customer":
				customer, err := h.store.GetCustomer(ctx, *folder.EntityID)
				if err != nil {
					logger.Error("failed to get customer for folder rename: %v", err)
					SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
					return
				}
				if customer == nil {
					SendError(ctx, fasthttp.StatusInternalServerError, "customer linked to folder was not found")
					return
				}
				customer.Name = req.Name
				renameEntity = func(tx *gorm.DB) error {
					if err := h.store.UpdateCustomer(ctx, customer, tx); err != nil {
						return fmt.Errorf("update customer name from folder rename: %w", err)
					}
					return nil
				}
			case "business_unit":
				renameEntity = func(tx *gorm.DB) error {
					result := tx.WithContext(ctx).Model(&tables.TableBusinessUnit{}).Where("id = ?", *folder.EntityID).Update("name", req.Name)
					if result.Error != nil {
						return fmt.Errorf("update business-unit name from folder rename: %w", result.Error)
					}
					if result.RowsAffected == 0 {
						return configstore.ErrNotFound
					}
					return nil
				}
			}
		}
		folder.Name = req.Name
	}
	parentChanged := false
	var oldAudience []string
	if req.ParentIDExists {
		newParent := ""
		if req.ParentID != nil {
			newParent = *req.ParentID
		}
		oldParent := ""
		if folder.ParentID != nil {
			oldParent = *folder.ParentID
		}
		if newParent != oldParent {
			if strings.HasPrefix(folder.Type, "system_") {
				SendError(ctx, fasthttp.StatusBadRequest, "system folders cannot be moved")
				return
			}
			if folder.Type == "customer" {
				if h.lifecycle == nil {
					SendError(ctx, fasthttp.StatusBadRequest, "lifecycle manager not initialized")
					return
				}
				resolvedParent, err := h.lifecycle.resolveCustomerPlacement(ctx, newParent)
				if err != nil {
					SendError(ctx, fasthttp.StatusBadRequest, err.Error())
					return
				}
				newParent = resolvedParent
			}
			if folder.Type == "business_unit" {
				if h.lifecycle == nil {
					SendError(ctx, fasthttp.StatusBadRequest, "lifecycle manager not initialized")
					return
				}
				_, resolvedParent, err := h.lifecycle.resolveBusinessUnitPlacement(ctx, newParent)
				if err != nil {
					SendError(ctx, fasthttp.StatusBadRequest, err.Error())
					return
				}
				if resolvedParent != "" {
					newParent = resolvedParent
				}
				if folder.EntityID != nil && *folder.EntityID != "" {
					if err := h.lifecycle.buMoveFitsCustomerBudget(ctx, *folder.EntityID, newParent); err != nil {
						SendError(ctx, fasthttp.StatusBadRequest, err.Error())
						return
					}
				}
			}
			if folder.Type == "team" {
				if h.lifecycle == nil {
					SendError(ctx, fasthttp.StatusBadRequest, "lifecycle manager not initialized")
					return
				}
				if newParent == "" {
					teamsRoot, tErr := h.lifecycle.EnsureSystemFolder(ctx, "Teams", "system_teams_root", nil)
					if tErr != nil || teamsRoot == nil {
						SendError(ctx, fasthttp.StatusBadRequest, "could not resolve Teams root folder")
						return
					}
					newParent = teamsRoot.ID
				}
				if _, _, err := h.lifecycle.resolveTeamPlacement(ctx, newParent); err != nil {
					SendError(ctx, fasthttp.StatusBadRequest, err.Error())
					return
				}
				if folder.EntityID != nil && *folder.EntityID != "" {
					if err := h.lifecycle.teamMoveFitsCustomerBudget(ctx, *folder.EntityID, newParent); err != nil {
						SendError(ctx, fasthttp.StatusBadRequest, err.Error())
						return
					}
				}
			}
			if folder.Type == "user" {
				if h.lifecycle == nil {
					SendError(ctx, fasthttp.StatusBadRequest, "lifecycle manager not initialized")
					return
				}
				resolvedParent, err := h.lifecycle.resolveUserPlacement(ctx, newParent)
				if err != nil {
					SendError(ctx, fasthttp.StatusBadRequest, err.Error())
					return
				}
				newParent = resolvedParent
			}
			if newParent != "" {
				if newParent == folder.ID || h.folderIsDescendant(ctx, newParent, folder.ID) {
					SendError(ctx, fasthttp.StatusBadRequest, "a folder cannot be moved into itself or one of its subfolders")
					return
				}
				if _, err := h.store.GetFolderByID(ctx, newParent); err != nil {
					SendError(ctx, fasthttp.StatusBadRequest, "parent folder not found")
					return
				}
			}
			parentChanged = true
			if h.lifecycle != nil {
				oldAudience = h.lifecycle.folderAudienceTeams(ctx, folder.ID)
			}
		}
		if newParent == "" {
			folder.ParentID = nil
		} else {
			folder.ParentID = &newParent
		}
	}
	if req.DescriptionExists {
		folder.Description = req.Description
	}

	updateFolder := func(tx *gorm.DB) error {
		if renameEntity != nil {
			if err := renameEntity(tx); err != nil {
				return err
			}
		}
		result := tx.WithContext(ctx).Where("id = ?", folder.ID).Save(folder)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return configstore.ErrNotFound
		}
		return nil
	}
	var updateErr error
	if renameEntity != nil {
		updateErr = h.store.ExecuteTransaction(ctx, updateFolder)
	} else {
		updateErr = h.store.UpdateFolder(ctx, folder)
	}
	if updateErr != nil {
		logger.Error("failed to update folder: %v", updateErr)
		SendError(ctx, fasthttp.StatusInternalServerError, updateErr.Error())
		return
	}
	if parentChanged && h.lifecycle != nil {
		h.lifecycle.OnFolderMoved(ctx, folder.ID, oldAudience)
		if folder.Type == "team" && folder.EntityID != nil && *folder.EntityID != "" && folder.ParentID != nil {
			if err := h.lifecycle.applyTeamPlacement(ctx, *folder.EntityID, *folder.ParentID); err != nil {
				logger.Error("failed to sync team after folder move: %v", err)
				SendError(ctx, fasthttp.StatusBadRequest, err.Error())
				return
			}
			if h.moveSync != nil {
				_, _ = h.moveSync.ReloadTeam(ctx, *folder.EntityID)
				h.moveSync.ReloadBusinessUnitTeamIndex(ctx)
			}
		}
		if folder.Type == "business_unit" && folder.EntityID != nil && *folder.EntityID != "" {
			parentID := ""
			if folder.ParentID != nil {
				parentID = *folder.ParentID
			}
			if err := h.lifecycle.applyBusinessUnitPlacement(ctx, *folder.EntityID, parentID); err != nil {
				logger.Error("failed to sync business unit after folder move: %v", err)
				SendError(ctx, fasthttp.StatusBadRequest, err.Error())
				return
			}
			if h.moveSync != nil {
				h.moveSync.ReloadBusinessUnitTeamIndex(ctx)
			}
		}
	}

	SendJSON(ctx, map[string]any{
		"folder": folder,
	})
}

// folderIsDescendant reports whether folderID sits somewhere below ancestorID.
func (h *PromptsHandler) folderIsDescendant(ctx context.Context, folderID, ancestorID string) bool {
	currID := folderID
	for i := 0; i < maxFolderDepth && currID != ""; i++ {
		f, err := h.store.GetFolderByID(ctx, currID)
		if err != nil || f == nil || f.ParentID == nil {
			return false
		}
		if *f.ParentID == ancestorID {
			return true
		}
		currID = *f.ParentID
	}
	// Depth exhausted: treat as a cycle rather than allowing the move.
	return currID != ""
}

// folderSubtreeIDs returns rootID and every folder below it.
func (h *PromptsHandler) folderSubtreeIDs(ctx context.Context, rootID string) ([]string, error) {
	ids := []string{rootID}
	if h.store == nil || h.store.DB() == nil {
		return nil, fmt.Errorf("database is unavailable while loading folder subtree")
	}
	db := h.store.DB().WithContext(ctx)
	seen := map[string]bool{rootID: true}
	frontier := []string{rootID}
	for depth := 0; depth < maxFolderDepth && len(frontier) > 0; depth++ {
		var children []tables.TableFolder
		if err := db.Select("id").Where("parent_id IN ?", frontier).Find(&children).Error; err != nil {
			return nil, fmt.Errorf("load child folders for %s: %w", rootID, err)
		}
		frontier = frontier[:0]
		for _, c := range children {
			if !seen[c.ID] {
				seen[c.ID] = true
				ids = append(ids, c.ID)
				frontier = append(frontier, c.ID)
			}
		}
	}
	if len(frontier) > 0 {
		return nil, fmt.Errorf("folder subtree exceeds maximum supported depth of %d", maxFolderDepth)
	}
	return ids, nil
}

// deleteFolder handles DELETE /api/prompt-repo/folders/{id}
func (h *PromptsHandler) deleteFolder(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "folder ID is required")
		return
	}
	id, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid folder ID")
		return
	}

	folder, err := h.store.GetFolderByID(ctx, id)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "folder not found")
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	// 1. Prevent deleting the primary system root folder unless it is a duplicate
	if (folder.ParentID == nil || *folder.ParentID == "") && strings.HasPrefix(folder.Type, "system_") {
		if h.store != nil && h.store.DB() != nil {
			var count int64
			if err := h.store.DB().WithContext(ctx).Model(&tables.TableFolder{}).
				Where("type = ? AND (parent_id IS NULL OR parent_id = '')", folder.Type).
				Count(&count).Error; err != nil {
				logger.Error("failed to count system root folders: %v", err)
				SendError(ctx, fasthttp.StatusInternalServerError, "Could not verify whether this system folder can be deleted")
				return
			}
			if count <= 1 {
				SendError(ctx, fasthttp.StatusBadRequest, "Primary system root folder cannot be deleted")
				return
			}
		} else {
			SendError(ctx, fasthttp.StatusBadRequest, "Primary system root folder cannot be deleted")
			return
		}
	}

	// Unassign every prompt in this folder and its subfolders from all users immediately
	subtreeIDs, err := h.folderSubtreeIDs(ctx, folder.ID)
	if err != nil {
		logger.Error("failed to load folder subtree before deletion: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Could not check folder contents")
		return
	}
	var childPrompts []tables.TablePrompt
	if err := h.store.DB().WithContext(ctx).Select("id").Where("folder_id IN ?", subtreeIDs).Find(&childPrompts).Error; err != nil {
		logger.Error("failed to load prompts in folder subtree: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Could not check folder contents")
		return
	}
	for _, cp := range childPrompts {
		if err := h.unassignPromptFromAllUsers(ctx, cp.ID); err != nil {
			logger.Error("failed to revoke prompt %s access before folder deletion: %v", cp.ID, err)
			SendError(ctx, fasthttp.StatusInternalServerError, "Could not revoke prompt access")
			return
		}
	}

	// 2. Check if folder is already inside one of the Removed roots or marked archived
	isAlreadyArchived := strings.HasPrefix(folder.Type, "archived_") || strings.HasSuffix(folder.Name, " (Archived)")
	if !isAlreadyArchived && folder.ParentID != nil {
		parent, pErr := h.store.GetFolderByID(ctx, *folder.ParentID)
		if pErr == nil && parent != nil && (strings.HasPrefix(parent.Type, "system_removed_") || strings.HasPrefix(parent.Name, "Removed ")) {
			isAlreadyArchived = true
		}
	}

	if !isAlreadyArchived && h.lifecycle != nil {
		// Non-destructive soft-archive: move folder to appropriate Removed root!
		var targetRootType, targetRootName string
		switch folder.Type {
		case "customer":
			targetRootType = "system_removed_customers_root"
			targetRootName = "Removed Customers"
		case "business_unit":
			targetRootType = "system_removed_business_units_root"
			targetRootName = "Removed Business Units"
		case "team":
			targetRootType = "system_removed_teams_root"
			targetRootName = "Removed Teams"
		default:
			targetRootType = "system_removed_users_root"
			targetRootName = "Removed Users"
		}

		removedRoot, err := h.lifecycle.EnsureSystemFolder(ctx, targetRootName, targetRootType, nil)
		if err != nil {
			logger.Error("failed to resolve archive folder %s: %v", targetRootName, err)
			SendError(ctx, fasthttp.StatusInternalServerError, "Could not prepare the archive folder")
			return
		}
		if removedRoot != nil {
			archivedName := folder.Name
			if !strings.HasSuffix(archivedName, " (Archived)") {
				archivedName += " (Archived)"
			}
			folder.ParentID = &removedRoot.ID
			folder.Name = archivedName
			if folder.Type != "" {
				folder.Type = "archived_" + folder.Type
			}
			if err := h.store.UpdateFolder(ctx, folder); err != nil {
				logger.Error("failed to archive folder %s: %v", folder.ID, err)
				SendError(ctx, fasthttp.StatusInternalServerError, "Could not move folder to the archive")
				return
			}
			h.reloadCache(ctx)
			SendJSON(ctx, map[string]any{
				"message": "folder safely moved to " + targetRootName,
			})
			return
		}
	}

	if err := h.store.DeleteFolder(ctx, id); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "folder not found")
			return
		}
		logger.Error("failed to delete folder: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	h.reloadCache(ctx)
	SendJSON(ctx, map[string]any{
		"message": "folder deleted permanently",
	})
}

// ============================================================================
// Prompt Handlers
// ============================================================================

// getPrompts handles GET /api/prompt-repo/prompts
func (h *PromptsHandler) getPrompts(ctx *fasthttp.RequestCtx) {
	var folderID *string
	if folderIDParam := string(ctx.QueryArgs().Peek("folder_id")); folderIDParam != "" {
		folderID = &folderIDParam
	}

	prompts, err := h.store.GetPrompts(ctx, folderID)
	if err != nil {
		logger.Error("failed to get prompts: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	tokenVal := ctx.UserValue(schemas.GatewayContextKeySessionToken)
	var isUserRole bool

	if token, ok := tokenVal.(string); ok && token != "" {
		session, err := h.store.GetSession(ctx, token)
		if err == nil && session != nil {
			if !isWorkspaceAdminRole(session.Role) {
				isUserRole = true
			}
		}
	}

	if isUserRole {
		var filteredPrompts []tables.TablePrompt
		for _, p := range prompts {
			if !h.checkPromptAccess(ctx, p.ID) {
				continue
			}
			filteredPrompts = append(filteredPrompts, p)
		}
		prompts = filteredPrompts
	}

	SendJSON(ctx, map[string]any{
		"prompts": prompts,
	})
}

// getPromptByID handles GET /api/prompt-repo/prompts/{id}
func (h *PromptsHandler) getPromptByID(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	id, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	if !h.checkPromptAccess(ctx, id) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	prompt, err := h.store.GetPromptByID(ctx, id)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
			return
		}
		logger.Error("failed to get prompt: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"prompt": prompt,
	})
}

// createPrompt handles POST /api/prompt-repo/prompts
func (h *PromptsHandler) createPrompt(ctx *fasthttp.RequestCtx) {
	var req CreatePromptRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name is required")
		return
	}

	// Auto-create folder if name contains a slash (e.g. "Agents/Support Bot")
	if strings.Contains(req.Name, "/") {
		parts := strings.Split(req.Name, "/")
		if len(parts) >= 2 {
			folderName := strings.TrimSpace(parts[0])
			promptName := strings.TrimSpace(strings.Join(parts[1:], "/"))
			if folderName != "" && promptName != "" {
				req.Name = promptName
				if req.FolderName == "" && (req.FolderID == nil || *req.FolderID == "") {
					req.FolderName = folderName
				}
			}
		}
	}

	// Auto-create or resolve folder by name if folder_name is provided
	if req.FolderName != "" && (req.FolderID == nil || *req.FolderID == "") {
		folders, _ := h.store.GetFolders(ctx)
		var existingFolder *tables.TableFolder
		for i := range folders {
			if strings.EqualFold(folders[i].Name, req.FolderName) {
				existingFolder = &folders[i]
				break
			}
		}
		if existingFolder != nil {
			req.FolderID = &existingFolder.ID
		} else {
			newFolder := &tables.TableFolder{
				ID:   uuid.New().String(),
				Name: req.FolderName,
			}
			if err := h.store.CreateFolder(ctx, newFolder); err == nil {
				req.FolderID = &newFolder.ID
			}
		}
	}

	// Normalize empty folder_id to nil (treat as root)
	if req.FolderID != nil && *req.FolderID == "" {
		req.FolderID = nil
	}
	// Verify folder exists if folder_id is provided, or auto-create/resolve if it matches a folder name
	if req.FolderID != nil {
		if _, err := h.store.GetFolderByID(ctx, *req.FolderID); err != nil {
			if errors.Is(err, configstore.ErrNotFound) {
				folders, _ := h.store.GetFolders(ctx)
				var match *tables.TableFolder
				for i := range folders {
					if strings.EqualFold(folders[i].Name, *req.FolderID) {
						match = &folders[i]
						break
					}
				}
				if match != nil {
					req.FolderID = &match.ID
				} else {
					newFolder := &tables.TableFolder{
						ID:   uuid.New().String(),
						Name: *req.FolderID,
					}
					if err := h.store.CreateFolder(ctx, newFolder); err == nil {
						req.FolderID = &newFolder.ID
					} else {
						SendError(ctx, fasthttp.StatusBadRequest, "folder not found")
						return
					}
				}
			} else {
				logger.Error("failed to get folder: %v", err)
				SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	prompt := &tables.TablePrompt{
		ID:       uuid.New().String(),
		Name:     req.Name,
		FolderID: req.FolderID,
	}

	if err := h.store.CreatePrompt(ctx, prompt); err != nil {
		logger.Error("failed to create prompt: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	var creatorUsername string
	tokenVal := ctx.UserValue(schemas.GatewayContextKeySessionToken)
	if token, ok := tokenVal.(string); ok && token != "" {
		if session, err := h.store.GetSession(ctx, token); err == nil && session != nil {
			creatorUsername = session.Username
		}
	}

	if h.lifecycle != nil {
		_ = h.lifecycle.OnPromptCreated(ctx, prompt, creatorUsername)
	}

	h.reloadCache(ctx)
	SendJSON(ctx, map[string]any{
		"prompt": prompt,
	})
}

// updatePrompt handles PUT /api/prompt-repo/prompts/{id}
func (h *PromptsHandler) updatePrompt(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	id, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	if !h.checkPromptAccess(ctx, id) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	var req UpdatePromptRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	// Detect if folder_id key was present in JSON (even if null)
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(ctx.PostBody(), &rawFields); err == nil {
		if _, exists := rawFields["folder_id"]; exists {
			req.FolderIDExists = true
		}
	}

	prompt, err := h.store.GetPromptByID(ctx, id)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
			return
		}
		logger.Error("failed to get prompt: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	oldFolderID := ""
	if prompt.FolderID != nil {
		oldFolderID = *prompt.FolderID
	}
	if req.Name != "" {
		prompt.Name = req.Name
	}
	if req.FolderIDExists {
		if req.FolderID == nil {
			// folder_id: null — move to root
			prompt.FolderID = nil
		} else if *req.FolderID == "" {
			prompt.FolderID = nil
		} else {
			// Verify folder exists
			if _, err := h.store.GetFolderByID(ctx, *req.FolderID); err != nil {
				if errors.Is(err, configstore.ErrNotFound) {
					SendError(ctx, fasthttp.StatusBadRequest, "folder not found")
					return
				}
				logger.Error("failed to get folder: %v", err)
				SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
				return
			}
			prompt.FolderID = req.FolderID
		}
	}

	if err := h.store.UpdatePrompt(ctx, prompt); err != nil {
		logger.Error("failed to update prompt: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if req.FolderIDExists && h.lifecycle != nil {
		targetFolder := ""
		if prompt.FolderID != nil {
			targetFolder = *prompt.FolderID
		}
		h.lifecycle.OnPromptMoved(ctx, prompt.ID, &oldFolderID, targetFolder)
	}

	h.reloadCache(ctx)
	SendJSON(ctx, map[string]any{
		"prompt": prompt,
	})
}

// deletePrompt handles DELETE /api/prompt-repo/prompts/{id}
func (h *PromptsHandler) deletePrompt(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	id, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	if !h.checkPromptAccess(ctx, id) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	prompt, err := h.store.GetPromptByID(ctx, id)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	// Check if prompt is already inside Removed Users or an archived folder
	isAlreadyInTrash := false
	if prompt.FolderID != nil {
		parent, pErr := h.store.GetFolderByID(ctx, *prompt.FolderID)
		if pErr == nil && parent != nil && (strings.HasPrefix(parent.Type, "system_removed_") || strings.HasPrefix(parent.Name, "Removed ") || strings.HasPrefix(parent.Type, "archived_")) {
			isAlreadyInTrash = true
		}
	}

	// Always unassign prompt from all users immediately so they cannot see or access it
	if err := h.unassignPromptFromAllUsers(ctx, prompt.ID); err != nil {
		logger.Error("failed to revoke prompt %s access before deletion: %v", prompt.ID, err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Could not revoke prompt access")
		return
	}

	if !isAlreadyInTrash && h.lifecycle != nil {
		removedUsersRoot, rErr := h.lifecycle.EnsureSystemFolder(ctx, "Removed Users", "system_removed_users_root", nil)
		if rErr != nil {
			logger.Error("failed to resolve Removed Users folder: %v", rErr)
			SendError(ctx, fasthttp.StatusInternalServerError, "Could not prepare the archive folder")
			return
		}
		if removedUsersRoot != nil {
			prompt.FolderID = &removedUsersRoot.ID
			if err := h.store.UpdatePrompt(ctx, prompt); err != nil {
				logger.Error("failed to archive prompt %s: %v", prompt.ID, err)
				SendError(ctx, fasthttp.StatusInternalServerError, "Could not move prompt to the archive")
				return
			}
			h.reloadCache(ctx)
			SendJSON(ctx, map[string]any{
				"message": "prompt moved to Removed Users",
			})
			return
		}
	}

	if err := h.store.DeletePrompt(ctx, id); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
			return
		}
		logger.Error("failed to delete prompt: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	h.reloadCache(ctx)
	SendJSON(ctx, map[string]any{
		"message": "prompt deleted permanently",
	})
}

// ============================================================================
// Version Handlers
// ============================================================================

// getPromptVersions handles GET /api/prompt-repo/prompts/{id}/versions
func (h *PromptsHandler) getPromptVersions(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	promptID, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	if !h.checkPromptAccess(ctx, promptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	versions, err := h.store.GetPromptVersions(ctx, promptID)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
			return
		}
		logger.Error("failed to get versions: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"versions": versions,
	})
}

// getVersionByID handles GET /api/prompt-repo/versions/{id}
func (h *PromptsHandler) getVersionByID(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "version ID is required")
		return
	}
	idStr, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid version ID")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid version ID")
		return
	}

	version, err := h.store.GetPromptVersionByID(ctx, uint(id))
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "version not found")
			return
		}
		logger.Error("failed to get version: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if !h.checkPromptAccess(ctx, version.PromptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	SendJSON(ctx, map[string]any{
		"version": version,
	})
}

// createVersion handles POST /api/prompt-repo/prompts/{id}/versions
func (h *PromptsHandler) createVersion(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	promptID, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	if !h.checkPromptAccess(ctx, promptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	var req CreateVersionRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	// Verify prompt exists
	if _, err := h.store.GetPromptByID(ctx, promptID); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
			return
		}
		logger.Error("failed to get prompt: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	// Convert messages
	var messages []tables.TablePromptVersionMessage
	for _, msg := range req.Messages {
		messages = append(messages, tables.TablePromptVersionMessage{
			PromptID: promptID,
			Message:  msg,
		})
	}

	// Strip variable values — versions store keys only; values live in sessions
	var versionVars tables.PromptVariables
	if len(req.Variables) > 0 {
		versionVars = make(tables.PromptVariables, len(req.Variables))
		for key := range req.Variables {
			versionVars[key] = ""
		}
	}

	version := &tables.TablePromptVersion{
		PromptID:      promptID,
		CommitMessage: req.CommitMessage,
		ModelParams:   req.ModelParams,
		Provider:      req.Provider,
		Model:         req.Model,
		Variables:     versionVars,
		Messages:      messages,
	}

	if err := h.store.CreatePromptVersion(ctx, version); err != nil {
		logger.Error("failed to create version: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	h.reloadCache(ctx)
	SendJSON(ctx, map[string]any{
		"version": version,
	})
}

// PromptAccessResponse represents the access configuration for a prompt
type PromptAccessResponse struct {
	PromptID       string             `json:"prompt_id"`
	PromptName     string             `json:"prompt_name"`
	CustomerIDs    []string           `json:"customer_ids"`
	TeamIDs        []string           `json:"team_ids"`
	UserIDs        []string           `json:"user_ids"`
	Users          []PromptAccessUser `json:"users"`
	EffectiveUsers []PromptAccessUser `json:"effective_users"`
}

type PromptAccessUser struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	Origin     string `json:"origin"`      // "direct", "team", "customer"
	OriginName string `json:"origin_name"` // "Direct Assignment", "Engineering", "Acme Corp / QA"
}

type UpdatePromptAccessRequest struct {
	CustomerIDs []string `json:"customer_ids"`
	TeamIDs     []string `json:"team_ids"`
	UserIDs     []string `json:"user_ids"`
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, sep)
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

func sanitizeIDList(ids []string) []string {
	seen := make(map[string]bool)
	res := make([]string, 0, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			res = append(res, trimmed)
		}
	}
	return res
}

// checkPromptAccess verifies if the current session has access to the given prompt ID
func (h *PromptsHandler) checkPromptAccess(ctx *fasthttp.RequestCtx, promptID string) bool {
	tokenVal := ctx.UserValue(schemas.GatewayContextKeySessionToken)
	token, ok := tokenVal.(string)
	if !ok || token == "" {
		return true
	}
	session, err := h.store.GetSession(ctx, token)
	if err != nil || session == nil {
		return true
	}
	if isWorkspaceAdminRole(session.Role) {
		return true
	}
	dbUser, err := h.store.GetUserByUsername(ctx, session.Username)
	if err != nil || dbUser == nil {
		return false
	}

	prompt, pErr := h.store.GetPromptByID(ctx, promptID)
	if pErr != nil || prompt == nil {
		return false
	}

	return userCanAccessPrompt(ctx, h.store, dbUser, prompt)
}

// userCanAccessPrompt is the shared, fail-closed assignment check used for both
// Prompt Repository reads and inference requests. An empty allowlist grants no
// access by itself; ownership, an explicit user grant, or team/customer scope is
// required.
func userCanAccessPrompt(ctx context.Context, store configstore.ConfigStore, dbUser *tables.TableUser, prompt *tables.TablePrompt) bool {
	if store == nil || dbUser == nil || prompt == nil {
		return false
	}
	if prompt.FolderID != nil && store.DB() != nil {
		var folder tables.TableFolder
		if err := store.DB().WithContext(ctx).Where("id = ?", *prompt.FolderID).First(&folder).Error; err == nil {
			if strings.HasPrefix(folder.Type, "system_removed_") ||
				strings.HasPrefix(folder.Type, "archived_") ||
				strings.HasPrefix(folder.Name, "Removed ") ||
				strings.HasSuffix(folder.Name, " (Archived)") {
				return false
			}
		}
	}
	if prompt.OwnerUserID != nil && *prompt.OwnerUserID == dbUser.ID {
		return true
	}
	for _, userID := range strings.Split(prompt.UserIDs, ",") {
		if strings.TrimSpace(userID) == dbUser.ID && dbUser.ID != "" {
			return true
		}
	}
	for _, promptID := range strings.Split(dbUser.AllowedPromptRepos, ",") {
		if id := strings.TrimSpace(promptID); id != "" && id == prompt.ID {
			return true
		}
	}
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil {
		return false
	}
	teams, err := ws.ListTeamsForUser(ctx, dbUser.ID)
	if err != nil {
		return false
	}
	teamIDs := make(map[string]bool, len(teams))
	customerIDs := make(map[string]bool)
	for _, membership := range teams {
		if membership.TeamID == "" {
			continue
		}
		teamIDs[membership.TeamID] = true
		team, teamErr := store.GetTeam(ctx, membership.TeamID)
		if teamErr == nil && team != nil && team.CustomerID != nil && *team.CustomerID != "" {
			customerIDs[*team.CustomerID] = true
		}
	}
	for _, teamID := range strings.Split(prompt.TeamIDs, ",") {
		if teamIDs[strings.TrimSpace(teamID)] {
			return true
		}
	}
	for _, customerID := range strings.Split(prompt.CustomerIDs, ",") {
		if customerIDs[strings.TrimSpace(customerID)] {
			return true
		}
	}
	return false
}

// getPromptAccess returns the Customers, Teams, Users, and effective users assigned to this prompt
func (h *PromptsHandler) getPromptAccess(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	id, ok := idVal.(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	prompt, err := h.store.GetPromptByID(ctx, id)
	if err != nil || prompt == nil {
		SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
		return
	}

	customerIDs := splitNonEmpty(prompt.CustomerIDs, ",")
	teamIDs := splitNonEmpty(prompt.TeamIDs, ",")
	userIDs := splitNonEmpty(prompt.UserIDs, ",")

	seenUserIDs := make(map[string]bool)
	users := make([]PromptAccessUser, 0)

	appendUser := func(userID, origin, originName string) error {
		if userID == "" || seenUserIDs[userID] {
			return nil
		}
		u, err := h.store.GetUserByID(ctx, userID)
		if errors.Is(err, configstore.ErrNotFound) || (err == nil && u == nil) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load prompt access user %s: %w", userID, err)
		}
		seenUserIDs[userID] = true
		name := u.Username
		if name == "" {
			name = u.Email
		}
		users = append(users, PromptAccessUser{
			ID:         u.ID,
			UserID:     u.ID,
			Name:       name,
			Username:   name,
			Email:      u.Email,
			Role:       u.Role,
			Origin:     origin,
			OriginName: originName,
		})
		return nil
	}

	// 1. Direct Users
	for _, uid := range userIDs {
		if err := appendUser(uid, "direct", "Direct Assignment"); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load prompt access: "+err.Error())
			return
		}
	}

	// 2. Users from Teams
	ws, _ := configstore.AsWorkspaceStore(h.store)
	if ws == nil && (len(teamIDs) > 0 || len(customerIDs) > 0) {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Workspace access data is unavailable")
		return
	}
	if ws != nil {
		for _, tid := range teamIDs {
			team, err := h.store.GetTeam(ctx, tid)
			teamName := "Team"
			if err != nil && !errors.Is(err, configstore.ErrNotFound) {
				SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load team access: "+err.Error())
				return
			}
			if team != nil {
				teamName = team.Name
			}
			members, err := ws.ListTeamMembers(ctx, tid)
			if err != nil {
				SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load team members: "+err.Error())
				return
			}
			for _, m := range members {
				if err := appendUser(m.UserID, "team", teamName); err != nil {
					SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load prompt access: "+err.Error())
					return
				}
			}
		}

		// 3. Users from Customers
		for _, cid := range customerIDs {
			cust, err := h.store.GetCustomer(ctx, cid)
			custName := "Customer"
			if err != nil && !errors.Is(err, configstore.ErrNotFound) {
				SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load customer access: "+err.Error())
				return
			}
			if cust != nil {
				custName = cust.Name
			}
			var custTeams []tables.TableTeam
			if err := h.store.DB().WithContext(ctx).Where("customer_id = ?", cid).Find(&custTeams).Error; err != nil {
				SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load customer teams: "+err.Error())
				return
			}
			for _, ct := range custTeams {
				members, err := ws.ListTeamMembers(ctx, ct.ID)
				if err != nil {
					SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load customer team members: "+err.Error())
					return
				}
				for _, m := range members {
					if err := appendUser(m.UserID, "customer", custName+" / "+ct.Name); err != nil {
						SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load prompt access: "+err.Error())
						return
					}
				}
			}
		}
	}

	SendJSON(ctx, PromptAccessResponse{
		PromptID:       prompt.ID,
		PromptName:     prompt.Name,
		CustomerIDs:    customerIDs,
		TeamIDs:        teamIDs,
		UserIDs:        userIDs,
		Users:          users,
		EffectiveUsers: users,
	})
}

// updatePromptAccess updates the Customers, Teams, and Users assigned to this prompt
func (h *PromptsHandler) updatePromptAccess(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	id, ok := idVal.(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	prompt, err := h.store.GetPromptByID(ctx, id)
	if err != nil || prompt == nil {
		SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
		return
	}

	callerID, role := h.promptCallerIdentity(ctx)
	if role != "" && !isWorkspaceAdminRole(role) && (prompt.OwnerUserID == nil || *prompt.OwnerUserID != callerID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden: admin access or prompt ownership required")
		return
	}

	var req UpdatePromptAccessRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	cleanCustomerIDs := sanitizeIDList(req.CustomerIDs)
	cleanTeamIDs := sanitizeIDList(req.TeamIDs)
	cleanUserIDs := sanitizeIDList(req.UserIDs)

	cStr := strings.Join(cleanCustomerIDs, ",")
	tStr := strings.Join(cleanTeamIDs, ",")
	uStr := strings.Join(cleanUserIDs, ",")

	if err := h.store.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
		oldAudience, err := promptAudienceUserIDs(ctx, tx, splitNonEmpty(prompt.UserIDs, ","), splitNonEmpty(prompt.TeamIDs, ","), splitNonEmpty(prompt.CustomerIDs, ","))
		if err != nil {
			return fmt.Errorf("resolve previous prompt audience: %w", err)
		}
		newAudience, err := promptAudienceUserIDs(ctx, tx, cleanUserIDs, cleanTeamIDs, cleanCustomerIDs)
		if err != nil {
			return fmt.Errorf("resolve requested prompt audience: %w", err)
		}
		if prompt.OwnerUserID != nil && strings.TrimSpace(*prompt.OwnerUserID) != "" {
			oldAudience[*prompt.OwnerUserID] = true
			newAudience[*prompt.OwnerUserID] = true
		}
		if err := tx.WithContext(ctx).Model(&tables.TablePrompt{}).Where("id = ?", id).Updates(map[string]any{
			"customer_ids": cStr,
			"team_ids":     tStr,
			"user_ids":     uStr,
		}).Error; err != nil {
			return fmt.Errorf("update prompt assignment: %w", err)
		}

		allUsers := make(map[string]struct{}, len(oldAudience)+len(newAudience))
		for uid := range oldAudience {
			allUsers[uid] = struct{}{}
		}
		for uid := range newAudience {
			allUsers[uid] = struct{}{}
		}
		for uid := range allUsers {
			var user tables.TableUser
			if err := tx.WithContext(ctx).Where("id = ?", uid).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue // stale user assignment; prompt rows remain the source of truth.
				}
				return fmt.Errorf("load prompt audience user %s: %w", uid, err)
			}
			repos := splitNonEmpty(user.AllowedPromptRepos, ",")
			filtered := make([]string, 0, len(repos)+1)
			found := false
			for _, repoID := range repos {
				if repoID == id {
					if newAudience[uid] && !found {
						filtered = append(filtered, id)
						found = true
					}
					continue
				}
				filtered = append(filtered, repoID)
			}
			if newAudience[uid] && !found {
				filtered = append(filtered, id)
			}
			newRepos := strings.Join(filtered, ",")
			if newRepos == user.AllowedPromptRepos {
				continue
			}
			if err := tx.WithContext(ctx).Model(&tables.TableUser{}).Where("id = ?", uid).Update("allowed_prompt_repos", newRepos).Error; err != nil {
				return fmt.Errorf("sync prompt assignment for user %s: %w", uid, err)
			}
		}
		return nil
	}); err != nil {
		logger.Error("failed to update prompt access: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to update prompt access: "+err.Error())
		return
	}

	prompt.CustomerIDs = cStr
	prompt.TeamIDs = tStr
	prompt.UserIDs = uStr

	h.reloadCache(ctx)
	h.getPromptAccess(ctx)
}

func promptAudienceUserIDs(ctx context.Context, tx *gorm.DB, userIDs, teamIDs, customerIDs []string) (map[string]bool, error) {
	users := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		if id = strings.TrimSpace(id); id != "" {
			users[id] = true
		}
	}
	teamSet := make(map[string]bool, len(teamIDs))
	for _, id := range teamIDs {
		if id = strings.TrimSpace(id); id != "" {
			teamSet[id] = true
		}
	}
	if len(customerIDs) > 0 {
		var customerTeams []tables.TableTeam
		if err := tx.WithContext(ctx).Where("customer_id IN ?", customerIDs).Find(&customerTeams).Error; err != nil {
			return nil, fmt.Errorf("list teams assigned through customers: %w", err)
		}
		for _, team := range customerTeams {
			if team.ID != "" {
				teamSet[team.ID] = true
			}
		}
	}
	if len(teamSet) > 0 {
		ids := make([]string, 0, len(teamSet))
		for id := range teamSet {
			ids = append(ids, id)
		}
		var members []tables.TableTeamMember
		if err := tx.WithContext(ctx).Where("team_id IN ?", ids).Find(&members).Error; err != nil {
			return nil, fmt.Errorf("list members of assigned teams: %w", err)
		}
		for _, member := range members {
			if member.UserID != "" {
				users[member.UserID] = true
			}
		}
	}
	return users, nil
}

// unassignPromptFromAllUsers removes promptID from allowed_prompt_repos across all users in the DB.
func (h *PromptsHandler) unassignPromptFromAllUsers(ctx context.Context, promptID string) error {
	if h == nil || h.store == nil || h.store.DB() == nil || strings.TrimSpace(promptID) == "" {
		return fmt.Errorf("prompt access store or prompt ID is unavailable")
	}
	db := h.store.DB().WithContext(ctx)
	return db.Transaction(func(tx *gorm.DB) error {
		var users []tables.TableUser
		if err := tx.WithContext(ctx).Where("allowed_prompt_repos LIKE ?", "%"+promptID+"%").Find(&users).Error; err != nil {
			return fmt.Errorf("list users assigned to prompt %s: %w", promptID, err)
		}
		for _, u := range users {
			repos := splitNonEmpty(u.AllowedPromptRepos, ",")
			var updated []string
			changed := false
			for _, r := range repos {
				trimmed := strings.TrimSpace(r)
				if trimmed != "" && trimmed != promptID {
					updated = append(updated, trimmed)
				} else if trimmed == promptID {
					changed = true
				}
			}
			if !changed {
				continue
			}
			newAllowed := strings.Join(updated, ",")
			if err := tx.WithContext(ctx).Model(&tables.TableUser{}).Where("id = ?", u.ID).Update("allowed_prompt_repos", newAllowed).Error; err != nil {
				return fmt.Errorf("remove prompt %s from user %s: %w", promptID, u.ID, err)
			}
		}
		return nil
	})
}

// promptCallerIdentity returns a stable owner id + role for prompt session history.
// Prefer governance_users.id; fall back to session username so env/admin logins still own rows.
func (h *PromptsHandler) promptCallerIdentity(ctx *fasthttp.RequestCtx) (userID, role string) {
	tokenVal := ctx.UserValue(schemas.GatewayContextKeySessionToken)
	token, ok := tokenVal.(string)
	if !ok || token == "" {
		return "", ""
	}
	session, err := h.store.GetSession(ctx, token)
	if err != nil || session == nil {
		return "", ""
	}
	role = strings.TrimSpace(session.Role)
	username := strings.TrimSpace(session.Username)
	if username == "" {
		return "", role
	}
	dbUser, err := h.store.GetUserByUsername(ctx, username)
	if err == nil && dbUser != nil && strings.TrimSpace(dbUser.ID) != "" {
		return dbUser.ID, role
	}
	// No governance_users row (e.g. bootstrap admin) — username is still unique per login.
	return username, role
}

// checkSessionOwnership: admins have full oversight across sessions; users access their own sessions or legacy unassigned sessions.
func (h *PromptsHandler) checkSessionOwnership(ctx *fasthttp.RequestCtx, session *tables.TablePromptSession) bool {
	if session == nil {
		return false
	}
	userID, role := h.promptCallerIdentity(ctx)
	if isWorkspaceAdminRole(role) {
		return true
	}
	if userID == "" {
		return false
	}
	if session.UserID == userID || session.UserID == "" {
		return true
	}
	if h.store != nil && h.store.DB() != nil {
		var user tables.TableUser
		if err := h.store.DB().WithContext(ctx).Where("id = ? OR username = ? OR email = ?", userID, userID, userID).First(&user).Error; err == nil {
			if session.UserID == user.ID || session.UserID == user.Username || session.UserID == user.Email {
				return true
			}
		}
	}
	return false
}

// deleteVersion handles DELETE /api/prompt-repo/versions/{id}
func (h *PromptsHandler) deleteVersion(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "version ID is required")
		return
	}
	idStr, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid version ID")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid version ID")
		return
	}

	version, err := h.store.GetPromptVersionByID(ctx, uint(id))
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "version not found")
			return
		}
		logger.Error("failed to get version: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if !h.checkPromptAccess(ctx, version.PromptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	if err := h.store.DeletePromptVersion(ctx, uint(id)); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "version not found")
			return
		}
		logger.Error("failed to delete version: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	h.reloadCache(ctx)
	SendJSON(ctx, map[string]any{
		"message": "version deleted successfully",
	})
}

// ============================================================================
// Session Handlers
// ============================================================================

// getPromptSessions handles GET /api/prompt-repo/prompts/{id}/sessions
func (h *PromptsHandler) getPromptSessions(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	promptID, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	if !h.checkPromptAccess(ctx, promptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	callerID, role := h.promptCallerIdentity(ctx)
	if callerID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "authentication required")
		return
	}

	// The playground auto-opens the first listed session and saves into it, so admins get their
	// own chats by default. ?user_id=<id> shows one user's chats and ?user_id=* shows everyone's.
	filterUserID := callerID
	if isWorkspaceAdminRole(role) {
		switch queryUser := strings.TrimSpace(string(ctx.QueryArgs().Peek("user_id"))); queryUser {
		case "":
		case "*":
			filterUserID = ""
		default:
			filterUserID = queryUser
		}
	}

	_, _ = h.applyPromptHistoryAutoDelete(ctx)

	sessions, err := h.store.GetPromptSessions(ctx, promptID, filterUserID)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
			return
		}
		logger.Error("failed to get sessions: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"sessions": sessions,
	})
}

// getSessionByID handles GET /api/prompt-repo/sessions/{id}
func (h *PromptsHandler) getSessionByID(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "session ID is required")
		return
	}
	idStr, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}

	session, err := h.store.GetPromptSessionByID(ctx, uint(id))
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "session not found")
			return
		}
		logger.Error("failed to get session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if !h.checkPromptAccess(ctx, session.PromptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	if !h.checkSessionOwnership(ctx, session) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	SendJSON(ctx, map[string]any{
		"session": session,
	})
}

// createSession handles POST /api/prompt-repo/prompts/{id}/sessions
func (h *PromptsHandler) createSession(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt ID is required")
		return
	}
	promptID, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid prompt ID")
		return
	}

	if !h.checkPromptAccess(ctx, promptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	var req CreateSessionRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	// Verify prompt exists
	if _, err := h.store.GetPromptByID(ctx, promptID); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "prompt not found")
			return
		}
		logger.Error("failed to get prompt: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	// If version_id is provided, copy messages from that version
	var messages []tables.TablePromptSessionMessage
	if req.VersionID != nil {
		version, err := h.store.GetPromptVersionByID(ctx, *req.VersionID)
		if err != nil {
			if errors.Is(err, configstore.ErrNotFound) {
				SendError(ctx, fasthttp.StatusBadRequest, "version not found")
				return
			}
			logger.Error("failed to get version: %v", err)
			SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
			return
		}
		// Verify version belongs to this prompt
		if version.PromptID != promptID {
			SendError(ctx, fasthttp.StatusBadRequest, "version does not belong to this prompt")
			return
		}
		// Copy messages from version
		for _, msg := range version.Messages {
			messages = append(messages, tables.TablePromptSessionMessage{
				PromptID: promptID,
				Message:  msg.Message,
			})
		}
		// Use version's model params, provider, model if not provided
		if req.Provider == "" {
			req.Provider = version.Provider
		}
		if req.Model == "" {
			req.Model = version.Model
		}
		if len(req.ModelParams) == 0 {
			req.ModelParams = version.ModelParams
		}
		if len(req.Variables) == 0 && len(version.Variables) > 0 {
			req.Variables = make(tables.PromptVariables, len(version.Variables))
			for key := range version.Variables {
				req.Variables[key] = ""
			}
		}
	} else {
		// Use provided messages
		for _, msg := range req.Messages {
			messages = append(messages, tables.TablePromptSessionMessage{
				PromptID: promptID,
				Message:  sanitizePromptMessageJSON(msg),
			})
		}
	}

	session := &tables.TablePromptSession{
		PromptID:    promptID,
		VersionID:   req.VersionID,
		Name:        req.Name,
		UserID:      "",
		ModelParams: req.ModelParams,
		Provider:    req.Provider,
		Model:       req.Model,
		Variables:   req.Variables,
		Messages:    messages,
	}
	callerID, _ := h.promptCallerIdentity(ctx)
	if callerID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "authentication required")
		return
	}
	session.UserID = callerID

	_, _ = h.applyPromptHistoryAutoDelete(ctx)

	if err := h.store.CreatePromptSession(ctx, session); err != nil {
		logger.Error("failed to create session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"session": session,
	})
}

// updateSession handles PUT /api/prompt-repo/sessions/{id}
func (h *PromptsHandler) updateSession(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "session ID is required")
		return
	}
	idStr, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}

	var req UpdateSessionRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	session, err := h.store.GetPromptSessionByID(ctx, uint(id))
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "session not found")
			return
		}
		logger.Error("failed to get session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if !h.checkPromptAccess(ctx, session.PromptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	if !h.checkSessionOwnership(ctx, session) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	if req.Name != "" {
		session.Name = req.Name
	}
	session.ModelParams = req.ModelParams
	session.Provider = req.Provider
	session.Model = req.Model
	if req.Variables != nil {
		session.Variables = req.Variables
	}

	// Update messages
	var messages []tables.TablePromptSessionMessage
	for _, msg := range req.Messages {
		messages = append(messages, tables.TablePromptSessionMessage{
			PromptID: session.PromptID,
			Message:  sanitizePromptMessageJSON(msg),
		})
	}
	session.Messages = messages

	if err := h.store.UpdatePromptSession(ctx, session); err != nil {
		logger.Error("failed to update session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"session": session,
	})
}

// deleteSession handles DELETE /api/prompt-repo/sessions/{id}
func (h *PromptsHandler) deleteSession(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "session ID is required")
		return
	}
	idStr, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}

	session, err := h.store.GetPromptSessionByID(ctx, uint(id))
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "session not found")
			return
		}
		logger.Error("failed to get session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if !h.checkPromptAccess(ctx, session.PromptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	if !h.checkSessionOwnership(ctx, session) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	if err := h.store.DeletePromptSession(ctx, uint(id)); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "session not found")
			return
		}
		logger.Error("failed to delete session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"message": "session deleted successfully",
	})
}

// renameSession handles PUT /api/prompt-repo/sessions/{id}/rename
func (h *PromptsHandler) renameSession(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "session ID is required")
		return
	}
	idStr, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}

	var req RenameSessionRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	session, err := h.store.GetPromptSessionByID(ctx, uint(id))
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "session not found")
			return
		}
		logger.Error("failed to get session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if !h.checkPromptAccess(ctx, session.PromptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	if !h.checkSessionOwnership(ctx, session) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	if err := h.store.RenamePromptSession(ctx, session.ID, req.Name); err != nil {
		logger.Error("failed to rename session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	session.Name = req.Name
	SendJSON(ctx, map[string]any{
		"session": session,
	})
}

// commitSession handles POST /api/prompt-repo/sessions/{id}/commit
func (h *PromptsHandler) commitSession(ctx *fasthttp.RequestCtx) {
	idVal := ctx.UserValue("id")
	if idVal == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "session ID is required")
		return
	}
	idStr, ok := idVal.(string)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid session ID")
		return
	}

	var req CommitSessionRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}

	if req.CommitMessage == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "commit_message is required")
		return
	}

	session, err := h.store.GetPromptSessionByID(ctx, uint(id))
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "session not found")
			return
		}
		logger.Error("failed to get session: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if !h.checkPromptAccess(ctx, session.PromptID) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	if !h.checkSessionOwnership(ctx, session) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}

	// Convert session messages to version messages
	var messages []tables.TablePromptVersionMessage
	if req.MessageIndices != nil {
		// Only include messages at the specified indices, deduplicating
		seen := make(map[int]struct{})
		for _, idx := range *req.MessageIndices {
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			if idx < 0 || idx >= len(session.Messages) {
				SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("message index %d out of range (0-%d)", idx, len(session.Messages)-1))
				return
			}
			msg := session.Messages[idx]
			messages = append(messages, tables.TablePromptVersionMessage{
				PromptID: session.PromptID,
				Message:  msg.Message,
			})
		}
	} else {
		for _, msg := range session.Messages {
			messages = append(messages, tables.TablePromptVersionMessage{
				PromptID: session.PromptID,
				Message:  msg.Message,
			})
		}
	}

	if len(messages) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "at least one message must be included in the version")
		return
	}

	// Copy variable keys from session with empty values for the version
	var versionVars tables.PromptVariables
	if len(session.Variables) > 0 {
		versionVars = make(tables.PromptVariables, len(session.Variables))
		for key := range session.Variables {
			versionVars[key] = ""
		}
	}

	version := &tables.TablePromptVersion{
		PromptID:      session.PromptID,
		CommitMessage: req.CommitMessage,
		ModelParams:   session.ModelParams,
		Provider:      session.Provider,
		Model:         session.Model,
		Variables:     versionVars,
		Messages:      messages,
	}

	if err := h.store.CreatePromptVersion(ctx, version); err != nil {
		logger.Error("failed to create version: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	h.reloadCache(ctx)
	SendJSON(ctx, map[string]any{
		"version": version,
	})
}

var (
	promptHistoryRetentionOnce sync.Once
	promptHistoryRetentionStop chan struct{}
)

// startPromptHistoryRetentionCleanup runs a recurring background loop (circle)
// every 5 minutes to purge prompt history older than the configured retention period
// when AutoDelete is enabled. It only deletes chat history (sessions & messages);
// it never touches users, teams, customers, or calculated budgets.
func (h *PromptsHandler) startPromptHistoryRetentionCleanup() {
	if h.store == nil {
		return
	}
	promptHistoryRetentionOnce.Do(func() {
		promptHistoryRetentionStop = make(chan struct{})
		go func() {
			// Initial sweep on boot
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			_, _ = h.applyPromptHistoryAutoDelete(ctx)
			cancel()

			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					sweepCtx, sweepCancel := context.WithTimeout(context.Background(), 2*time.Minute)
					_, _ = h.applyPromptHistoryAutoDelete(sweepCtx)
					sweepCancel()
				case <-promptHistoryRetentionStop:
					return
				}
			}
		}()
	})
}

func parsePromptHistoryRetentionDuration(retention string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(retention)) {
	case "24h", "1d":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	case "90d":
		return 90 * 24 * time.Hour
	case "180d":
		return 180 * 24 * time.Hour
	case "365d":
		return 365 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

func (h *PromptsHandler) applyPromptHistoryAutoDelete(ctx context.Context) (int64, error) {
	if h.store == nil {
		return 0, nil
	}
	settings, err := h.store.GetPromptHistorySettings(ctx)
	if err != nil || settings == nil || !settings.AutoDelete {
		return 0, nil
	}
	dur := parsePromptHistoryRetentionDuration(settings.Retention)
	cutoff := time.Now().Add(-dur)
	return h.store.DeletePromptSessionsBefore(ctx, cutoff)
}

// getPromptSettings handles GET /api/prompt-repo/settings
func (h *PromptsHandler) getPromptSettings(ctx *fasthttp.RequestCtx) {
	_, _ = h.applyPromptHistoryAutoDelete(ctx)

	settings, err := h.store.GetPromptHistorySettings(ctx)
	if err != nil {
		logger.Error("failed to get prompt history settings: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, settings)
}

// updatePromptSettings handles PUT /api/prompt-repo/settings
func (h *PromptsHandler) updatePromptSettings(ctx *fasthttp.RequestCtx) {
	callerID, _ := h.promptCallerIdentity(ctx)
	if callerID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "authentication required")
		return
	}

	var settings tables.PromptHistoryRetentionSettings
	if err := json.Unmarshal(ctx.PostBody(), &settings); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.store.SetPromptHistorySettings(ctx, &settings); err != nil {
		logger.Error("failed to update prompt history settings: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if settings.AutoDelete {
		_, _ = h.applyPromptHistoryAutoDelete(ctx)
	}
	SendJSON(ctx, settings)
}

// clearAllPromptHistory handles POST /api/prompt-repo/history/clear
func (h *PromptsHandler) clearAllPromptHistory(ctx *fasthttp.RequestCtx) {
	callerID, _ := h.promptCallerIdentity(ctx)
	if callerID == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "authentication required")
		return
	}

	count, err := h.store.ClearAllPromptSessions(ctx)
	if err != nil {
		logger.Error("failed to clear prompt history: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{
		"message":       "All prompt chat history cleared successfully",
		"deleted_count": count,
	})
}

// sanitizePromptMessageJSON ensures that any raw binary files or large data URLs in message content
// are replaced with their extracted text representation, preventing raw file payloads from being stored in the database.
func sanitizePromptMessageJSON(raw tables.PromptMessage) tables.PromptMessage {
	if len(raw) == 0 {
		return raw
	}
	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		return raw
	}
	payload, ok := msg["payload"].(map[string]any)
	if !ok {
		return raw
	}
	content, ok := payload["content"].([]any)
	if !ok {
		return raw
	}
	modified := false
	newContent := make([]any, 0, len(content))
	for _, item := range content {
		itemMap, ok := item.(map[string]any)
		if !ok {
			newContent = append(newContent, item)
			continue
		}
		itemType, _ := itemMap["type"].(string)
		switch itemType {
		case "file":
			modified = true
			filename := "file"
			if fileObj, ok := itemMap["file"].(map[string]any); ok {
				if fn, ok := fileObj["filename"].(string); ok && fn != "" {
					filename = fn
				}
			}
			newContent = append(newContent, map[string]any{
				"type": "text",
				"text": fmt.Sprintf("Attached file: %s\n\n[Extracted text preserved — raw binary file purged]", filename),
			})
		case "image_url":
			if imgObj, ok := itemMap["image_url"].(map[string]any); ok {
				if url, ok := imgObj["url"].(string); ok && strings.HasPrefix(url, "data:") {
					modified = true
					filename := "image"
					if fn, ok := imgObj["filename"].(string); ok && fn != "" {
						filename = fn
					}
					// If content already contains extracted OCR text for this image, do not add a redundant placeholder
					hasOcrText := false
					for _, other := range content {
						if oMap, ok := other.(map[string]any); ok {
							if oType, _ := oMap["type"].(string); oType == "text" {
								if oText, _ := oMap["text"].(string); strings.Contains(oText, filename) && strings.Contains(oText, "OCR extracted content") {
									hasOcrText = true
									break
								}
							}
						}
					}
					if !hasOcrText {
						newContent = append(newContent, map[string]any{
							"type": "text",
							"text": fmt.Sprintf("Attached file: %s\n\n[Visual text content preserved — raw image binary purged]", filename),
						})
					}
					continue
				}
			}
			newContent = append(newContent, item)
		case "input_audio":
			modified = true
			newContent = append(newContent, map[string]any{
				"type": "text",
				"text": "Attached voice audio\n\n[Transcribed audio text preserved — raw audio binary purged]",
			})
		default:
			newContent = append(newContent, item)
		}
	}
	if modified {
		payload["content"] = newContent
		msg["payload"] = payload
		if data, err := json.Marshal(msg); err == nil {
			return tables.PromptMessage(data)
		}
	}
	return raw
}
