# CLAUDE.md - AI Agent Master Technical Guide & Repository Context

Welcome to **sub2api** and the **AI Session Hypervisor (ASH / Yunwu Engine)** repository. This document serves as the authoritative, comprehensive technical guide for AI assistants, agents, and developers working on this codebase.

> ⚠️ **MANDATORY RULE FOR ALL AI AGENTS (PRE-PUSH REQUIREMENT)**:
> Before executing any `git push`, you MUST inspect all architectural changes, new endpoints, bug fixes, or schema modifications made during the task and update this document (`CLAUDE.md`). This document MUST ALWAYS contain complete, up-to-date information so that all AI agents possess full context.

---

## 🚀 1. Overview & Business Model

**sub2api** is an enterprise-grade AI Gateway and Hypervisor system managing pools of physical AI accounts (Claude Max 20x, OpenAI / Codex, Yunwu, and Sora) and distributing capacity across users through unified API keys, dynamic fair-share quotas, and resilient proxies.

### Key Value Proposition
- **Pooled Capacity Model**: Users obtain high-throughput access to Claude, OpenAI, Yunwu, and Sora at lower costs by leveraging shared physical account pools.
- **Virtual Sessions**: Decouples client software (Claude Code CLI, VS Code extensions, Codex, OpenAI clients) from physical accounts. The system maintains conversation state while transparently switching underlying physical execution nodes on failure.
- **Automatic Rebinding**: When an upstream node encounters `429 Too Many Requests`, rate limits, or timeouts, the Hypervisor locks the session, selects a healthy node, recompiles prompt context, and retries the request seamlessly.
- **Fair-Share Load Balancing**: Ensures heavy users do not exhaust capacity, distributing requests via dynamic quota limits and organic token load balancing.

```text
+--------+     +---------------+     +----------------+     +-----------+     +-------------------------------+
| Client | --> | Service Key / | --> | VirtualSession | --> | Scheduler | --> | Physical Execution Node       |
| (CLI)  |     | Gateway API   |     | (State/Context)|     | & Lease   |     | (Claude / OpenAI / Yunwu)     |
+--------+     +---------------+     +----------------+     +-----------+     +-------------------------------+
```

---

## 🔍 2. Deep Dive: Upstream Provider Engines & Integrations

### 2.1 Claude (Anthropic) Engine
- **Target**: Claude Max 20x accounts, Anthropic `/v1/messages` endpoints.
- **OAuth & Setup-Token Protocol Alignment**: For accounts configured with `setup-token` or `oauth` (Claude Code tokens), Hypervisor runtime automatically appends `?beta=true` to the upstream messages URL and ensures the Claude Code system banner (`claude.ClaudeCodeSystemPrompt`) and valid metadata `user_id` session identifiers are injected. This prevents Anthropic's upstream firewalls from rejecting requests on Sonnet and Opus models with masked 429 rate limit errors.
- **Virtual Sessions & Rebinding**: Maintains `VirtualSession` state in Redis and PostgreSQL. Rebinds transparently to a new Claude node on `429` or timeout.
- **Context Compiler**: Reconstructs prompt blocks (`SystemPrompt`, `Summary`, `PinnedFacts`, `TerminalState`, `ToolState`, `ProjectState`, `RecentMessages`) when migrating sessions between physical accounts. Compiled automatically prior to rebinding retries.

### 2.2 OpenAI & Codex Engine
- **Target**: OpenAI `/v1/chat/completions` and Codex `/responses` endpoints.
- **Payload Normalization**: Includes `constructMessagesFromResponsesPayload` in `pkg/hypervisor/runtime` to handle structured message arrays from Codex CLI.
- **SSE Transformer Bypass**: Direct `/responses` API calls bypass `responsesSSETransformer` to prevent truncated or empty responses when streaming native OpenAI/Codex format.

### 2.3 Unofficial Providers Engine (`pkg/hypervisor/yunwu`)
- **Target**: Multi-channel third-party provider accounts (OpenLux / Yunwu, Portdan, Top-API, Yuanqiaoyun, and extensible to arbitrary upstream aggregators).
- **General Hub for Unofficial Providers**:
  - The `yunwu` platform serves as the unified general hub for unofficial / third-party AI providers and aggregators.
  - Upstream provider identifier is stored per-node in `account.Extra["provider"]` (defaults to `"openlux"`). First-class supported presets:
    1. `openlux`: OpenLux AI (`https://api.openlux.ai/v1`, pricing: `https://api.openlux.ai/api/pricing`)
    2. `portdan`: Portdan AI / 蛋蛋中转 (`https://portdan.com/v1`, sub2api architecture; user channel pricing: `https://portdan.com/api/v1/channels/available` with Bearer auth is the SOLE pricing source)
    3. `api-top`: Top-API (`https://api-top.com/v1`, pricing: `https://api-top.com/api/pricing`)
    4. `yuanqiaoyun`: Yuanqiaoyun / New API (`https://yuanqiaoyun.com/v1`, pricing: `https://yuanqiaoyun.com/api/pricing`)
    5. `custom`: Arbitrary user-defined upstream provider key and endpoint.
  - `DefaultBaseURLForProvider(provider)` resolves provider-specific default base URLs in backend node management, tests, and balance sync fallback.
  - Admin UI (`EditAccountModal.vue`, `CreateAccountModal.vue`, `GroupsView.vue`) exposes provider dropdown presets with auto-filling Base URLs and model family routing.
