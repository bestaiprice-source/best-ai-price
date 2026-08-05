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
- **Virtual Sessions & Rebinding**: Maintains `VirtualSession` state in Redis and PostgreSQL. Rebinds transparently to a new Claude node on `429` or timeout.
- **Context Compiler**: Reconstructs prompt blocks (`SystemPrompt`, `Summary`, `PinnedFacts`, `TerminalState`, `ToolState`, `ProjectState`, `RecentMessages`, `CurrentUserPrompt`) when migrating sessions between physical accounts.

### 2.2 OpenAI & Codex Engine
- **Target**: OpenAI `/v1/chat/completions` and Codex `/responses` endpoints.
- **Payload Normalization**: Includes `constructMessagesFromResponsesPayload` in `pkg/hypervisor/runtime` to handle structured message arrays from Codex CLI:
  ```json
  "input": [
    {
      "type": "message",
      "role": "user",
      "content": [{"type": "input_text", "text": "Hello"}]
    }
  ]
  ```
  Extracts `input_text` into clean message arrays (`role: "user", content: "Hello"`), avoiding lost or unparsed prompts.
- **SSE Transformer Bypass**: Direct `/responses` API calls bypass `responsesSSETransformer` to prevent truncated or empty responses when streaming native OpenAI/Codex format.

### 2.3 Yunwu Engine (`pkg/hypervisor/yunwu`)
- **Target**: Yunwu multi-channel provider API accounts.
- **Dedicated Subsystems**:
  - `YunwuScheduler`: Tailored node selection for Yunwu execution nodes based on node health, weight, and rate limits.
  - `YunwuRebindingManager`: Handles failover and node cooldown transitions specifically for Yunwu provider clusters.
  - Administration endpoints under `/api/v1/ash/admin/yunwu/*`.

### 2.4 Sora Engine
- **Target**: Sora image and video generation endpoints.
- **Normalizer & Pricing**: Normalizes base URLs for Sora accounts and calculates request pricing based on resolution and mode (`sora_image_price`, `sora_video_price_per_request`).

---

## ⚡ 3. Load Balancing & Traffic Capacity Management

### 3.1 Organic Token Load Balancer
- **State Tracking**: `NodeRuntimeState` tracks `UsedTokens` (total tokens consumed by the node).
- **Dynamic Penalty**: Incremented upon request completion and factored into `Scheduler` node scoring as a penalty.
- **Traffic Spread**: Forces traffic away from recently heavily-used nodes toward idle nodes, ensuring organic load distribution even across nodes with identical latencies.

### 3.2 Dynamic Fair Share Quota Manager (`pkg/hypervisor/quota`)
- **Dynamic Limits**: Uses Redis-based dynamic limit calculation (`GetDynamicLimitPerWeight`) relative to total pool capacity and active user weight.
- **Concurrent Request Slots**: Enforces concurrent slot limits (`AcquireConcurrentSlot`, max 5 parallel requests per user) via Redis counters.
- **Preventing Pool Exhaustion**: Prevents a single user from consuming the entire pool capacity, automatically queuing or prioritizing requests based on user tier.

---

## 🔐 4. Admin API Key & System Management

Administrators can view, copy, edit, and delete all API keys across all users in the system.

### Backend Admin API Keys Endpoints (`internal/handler/admin/apikey_handler.go`)
- `GET /api/v1/admin/api-keys`: Returns paginated list of all API keys across all users. Accepts `search`, `status`, `group_id`, and `user_id` filters.
- `PUT /api/v1/admin/api-keys/:id`: Updates any user's API key (name, group bindings, status, quota, expiration, rate limits, IP whitelists/blacklists).
- `DELETE /api/v1/admin/api-keys/:id`: Deletes any user's API key and invalidates auth cache.

### Frontend Integration (`frontend/src/views/user/KeysView.vue`)
- **Admin Detection**: Automatically calls `listAdmin`, `updateAdmin`, and `removeAdmin` when `isAdmin` is true.
- **Owner Column**: Displays the owner's username, email, and User ID in the table for admins.
- **User Filter**: Adds a user selection dropdown for filtering keys by owner.

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
| `ash:daily_usage:{owner_id}:{pool}:{date}` | Per-day usage counter for DailyBurst limit enforcement | 25 hours |
| `ash:system:recovery_until:{platform}` | Recovery degradation state timer | 5 minutes |
| `quota:concurrent:{user_id}` | Active concurrent request counter | Transient (Max 5) |

### Key PostgreSQL Tables
- `api_keys`: User API keys with group bindings, rate limits (5h/1d/7d), quotas, and IP rules.
- `ash_sessions`: Session metadata persistence.
- `ash_events`: System event audit logs.
- `ash_usage`: Token and request usage consumption records.
- `ash_nodes`: Execution node pool registry.

---

## 🌐 9. API Endpoints Quick Reference

### Client Gateways
- `POST /v1/messages`: Anthropic Messages API (Claude Code CLI / VS Code).
- `POST /v1/chat/completions`: OpenAI Chat Completions API.
- `POST /responses`: Codex / OpenAI Responses API gateway.
- `GET /v1/models`: Available model catalog.

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

---

## 🧪 10. Testing & Development Rules

### Workspace Rules (`.agents/AGENTS.md`)
1. **80% Unit Test Coverage Rule**: Modules in `pkg/hypervisor/...` MUST maintain at least 80% unit test coverage.
2. **Clean Test Suite**: All unit tests must pass before pushing (`git push`).
3. **Pre-Push Documentation Rule**: You MUST update `CLAUDE.md` with any architectural changes, endpoints, or fixes before pushing.

### Running Backend Tests & Build Verification
```bash
cd backend
go test -cover ./pkg/hypervisor/...
go build ./...
```

### Running Frontend Verification
```bash
cd frontend
npm run lint
```
