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

## Medium-term

| # | Änderung | Impact | Effort | Warum |
|---|----------|--------|--------|-------|
| 12 | Prometheus-Metriken + `traceContext` in Logs/Audit/Graph | High | Medium | Delivery-Pipeline ohne Queue-/Latenz-Metriken |
| 14b | Per-Tenant AuthZ am Send (Token/JWT an `appTag`); Admin per-Tenant-Scopes | Medium | Medium | P0 = Cluster-Token (AuthN); Tenant-Spoofing mit gestohlenem Token bleibt |
| 15 | Quota: Per-Minute-Buckets statt Per-Request-Einträge | Medium | Medium | 1MB KV-Value-Limit bei Top-Tenants |
| 16 | Sender-Cache via KV Watch; KV-Config-Drift reconcilieren (oder dokumentieren) | Medium | Medium | 10-min Staleness über Gateway-Replicas |

## Strategic

| # | Änderung | Impact | Effort |
|---|----------|--------|--------|
| 17 | Admin-Queries: indexiert / paging statt Full-Stream-Scan | High | High |
| 18 | Worker-Concurrency mit Per-Sender-Ordering | Medium | High |
| 19 | Deploy-Manifeste (Kustomize/Helm) + Version aus Git-Tag | Medium | Medium |
| 20 | `graph-gophers/graphql-go` neu bewerten wenn Admin-API wächst | Low | High |

## Priorität (Empfehlung)

1. **#12** Metriken — Operability  
2. **#15** Quota-Buckets — Scale-Fail-Closed vermeiden  
3. **#14b** per-Tenant AuthZ — wenn Cluster-Token geteilt wird  
4. **#17–#19** wenn Volumen/Deploy schmerzhaft wird  
