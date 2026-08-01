# CLAUDE.md - AI Agent Context & Repository Guide

Welcome to **sub2api** and the **AI Session Hypervisor (ASH / Yunwu Engine)** repository. This document serves as the master technical guide for AI assistants, agents, and human developers working on this codebase.

---

## 🚀 1. Overview & Business Model

**sub2api** is an enterprise-grade AI Gateway and Hypervisor system managing a pool of physical AI accounts (Claude Max 20x, OpenAI, and Yunwu accounts) and distributing capacity across users through unified API keys and endpoints.

### Key Value Proposition
- **Pooled Capacity Model**: Users get high-throughput Claude Max / OpenAI access cheaper than buying dedicated subscriptions, leveraging unused capacity across pooled accounts.
- **Virtual Sessions**: Users are decoupled from physical accounts. The system maintains continuous conversation state while transparently switching underlying physical nodes on failure.
- **Automatic Rebinding**: When an upstream node encounters `429 Too Many Requests`, timeouts, or disconnection, the Hypervisor locks the session, selects a healthy alternative node, recompiles context, and retries the request seamlessly.

```text
+--------+     +---------------+     +----------------+     +-----------+     +-------------------+
| Client | --> | Service Key / | --> | VirtualSession | --> | Scheduler | --> | Physical Node     |
| (CLI)  |     | Gateway API   |     | (State & Context)|   | & Lease   |     | (Claude / Yunwu)  |
+--------+     +---------------+     +----------------+     +-----------+     +-------------------+
```

---

## 🔍 2. Deep Dive: What We Built & How It Works Under the Hood

### 2.1 The Core Transformation
Sub2API was expanded from a simple single-account proxy into an **AI Session Hypervisor (ASH)** architecture. Instead of hardcoding a user to a physical account (`User -> Account`), we decoupled execution into virtual entities:

```text
User -> Service API Key -> VirtualSession -> Lease -> ExecutionNode (Claude / Yunwu Account)
```

- **`VirtualSession`** maintains the perpetual user conversation state, prompt history, summary, and metadata.
- **`Lease`** provides a short-lived binding to a specific physical account (`ExecutionNode`).
- **`Scheduler`** dynamically routes each request to the best available physical node.
- **`Rebinding`** transparently migrates a session to a new physical account if the current node fails (`429`, timeout, disconnect).

---

### 2.2 Detailed Subsystem Breakdown

#### 1. Virtual Session Management (`pkg/hypervisor/session`)
- **State Machine**: Tracks session state: `new` → `active` → `rebinding` → `queued` → `closed`.
- **Storage**: Caches hot session state in Redis (`ash:session:{session_id}`) with atomic versioning and persists metadata in PostgreSQL (`ash_sessions`).
- **Concurrency**: Prevents race conditions during state transitions.

#### 2. Lease Allocation & Binding (`pkg/hypervisor/lease`)
- Manages temporary leases connecting a `VirtualSession` to an `ExecutionNode`.
- Handles automatic lease granting, renewal, and release.
- Ensures a session does not stay bound to a node that has entered a limited or cooldown state.

#### 3. Behavioral Node Scheduler (`pkg/hypervisor/scheduler` & `pkg/hypervisor/yunwu`)
- Evaluates node health dynamically without relying on opaque upstream quota counters.
- **Scoring Equation**:

$$\text{score} = \text{success\_rate} \times 100 + \text{affinity\_bonus} + \text{priority\_bonus} - (\text{queue\_depth} \times 5) - \frac{\text{avg\_latency\_ms}}{100} - (\text{recent\_429} \times 30) - (\text{recent\_timeout} \times 20)$$

- **Node Filtering Rules**:
  - `unavailable` nodes are strictly excluded.
  - `cooldown` nodes are excluded until their cooldown timer expires.
  - `limited` nodes are used only if no `healthy` nodes are available.
  - Affinity provides a preference bonus but will never route to an unhealthy node.

