package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterPublicRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	h := &handler.Handlers{
		Gateway:     &handler.GatewayHandler{},
		AdminVendor: &handler.AdminVendorHandler{},
	}

	RegisterPublicRoutes(v1, h)

	// Test GET /api/v1/models
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestExecutablePublicRoutes(t *testing.T) {
	// Nil handlers
	defsNil := ExecutablePublicRoutes(nil)
	assert.Nil(t, defsNil)

	hWithoutGateway := &handler.Handlers{}
	assert.Nil(t, ExecutablePublicRoutes(hWithoutGateway))

	h := &handler.Handlers{
		Gateway: &handler.GatewayHandler{},
	}
	defs := ExecutablePublicRoutes(h)
	require.Len(t, defs, 2)

	assert.Equal(t, http.MethodGet, defs[0].Method)
	assert.Equal(t, "/api/v1/models", defs[0].Path)
	assert.Equal(t, http.MethodGet, defs[1].Method)
	assert.Equal(t, "/models", defs[1].Path)
}