- **Per-Node Model Family Routing (`pkg/hypervisor/yunwu/model_family.go`)**:
  - Each node can be independently configured in Admin UI to route any subset of four primary model families:
    1. `claude`: Claude models (`claude-3-5-sonnet`, `claude-3-7-sonnet`, `claude-3-opus`, etc., Anthropic messages protocol).
    2. `gpt`: OpenAI models (`gpt-4o`, `gpt-4o-mini`, `o1`, `o3`, `chatgpt`, etc., OpenAI chat completions protocol).
    3. `gemini`: Google models (`gemini-1.5`, `gemini-2.0`, `gemini-2.5-flash`, `imagen`, `veo`, `lyria-realtime`, etc.).
    4. `other`: Alternative and open-source models (`deepseek-chat`, `qwen-2.5`, `llama-3.3`, `mistral`, `grok`, etc.).
  - Persisted in `account.Extra["supported_model_families"]` as `[]string`. Defaults to all four families if unset for 100% backward compatibility.
  - `YunwuHypervisor.HandleRequest` classifies incoming requests via `ClassifyModelFamily(requestedModel)` and filters execution nodes with BOTH `NodeSupportsFamily(node.Capabilities, family)` AND provider pricing availability (`ResolveNodePrice(ctx, node, requestedModel)`).
  - **Per-Node Pricing Availability & Intelligent Failover Routing**:
    - If a candidate node in the pool does not have pricing for the requested model on its upstream provider, that node is skipped during selection.
    - The request is automatically directed to an alternative candidate node in the pool supporting the same model family that HAS valid pricing for the model.
    - If no candidate node in the pool has pricing for the requested model, execution halts immediately: `ErrNoPricingAvailable` is returned (HTTP 400 `invalid_request_error`), no upstream request is sent, and zero funds/quota are deducted.
    - During node error failover, `YunwuRebindingManager` only considers candidate nodes that have verified pricing for the active request model.
    - When execution succeeds, user balance deduction is strictly calculated using the price resolved for that execution node's provider. Arbitrary default fallback prices (e.g. $0.003) are never charged if pricing was unresolvable.
    - If no candidate nodes support the requested family, returns `ErrNoCompatibleNodes` (HTTP 400).
  - Session affinity / leases seamlessly respect family filtering and pricing availability: switching model families or requesting unpriced models mid-session invalidates incompatible node leases and binds to a compatible node.
  - `HandleLiveWebSocket` automatically filters candidates to the `gemini` family.
  - Real-time balance deduction and pricing (`/api/pricing`) operate identically across all routed model families.
  - **Hypervisor QuotaManager Bypass**: For Yunwu and unofficial resellers, Hypervisor `QuotaManager` fair-share capacity limits, concurrency slots, and micro-USD reservations are bypassed. User balance validation, admission guards, and concurrency limits for unofficial providers are handled exclusively at the gateway level (`BillingService`, `MinOperationalBalance = $0.002`, and `CalculateEffectiveConcurrency`).
