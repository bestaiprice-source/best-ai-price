package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AuthProvidersHandler struct {
	authProvidersService *service.AuthProvidersService
}

func NewAuthProvidersHandler(authProvidersService *service.AuthProvidersService) *AuthProvidersHandler {
	return &AuthProvidersHandler{
		authProvidersService: authProvidersService,
	}
}

// SaveAuthProviders saves auth_providers.json content
func (h *AuthProvidersHandler) SaveAuthProviders(c *gin.Context) {
	h.SaveAuthProvidersGateway(gatewayctx.FromGin(c))
}

func (h *AuthProvidersHandler) SaveAuthProvidersGateway(c gatewayctx.GatewayContext) {
	if c == nil || c.Request() == nil {
		return
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "failed to read request body")
		return
	}

	if err := h.authProvidersService.Save(body); err != nil {
		if errors.Is(err, service.ErrInvalidJSON) {
			response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusBadRequest, "invalid json payload")
			return
		}
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusInternalServerError, err.Error())
		return
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, map[string]any{
		"message":       "auth_providers.json saved successfully",
		"bytes_written": len(body),
	})
}

// GetAuthProviders retrieves auth_providers.json content
func (h *AuthProvidersHandler) GetAuthProviders(c *gin.Context) {
	h.GetAuthProvidersGateway(gatewayctx.FromGin(c))
}

func (h *AuthProvidersHandler) GetAuthProvidersGateway(c gatewayctx.GatewayContext) {
	if c == nil {
		return
	}

	data, err := h.authProvidersService.Read()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusNotFound, "auth_providers.json not found")
			return
		}
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusInternalServerError, err.Error())
		return
	}

	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusInternalServerError, "corrupted json file")
		return
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, parsed)
}