#### 4. Distributed Rebinding Workflow (`pkg/hypervisor/rebinding`)
- **Distributed Redis Locking**: Uses `SETNX` on `ash:lock:session:{session_id}` (TTL: 120s) to guarantee only one rebinding operation occurs per session at a time.
- **Failover Sequence**:
  1. Lock session.
  2. Degrade failing node state to `limited` or `cooldown`.
  3. Emit `RebindingStarted` event.
  4. Invoke `Scheduler` to select a new healthy execution node.
  5. Run `ContextCompiler` to reconstruct prompt context.
  6. Grant a new `Lease` bound to the selected node.
  7. Execute a single upstream retry request.
  8. Emit `RebindingSucceeded` (or `RebindingFailed`) and release lock.

#### 5. Context Compiler (`pkg/hypervisor/compiler`)
- Reconstructs portable conversation context in a deterministic block order when migrating between physical accounts:
  1. `SystemPrompt`
  2. `Summary`
  3. `PinnedFacts`
  4. `TerminalState`
  5. `ToolState`
  6. `ProjectState`
  7. `RecentMessages`
  8. `CurrentUserPrompt`
- Guarantees full dialogue context continuity across physical node switches without prompt cache dependence.

#### 6. Responses API Normalization (`pkg/hypervisor/runtime`)
- Solves structured message array issues in Codex CLI Responses API (`/responses`).
- Includes `constructMessagesFromResponsesPayload`, which recursively inspects incoming `input` objects:
  ```json
  "input": [
    {
      "type": "message",
      "role": "user",
      "content": [
        {"type": "input_text", "text": "What is Telegram?"}
      ]
    }
  ]
  ```
- Extracts `input_text` elements into clean `messages` arrays (`"role": "user", "content": "What is Telegram?"`), ensuring prompts are never lost or ignored by upstream providers.

#### 7. Health Controller & System Degradation (`pkg/hypervisor/health`)
- Runs a background reconciliation loop every 10 seconds.
- Recalculates node metrics (success rates, latencies, recent 429s).
- Manages node state lifecycle: `healthy` → `busy` → `limited` → `unavailable` → `cooldown` → `healthy`.
- Automatically applies cooldowns (default 15 minutes) after 429 errors.
- Manages global degradation modes:
  - `NORMAL`: Full capacity, rebinding enabled.
  - `WARNING`: High latency/429 rates, penalized node scores.
  - `DEGRADED`: Node shortages, background tasks disabled, request queueing.
  - `SURVIVAL`: Critical shortage, high-priority requests only, short timeouts.
  - `RECOVERY`: Nodes returning from cooldown, gradual traffic ramp-up.

#### 8. Fair Use Quotas & Audit Logging (`pkg/hypervisor/quota`, `events`, `storage`)
- **Quota Manager**: Enforces `UserAccess` monthly limits, daily burst allowances, and priority queueing.
- **Event Store**: Writes append-only audit logs to `ash_events` table for full system traceability.
- **Usage Recorder**: Records detailed token consumption (`input_tokens`, `output_tokens`, `cost_units`) in `ash_usage` table.

#### 9. Fault Injection Testing (`pkg/hypervisor/runtime/mock.go`)
- Includes `MockRuntime` for simulating 429 rate limits, timeouts, slow SSE streams, malformed payloads, and disconnections without hitting live provider accounts.

#### 10. Backup & Recovery (`pkg/hypervisor/backup`)
- Handles session snapshots, git state, terminal snapshots, and project restoration.

---

## 📁 3. Repository Structure

