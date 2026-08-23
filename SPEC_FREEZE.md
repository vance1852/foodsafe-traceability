# Foundation Specification Freeze

| Area | Frozen decision |
| --- | --- |
| Batch | `food-safety-20260823-01`, target 30, backend |
| Repository | `https://github.com/vance1852/foodsafe-traceability`, public, main baseline |
| Business boundary | Food facilities, production zones, inspection stations, sampling and custody, laboratory review, operating controls, contamination incidents, corrective actions, telemetry and audit |
| Persistence | Real SQLite relational database through `modernc.org/sqlite`; production paths execute SQL and never replace persistence with maps |
| Migration | Embedded ordered migrations with schema history, foreign keys, unique/check constraints, indexes, conflict rejection and repeatable restart behavior |
| Tables | Organizations, users, sessions, food facilities, production zones, inspection stations, plans, samples, custody, results, incidents, actions, audit, outbox, idempotency and worker jobs |
| Transactions | Cross-entity state, audit and outbox changes commit or roll back together; mid-flight failure and commit behavior are tested |
| State machines | Plans, samples, custody, laboratory results, permits, incidents and corrective actions reject illegal transitions |
| Concurrency | Optimistic versions, conditional SQL updates, unique allocations, capacity rules, deterministic concurrency tests and race-detector verification |
| Context | HTTP, service, repository and worker boundaries propagate cancellation and deadlines; wrapped errors preserve causes |
| Worker | Durable alert/outbox work supports leases, bounded retry, permanent failure, restart recovery, cancellation and graceful shutdown |
| HTTP | JSON API, stable error codes, request IDs, recovery middleware, structured logging, pagination, health and database-backed readiness |
| Identity | Opaque expiring server-side sessions, logout revocation, authentication-generation fencing and expired-token rejection |
| Roles | Field operator, laboratory analyst and protection supervisor have distinct service and HTTP permissions |
| Docker | Go 1.22 builder, real dependency files, `WORKDIR /src`, build `./cmd/server`, entrypoint `/app/foodsafe-traceability` |
| Tests | Domain, service, SQL integration, HTTP, authentication, rollback, concurrency, restart, worker, pagination and time-boundary coverage |
| Scale | Minimum 5000 production physical lines, 30 production Go files, 10 production packages and 1500 test physical lines |
| Exclusions | No benchmark sources, demos, generic CRUD, medical diagnosis, weapons, surveillance, gambling, social media, seeded bugs, gold answers or private task material |
| Candidate capacity | The 30 independent runtime boundaries below are capabilities only, without concrete bugs or task designs |

## Runtime Boundary Capacity

1. Login credential verification and failure accounting.
2. Session creation with expiry and digest persistence.
3. Logout revocation and authentication-generation fencing.
4. Role authorization across operator, analyst and supervisor actions.
5. Food-facility registration with audit atomicity.
6. Production-zone ownership and active-facility validation.
7. Inspection-station registration and cross-entity consistency.
8. Sampling-plan publication state transitions.
9. Sampling window and business-timezone enforcement.
10. Operator assignment eligibility and ownership.
11. Per-station daily sample sequence allocation.
12. Sample creation plus first custody event transaction.
13. Custody handoff ordering and receiver validation.
14. Laboratory receipt and chain-of-custody closure.
15. Laboratory result limits and review transitions.
16. Independent review segregation of duties.
17. Food-quality exceedance to incident creation.
18. Operating-permit activation blocked by unresolved findings.
19. Permit capacity and validity-window checks.
20. Shipment release recording under active permits.
21. Contamination-incident command claim concurrency.
22. Containment assignment transaction and ownership.
23. Corrective-action plan approval and action lifecycle.
24. Idempotent command replay and payload conflicts.
25. Optimistic version conflicts and conditional updates.
26. Telemetry deduplication and threshold alert creation.
27. Durable worker lease, retry and permanent failure.
28. Worker cancellation, graceful shutdown and lease recovery.
29. Pagination, filtering and stable ordering.
30. Audit/outbox atomicity, restart recovery and readiness probing.
