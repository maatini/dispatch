# msgraph: Gotchas

## Circuit Breaker: Permanent Errors Don't Count

The circuit breaker's `IsSuccessful` function treats `GraphPermanentError` as success — meaning 4xx errors (except 429) do NOT count toward the 5-consecutive-failure threshold that opens the breaker. Only transient errors (5xx, 429, IO) contribute to the failure count.

This is correct behavior: a 400 Bad Request isn't a sign that MS Graph is down; it's a sign that the request was wrong.

## Token Cache Has 60s Expiry Buffer

`tokenCache.get()` checks `time.Now().Add(60 * time.Second).After(expiresAt)` — tokens are refreshed 60 seconds before actual expiry. This prevents edge-case failures where a token expires between the check and the HTTP request.

## Dev Proxy Mode Disables TLS Verification

When `MS_GRAPH_PROXY_URL` is set, `buildTransport()` creates a transport with `InsecureSkipVerify: true`. This is marked with `//nolint:gosec`. This should NEVER be enabled in production — it's solely for the local MS Graph Developer Proxy.

## Mock Token Makes Graph Credentials Optional

When `MS_GRAPH_MOCK_TOKEN` is set, the `Client` skips OAuth2 entirely and uses the mock token directly. This also makes `MS_GRAPH_TENANT_ID`, `MS_GRAPH_CLIENT_ID`, and `MS_GRAPH_CLIENT_SECRET` optional (validated in `config.Load()`).

## Inline vs Upload Session Threshold: 3 MB Total

The threshold is based on **total** attachment size, not per-attachment. Even if individual attachments are small, if the combined size exceeds 3 MB, the upload session path is used. Within the upload session path, individual attachments under 3 MB use `POST .../attachments` (small), and those over 3 MB use `createUploadSession` (large).

## Draft Cleanup on Error

When `sendViaUploadSession` fails during attachment upload, the `cleanup()` function sends a DELETE to remove the draft. The cleanup uses `context.Background()` (not the request context) because the request context may already be cancelled. This is a deliberate best-effort cleanup — if it fails, the draft remains in the mailbox.

## Chunked Upload Uses PUT with Content-Range

`uploadChunks()` iterates over the content in 1.25 MB chunks (`chunkSize = 4 * 327_680`). Each chunk is PUT with a `Content-Range` header like `bytes 0-1310719/5242880`. If any chunk fails, the entire upload fails with a `GraphTransientError`.

## Upload Chunk Errors Are Transient

All upload chunk errors (HTTP ≥ 400 or network errors) are wrapped as `GraphTransientError`. This means a chunk failure during a large attachment upload will cause the worker to not ACK, and the entire send will be retried from scratch.
