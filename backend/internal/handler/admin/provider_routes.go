package admin

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/gatewayctx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/yunwu"
	"github.com/gin-gonic/gin"
)

// routeAccount is one execution node participating in routing.
type routeAccount struct {
	acc      *service.Account
	provider string
	domain   string
	mapping  map[string]string
}

// Model routing read-model (§6). Exposes, for every sellable canonical model, the
// resolved Primary/Backup route — the two providers that will actually serve it —
// together with each route's upstream model id, the applied retail price, the
// route status and when both routes were last verified. This is a projection over
// existing data (accounts, group.ModelRouting, node runtime state, upstream
// pricing); it stores nothing and never computes a price of its own beyond the
// shared max-across-providers retail table the gateway bills from.

const modelRouteDefaultMultiplier = 1.10

// ModelRouteProvider describes one leg of a model route.
type ModelRouteProvider struct {
	AccountID     int64   `json:"account_id"`
	Provider      string  `json:"provider"`
	Name          string  `json:"name"`
	Domain        string  `json:"domain"`
	UpstreamModel string  `json:"upstream_model"`
	Available     bool    `json:"available"`
	LastSuccessAt *string `json:"last_success_at,omitempty"`
}

// ModelRouteItem is one canonical model row in the routing table.
type ModelRouteItem struct {
	CanonicalModelID  string              `json:"canonical_model_id"`
	Primary           *ModelRouteProvider `json:"primary"`
	Backup            *ModelRouteProvider `json:"backup"`
	RetailInputPer1M  *float64            `json:"retail_input_per_1m"`
	RetailOutputPer1M *float64            `json:"retail_output_per_1m"`
	Status            string              `json:"status"`       // available / degraded / unavailable
	RouteSource       string              `json:"route_source"` // explicit / price
	LastVerified      *string             `json:"last_verified,omitempty"`
}

// ModelRoutesResponse is the routing table payload.
type ModelRoutesResponse struct {
	GroupID int64             `json:"group_id,omitempty"`
	Items   []*ModelRouteItem `json:"items"`
	Total   int               `json:"total"`
}

// ModelRoutes GET /api/v1/admin/model-routes?group_id=&model=
func (h *ProviderHandler) ModelRoutes(c *gin.Context) {
	h.ModelRoutesGateway(gatewayctx.FromGin(c))
}

func (h *ProviderHandler) ModelRoutesGateway(c gatewayctx.GatewayContext) {
	ctx := c.Request().Context()

	groupID := int64(0)
	if raw := strings.TrimSpace(c.QueryValue("group_id")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			groupID = v
		}
	}
	modelFilter := strings.TrimSpace(c.QueryValue("model"))

	groups, err := h.loadYunwuGroups(ctx, groupID)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	accounts, _, err := h.adminService.ListAccounts(ctx, 1, 1000, "", "", "", "", "", "", "", 0)
	if err != nil {
		response.ErrorFromContext(gatewayJSONResponder{ctx: c}, err)
		return
	}

	var pool []routeAccount
	for i := range accounts {
		acc := &accounts[i]
		if !isUnofficialYunwuAccount(acc) {
			continue
		}
		if groupID > 0 && !accountInGroup(acc, groupID) {
			continue
		}
		if groupID == 0 && !accountInAnyGroup(acc, groups) {
			continue
		}
		pool = append(pool, routeAccount{
			acc:      acc,
			provider: providerKey(acc),
			domain:   providerDomain(acc),
			mapping:  accountModelMapping(acc),
		})
	}

	models := h.modelRouteModels(ctx, groups, modelFilter)

	items := make([]*ModelRouteItem, 0, len(models))
	for _, model := range models {
		item := h.buildModelRoute(ctx, model, pool, groups, groupID)
		if item != nil {
			items = append(items, item)
		}
	}

	response.SuccessContext(gatewayJSONResponder{ctx: c}, ModelRoutesResponse{
		GroupID: groupID,
		Items:   items,
		Total:   len(items),
	})
}

// modelRouteModels resolves which canonical models the routing table should list.
func (h *ProviderHandler) modelRouteModels(ctx context.Context, groups []*service.Group, modelFilter string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m == "" || strings.Contains(m, "*") {
			return
		}
		if _, ok := seen[m]; ok {
			return
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}

	if modelFilter != "" {
		add(modelFilter)
	}
	for _, g := range groups {
		for model := range g.ModelRouting {
			add(model)
		}
	}
	// No explicit rules and no filter: fall back to the models the connected
	// upstream providers actually price (bounded).
	if len(out) == 0 && modelFilter == "" && h.yunwuPricing != nil {
		known := h.yunwuPricing.GetKnownModelNames(ctx)
		sort.Strings(known)
		if len(known) > 200 {
			known = known[:200]
		}
		for _, m := range known {
			add(m)
		}
	}
	sort.Strings(out)
	return out
}

