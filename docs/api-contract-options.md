# API contract publication: audit options

The REST implementation lives in `pkg/adapters/api/`. `docs/api-reference.md`
describes routes and examples, and `docs/openapi.yaml` publishes the current
machine-readable contract. There is no backend response contract test today.
Frontend response validation is local to the frontend.

Options considered for publication:

1. **Hand-maintained OpenAPI plus response contract tests.** Smallest change,
   but every endpoint edit must update both the specification and tests.
2. **Code-first OpenAPI generation.** Reduces duplicated route/schema text,
   but adds annotations or a generator dependency and can obscure the current
   plain `net/http` handlers.
3. **Schema-first generation.** Strongest shared contract for clients, but a
   larger migration that would reshape handler types and build steps.

The repository uses the hand-maintained specification from option 1. The
response contract test remains a separate follow-up; this documentation change
does not add one or define a compatibility policy.
