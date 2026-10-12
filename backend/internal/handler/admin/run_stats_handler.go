package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type RunStatsHandler struct {
	runStatsService *service.RunStatsService
}

func NewRunStatsHandler(runStatsService *service.RunStatsService) *RunStatsHandler {
	return &RunStatsHandler{
		runStatsService: runStatsService,
	}
}

// GetRunStats retrieves parsed run_stats.json content
func (h *RunStatsHandler) GetRunStats(c *gin.Context) {
	h.GetRunStatsGateway(gatewayctx.FromGin(c))
}

func (h *RunStatsHandler) GetRunStatsGateway(c gatewayctx.GatewayContext) {
	if c == nil {
		return
	}

	stats, err := h.runStatsService.GetStats()
	if err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusInternalServerError, "failed to read run stats: "+err.Error())
		return
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, stats)
}

// GetRunStatsRaw returns the raw JSON bytes of run_stats.json
func (h *RunStatsHandler) GetRunStatsRaw(c *gin.Context) {
	h.GetRunStatsRawGateway(gatewayctx.FromGin(c))
}

func (h *RunStatsHandler) GetRunStatsRawGateway(c gatewayctx.GatewayContext) {
	if c == nil {
		return
	}

	data, _, _, err := h.runStatsService.ReadRaw()
	if err != nil {
		response.ErrorContext(gatewayJSONResponder{ctx: c}, http.StatusNotFound, "run_stats.json not found")
		return
	}

	c.SetHeader("Content-Type", "application/json; charset=utf-8")
	_, _ = c.WriteBytes(http.StatusOK, data)
}