// buildModelRoute resolves Primary/Backup for a single model.
func (h *ProviderHandler) buildModelRoute(
	ctx context.Context,
	model string,
	pool []routeAccount,
	groups []*service.Group,
	groupID int64,
) *ModelRouteItem {
	type priced struct {
		ra       int
		cost     float64
		hasPrice bool
	}
	var candidates []priced
	for i, ra := range pool {
		cost, ok := h.providerModelCost(ctx, ra.provider, model)
		candidates = append(candidates, priced{ra: i, cost: cost, hasPrice: ok})
	}

	// Explicit admin route (ordered account IDs) wins over price ranking.
	var explicit []int64
	for _, g := range groups {
		if !g.ModelRoutingEnabled {
			continue
		}
		for pattern, ids := range g.ModelRouting {
			if matchModelPattern(pattern, model) && len(ids) > 0 {
				explicit = ids
				break
			}
		}
		if explicit != nil {
			break
		}
	}

	orderedIdx := make([]int, 0, len(candidates))
	routeSource := "price"
	if explicit != nil {
		routeSource = "explicit"
		want := map[int64]int{}
		for pos, id := range explicit {
			want[id] = pos
		}
		ranked := make([]int, 0, len(candidates))
		for _, cand := range candidates {
			if _, ok := want[pool[cand.ra].acc.ID]; ok {
				ranked = append(ranked, cand.ra)
			}
		}
		sort.SliceStable(ranked, func(a, b int) bool {
			return want[pool[ranked[a]].acc.ID] < want[pool[ranked[b]].acc.ID]
		})
		orderedIdx = ranked
	} else {
		// Price-based: cheapest provider for this model is Primary, next is Backup.
		available := make([]priced, 0, len(candidates))
		for _, cand := range candidates {
			if cand.hasPrice {
				available = append(available, cand)
			}
		}
		sort.SliceStable(available, func(a, b int) bool {
			if available[a].cost != available[b].cost {
				return available[a].cost < available[b].cost
			}
			return pool[available[a].ra].acc.ID < pool[available[b].ra].acc.ID
		})
		for _, cand := range available {
			orderedIdx = append(orderedIdx, cand.ra)
		}
	}

	if len(orderedIdx) == 0 {
		return &ModelRouteItem{
			CanonicalModelID: model,
			Status:           "unavailable",
			RouteSource:      routeSource,
		}
	}

	primary := h.toRouteProvider(ctx, pool[orderedIdx[0]].acc, pool[orderedIdx[0]].provider, pool[orderedIdx[0]].domain, pool[orderedIdx[0]].mapping, model)
	var backup *ModelRouteProvider
	if len(orderedIdx) > 1 {
		backup = h.toRouteProvider(ctx, pool[orderedIdx[1]].acc, pool[orderedIdx[1]].provider, pool[orderedIdx[1]].domain, pool[orderedIdx[1]].mapping, model)
	}

	item := &ModelRouteItem{
		CanonicalModelID: model,
		Primary:          primary,
		Backup:           backup,
		RouteSource:      routeSource,
	}
	item.Status = routeStatus(primary, backup)

	// Retail price: the same primary-provider table the gateway bills from,
	// scaled by the effective rate multiplier.
	if h.yunwuPricing != nil {
		if primaryPricing, ok := h.yunwuPricing.GetModelPricingPrimaryAcrossProviders(ctx, model); ok && primaryPricing != nil {
			mult := h.routeMultiplier(groups, pool, groupID)
			in := primaryPricing.InputPricePerToken * 1_000_000 * mult
			out := primaryPricing.OutputPricePerToken * 1_000_000 * mult
			item.RetailInputPer1M = &in
			item.RetailOutputPer1M = &out
		}
	}

	item.LastVerified = lastVerified(primary, backup)
	return item
}

func (h *ProviderHandler) toRouteProvider(ctx context.Context, acc *service.Account, provider, dom string, mapping map[string]string, model string) *ModelRouteProvider {
	p := &ModelRouteProvider{
		AccountID:     acc.ID,
		Provider:      provider,
		Name:          acc.Name,
		Domain:        dom,
		UpstreamModel: yunwu.ResolveUpstreamModelID(provider, model, mapping),
		Available:     strings.EqualFold(acc.Status, "active"),
	}
	if h.nodeManager != nil {
		nodeID := strconv.FormatInt(acc.ID, 10)
		if node, err := h.nodeManager.GetNode(ctx, nodeID); err == nil && node != nil {
			switch node.State {
			case "unavailable", "cooldown":
				p.Available = false
			}
		}
		if state, err := h.nodeManager.GetNodeRuntimeState(ctx, nodeID); err == nil && state != nil {
			if state.LastSuccessAt != nil {
				s := state.LastSuccessAt.Format(time.RFC3339)
				p.LastSuccessAt = &s
			}
		}
	}
	return p
}

