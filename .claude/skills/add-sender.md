---
name: add-sender
description: Generate a GraphQL createSender mutation for the dispatch admin API
---

Generate a `createSender` mutation for the dispatch mail-admin GraphQL API.

If args were provided (e.g. `/add-sender sunshine-app noreply@example.com 500 example.com,partner.de`), use them directly in order: appTag, email, dailyQuota, allowedDomains.

Otherwise, ask the user for:
1. **appTag** — tenant identifier, e.g. `sunshine-app`
2. **email** — technical sender address, e.g. `noreply@example.com`
3. **dailyQuota** — max recipients per 24h window (0 = unlimited)
4. **allowedDomains** — comma-separated recipient domains, e.g. `example.com,partner.de`
5. **test** — true/false, whether this sender runs in test mode (no actual Graph call)

Then output the ready-to-paste GraphQL mutation:

```graphql
mutation {
  createSender(input: {
    appTag: "<appTag>"
    email: "<email>"
    dailyQuota: <dailyQuota>
    allowedDomains: "<allowedDomains>"
    test: <test>
  }) {
    appTag
    email
    dailyQuota
    allowedDomains
    test
  }
}
```

And the curl command to run it against the local admin API (port 8080):

```bash
curl -s -X POST http://localhost:8080/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"mutation { createSender(input: { appTag: \"<appTag>\" email: \"<email>\" dailyQuota: <dailyQuota> allowedDomains: \"<allowedDomains>\" test: <test> }) { appTag email dailyQuota } }"}'
```

Do not execute the curl — only print it.
