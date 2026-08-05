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
* `ASH_ENABLE_SUMMARY=true`: Enable portable context summaries during rebinding.
* `ASH_ENABLE_BACKUP=true`: Enable project snapshot/restore support.
* `ASH_ENABLE_MOCK_RUNTIME=false`: Use synthetic upstream responses. Set to `true` only in development and tests.
* `ASH_MAX_RETRY=1`: Maximum rebinding attempts per request (recommended: `1`).
* `ASH_NODE_COOLDOWN_SECONDS=900`: Time a node stays in cooldown after a 429 error before warm-up.
* `ASH_SESSION_LOCK_TTL_SECONDS=120`: Redis lock lifetime for one rebinding workflow.
* `ASH_ACTIVE_USER_WINDOW_SECONDS=86400`: Fair-share activity window.
* `ASH_YUNWU_BALANCE_SYNC_SECONDS=300`: Yunwu API balance refresh interval.
* `ASH_YUNWU_BALANCE_STALE_SECONDS=1800`: Maximum snapshot age before a node is excluded.
* `ASH_YUNWU_BALANCE_RESERVE_PERCENT=10`: Balance kept as an operational reserve.
* `ASH_YUNWU_DEFAULT_INPUT_USD_PER_1K=0.002` and `ASH_YUNWU_DEFAULT_OUTPUT_USD_PER_1K=0.002`: fallback model prices.

Standard tariff usage is shared, while dynamic capacity is calculated independently for `standard:anthropic` and `standard:openai`. Active weights are the sum of priorities for users seen in the last 24 hours.

Yunwu quota uses integer micro-USD. A request is billed as `ceil(input_tokens × input_USD_per_1k × 1000 + output_tokens × output_USD_per_1k × 1000)`. Allocatable remaining balance is the sum of fresh API-confirmed balances minus the configured reserve; the monthly pool budget adds local month-to-date spend for reconciliation. If both balance endpoints fail, the last successful snapshot remains usable until the stale timeout, after which the node is removed from scheduling. Unknown, zero, stale, and reserve-exhausted balances are never assigned.

---

## API Endpoints Reference

The canonical monitoring namespace is `/api/v1/ash`. The legacy `/ash` namespace is deprecated and returns `Deprecation`/`Sunset` headers.

### User & Public APIs
* **`GET /api/v1/ash/health`**: Liveness of the hypervisor. (Public)
* **`GET /api/v1/ash/me/stats?pool_type={standard|yunwu}`**: Get the authenticated user's plan and usage. (Requires user JWT.)
* **`GET /api/v1/ash/me/sessions/{session_id}`**: Get a session owned by the authenticated user. (Requires user JWT.)
* **`POST /api/v1/ash/me/sessions/{session_id}/snapshot`**: Persist project metadata and the complete portable session context. (Requires user JWT.)
* **`POST /api/v1/ash/me/sessions/{session_id}/restore`**: Restore an owned session after hot-state loss. (Requires user JWT.)

### Administrative APIs (Requires Admin Auth JWT Token)
Replace `/ash` below with canonical `/api/v1/ash` in new integrations.

* **`GET /ash/stats`**: Retrieve global usage plus the mandatory live request, failure, 429, rebinding, latency, queue, utilization, summary, and snapshot metrics.
* **`GET /ash/stats/users/{owner_id}`**: Retrieve plan/fair-use, active sessions, rebinding count, recent errors, and deposit state for one user.
* **`GET /ash/stats/nodes`**: Retrieve runtime stats for all accounts in the pool (success rate, average latency, recent 429s/timeouts, cooldown timers).
* **`GET /ash/events?owner_id=...`**: Fetch audit trail event logs (`RebindingStarted`, `QuotaExceeded`, `NodeLimited`, etc.).
* **`GET /ash/usage?owner_id=...`**: Fetch detailed historical usage reports.
* **`POST /ash/admin/nodes/{node_id}/cooldown`**: Manually trigger a 10-minute cooldown on an account.
* **`POST /ash/admin/nodes/{node_id}/disable`**: Disable a pool account manually.
* **`POST /ash/admin/nodes/{node_id}/enable`**: Reset counters and re-enable a pool account.
* **`POST /ash/admin/users/{owner_id}/plan`**: Assign plan ID to a user.
* **`POST /ash/admin/users/{owner_id}/quota`**: Update monthly limit, daily burst, priority, or active status.
* **`POST /ash/admin/deposits/{deposit_id}/release`**: Change deposit state to released.
* **`GET /ash/admin/yunwu/health`**: Retrieve isolated Yunwu pool health.
* **`GET /ash/admin/yunwu/nodes`**: List Yunwu balance and runtime state.
* **`POST /ash/admin/yunwu/nodes/{node_id}/sync`**: Refresh a Yunwu node balance.

## Provider setup

Create active upstream accounts in the existing admin account UI:

* Claude: `platform=anthropic`, with `access_token` or `api_key` and optional `base_url`.
* OpenAI: `platform=openai`, with `api_key` and optional OpenAI-compatible `base_url`.
* Yunwu: `platform=yunwu`, with `api_key` and `base_url`. Yunwu accounts are scheduled by the isolated Yunwu engine and are not mixed into the standard pool.
  Account `extra.yunwu_protocol` may be `openai`, `responses`, `anthropic`, or `gemini`; when omitted, Gemini/Imagen/Veo/Lyria/Chirp and OpenAI model families use the OpenAI-compatible transport while Claude models use Anthropic Messages.