- **Dedicated Subsystems**:
  - `ProviderPriceManager` (`pkg/hypervisor/yunwu/pricing.go`): Manages multi-provider pricing resolution for unofficial nodes:
    - Resolves from DB `ash_vendor_models` for other providers (bypassed for Portdan).
    - For Portdan, `GET /api/v1/channels/available` with Bearer auth is the SOLE pricing truth; static table fallbacks and generic benchmark prices are disabled.
    - 30-Minute Pricing Cache with Strict Provider Isolation: Upstream pricing tables are cached in-memory with a 30-minute expiration (`DefaultPricingCacheTTL = 30 * time.Minute`), keyed strictly by `node.NodeID` and endpoint `pricingURL` (e.g. `https://bestaiprice.com/api/pricing`, `https://api.openlux.ai/api/pricing`). This ensures prices from different upstream providers or custom aggregator URLs are completely isolated and never cross-contaminate. `singleflight` deduplication prevents parallel requests from hammering upstream endpoints.
    - Graceful Error Fallback: Network errors, timeouts, or 5xx/429 responses from pricing endpoints do NOT poison nodes with artificial negative cooldowns. Expired cached pricing is retained as fallback if available, or the node is safely skipped without long-term negative lockout.
    - Pricing Security Guards:
      - Rigorous sanitization against negative, `NaN`, and `Inf` price injections in both One-API and sub2api parsers.
      - Mathematical guard in `CalculateBilledMicroUSD`: enforces non-negative inputs and ceiling rounding to microUSD, preventing integer truncation errors or negative balance deductions.
      - Pre-Admission Price Lock: if a transient lookup failure occurs post-execution, the hypervisor falls back to the pre-admission pricing verified at session admission, completely eliminating free-generation loopholes.
    - Formats supported:
      - One-API / New-API format: `{"data": [{"model_name", "model_ratio", "model_price", "completion_ratio"}]}` (`api.openlux.ai`, `api-top.com`, `yuanqiaoyun.com`, or `<baseURL>/api/pricing`).
      - sub2api format: `[{"platforms": [{"supported_models": [{"name", "pricing": {"input_price", "output_price"}}]}]}]` (`portdan.com/api/v1/channels/available`), with automatic per-token to per-1k normalization.
      - BestAIPrice format (`providers.json`): strictly filtered by provider domain/name matching `portdan` to guarantee complete pricing isolation and prevent non-Portdan model pricing leakage.
    - Resolves model aliases (e.g., `claude-3-7-sonnet` -> `claude-3-7-sonnet-20250219`, `gemini-3-pro` -> `gemini-3-pro-preview`, `gemini-3-flash` -> `gemini-3-flash-preview`).
    - Exposes `ResolveNodePrice(ctx, node, model) (in, out, found)` and `SetProviderPrices` for mock testing.
  - **Mandatory Group Assignment for Unofficial Nodes**:
    - All unofficial (`yunwu`) nodes MUST be assigned to at least one group.
    - Enforced at the hypervisor level: `getAllYunwuNodes` applies `account.HasAccountGroups()` to strictly exclude ungrouped unofficial nodes from execution candidate selection.
    - Enforced at service level: `AccountService.Create` and `AccountService.Update` reject accounts with platform `yunwu` if no groups are specified (`ErrYunwuAccountGroupRequired`).
  - **Upstream Error Recognition & Failover (`model_not_found` / `no available channel`)**:
    - `isUpstreamError` in `pkg/hypervisor/rebinding` recognizes `model_not_found`, `no available channel`, and HTTP 404 upstream errors as failover-eligible events.
    - When a third-party aggregator token lacks channel bindings for a model, the hypervisor places the failing node on cooldown and automatically triggers session rebinding to an alternative compatible node in the pool.
  - **Cheapest-First Routing with Failover (`YunwuScheduler`)**:
    - When multiple eligible candidate nodes support the requested model family, the scheduler strictly prioritizes the node with the lowest price rate (`inputPrice + outputPrice`).
    - More expensive nodes act as fallback candidates: they are selected only when cheaper nodes are in active cooldown, unavailable, or fail during request execution.
    - If multiple nodes share the lowest price tier, traffic is balanced organically between them based on health scoring, latency, queue depth, and `UsedTokens` load balancing.
    - Node runtime metrics update `state.UsedTokens += meta.InputTokens + meta.OutputTokens` upon each successful request to maintain fair distribution among equally-priced nodes.
    - `HandleYunwuFailure` in `YunwuRebindingManager` receives node prices and seamlessly rebinds failing sessions to fallback nodes.
  - **Intelligent Connection Testing for Unofficial (Yunwu) Nodes (`AccountTestService`)**:
    - `TestAccountConnection` routes `yunwu` nodes based on their configured `supported_model_families`: if a node supports `gpt` or `gemini` (and not `claude`), tests are executed via OpenAI `/v1/chat/completions` instead of Anthropic `/v1/messages`.
    - If a Claude test request returns HTTP 403 with `This group does not allow /v1/messages dispatch`, it automatically falls back to an OpenAI test (`/v1/chat/completions`) and avoids erroneously setting the node status to `error`.
    - `processOpenAIStream` handles both standard OpenAI chat completion SSE chunks (`choices[0].delta.content`) and Responses API chunks (`response.output_text.delta`).
  - `YunwuScheduler`: Tailored node selection for Yunwu execution nodes based on price tiering, node health, queue depth, latency, and `UsedTokens` organic load balancing penalty (ensuring fair traffic distribution across active channels even when balance APIs are untracked). `YunwuRequireFreshBalance` defaults to `false` so nodes without balance API endpoints are not excluded.
  - `YunwuRebindingManager`: Handles failover and node cooldown transitions specifically for Yunwu provider clusters, enforcing Redis session-scoped distributed lock (`SETNX`) and running `YunwuScheduler` with user priority, credits, prices, and system state during rebinding.
  - `StatsWorker`: Syncs upstream credit balance from `/api/user/self`, `/dashboard/billing/subscription`, and credit grants across One-API/New-API compatible providers. In `/api/user/self`, `data.quota` is parsed directly as the current available balance (avoiding double-subtraction of `used_quota`).
  - Runtime credentials: `AnthropicRuntime.Execute` attaches `Authorization: Bearer <api_key>` headers for Yunwu platform accounts.
  - **Dynamic Multi-Provider Real-Time Pricing (`YunwuPricingService`) & Gateway Pre-Admission**: Resolves model pricing on-demand from upstream pricing endpoints (`https://api.openlux.ai/api/pricing`, `https://api-top.com/api/pricing`, `https://yuanqiaoyun.com/api/pricing` in standard One-API/New-API format: base rate $0.002/1k tokens, `model_ratio`, `completion_ratio`, `cache_ratio`, `model_price`, and for **Portdan** via `https://bestaiprice.com/gateway/data/providers.json` with per-provider/model price indexing). Employs `singleflight.Group` to prevent redundant parallel HTTP requests, maintains strictly isolated pricing caches per provider (`FetchPricingForProvider`, `GetModelPricingForProvider`, `ContextWithPricingProvider`) to eliminate cross-provider model leakage and pricing pollution, and caches snapshots in-memory (30-minute TTL) as a fallback against upstream network drops. Integrated into `BillingService` so gateway usage logs (`RecordUsage`) accurately calculate costs and deduct funds from user balances for Yunwu and Gemini models (`gemini-3.8-flash`, `gemini-3.5-flash`, `gemini-3-pro-preview`, etc.). Enforces pre-routing admission guard (`CheckModelPricingAvailable`): if pricing cannot be resolved for a model, the request is rejected immediately with HTTP 400 without forwarding to upstream and without touching user balance.
  - **Configurable Pricing Markup (Default 10% / 1.10x)**: Supports global and per-group markup multipliers. System-wide default rate multiplier is set to `1.10` (+10% margin over base USD prices). Configurable in Admin Settings -> Defaults (`default_rate_multiplier`), in group creation forms (defaults to `1.1`), and stored in DB settings (`SettingKeyDefaultRateMultiplier = "default_rate_multiplier"`). Deductions calculate as `TotalCost * RateMultiplier`.
  - **Operational Balance Thresholds, Concurrency Clamping & Anti-Overdraft Guard**:
    - `MinOperationalBalance = 0.002` (USD): Minimum account balance required to initiate requests in balance-billed mode. Enforced in `billing_cache_service.go` (`checkBalanceEligibility`), `api_key_auth.go`, and `api_key_auth_google.go`. Requests from accounts below $0.002 are rejected upfront with HTTP 403 / "Insufficient account balance" before reaching upstream providers.
    - `LowBalanceConcurrencyThreshold = 1.00` (USD) & `MinBalancePerConcurrentSlot = 0.50` (USD): For balance-billed (pay-as-you-go) users without an active subscription, concurrency is dynamically clamped via `CalculateEffectiveConcurrency(concurrency, balance, hasSub)`:
      - Balance < $1.00 => effective concurrency is strictly clamped to `1`.
      - Balance >= $1.00 => effective concurrency is capped at `min(user.Concurrency, max(1, int(balance / 0.50)))` (e.g. $1.00-$1.49 allows 2 slots; $2.50+ allows 5 slots). Subscription users always retain their configured concurrency.
      - Combined with slot acquisition in `ConcurrencyHelper`, this completely prevents parallel request fan-out race conditions (e.g. concurrent Opus/GPT-5 bursts) that could exploit post-payment billing to drive user balances deeply negative.
    - **No Debt Forgiveness & Atomic Overdraft Tracking**:
      - `deductUsageBillingBalance` in `usage_billing_repo.go` uses atomic `UPDATE users SET balance = balance - $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL RETURNING balance`. Debt forgiveness (clamping balance to -$0.50) has been eliminated; the full real negative balance is accurately recorded in `users.balance` to be recovered upon user recharge.
      - If balance drops below `-MaxAllowableOverdraftUSD` (-$0.50), a warning alert (`billing.overdraft_threshold_exceeded`) is emitted for operator awareness.
      - Float64 monetary precision guards reject NaN, Inf, and non-positive deduction amounts upfront.
    - **Redis Billing Cache CAS Versioning**:
      - `billing:balance:<userID>` in Redis is stored as a hash with `balance` and `version` (nanoseconds timestamp from `user.UpdatedAt.UnixNano()`).
      - Atomic Lua scripts enforce Compare-And-Swap: `setUserBalanceScript` only overwrites if `incoming_version >= current_version`, completely preventing stale DB reads on cache miss from overwriting newer balance deductions.
      - `deductBalanceScript` atomically deducts balance, bumps `version` to the current timestamp, and returns no-op on non-existent keys.
      - Transparent on-the-fly migration safely handles legacy string keys (`WRONGTYPE` fallback).
    - **Bounded Rate Limit Resets & Error Alerting**:
      - Expired rate limit window resets and cache invalidations in `evaluateRateLimits` are enqueued into the bounded worker pool (`cacheWriteResetRateLimit`), eliminating unbounded goroutine spawning.
      - Database query failures in `checkAPIKeyRateLimits` emit structured warning logs before failing open.
    - **Overdraft Monitoring**: `UserRepository.GetOverdraftStats` and `UserService.GetOverdraftStats` query real-time negative balances across the system (`SELECT COALESCE(ABS(SUM(balance)), 0), COUNT(*) FROM users WHERE balance < 0 AND deleted_at IS NULL`). Integrated directly into `usagestats.DashboardStats` (`total_overdraft_usd`, `overdraft_user_count`) for instant administrative visibility.
  - **Guaranteed Billable Task Worker Pool**:
    - `gateway.usage_record.overflow_policy` defaults to `sync` (instead of `sample`).
    - `UsageRecordWorkerPool.SubmitGuaranteed` ensures billable usage records are never dropped when the worker pool queue is full or stopped, falling back immediately to synchronous execution.
    - `GatewayHandler`, `OpenAIGatewayHandler`, and `SoraGatewayHandler` wrap usage recording in fail-safe synchronous execution with panic recovery.
  - **Zero-Cost Pricing Fallback Elimination**:
    - `BillingService.CalculateFailSafeCost` and `GetFailSafePricing` guarantee that unpriced or misconfigured models never receive $0.00 free upstream access. If dynamic LiteLLM or DB pricing lookups fail during `RecordUsage`, conservative baseline pricing is applied alongside high-priority error logging.
  - **Payment Webhook Ingress & Resolution Hardening**:
    - `PaymentWebhookHandler` enforces strict order number format validation (`isValidOutTradeNo`) prior to executing database queries or provider candidate lookups. Rejects empty, over-length (>64 chars), or malformed inputs with HTTP 400 immediately.
    - Stripe multi-instance resolution and event parsing extracts `orderId`, `order_id`, `out_trade_no`, `outTradeNo`, `orderNo`, and `client_reference_id` from metadata. If instance resolution fails, returns HTTP 500 (retryable) instead of silently acknowledging with 200 OK. Missing order IDs log explicit warnings before acking to prevent provider retry floods.
    - `checkCancelRateLimit` in `payment_order_lifecycle.go` fails closed with HTTP 500 on database error rather than bypassing rate limiting.
    - NOWPayments status mapping: `sending`, `confirming`, and `waiting` map to `ProviderStatusPending`. Only `finished` and `confirmed` mark orders as paid.
  - **Deterministic & Idempotent Refund Operations**:
    - `payment.RefundRequest` includes `RefundID string` (set to deterministic `rf_<out_trade_no>` by `PaymentService.gwRefund`).
    - Alipay (`OutRequestNo`) and WeChat Pay (`OutRefundNo`) use this deterministic ID instead of timestamp-based random strings, preventing duplicate merchant refunds upon retries.
    - Stripe attaches `IdempotencyKey` to refund creation parameters.