// providerModelCost returns the upstream price rate (input+output) for a model on
// a provider, and whether that provider actually prices the model.
func (h *ProviderHandler) providerModelCost(ctx context.Context, provider, model string) (float64, bool) {
	if h.yunwuPricing == nil {
		return 0, false
	}
	pricing, ok := h.yunwuPricing.GetModelPricingForProvider(ctx, provider, model)
	if !ok || pricing == nil {
		return 0, false
	}
	return pricing.InputPricePerToken + pricing.OutputPricePerToken, true
}

func (h *ProviderHandler) routeMultiplier(groups []*service.Group, pool []routeAccount, groupID int64) float64 {
	for _, g := range groups {
		if groupID > 0 && g.ID != groupID {
			continue
		}
		if g.RateMultiplier > 0 {
			return g.RateMultiplier
		}
	}
	for _, ra := range pool {
		if ra.acc.RateMultiplier != nil && *ra.acc.RateMultiplier > 0 {
			return *ra.acc.RateMultiplier
		}
	}
	return modelRouteDefaultMultiplier
}

func (h *ProviderHandler) loadYunwuGroups(ctx context.Context, groupID int64) ([]*service.Group, error) {
	if groupID > 0 {
		g, err := h.adminService.GetGroup(ctx, groupID)
		if err != nil {
			return nil, err
		}
		if g == nil {
			return nil, nil
		}
		return []*service.Group{g}, nil
	}
	all, err := h.adminService.GetAllGroups(ctx)
	if err != nil {
		return nil, err
	}
	var out []*service.Group
	for i := range all {
		g := &all[i]
		if strings.EqualFold(g.Platform, domain.PlatformYunwu) || strings.Contains(strings.ToLower(g.Name), "yunwu") {
			out = append(out, g)
		}
	}
	return out, nil
}

// --- small helpers -------------------------------------------------------

func accountInGroup(acc *service.Account, groupID int64) bool {
	for _, id := range acc.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

func accountInAnyGroup(acc *service.Account, groups []*service.Group) bool {
	if len(groups) == 0 {
		return true
	}
	for _, g := range groups {
		if accountInGroup(acc, g.ID) {
			return true
		}
	}
	return false
}

func providerKey(acc *service.Account) string {
	prov := yunwu.ParseNodeProvider(acc.Extra)
	if prov == "openlux" && acc.Platform != "" && acc.Platform != domain.PlatformYunwu {
		prov = acc.Platform
	}
	return prov
}

func providerDomain(acc *service.Account) string {
	if acc.Extra != nil {
		if d, ok := acc.Extra["domain"].(string); ok && d != "" {
			return d
		}
		if b, ok := acc.Extra["base_url"].(string); ok && b != "" {
			return parseProviderDomain("", b)
		}
	}
	return ""
}

func accountModelMapping(acc *service.Account) map[string]string {
	if acc.Extra == nil {
		return nil
	}
	if m, ok := acc.Extra["model_mapping"].(map[string]string); ok {
		return m
	}
	raw, ok := acc.Extra["model_mapping"].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if vs, ok := v.(string); ok {
			out[k] = vs
		}
	}
	return out
}

// matchModelPattern matches a canonical model against a routing pattern that may
// use a trailing/leading wildcard (e.g. "claude-opus-*").
func matchModelPattern(pattern, model string) bool {
	pattern = strings.TrimSpace(pattern)
	model = strings.TrimSpace(model)
	if pattern == "" || model == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return strings.EqualFold(pattern, model)
	}
	parts := strings.Split(pattern, "*")
	idx := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		pos := strings.Index(model[idx:], part)
		if pos < 0 {
			return false
		}
		idx += pos + len(part)
	}
	return true
}

func routeStatus(primary, backup *ModelRouteProvider) string {
	if primary == nil {
		return "unavailable"
	}
	if !primary.Available {
		if backup != nil && backup.Available {
			return "degraded"
		}
		return "unavailable"
	}
	if backup != nil && !backup.Available {
		return "degraded"
	}
	return "available"
}

// lastVerified is the most recent moment both legs of the route were confirmed
// healthy (the older of the two last successes) — §6's "last successful check of
// both routes".
func lastVerified(primary, backup *ModelRouteProvider) *string {
	if primary == nil {
		return nil
	}
	if backup == nil {
		return primary.LastSuccessAt
	}
	if primary.LastSuccessAt == nil || backup.LastSuccessAt == nil {
		return nil
	}
	p, errP := time.Parse(time.RFC3339, *primary.LastSuccessAt)
	b, errB := time.Parse(time.RFC3339, *backup.LastSuccessAt)
	if errP != nil || errB != nil {
		return nil
	}
	older := p
	if b.Before(p) {
		older = b
	}
	s := older.Format(time.RFC3339)
	return &s
}
