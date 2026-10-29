package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type modelTrafficRepoStub struct {
	rows []service.ModelTrafficStatRow
}

func (s modelTrafficRepoStub) ModelTrafficStats(context.Context, time.Time) ([]service.ModelTrafficStatRow, error) {
	return s.rows, nil
}

func mountModelHealthHandler(t *testing.T, rows []service.ModelTrafficStatRow) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/model-health", nil)

	// nil setting repo ⇒ the channel-monitor feature reads as DISABLED, which is
	// how the production install is configured today. The probe service must not
	// be touched in that case (its repository is nil here on purpose).
	traffic := service.NewModelTrafficHealthService(modelTrafficRepoStub{rows: rows})
	h := NewChannelMonitorUserHandler(&service.ChannelMonitorService{}, service.NewSettingService(nil, nil), traffic)

	h.ModelHealth(c)
	return c, rec
}

func TestChannelMonitorUserHandler_ModelHealthServesTrafficWithoutMonitors(t *testing.T) {
	_, rec := mountModelHealthHandler(t, []service.ModelTrafficStatRow{
		{Model: "gpt-5.2", Successes: 35, Failures: 1},
		// Below the sample threshold: no percentage may be invented for it.
		{Model: "claude-sonnet-5", Successes: 2},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				Model           string   `json:"model"`
				Status          string   `json:"status"`
				AvailabilityPct *float64 `json:"availability_pct"`
				Source          string   `json:"source"`
				Requests        int64    `json:"requests"`
			} `json:"items"`
			WindowDays int `json:"window_days"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data.Items, 1)
	assert.Equal(t, 7, resp.Data.WindowDays)
	assert.Equal(t, "gpt-5.2", resp.Data.Items[0].Model)
	assert.Equal(t, "traffic", resp.Data.Items[0].Source)
	assert.Equal(t, int64(36), resp.Data.Items[0].Requests)
	require.NotNil(t, resp.Data.Items[0].AvailabilityPct)
	assert.Equal(t, 97.22, *resp.Data.Items[0].AvailabilityPct)
	assert.Equal(t, "degraded", resp.Data.Items[0].Status)
}

func TestChannelMonitorUserHandler_ModelHealthRejectsUnknownWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/model-health?window_days=99", nil)

	h := NewChannelMonitorUserHandler(&service.ChannelMonitorService{}, service.NewSettingService(nil, nil), nil)
	h.ModelHealth(c)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestChannelMonitorUserHandler_ModelHealthGatewayContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/model-health", nil)

	traffic := service.NewModelTrafficHealthService(modelTrafficRepoStub{})
	h := NewChannelMonitorUserHandler(&service.ChannelMonitorService{}, service.NewSettingService(nil, nil), traffic)

	h.ModelHealthGateway(gatewayctx.FromGin(c))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}