### 2.4 Sora Engine
- **Target**: Sora image and video generation endpoints.
- **Normalizer & Pricing**: Normalizes base URLs for Sora accounts and calculates request pricing based on resolution and mode.

---

## ⚡ 3. Load Balancing & Traffic Capacity Management

### 3.1 Organic Token Load Balancer & UsedTokens Decay
- **State Tracking**: `NodeRuntimeState` tracks `UsedTokens` (total tokens consumed by the node).
- **Dynamic Penalty**: Incremented upon request completion and factored into `Scheduler` node scoring. Capped at 25 points maximum so long-lived node usage statistics do not permanently penalize active nodes.
- **Priority-Scaled Sensitivity**: Higher user priority scales queue depth and latency penalties, ensuring premium traffic is automatically routed to idle and lower-latency nodes.

### 3.2 Dynamic Fair Share Quota Manager & Degradation FIFO Queue (`pkg/hypervisor/quota`)
- **DEGRADED Mode FIFO Priority Queue**: When system state transitions to `DEGRADED`, low-priority requests (`userPriority < 2`) enter a real Redis FIFO priority queue `ash:queue:{priority}` via `waitForPriorityQueueSlot`, polling until they reach the head of the queue or until a 3-second max queue timeout occurs.
- **Dynamic Limits & Anti-Drain Reserves**: Uses Redis-based dynamic limit calculation (`GetDynamicLimitPerWeight`) relative to total pool capacity and active user weight. Applies `PoolReservePercent` (default: 20%) to keep emergency capacity unallocated, and caps individual active user shares at `MaxOwnerPoolSharePercent` (default: 50%) to prevent pool draining by a single user.
- **Pre-Request Token Reservations**: `ReserveQuotaTokens` atomically checks limits and reserves estimated tokens in Redis Lua before request execution; `estimateRequestTokens` parses `max_tokens` and `max_output_tokens` taking `max(max_tokens, max_output_tokens)` (defaulting to 4096 output tokens if omitted, capped at 128,000 max). `ReconcileQuotaTokens` reconciles reservations against actual consumption upon completion; for unlimited plans (`unlimited` or `MonthlyLimit <= 0`) or expired holds, it cleanly records actual usage without generating spurious "token reservation not found" errors.
- **Unlimited Keys & Policy Reactivation**: API keys with `quota <= 0` (or `PlanID == "unlimited"`) bypass all dynamic pool caps (`dynLimit`) and fixed monthly limits. Updating an existing key's quota to 0 or resetting its quota automatically reactivates keys previously in `quota_exhausted` status.
- **Concurrent Request Slots**: Enforces concurrent slot limits (`AcquireConcurrentSlot`, max 5 parallel requests per user) via Redis Lua scripts bounded to prevent negative counter underflow (`ReleaseConcurrentSlot`).
- **Preventing Pool Exhaustion**: Prevents heavy users from exhausting pool capacity, automatically queuing or prioritizing requests based on user tier.