```text
sub2api/
├── .agents/                    # Agent customization rules (AGENTS.md)
├── CLAUDE.md                   # Master agent guide (This document)
├── ASH-Ideal-Project-TZ.md     # Original ASH Technical Specification
├── Claude-Pool-Master-TZ.md    # Master Specification for Claude Pool Hypervisor
├── deploy/                     # Docker Compose deployment manifests
│   ├── Dockerfile
│   └── docker-compose.yml
└── backend/
    ├── cmd/server/             # Application entrypoint
    ├── internal/
    │   ├── handler/            # HTTP Handlers (gateway_handler.go, openai_gateway_handler.go, ash.go)
    │   ├── server/             # Router, Middleware, Routes configuration
    │   └── service/            # Core business services (pricing, auth, email, etc.)
    ├── migrations/             # SQL Migrations (135_add_ash_hypervisor_tables.sql)
    └── pkg/hypervisor/         # ASH Core Package
        ├── backup/             # Session & project snapshot backup/recovery
        ├── compiler/           # Portable context block compilation
        ├── config/             # Environment flags & runtime parameters
        ├── events/             # Append-only audit & operation event logging
        ├── health/             # Background health reconciliation & degradation policy
        ├── lease/              # Session-to-node lease management
        ├── quota/              # Fair Use & quota enforcement
        ├── rebinding/          # Distributed Redis-locked session rebinding workflow
        ├── runtime/            # Anthropic, OpenAI, & Mock runtime execution engines
        ├── scheduler/          # Behavioral scoring & node selection algorithm
        ├── session/            # VirtualSession state machine & Redis lifecycle
        ├── stats/              # User & Admin statistics aggregation APIs
        ├── storage/            # Redis & PostgreSQL persistence layer
        ├── yunwu/              # Dedicated Yunwu Hypervisor Engine & Scheduler
        ├── hypervisor.go       # Core Hypervisor coordinator facade
        └── hypervisor_test.go  # End-to-end hypervisor suite
```

---

## 🛠 4. Hypervisor Package Architecture (`backend/pkg/hypervisor`)

| Package | Purpose & Functionality | Test Coverage |
| :--- | :--- | :---: |
| `config` | Environment variables (`ASH_ENABLED`, `ASH_ENABLE_REBINDING`, etc.) and default settings. | **87.5%** |
| `session` | `VirtualSession` state machine (`new` → `active` → `rebinding` → `queued` → `closed`), Redis TTL, versioning. | **87.5%** |
| `lease` | Grants, renews, and releases temporary bindings (`Lease`) between a session and an `ExecutionNode`. | **90.5%** |
| `scheduler` | Dynamic node selection score algorithm (success rate, latency, queue depth, 429s, affinity). | **92.5%** |
| `runtime` | Upstream HTTP streaming (`/v1/messages`, `/v1/chat/completions`), SSE transformers, `MockRuntime`. | **83.8%** |
| `yunwu` | Dedicated execution engine, `YunwuScheduler`, and `YunwuRebindingManager` for Yunwu provider nodes. | **80.4%** |
| `compiler` | Compiles execution contexts into stable prompt blocks. | **92.0%** |
| `storage` | Provides Redis hot-state caching and PostgreSQL persistent storage helpers. | **92.3%** |
| `events` | Append-only event writer logging system operations (`SessionCreated`, `RebindingStarted`, etc.). | **100.0%** |
| `health` | Periodically reconciles node health states and manages degradation modes. | **82.5%** |
| `rebinding` | Executes session failover workflow with Redis distributed locking (`SETNX`) and single retry. | **80.4%** |
| `quota` | Enforces plan limits, monthly usage units, daily burst budgets, and fair use priorities. | **86.1%** |
| `stats` | Aggregates and serves analytics for user usage and administrator monitoring dashboards. | **81.6%** |
| `backup` | Manages project snapshots and session state recovery. | **86.1%** |

---

## 📊 5. Data Layer & Storage Schemas

### Redis Hot State Keys

