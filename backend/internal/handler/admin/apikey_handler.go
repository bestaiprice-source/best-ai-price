package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService service.AdminService
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		adminService: adminService,
	}
}

// AdminUpdateAPIKeyGroupRequest represents the request to update an API key's group
type AdminUpdateAPIKeyGroupRequest struct {
	GroupID *int64 `json:"group_id"` // nil=不修改, 0=解绑, >0=绑定到目标分组
}

// List handles listing API keys for admins via Gin
// GET /api/v1/admin/api-keys
func (h *AdminAPIKeyHandler) List(c *gin.Context) {
	h.ListGateway(gatewayctx.FromGin(c))
}

// Update handles updating an API key via Gin
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) Update(c *gin.Context) {
	h.UpdateGateway(gatewayctx.FromGin(c))
}

// Delete handles deleting an API key via Gin
// DELETE /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) Delete(c *gin.Context) {
	h.DeleteGateway(gatewayctx.FromGin(c))
}

// UpdateGroup handles updating an API key's group binding
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) UpdateGroup(c *gin.Context) {
	h.UpdateGroupGateway(gatewayctx.FromGin(c))
}

// ListGateway handles listing API keys for admins
// GET /api/v1/admin/api-keys
func (h *AdminAPIKeyHandler) ListGateway(c gatewayctx.GatewayContext) {
	page, pageSize := response.ParsePaginationValues(c)

	var filters service.APIKeyListFilters
	if search := strings.TrimSpace(c.QueryValue("search")); search != "" {
		if len(search) > 100 {
			search = search[:100]
		}
		filters.Search = search
	}
	filters.Status = c.QueryValue("status")
	if groupIDStr := c.QueryValue("group_id"); groupIDStr != "" {
		if gid, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			filters.GroupID = &gid
		}
	}
	if userIDStr := c.QueryValue("user_id"); userIDStr != "" {
		if uid, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			filters.UserID = &uid
		}
	}

	keys, result, err := h.adminService.AdminListAPIKeys(c.Request().Context(), page, pageSize, filters)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.PaginatedContext(gatewayJSONResponder{ctx: c}, out, result.Total, page, pageSize)
}

// UpdateGateway handles updating an API key
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) UpdateGateway(c gatewayctx.GatewayContext) {
	keyID, err := strconv.ParseInt(c.PathParam("id"), 10, 64)
	if err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Invalid API key ID")
		return
	}

	var req service.UpdateAPIKeyRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	key, err := h.adminService.AdminUpdateAPIKey(c.Request().Context(), keyID, req)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, dto.APIKeyFromService(key))
}

// DeleteGateway handles deleting an API key
// DELETE /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) DeleteGateway(c gatewayctx.GatewayContext) {
	keyID, err := strconv.ParseInt(c.PathParam("id"), 10, 64)
	if err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Invalid API key ID")
		return
	}

	if err := h.adminService.AdminDeleteAPIKey(c.Request().Context(), keyID); err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, nil)
}

func (h *AdminAPIKeyHandler) UpdateGroupGateway(c gatewayctx.GatewayContext) {
	keyID, err := strconv.ParseInt(c.PathParam("id"), 10, 64)
	if err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Invalid API key ID")
		return
	}

	var req AdminUpdateAPIKeyGroupRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	result, err := h.adminService.AdminUpdateAPIKeyGroupID(c.Request().Context(), keyID, req.GroupID)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey:                 dto.APIKeyFromService(result.APIKey),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.SuccessContext(gatewayJSONResponder{ctx: c}, resp)
}