---

## 🔐 4. Admin API Key & System Management

Administrators can view, copy, edit, and delete all API keys across all users in the system.

### Backend Admin API Keys Endpoints (`internal/handler/admin/apikey_handler.go`)
- `GET /api/v1/admin/api-keys`: Returns paginated list of all API keys across all users. Accepts `search`, `status`, `group_id`, and `user_id` filters.
- `PUT /api/v1/admin/api-keys/:id`: Updates any user's API key (name, group bindings, status, quota, expiration, rate limits, IP whitelists/blacklists).
- `DELETE /api/v1/admin/api-keys/:id`: Deletes any user's API key and invalidates auth cache.

### Backend Admin Auth Providers File Management (`internal/handler/admin/auth_providers_handler.go`, `internal/service/auth_providers_service.go`)
- **File Target**: `/data/auth_providers.json` (also synced to `./data/auth_providers.json`, `/app/data/auth_providers.json`, `/srv/parser-data/data/auth_providers.json`).
- **Security Rule**: `/data/auth_providers.json` is strictly **private** and NOT distributed publicly (unlike public parser json files).
- **Admin Write Endpoints**:
  - `POST /api/v1/admin/data/auth-providers` & `PUT /api/v1/admin/data/auth-providers`
  - `POST /api/v1/admin/auth-providers` & `PUT /api/v1/admin/auth-providers`
  - `POST /api/v1/admin/data/auth_providers.json` & `PUT /api/v1/admin/data/auth_providers.json`
  - Direct routes `POST /data/auth_providers.json` & `PUT /data/auth_providers.json` (protected by `adminAuth`)
- **Admin Read Endpoints**: `GET /api/v1/admin/data/auth-providers`, `GET /api/v1/admin/auth-providers`, `GET /api/v1/admin/data/auth_providers.json` (requires admin credentials).
- **Authentication**: Requires header `x-api-key: <admin-api-key>` or Bearer JWT token with admin role.

### Frontend Integration (`frontend/src/views/user/KeysView.vue` & `RegisterView.vue`)
- **Admin Detection**: Automatically calls `listAdmin`, `updateAdmin`, and `removeAdmin` when `isAdmin` is true.
- **Owner Column**: Displays the owner's username, email, and User ID in the table for admins.
- **User Filter**: Adds a user selection dropdown for filtering keys by owner.
- **Non-Admin Direct Key Creation**: For non-admin users, clicking "Create API Key" bypasses the configuration modal and directly creates a key bound to the `yunwu` group, automatically popping up the key token modal for one-time copying. Admin users retain the modal for full customization (owner, quota, custom key, expiration, rate limits, model restrictions).
- **Promo Code Disablement**: Promo code input has been removed from user registration (`frontend/src/views/auth/RegisterView.vue`) and `promo_code_enabled` defaults to `false` in backend system settings.

---

## 🛠 5. Hypervisor Subsystems Breakdown (`backend/pkg/hypervisor`)

| Package | Purpose & Functionality |
| :--- | :--- |
| `config` | Environment variables (`ASH_ENABLED`, `ASH_ENABLE_REBINDING`, etc.) and default settings. |
| `session` | `VirtualSession` state machine (`new` → `active` → `rebinding` → `queued` → `closed`), Redis TTL, versioning. |
| `lease` | Grants, renews, and releases temporary bindings (`Lease`) between a session and an `ExecutionNode`. |
| `scheduler` | Dynamic node selection score algorithm (success rate, latency, queue depth, 429s, token usage penalty, affinity). |
| `runtime` | Upstream HTTP streaming (`/v1/messages`, `/v1/chat/completions`, `/responses`), SSE transformers, `MockRuntime`. |
| `yunwu` | Dedicated execution engine, `YunwuScheduler`, and `YunwuRebindingManager` for Yunwu provider nodes. |
| `compiler` | Compiles execution contexts into stable prompt blocks across failovers. |
| `storage` | Provides Redis hot-state caching and PostgreSQL persistent storage helpers. |
| `events` | Append-only event writer logging system operations (`SessionCreated`, `RebindingStarted`, etc.). |
| `health` | Periodically reconciles node health states and manages degradation modes (`NORMAL`, `WARNING`, `DEGRADED`, `SURVIVAL`, `RECOVERY`). |
| `rebinding` | Executes session failover workflow with Redis distributed locking (`SETNX`) and single retry. |
| `quota` | Enforces plan limits, monthly usage units, daily burst budgets, concurrent slots, and fair use priorities. |
| `stats` | Aggregates analytics for user usage and administrator monitoring dashboards. |
| `backup` | Manages project snapshots and session state recovery. |

---

## 📊 6. Scoring Equation & Node States

### Node Scoring Equation (`pkg/hypervisor/scheduler`)
$$\text{score} = \text{success\_rate} \times 100 + \text{affinity\_bonus} + \text{priority\_bonus} - (\text{queue\_depth} \times 5) - \frac{\text{avg\_latency\_ms}}{100} - (\text{recent\_429} \times 30) - (\text{recent\_timeout} \times 20) - (\text{used\_tokens\_penalty})$$

### Node Health Lifecycle
- `healthy`: Ready for traffic.
- `busy`: Near max concurrency capacity.
- `limited`: Rate limited or elevated errors; used only if no healthy nodes exist.
- `cooldown`: Temporarily disabled (default 15 mins) after encountering `429`.
- `unavailable`: Disabled or offline.

---

## 📁 7. Repository Structure Map