| Key Pattern | Purpose | TTL / Scope |
| :--- | :--- | :--- |
| `ash:session:{session_id}` | Serialized `VirtualSession` state | Active session lifetime |
| `ash:lease:{session_id}` | Active `Lease` binding | Lease duration |
| `ash:node:{node_id}` | Live `NodeRuntimeState` metrics | Persistent / Updated per request |
| `ash:lock:session:{session_id}` | Rebinding distributed lock | 120 seconds |
| `ash:affinity:{owner_id}` | Preferred node assignment | 1-4 hours |
| `ash:queue:{priority}` | High-priority request queue | Transient |

### PostgreSQL Tables (`migrations/135_add_ash_hypervisor_tables.sql`)

- `ash_events`: Append-only system audit log (`id`, `owner_id`, `session_id`, `node_id`, `event_type`, `payload`, `created_at`).
- `ash_usage`: Detailed token & unit consumption log (`id`, `owner_id`, `session_id`, `node_id`, `input_tokens`, `output_tokens`, `cost_units`, `created_at`).
- `ash_sessions`: Session metadata persistence (`id`, `owner_id`, `conversation_id`, `state`, `summary`, `metadata`, timestamps).
- `ash_deposits`: User deposit tracking (`id`, `owner_id`, `amount_cents`, `currency`, `state`, `notice_days`, timestamps).
- `ash_nodes`: Execution node pool registry (`id`, `account_name`, `provider`, `state`, `capabilities`, `priority`, timestamps).

---

## ⚙️ 6. Environment Variables & Feature Flags

| Variable | Default | Description |
| :--- | :--- | :--- |
| `ASH_ENABLED` | `true` | Master toggle for AI Session Hypervisor routing |
| `ASH_ENABLE_REBINDING` | `true` | Enables automatic session failover on 429/timeout |
| `ASH_ENABLE_AFFINITY` | `true` | Enables user-to-node sticky session preference |
| `ASH_ENABLE_SUMMARY` | `true` | Enables background conversation summarization |
| `ASH_ENABLE_HEALTH_CONTROLLER` | `true` | Runs background health reconciliation loop |
| `ASH_ENABLE_QUOTA_MANAGER` | `true` | Enforces fair use quota limits |
| `ASH_ENABLE_MOCK_RUNTIME` | `false` | Enables mock fault injection for testing |
| `ASH_MAX_RETRY` | `1` | Maximum automated retries per user request |
| `ASH_NODE_COOLDOWN_SECONDS` | `900` | Cooldown period (15m) for nodes hitting 429 |
| `ASH_SESSION_LOCK_TTL_SECONDS` | `120` | Distributed lock TTL for rebinding operations |

---

## 🌐 7. Main API Endpoints

### Client Gateway APIs
- `POST /v1/messages`: Anthropic Messages API (Claude Code CLI, VS Code extension).
- `GET /v1/models`: Returns list of available models.
- `POST /responses`: OpenAI / Codex Responses API gateway.

### ASH Monitoring & Stats APIs
- `GET /api/v1/ash/health`: System health and current degradation mode.
- `GET /api/v1/ash/stats`: Global usage, session, and rebinding statistics.
- `GET /api/v1/ash/stats/nodes`: Real-time node runtime metrics and health states.
- `GET /api/v1/ash/stats/analytics`: Historical usage trends and performance charts.
- `GET /api/v1/ash/admin/yunwu/nodes`: List active Yunwu execution nodes.
- `GET /api/v1/ash/admin/yunwu/health`: Health state of Yunwu provider cluster.

---

## 🧪 8. Testing & Development Rules

### Mandatory Workspace Rules (`AGENTS.md`)
1. **80% Unit Test Coverage Rule**: All modules and packages inside `pkg/hypervisor/...` MUST maintain at least **80% unit test coverage**.
2. **Clean Passing Tests**: All unit tests must pass before any code is pushed (`git push`).

### Running Tests
To run all hypervisor unit tests and verify coverage:

```bash
cd backend
go test -cover ./pkg/hypervisor/...
```

### Deployment & Verification
To rebuild and restart the Docker container stack:

```bash
cd deploy
docker compose up -d --force-recreate --build
```
