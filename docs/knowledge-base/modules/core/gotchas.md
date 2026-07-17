# core: Gotchas

## MailRequest vs MailRequestDO: Different Attachment Models

`MailRequest` carries attachments as base64-encoded strings (`Attachment.Content`). `MailRequestDO` carries attachments as Object Store keys (`AttachmentDO.ObjectKey`) in the gateway context, or raw bytes (`AttachmentDO.Content`) in the worker context. The `Content` field has `json:"-"` — it's never serialized to NATS.

The transformation happens in `gateway.handler.handleSend()` (base64 → Object Store) and `worker.processor.Handle()` (Object Store → []byte).

## DailyQuota: Zero Means Unlimited

In `domain.Sender`, `DailyQuota` of 0 (or negative) means unlimited. The check is in `quota.Checker.Check()`: `if limit <= 0 { return nil }`.

Current default for `SenderInput` in admin: `int32` zero value = 0 = unlimited. Be careful not to accidentally set `dailyQuota: 0` when you mean a real limit.

## AllowedDomains: Empty Means All Allowed

An empty `AllowedDomains` string means all recipient domains are permitted. The gateway's `checkDomains()` function returns nil immediately when `sender.AllowedDomains == ""`.

## ApiError Is Not Used as a Go Error

`ApiError` does NOT implement the `error` interface. It's a JSON serialization struct for HTTP responses. The actual Go errors are `ValidationError`, `QuotaError`, `QuotaStateError`, and `NatsPublishError`.

## QuotaStateError and NatsPublishError Wrap Causes

Both `QuotaStateError` and `NatsPublishError` implement `Unwrap()` — use `errors.Is()` or `errors.As()` to inspect the underlying cause. The gateway handler checks for specific error types via `errors.As()`.

## Config: Mock Token Makes Graph Credentials Optional

When `MS_GRAPH_MOCK_TOKEN` is set, `config.Load()` skips validation of `MS_GRAPH_TENANT_ID`, `MS_GRAPH_CLIENT_ID`, `MS_GRAPH_CLIENT_SECRET`, and `MS_GRAPH_SENDER_EMAIL`. This is only for local dev.

## Validator Tags Are Imported But Not Called Directly

`MailRequest` uses `validate` struct tags from `go-playground/validator`, but the actual validation call (`validate.Struct(req)`) is in `gateway/validation.go`. The domain package itself doesn't validate — it just defines the contract.