```text
sub2api/
├── .agents/                    # Agent workspace rules (AGENTS.md)
├── CLAUDE.md                   # Master agent guide (This document)
├── ASH-Ideal-Project-TZ.md     # Technical Specification
├── Claude-Pool-Master-TZ.md    # Claude Pool Master Specification
├── deploy/                     # Docker Compose deployment manifests
│   ├── Dockerfile
│   └── docker-compose.yml
├── frontend/                   # Vue 3 / Vite Frontend Application
│   └── src/
│       ├── api/                # API clients (keys.ts, admin.ts, etc.)
│       ├── views/              # Pages (user/KeysView.vue, admin/*)
│       └── stores/             # Pinia stores (auth.ts, app.ts)
└── backend/
    ├── cmd/server/             # Application entrypoint
    ├── internal/
    │   ├── handler/            # HTTP Handlers (admin/, api_key_handler.go, gateway_handler.go)
    │   ├── server/             # Router, Middleware, Routes configuration (routes/admin.go, ash.go)
    │   ├── service/            # Core services (admin_service.go, api_key_service.go, etc.)
    │   └── repository/         # Data access repositories (api_key_repo.go, etc.)
    ├── migrations/             # SQL Migrations
    └── pkg/hypervisor/         # ASH Core Package
        ├── backup/             # Session & project backup/recovery
        ├── compiler/           # Context compiler
        ├── config/             # Runtime configurations & flags
        ├── events/             # Audit event store
        ├── health/             # Health controller & reconciliation
        ├── lease/              # Lease management
        ├── quota/              # Quota manager & concurrent slot limits
        ├── rebinding/          # Failover & rebinding engine
        ├── runtime/            # Provider runtime engines (Claude, OpenAI, Mock)
        ├── scheduler/          # Behavioral node scheduler
        ├── session/            # VirtualSession state machine
        ├── stats/              # Analytics & metrics aggregation
        ├── storage/            # Redis & PostgreSQL storage
        └── yunwu/              # Yunwu provider engine & scheduler
```

---

## 🗄 8. Redis Keys & Database Reference

### Redis Key Patterns
| Key Pattern | Purpose | Scope / TTL |
| :--- | :--- | :--- |
| `ash:session:{session_id}` | Serialized `VirtualSession` state | Active session lifetime |
| `ash:lease:{session_id}` | Active `Lease` binding | Lease duration |
| `ash:node:{node_id}` | `NodeRuntimeState` metrics (`UsedTokens`, latencies, errors) | Persistent / Request updated |
| `ash:node:{node_id}:runtime` | Detailed `NodeRuntimeState` execution metrics | Persistent |
| `ash:lock:session:{session_id}` | Rebinding distributed lock (`SETNX`) | 120 seconds |
| `ash:affinity:{owner_id}` | User node sticky assignment | 1-4 hours |
| `ash:quota_used:{owner_id}:{pool}` | Atomic persistent counter for user used tokens | 31 days |
| `ash:daily_usage:{owner_id}:{pool}:{date}` | Per-day usage counter for DailyBurst limit enforcement (`ash:daily_usage:<owner>:<pool>:<date>`) | 49 hours |
| `ash:daily_reserved:{owner_id}:{pool}:{date}` | Atomic in-flight daily reservation hold for token limits | 48 hours |
| `ash:quota_reserved:{owner_id}:{scope}` | Atomic in-flight monthly reservation hold for provider/shared token limits | 31 days |
| `ash:quota_token_reservations:{owner_id}:{scope}` | Hash storing per-request token reservation holds | TTL per request |
| `ash:quota_token_reservation_expiry:{owner_id}:{scope}` | Sorted set tracking expiring token reservation holds | TTL per request |
| `ash:daily_usage_micro_usd:{owner_id}:{date}` | Per-day monetary usage counter for Yunwu burst enforcement | 49 hours |
| `ash:daily_reserved_micro_usd:{owner_id}:{date}` | Atomic in-flight daily monetary reservation hold for Yunwu | 48 hours |
| `ash:quota_reserved_micro_usd:{owner_id}:yunwu:{month}` | Atomic in-flight monthly monetary reservation hold for Yunwu | 31 days |
| `ash:quota_reservations_micro_usd:{owner_id}` | Hash storing per-request Yunwu monetary reservation holds | TTL per request |
| `ash:quota_reservation_expiry_micro_usd:{owner_id}` | Sorted set tracking expiring Yunwu monetary reservation holds | TTL per request |
| `ash:system:recovery_until:{platform}` | Recovery degradation state timer | 5 minutes |
| `quota:concurrent:{user_id}` | Active concurrent request counter | Transient (Max 5) |

### Key PostgreSQL Tables
- `api_keys`: User API keys with group bindings, rate limits (5h/1d/7d), quotas, and IP rules.
- `ash_sessions`: Session metadata persistence.
- `ash_events`: System event audit logs.
- `ash_usage`: Token and request usage consumption records.
- `ash_nodes`: Execution node pool registry.
- `provider_comments`: User provider feedback, sentiment (`positive` / `negative` / `neutral`), nested parent/child replies, and reaction counters (`like_count`, `dislike_count`, `reply_count`).
- `provider_comment_votes`: 1:1 user reactions per comment (`like` / `dislike`), removable via vote cancellation (`vote_type: none`).
- `provider_popularity`: AI provider popularity click counters (`click_count`, `last_clicked_at`), protected by anti-abuse cooldown and rate limiters.

---

## 🌐 9. API Endpoints Quick Reference

### Client Gateways
- `POST /v1/messages`: Anthropic Messages API (Claude Code CLI / VS Code).
- `POST /v1/chat/completions`: OpenAI Chat Completions API.
- `POST /responses`: Codex / OpenAI Responses API gateway.
- `GET /v1/models`: Available model catalog.
- `GET /api/v1/data/providers.json`: Parser providers pricing JSON endpoint (also `/api/data/providers.json`, `/data/providers.json`).
- `GET /api/v1/data/models.json`: Parser models JSON endpoint (also `/api/data/models.json`, `/data/models.json`).
- `GET /api/v1/data/api_descriptions.json`: Parser API descriptions JSON endpoint (also `/api/data/api_descriptions.json`, `/data/api_descriptions.json`).
- `GET /api/v1/data/llm_rankings.json`: Parser LLM rankings JSON endpoint (also `/api/data/llm_rankings.json`, `/data/llm_rankings.json`).
- `GET /api/v1/data/image_generation_rankings.json`: Parser Image Generation rankings JSON endpoint (also `/api/data/image_generation_rankings.json`, `/data/image_generation_rankings.json`).
- `GET /api/v1/data/video_generation_rankings.json`: Parser Video Generation rankings JSON endpoint (also `/api/data/video_generation_rankings.json`, `/data/video_generation_rankings.json`).
- `GET /api/v1/data/text_to_speech_rankings.json`: Parser Text-to-Speech rankings JSON endpoint (also `/api/data/text_to_speech_rankings.json`, `/data/text_to_speech_rankings.json`).
- `GET /api/v1/data/speech_to_text_rankings.json`: Parser Speech-to-Text rankings JSON endpoint (also `/api/data/speech_to_text_rankings.json`, `/data/speech_to_text_rankings.json`).
- `GET /api/v1/data/provider_descriptions.json`: Parser provider descriptions JSON endpoint (also `/api/data/provider_descriptions.json`, `/data/provider_descriptions.json`).

