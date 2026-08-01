# CLAUDE.md - AI Agent Context & Repository Guide

Welcome to **sub2api** and the **AI Session Hypervisor (ASH / Yunwu Engine)** repository. This document serves as the master technical guide for AI assistants, agents, and human developers working on this codebase.

---

## 🚀 1. Overview & Business Model

**sub2api** is an enterprise-grade AI Gateway and Hypervisor system managing a pool of physical AI accounts (Claude Max 20x, OpenAI, and Yunwu accounts) and distributing capacity across users through unified API keys and endpoints.

### Key Value Proposition
- **Pooled Capacity Model**: Users get high-throughput Claude Max / OpenAI access cheaper than buying dedicated subscriptions, leveraging unused capacity across pooled accounts.
- **Virtual Sessions**: Users are decoupled from physical accounts. The system maintains continuous conversation state while transparently switching underlying physical nodes on failure.
- **Automatic Rebinding**: When an upstream node encounters `429 Too Many Requests`, timeouts, or disconnection, the Hypervisor locks the session, selects a healthy alternative node, recompiles context, and retries the request seamlessly.

```
+--------+     +---------------+     +----------------+     +-----------+     +-------------------+
| Client | --> | Service Key / | --> | VirtualSession | --> | Scheduler | --> | Physical Node     |
| (CLI)  |     | Gateway API   |     | (State & Context)|   | & Lease   |     | (Claude / Yunwu)  |
+--------+     +---------------+     +----------------+     +-----------+     +-------------------+
```

---

## 📁 2. Repository Structure

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

## 🛠 3. Hypervisor Package Architecture (`backend/pkg/hypervisor`)

| Package | Purpose & Functionality |
| :--- | :--- |
| `config` | Manages environment variables (`ASH_ENABLED`, `ASH_ENABLE_REBINDING`, etc.) and default runtime settings. |
| `session` | Manages `VirtualSession` state machine (`new` → `active` → `rebinding` → `queued` → `closed`), Redis state TTL, and versioning. |
| `lease` | Grants, renews, and releases temporary bindings (`Lease`) between a session and an `ExecutionNode`. |
| `scheduler` | Implements the node selection algorithm based on observed metrics (success rate, latency, queue depth, 429s, affinity). |
| `runtime` | Handles upstream HTTP streaming (Anthropic `/v1/messages`, OpenAI `/v1/chat/completions`), SSE transformation, and `MockRuntime` for fault injection. |
| `yunwu` | Dedicated execution engine, scheduler (`YunwuScheduler`), and rebinding manager (`YunwuRebindingManager`) tailored for Yunwu provider nodes. |
| `compiler` | Compiles execution contexts into stable prompt blocks (`SystemPrompt` → `Summary` → `PinnedFacts` → `TerminalState` → `ToolState` → `ProjectState` → `RecentMessages` → `CurrentUserPrompt`). |
| `storage` | Provides Redis hot-state caching and PostgreSQL persistent storage helpers. |
| `events` | Append-only event writer logging system operations (`SessionCreated`, `RebindingStarted`, `UsageRecorded`, `NodeLimited`, etc.). |
| `health` | Periodically reconciles node health states (`healthy` → `busy` → `limited` → `unavailable` → `cooldown`) and manages degradation modes. |
| `rebinding` | Executes session failover workflow with Redis distributed locking (`SETNX`), single-retry policy, and event logging. |
| `quota` | Enforces plan limits, monthly usage units, daily burst budgets, and fair use priorities. |
| `stats` | Aggregates and serves analytics for user usage and administrator monitoring dashboards. |
| `backup` | Manages project snapshots and session state recovery. |

---

## 🧮 4. Core Algorithms & Logic

### 4.1 Scheduler Node Selection Formula

The scheduler does not rely on opaque upstream quota counters; it evaluates nodes dynamically based on observed health metrics:

$$\text{score} = \text{success\_rate} \times 100 + \text{affinity\_bonus} + \text{priority\_bonus} - (\text{queue\_depth} \times 5) - \frac{\text{avg\_latency\_ms}}{100} - (\text{recent\_429} \times 30) - (\text{recent\_timeout} \times 20)$$

**Rules:**
- `unavailable` nodes are strictly excluded.
- `cooldown` nodes are excluded until their cooldown timer expires.
- `limited` nodes are used only if no `healthy` nodes are available.
- Node affinity grants a bonus score but will not override an unhealthy node status.

### 4.2 Distributed Rebinding Workflow

When an upstream execution returns `429` or times out:
1. **Acquire Redis Lock**: `ash:lock:session:{session_id}` (TTL: 120s).
2. **Transition State**: Set `VirtualSession.State = "rebinding"`.
3. **Degrade Node**: Mark the failing node as `limited` or `cooldown`.
4. **Emit Event**: Record `RebindingStarted` in `ash_events`.
5. **Select New Node**: Execute `Scheduler.SelectNode()` to find an alternative healthy node.
6. **Compile Context**: Run `ContextCompiler` to prepare portable prompt state.
7. **Grant Lease**: Assign new `Lease` bound to the newly selected node.
8. **Retry Request**: Execute single retry upstream.
9. **Release Lock & Finalize**: Record `RebindingSucceeded` (or `RebindingFailed`) and restore state to `active`.

### 4.3 Responses API Input Normalization

When handling OpenAI Responses API payloads (`/responses`), incoming user prompts can arrive as either raw strings or complex item arrays:

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

`runtime.go` provides `constructMessagesFromResponsesPayload`, which recursively inspects and extracts `input_text` parts into clean OpenAI/Yunwu compatible `messages` arrays, preventing empty or unparsed prompt submissions.

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