Associate service API keys with a group for the intended platform. Yunwu groups can serve both Anthropic-compatible `/v1/messages` and OpenAI-compatible `/v1/chat/completions` or `/v1/responses`, depending on the requested model/payload.
Yunwu groups can also call native Gemini `generateContent` and `streamGenerateContent` through `/v1beta/models/{model}:...`. Native request parts such as `inlineData`, `fileData`, images, audio, and video are forwarded without schema conversion, and upstream media response headers are preserved.

Media-compatible Yunwu routes currently include:

- `POST /v1/images/generations` and `/v1/images/edits`;
- `POST /v1/audio/speech`, `/v1/audio/transcriptions`, and `/v1/audio/translations`;
- `POST|GET|DELETE /v1/videos...` plus `POST /v1/video/generations`;
- native Gemini `generateContent`, `streamGenerateContent`, and `predictLongRunning` model actions;
- `GET|POST|DELETE /v1beta/operations/...` for long-running Veo/media jobs;
- Gemini Files API under `/v1beta/files...` and `/upload/v1beta/files`.
- Gemini Live bidirectional WebSocket under `/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent` (the `v1alpha` alias is also accepted), including realtime text, PCM audio, images, and video frames.
- Lyria RealTime Live Music under `/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateMusic`, including prompt/control messages and streamed audio chunks.
- the current Gemini Interactions API under `/v1beta/interactions...` and `/v1/interactions...`, including multimodal, stored/background, cancellation, retrieval, deletion, and SSE interactions.
- browser-safe ephemeral Live tokens through `POST /v1beta/auth_tokens` and `BidiGenerateContentConstrained`; vendor tokens are replaced with opaque, short-lived gateway tokens and pinned to their issuing Yunwu node.
- native batch, cached content, generated file, File Search, corpus, tuned-model, and dynamic resource endpoints with stateful Yunwu node affinity.

The exact model and endpoint availability is determined by the configured Yunwu vendor. Unsupported upstream operations are returned as vendor errors; the gateway does not fabricate media capabilities.

Media generation is reserved and billed in micro-USD before the upstream call. Configure account `extra.yunwu_media_prices_micro_usd` as a map of prices per produced unit, for example `{"imagen-3": 40000, "images": 50000, "audio": 20000, "video": 120000, "default": 50000}`. An exact model key wins, then the media kind, then `default`. Image `n` and video `duration` multiply the unit price. File upload/list/get/delete and operation polling/cancellation are control-plane calls: they still require a fresh allocatable Yunwu balance but do not create another user charge. If no media price is configured, the Yunwu fallback price is used and the fallback-pricing metric is incremented.

Gemini Live uses the `live_audio_minute` media price and Lyria RealTime uses `live_music_minute`; both bill started sessions by rounded-up elapsed minute. `extra.yunwu_live_max_minutes` bounds a session to 1–120 minutes (default 15); that full bound is reserved atomically before the WebSocket is connected and unused minutes are refunded when it closes. Live sessions use the same active/schedulable/healthy/fresh-balance filters, per-user concurrency, monetary quota, and isolated Yunwu pool as HTTP media requests.
Live upstream authentication defaults to a Bearer header, as used by Yunwu-compatible gateways. Set account `extra.yunwu_live_auth="query"` only for vendors that implement Google's native WebSocket `?key=` authentication; dial errors are sanitized so this credential cannot enter logs or client responses.

Service API keys are returned in cleartext only by the create-key response. The database stores a non-reversible SHA-256 lookup token; save the newly created key immediately because it cannot be recovered later.

Sensitive upstream account credential fields (API keys, OAuth tokens, client secrets, passwords, cookies, and session keys) are stored as versioned AES-256-GCM envelopes using `totp.encryption_key`. Routing metadata such as `base_url` and `chatgpt_account_id` remains queryable. Existing plaintext account rows are encrypted during startup before request handlers are served, with online migration retained for rolling replicas; keep the encryption key stable across restarts and replicas.

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

   # Test OpenAI Responses format (also supported by Yunwu groups):
   curl -X POST http://localhost:8080/v1/responses \
     -H "Authorization: Bearer <your_user_api_key>" \
     -H "Content-Type: application/json" \
     -d '{"model": "gpt-4o", "input": "Hello!", "stream": true}'
   ```
4. Query node statistics:
   ```bash
   curl -X GET http://localhost:8080/api/v1/ash/health
   ```

---

## Known Limitations (MVP)

1. **Warmup Priority**: During recovery warmup mode, new requests are funneled gradually, but the queue manager doesn't prioritize basic vs. priority queue scheduling outside of survival/degradation modes.
2. **Concurrent Rebinding**: Rebinding is serialized per session with a Redis lock. A concurrent loser receives a lock-conflict result and can resolve the lease written by the winner on its next request.
3. **Upstream Response Validation**: The mock runtime handles malformed stream simulation, but in production, raw stream parsing relies on client compatibility (e.g. Claude Code or VS Code extension parsing the SSE stream).
