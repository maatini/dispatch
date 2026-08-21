# Offene Verbesserungen — dispatch

Lebende Backlog-Liste. Erledigte Audits/Pläne (`plan07`, `tests07`, P0 Auth/JWT/delivered-Put)
sind in `docs/ai-changes.md` dokumentiert — hier nur **noch offene** Themen.

## Umgesetzt (Referenz, nicht erneut anfassen)

| Thema | Nachweis |
|-------|----------|
| Circuit-Breaker → transient, Token 4xx permanent, Bounce-Header, DL-Reprocess-Headers, Spam atomic Create, Readiness real, Integrationstests | `docs/ai-changes.md` 2026-07-17 |
| Dedup Get fail-closed, uploadChunks 4xx permanent, Coverage-Hebel | `docs/ai-changes.md` 2026-07-17 tests07 |
| Gateway Bearer-Auth, JWT `exp` required, delivered-Put fail-closed vor ACK | `docs/ai-changes.md` 2026-07-18 P0 |
| Worker AckWait 5m / MaxDeliver 8 / InProgress heartbeat + DLQ on exhaustion | `docs/ai-changes.md` 2026-07-18 #13 |
| Fetch(1) + fetch-Backoff; Quota CAS-Pause; KV-TTL reconcile; Sender-Cache 30s | `docs/ai-changes.md` 2026-08-21 |
| Prometheus `/metrics` + `traceContext` in Logs/Audit/Graph (W3C-Allowlist) | `docs/ai-changes.md` 2026-08-21 |

## Medium-term

| # | Änderung | Impact | Effort | Warum |
|---|----------|--------|--------|-------|
| 14b | Per-Tenant AuthZ am Send (Token/JWT an `appTag`); Admin per-Tenant-Scopes | Medium | Medium | P0 = Cluster-Token (AuthN); Tenant-Spoofing mit gestohlenem Token bleibt |
| 15 | Quota: Per-Minute-Buckets statt Per-Request-Einträge | Medium | Medium | 1MB KV-Value-Limit bei Top-Tenants |
| 16 | Sender-Cache via KV Watch (optional) | Low | Medium | TTL jetzt 30s; Watch nur wenn sofortige Invalidierung nötig ist |
| 21 | Bounce: Graph `@odata.nextLink` (ggf. `$top`) | Medium | Small | Eine GET-Seite (~10); Backlog leert sich nur 10/15 min |
| 22 | Object-Store TTL reconcile (wie KV) | Low | Small | `ProvisionObjectStore` ist create-only; TTL-Drift nach Erst-Provision |

## Strategic

| # | Änderung | Impact | Effort | Warum |
|---|----------|--------|--------|-------|
| 17 | Admin-Queries: indexiert / paging statt Full-Stream-Scan | High | High | `readStream` DeliverAll + In-Memory-Page |
| 18 | Worker-Concurrency mit Per-Sender-Ordering | Medium | High | Fetch(1) = 1 Graph-Send/Prozess; Throughput nur über Replicas |
| 19 | Deploy-Manifeste (Kustomize/Helm) + Version aus Git-Tag | Medium | Medium | Images ja, Cluster-Manifeste nein; `GOARCH=amd64` hardcoded |
| 20 | `graph-gophers/graphql-go` neu bewerten wenn Admin-API wächst | Low | High | |

## Priorität (Empfehlung)

1. **#15** Quota-Buckets — Scale-Fail-Closed vermeiden  
2. **#21** Bounce-Pagination — NDR-Backlog sonst stundenlang  
3. **#14b** per-Tenant AuthZ — wenn Cluster-Token geteilt wird  
4. **#22** Object-Store TTL — Parität zu KV-Reconcile  
5. **#17–#19** wenn Volumen/Deploy schmerzhaft wird  
