---
name: audit
description: Generate a GraphQL mails query for the dispatch audit log
---

Generate a `mails` query for the dispatch mail-admin GraphQL API.

Parse args if provided. Supported formats:
- `/audit <appTag>` — filter by appTag, all statuses
- `/audit <appTag> <status>` — filter by appTag and status (DELIVERED | FAILED | TEST_SUCCESS)
- `/audit <appTag> <status> <page>` — additionally set page (0-based)

If no args, ask the user for:
1. **appTag** — leave blank for all tenants
2. **status** — DELIVERED, FAILED, TEST_SUCCESS, or blank for all
3. **page** — default 0
4. **size** — default 20

Output the ready-to-paste GraphQL query:

```graphql
query {
  mails(
    filter: { appTag: "<appTag>", status: "<status>" }
    page: <page>
    size: <size>
  ) {
    total
    items {
      traceId
      appTag
      status
      timestamp
      recipients
      error
    }
  }
}
```

Omit `appTag` from the filter object if blank. Omit `status` from the filter object if blank.

And the curl command against the local admin API (port 8080):

```bash
curl -s -X POST http://localhost:8080/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ mails(filter: { <filter> }, page: <page>, size: <size>) { total items { traceId appTag status timestamp recipients error } } }"}'
```

Do not execute the curl — only print it.
