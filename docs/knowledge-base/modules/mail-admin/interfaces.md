# mail-admin: Interfaces

## GraphQL Schema (Inferred from Resolver)

### Queries

```graphql
type Query {
    senders(filter: SenderFilter): [Sender!]!
    mails(filter: MailFilter, page: Int, size: Int): PagedMailResponse!
    bounces(page: Int, size: Int): PagedBounceResponse!
    deadLetters(page: Int, size: Int): PagedDeadLetterResponse!
}
```

### Mutations

```graphql
type Mutation {
    createSender(input: SenderInput!): Sender!
    updateSender(appTag: String!, input: SenderInput!): Sender!
    deleteSender(appTag: String!): Boolean!
    reprocessDeadLetter(payload: String!): Boolean!
}
```

### Types

```graphql
type Sender {
    appTag: String!
    email: String!
    test: Boolean!
    dailyQuota: Int!
    allowedDomains: String
}

input SenderInput {
    appTag: String!
    email: String!
    test: Boolean!
    dailyQuota: Int!
    allowedDomains: String
}

input SenderFilter {
    appTag: String
}

type MailRecord {
    traceId: String!
    appTag: String!
    status: String!
    sender: String!
    subject: String
    recipients: [String!]!
    error: String
    timestamp: String!
}

input MailFilter {
    appTag: String
    status: String
    traceId: String
}

type PagedMailResponse {
    items: [MailRecord!]!
    total: Int!
}

type BounceRecord {
    originalTraceId: String!
    bouncedAt: String!
    bounceReason: String!
    bouncedRecipient: String!
    processedAt: String!
}

type PagedBounceResponse {
    items: [BounceRecord!]!
    total: Int!
}

type DeadLetter {
    payload: String!
    error: String!
    timestamp: String!
}

type PagedDeadLetterResponse {
    items: [DeadLetter!]!
    total: Int!
}
```

## JWT Auth Contract

**Header:** `Authorization: Bearer <token>`

**Token spec:**
- Algorithm: HMAC-SHA256
- Signing key: value of `DISPATCH_ADMIN_AUTH_SECRET` env var
- Required claim: `exp` (expiration time)
- Token generated via `tools/gen-admin-token/` CLI

**Error response (401):**
```json
{"status": 401, "code": "UNAUTHORIZED", "message": "invalid or missing token"}
```

## Health Endpoint

`GET /health` — not behind auth middleware.
```json
{"status": "UP"}
```
