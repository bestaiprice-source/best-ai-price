package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayHandler_PublicModelsPricing_Gin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)

	gatewayService := &service.GatewayService{}
	h := &GatewayHandler{
		gatewayService: gatewayService,
	}

	h.PublicModelsPricing(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code    int                            `json:"code"`
		Message string                         `json:"message"`
		Data    []service.PublicModelPricingItem `json:"data"`
	}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "success", resp.Message)
	assert.NotEmpty(t, resp.Data)

	// Check sample item structure
	var foundSonnet5 bool
	for _, item := range resp.Data {
		if item.CanonicalModelID == "anthropic/claude-sonnet-5" {
			foundSonnet5 = true
			assert.Equal(t, "anthropic/claude-sonnet-5", item.ModelID)
			assert.Nil(t, item.InputPricePer1M)
			assert.Nil(t, item.OutputPricePer1M)
			assert.Equal(t, "unavailable", item.Status)
			assert.False(t, item.Available)
			break
		}
	}
	assert.True(t, foundSonnet5, "expected anthropic/claude-sonnet-5 in public models pricing list")
}

func TestGatewayHandler_PublicModelsPricingGateway_NilService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/models", nil)

	h := &GatewayHandler{
		gatewayService: nil,
	}

	h.PublicModelsPricingGateway(gatewayctx.FromGin(c))

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code    int                            `json:"code"`
		Message string                         `json:"message"`
		Data    []service.PublicModelPricingItem `json:"data"`
	}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "success", resp.Message)
	assert.Empty(t, resp.Data)
}
