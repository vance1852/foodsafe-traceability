# FoodSafe Traceability

FoodSafe Traceability is a production-oriented Go backend for food facility oversight, lot sampling, laboratory review, operating controls, contamination response, corrective actions, audit history, and durable background delivery.

The primary workflow is `facility -> production zone -> inspection station -> sampling plan -> chain of custody -> laboratory review -> release or corrective action`. A second path ingests temperature and quality telemetry, creates alert work, retries bounded failures, and records state-changing outcomes in an audit chain.

## Runtime

- Go 1.22 with `GOTOOLCHAIN=local`.
- SQLite through the pure-Go `modernc.org/sqlite` driver.
- `cmd/server` serves HTTP and runs alert/outbox workers under one cancellation lifecycle.
- The default address is `:8080`; the default database is `foodsafe.db`.
- `GET /healthz` reports liveness. `GET /readyz` checks the database and foreign-key enforcement.

Configure an initial supervisor with `FOODSAFE_BOOTSTRAP_ORG_ID`, `FOODSAFE_BOOTSTRAP_ORG_NAME`, `FOODSAFE_BOOTSTRAP_EMAIL`, and `FOODSAFE_BOOTSTRAP_PASSWORD`. Bootstrap is idempotent and stores a bcrypt password hash.

```bash
GOTOOLCHAIN=local go run ./cmd/server
```

## Identity and authorization

Clients log in through `POST /v1/auth/login` and receive an opaque bearer token whose SHA-256 digest is persisted. Sessions expire, can be revoked by logout, and are fenced by an authentication generation so deactivated accounts cannot reuse old tokens.

- `field_operator` collects food samples, transfers custody, reports contamination, and completes assigned corrective actions.
- `lab_analyst` receives custody, records results, and submits results for independent review.
- `safety_supervisor` registers food facilities, publishes plans, reviews results, controls operating permits, leads incidents, and approves corrective actions.

Authentication, expiry, revocation, role denial, persistence, and HTTP error mapping are covered by service, database, and API tests.

## Persistence and workflows

Versioned migrations create organizations, users, sessions, food facilities, production zones, inspection stations, sampling plans, sample sequences, samples, custody events, laboratory results, permits, shipment release events, contamination incidents, containment assignments, corrective-action plans, telemetry readings, alert jobs, audit events, outbox events, and idempotency records.

Foreign keys, unique and check constraints, optimistic versions, business indexes, and time fields preserve invariants. State changes and audit/outbox records commit in the same transaction. Conditional updates protect capacity and ownership under concurrency. Restart tests reopen the database and recover persisted workflow and worker state.

Workers lease due jobs, propagate cancellation, retry transient failures with bounds, record permanent failures, and stop gracefully. HTTP requests carry context and request IDs through service and repository layers, and errors retain causal chains while returning stable public codes.

## Verification

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
docker build -t foodsafe-traceability:local .
```

The root Dockerfile builds the real `./cmd/server` entrypoint and starts it with a persistent `/data` volume. No external online service is required for tests.

`SPEC_FREEZE.md` records the FOUNDATION contract and 30 independent runtime boundaries reserved for later candidate design. This baseline contains no seeded defect, task branch, private test, prompt, gold answer, or intake record.
