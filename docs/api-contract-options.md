# API contract publication: audit options

The REST implementation lives in `pkg/adapters/api/`; `docs/api-reference.md`
describes routes and examples. There is no published OpenAPI document or
backend contract test today. Frontend response validation is local to the
frontend and does not publish the server contract.

Options for a separate follow-up:

1. **Hand-maintained OpenAPI plus response contract tests.** Smallest change,
   but every endpoint edit must update both the specification and tests.
2. **Code-first OpenAPI generation.** Reduces duplicated route/schema text,
   but adds annotations or a generator dependency and can obscure the current
   plain `net/http` handlers.
3. **Schema-first generation.** Strongest shared contract for clients, but a
   larger migration that would reshape handler types and build steps.

Prefer option 1 for this repository's current API size. Test the published
document against representative handler responses and route/parameter
coverage, then add it to CI. This item is a finding and decision record only;
the API specification and contract test are not part of the current audit fix.
