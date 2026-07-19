# Claude Pool Gateway / AI Session Hypervisor (ASH)

ASH is an intelligent, high-performance gateway and session hypervisor designed to coordinate a shared pool of premium upstream accounts (Claude Max and ChatGPT Pro) and distribute access to end-users via virtual sessions.

It provides automatic request retrying, upstream token/quota tracking, session affinity (keeping a conversation locked to a physical account to optimize prompt caching), and dynamic degradation modes (Normal, Warning, Degraded, Survival) depending on the availability of healthy accounts in the pool.

---

## Feature Flags & Environment Variables

Configure ASH by setting these variables in your `.env` or system environment:

* `ASH_ENABLED=true`: Enable or disable the session hypervisor.
* `ASH_ENABLE_REBINDING=true`: Enable automatic session migration/retry to another healthy node upon 429 rate limit or timeout.
* `ASH_ENABLE_AFFINITY=true`: Bind virtual sessions to the same physical node for better prompt caching.
* `ASH_ENABLE_HEALTH_CONTROLLER=true`: Enable background inspector that monitors upstream success rates, handles node cooldowns, and updates the degradation status.
* `ASH_ENABLE_QUOTA_MANAGER=true`: Enable user monthly quota limit and daily burst tracking.
* `ASH_ENABLE_USAGE_RECORDER=true`: Save all processed request usage statistics.
* `ASH_ENABLE_MOCK_RUNTIME=true`: Simulate upstream calls without calling actual APIs. Return simulated stream chunks.
* `ASH_MAX_RETRY=1`: Maximum rebinding attempts per request (recommended: `1`).
* `ASH_NODE_COOLDOWN_SECONDS=900`: Time a node stays in cooldown after a 429 error before warm-up.

---

## API Endpoints Reference

All monitoring and administrative APIs are located under the `/ash` path:

### User & Public APIs
* **`GET /ash/health?platform={anthropic|openai}`**: Get current degradation mode (`NORMAL`, `WARNING`, `DEGRADED`, `SURVIVAL`) and node counts. (Public)
* **`GET /ash/stats/users/{owner_id}`**: Get user plan, monthly/daily limits, consumed units, and state. (Requires standard JWT Token)
* **`GET /ash/sessions/{session_id}`**: Get virtual session state and active node lease duration. (Requires standard JWT Token)

### Administrative APIs (Requires Admin Auth JWT Token)
* **`GET /ash/stats`**: Retrieve global token usage statistics (input/output tokens, cost units) and count of active sessions.
* **`GET /ash/stats/nodes`**: Retrieve runtime stats for all accounts in the pool (success rate, average latency, recent 429s/timeouts, cooldown timers).
* **`GET /ash/events?owner_id=...`**: Fetch audit trail event logs (`RebindingStarted`, `QuotaExceeded`, `NodeLimited`, etc.).
* **`GET /ash/usage?owner_id=...`**: Fetch detailed historical usage reports.
* **`POST /ash/admin/nodes/{node_id}/cooldown`**: Manually trigger a 10-minute cooldown on an account.
* **`POST /ash/admin/nodes/{node_id}/disable`**: Disable a pool account manually.
* **`POST /ash/admin/nodes/{node_id}/enable`**: Reset counters and re-enable a pool account.
* **`POST /ash/admin/users/{owner_id}/plan`**: Assign plan ID to a user.
* **`POST /ash/admin/users/{owner_id}/quota`**: Update monthly limit, daily burst, priority, or active status.
* **`POST /ash/admin/deposits/{deposit_id}/release`**: Change deposit state to released.

---

## Testing Guide

### 1. Mock Testing (Without real upstream accounts)
1. Add `ASH_ENABLED=true` and `ASH_ENABLE_MOCK_RUNTIME=true` to your `.env` file.
2. Start the server using: `go run cmd/server/main.go`
3. Request message generation via curl:
   ```bash
   # Test Claude format:
   curl -X POST http://localhost:8080/v1/messages \
     -H "Authorization: Bearer <your_user_api_key>" \
     -H "Content-Type: application/json" \
     -d '{"model": "claude-3-5-sonnet", "messages": [{"role": "user", "content": "Hello!"}], "stream": true}'

   # Test OpenAI format:
   curl -X POST http://localhost:8080/v1/chat/completions \
     -H "Authorization: Bearer <your_user_api_key>" \
     -H "Content-Type: application/json" \
     -d '{"model": "gpt-4o", "messages": [{"role": "user", "content": "Hello!"}], "stream": true}'
   ```
4. Query node statistics:
   ```bash
   curl -X GET http://localhost:8080/ash/health?platform=anthropic
   ```

---

## Known Limitations (MVP)

1. **Warmup Priority**: During recovery warmup mode, new requests are funneled gradually, but the queue manager doesn't prioritize basic vs. priority queue scheduling outside of survival/degradation modes.
2. **Concurrent Rebinding**: If multiple requests for the *same session* fail concurrently, they might both trigger rebinding and race to update the lease. Redis lock limits this window, but concurrent retries are sent to the newly leased node.
3. **Upstream Response Validation**: The mock runtime handles malformed stream simulation, but in production, raw stream parsing relies on client compatibility (e.g. Claude Code or VS Code extension parsing the SSE stream).