### Provider Popularity & Anti-Abuse Click APIs (`/api/v1/providers/*`)
- `POST /api/v1/providers/:provider/click`: Public / optional auth. Records a click with anti-abuse protection: 15-minute per-IP & per-user deduplication cooldown, global IP rate limiter (max 30 req/min), and bot/crawler filter.
- `GET /api/v1/providers/popularity`: Public. Lists all providers ranked by popularity (`click_count DESC`).
- `GET /api/v1/providers/:provider/popularity`: Public. Gets popularity metrics for a single provider.

### Provider Comments & Reactions APIs (`/api/v1/providers/:provider/comments`, `/api/v1/comments/*`)
- `GET /api/v1/providers/:provider/comments`: Public (with optional auth). Lists root reviews for a provider with summary metrics (`positive_ratio`, `positive_count`, `negative_count`, `total_comments`), sentiment filters, and caller's `user_vote`.
- `POST /api/v1/providers/:provider/comments`: Authenticated (JWT). Creates a root review with positive or negative sentiment (`{ content, sentiment }`).
- `GET /api/v1/comments/:id/replies`: Public (with optional auth). Lists paginated replies for a specific comment thread.
- `POST /api/v1/comments/:id/reply`: Authenticated (JWT). Posts a nested reply under a comment.
- `POST /api/v1/comments/:id/vote`: Authenticated (JWT). Votes on a comment with `vote_type: "like" | "dislike" | "none"` (1 vote per user per comment).
- `DELETE /api/v1/comments/:id`: Always returns 403 Forbidden (`ErrCannotDeleteComment`) since comments are immutable and only votes can be deleted/cancelled.
- `GET /api/v1/admin/provider-comments`: Admin moderation list with provider, status, and sentiment filters.
- `PUT /api/v1/admin/provider-comments/:id/status`: Admin status toggle (`active` / `hidden`).

### Payment System & Gateway Providers (`/api/v1/payment/*` & `/api/v1/admin/payment/*`)
- **Supported Payment Types**:
  - `alipay`: Alipay aggregation / direct.
  - `wxpay`: WeChat Pay direct (native & JSAPI).
  - `stripe`: Stripe credit cards and alternative methods.
  - `easypay`: EasyPay aggregation platform.
  - `nowpayments`: NOWPayments cryptocurrency payment gateway (accepts 300+ cryptocurrencies with automated blockchain confirmations).
- **Core Endpoints**:
  - `GET /api/v1/payment/config`: User checkout configuration.
  - `GET /api/v1/payment/checkout-info`: Single-call payment page bootstrap data.
  - `POST /api/v1/payment/orders`: Create payment order (balance recharge or subscription).
  - `POST /api/v1/payment/orders/verify`: Verify payment order status.
  - `POST /api/v1/payment/webhook/nowpayments`: Public webhook callback verifying `x-nowpayments-sig` HMAC-SHA512 signature over alphabetically sorted JSON payload keys and idempotently fulfilling balances.
  - `GET/POST /api/v1/admin/payment/providers`: Manage payment gateway provider instances (NOWPayments, Stripe, Alipay, WeChat Pay, EasyPay). Backed by `payment_provider_instances` table (`145_add_payment_provider_instances.sql`).
  - `PUT/DELETE /api/v1/admin/payment/providers/:id`: Update credentials/status/limits or delete gateway provider instance.
- **User Payment UI Integration**:
  - Direct balance top-up flow: header user balance pill (`$0.00`) is an interactive button navigating directly to `/purchase` (`/payment`). Subscriptions mode and legacy plan tabs are removed from client payment views, focusing checkout exclusively on pure balance recharge.
  - Active payment method resolution: `paymentFlow.ts` recognizes `nowpayments` in `VISIBLE_METHOD_ALIASES` and `VisiblePaymentMethod`, mapping NOWPayments provider instances into active selectable recharge methods and launching checkout via `redirect_waiting` to the hosted invoice URL.
  - Payment result view (`PaymentResultView.vue`): Resolves payment order verification via token or public out_trade_no, formats amounts safely (`Number(fee_rate) || 0`), handles popup window opener focus/close navigation, and displays complete translated order details and status badges (`payment.orders.*`, `payment.status.*`).
- **Admin Panel UI Integration**:
  - Dedicated admin view `/admin/nowpayments` (`AdminNowPaymentsView.vue`) integrated into sidebar navigation as "NOWPayments" (replaces legacy multi-item payment menus with a single streamlined configuration view). Provides master toggle (syncs `payment_enabled` setting), sandbox mode switch, API key and IPN secret configuration with visibility toggles, currency override, and 1-click IPN webhook URL copy. Legacy `/admin/orders/providers` automatically redirects to `/admin/nowpayments`.
- **Platform Currency & Settlement**:
  - Standard payment currency across frontend UI and international gateways is USD (`$`).
  - Stripe initializes `PaymentIntent` in `usd`.
  - NOWPayments creates invoices in `usd` (matching system order `pay_amount`).
- **NOWPayments Provider Specs (`backend/internal/payment/provider/nowpayments.go`)**:
  - Configuration keys: `apiKey` (sensitive), `ipnSecret` (sensitive), `sandbox` (`true`/`false`, optional), `currency` (default `usd`, optional), `notifyUrl` (optional, auto-derived from host or return URL origin).
  - Host resolution: `https://api.nowpayments.io/v1` in production, `https://api-sandbox.nowpayments.io/v1` when `sandbox=true`.
  - Order creation: calls `POST /v1/invoice`, returns hosted `invoice_url` for customer redirection and coin selection. Empty or unset optional URL parameters (`ipn_callback_url`, `success_url`, `cancel_url`) are cleanly omitted from the JSON payload to avoid NOWPayments 400 validation rejections.
  - Reverse Proxy Prefix Support: When deployed behind a subpath (such as `/gateway` in front of Next.js root), `getPublicAppBaseUrl()` in frontend dynamically identifies the subpath prefix so checkout `origin` and `return_url` lead back to `/gateway/payment/result`. `CanonicalizeReturnURL` validates and accepts canonical return paths ending in `/payment/result` (including reverse proxy prefixes). In the backend (`payment_order.go` & `nowpayments.go`), auto-derived `notifyURL` and `ipn_callback_url` preserve this subpath prefix (e.g. `https://domain.com/gateway/api/v1/payment/webhook/nowpayments`), avoiding 404s from reverse proxies routing `/` to other services.
  - Order query: maps `finished`, `confirmed`, and `sending` to `ProviderStatusPaid` (prioritizes successful payments in invoice transaction lists).
  - Security & Webhooks: Alphabetical key sorting with `json.Number` preservation and unescaped HTML characters (`SetEscapeHTML(false)`) to guarantee HMAC-SHA512 verification fidelity.

### Forum APIs (`/api/v1/forum/*` & `/api/v1/admin/forum/*`)
- `GET /api/v1/forum/topics`: Paginated list of forum topics with sort options (`latest`, `top_liked`, `most_comments`), keyword `search`, and `user_vote` for authenticated viewers.
- `POST /api/v1/forum/topics`: Creates a new forum topic with `title` (up to 255 chars) and `content`/description (up to 10000 chars).
- `GET /api/v1/forum/topics/:id`: Fetches detailed topic information by ID.
- `POST /api/v1/forum/topics/:id/vote`: Sets reaction on topic (`like`, `dislike`, or `none` to remove vote) with transactional counter updates.
- `DELETE /api/v1/forum/topics/:id`: Always returns 403 Forbidden (`ErrCannotDeleteForumPost`).
- `GET /api/v1/forum/topics/:id/comments`: Paginated list of root comments for a topic (`sort`: `latest`, `oldest`, `top_liked`) with `user_vote`.
- `POST /api/v1/forum/topics/:id/comments`: Creates a root comment under a topic, automatically incrementing `comment_count`.
- `GET /api/v1/forum/comments/:id/replies`: Paginated list of replies to a specific comment.
- `POST /api/v1/forum/comments/:id/reply`: Posts a nested reply under a comment, automatically incrementing parent's `reply_count` and topic's `comment_count`.
- `POST /api/v1/forum/comments/:id/vote`: Sets reaction on comment (`like`, `dislike`, `none`).
- `DELETE /api/v1/forum/comments/:id`: Always returns 403 Forbidden (`ErrCannotDeleteForumPost`).
- `GET /api/v1/admin/forum/topics` & `PUT /api/v1/admin/forum/topics/:id/status`: Admin topic moderation (`active` / `hidden`).
- `GET /api/v1/admin/forum/comments` & `PUT /api/v1/admin/forum/comments/:id/status`: Admin comment moderation (`active` / `hidden`).

### Admin APIs (`/api/v1/admin/*`)
- `GET /api/v1/admin/api-keys`: Search and list all users' API keys.
- `PUT /api/v1/admin/api-keys/:id`: Edit any user's API key.
- `DELETE /api/v1/admin/api-keys/:id`: Delete any user's API key.
- `GET /api/v1/admin/users`: User account management.
- `GET /api/v1/admin/groups`: Group & channel management.

### ASH Diagnostics (`/api/v1/ash/*`)
- `GET /api/v1/ash/health`: Hypervisor health state and degradation mode.
- `GET /api/v1/ash/stats/nodes`: Real-time node runtime metrics.
- `GET /api/v1/ash/admin/yunwu/health`: Health status of Yunwu provider cluster.
- `GET /api/v1/ash/admin/yunwu/nodes`: List Yunwu nodes with credit balances and allocatable capacity.
- `POST /api/v1/ash/admin/yunwu/nodes/:id/sync`: Trigger manual credit balance synchronization for a Yunwu node.

### Server Binding & Security Defaults
- **Localhost Binding**: By default, `server.host` and `BIND_HOST` default to `127.0.0.1` across `config.go`, setup wizards, and systemd service. Docker compose configuration `deploy/docker-compose.local.yml` supports configurable host port mapping via `${HOST_PORT:-8000}:${SERVER_PORT:-8080}` to avoid port collisions on Windows Hyper-V / WSL reserved ranges.
- **Frontend Registration Route**: The `/register` route maps to `RegisterView.vue` with a sign-up link rendered on `LoginView.vue` (when `backend_mode_enabled` is disabled). Registration availability is dynamically driven by `settings.registration_enabled` (default initialized to `true` by `SettingService.InitializeDefaultSettings`).
- **Concurrency & Load Rate Scheduler Behavior**:
  - `ConcurrencyService`: `cachedAccountLoadSnapshot` and `cachedUserLoadSnapshot` store and preserve precomputed `LoadRate`. Upstream or mock-provided load rates are retained when concurrency and waiting counters are zero, rather than resetting to 0.
  - `gatewayAccountReadyTier` & `openAIAccountReadyTier`: Accounts with `LoadRate >= 100` are explicitly marked with `slotFree = 0` (ReadyTier 2 / saturated), ensuring tiered schedulers properly prioritize accounts with available capacity over saturated accounts.

---

## 🧪 10. Testing & Development Rules

### Workspace Rules (`.agents/AGENTS.md`)
1. **80% Unit Test Coverage Rule**: Modules in `pkg/hypervisor/...` and packages across the repository MUST maintain at least 80% unit test coverage.
2. **Clean Test Suite**: All unit tests must pass cleanly before pushing (`git push`).
3. **Pre-Push Documentation Rule**: You MUST update `CLAUDE.md` with any architectural changes, endpoints, or fixes before pushing.

### Running Backend Tests & Build Verification
```bash
cd backend
go test -tags=unit ./...
go build ./...
```

### Running Frontend Verification
```bash
cd frontend
npm run lint:check
npm run typecheck
```

