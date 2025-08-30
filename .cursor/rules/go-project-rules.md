# Go Project Rules for AI-assisted Code Generation

The following rules consolidate widely adopted Go practices from Effective Go, Go Code Review Comments, Uber Go Style Guide, and community experience. They are intended to guide both human and AI-generated changes.

Section: Error handling

- Functions that can fail must return an error as the last return value.
- Do not use panics for expected failures; reserve panics for programmer bugs and irrecoverable process conditions.
- Add context when returning errors using standard library error wrapping; include actionable information without leaking sensitive details.
- Compare and classify errors using standard library helpers rather than string matching; define sentinel errors only for stable, domain-level cases.
- Handle errors once at the appropriate boundary; avoid logging and returning the same error twice.
- Never ignore returned errors; if intentionally ignored, document why.
- Keep error messages lower-case without trailing punctuation; prefer concise wording.

Section: Dependencies and modules

- Use Go modules; keep [go.mod](./go.mod) and go.sum tidy and committed.
- Prefer the standard library; add third-party libraries only with clear justification and due diligence on maintenance, license, and security.
- Pin versions; schedule periodic dependency updates and security reviews.
- Avoid using replace directives in production except for vetted forks; document any replacements.
- For private modules, configure the appropriate environment variables and proxies; do not commit secrets.
- Track developer tool dependencies via a dedicated tools file committed to the repo, for example [tools.go](./tools.go) with build constraints.

Section: Concurrency

- Accept a context as the first parameter on operations that may block, perform I/O, time out, or be canceled.
- Tie the lifetime of background tasks to a context to prevent leaks; ensure all background work terminates on shutdown.
- Prefer message passing or clear ownership to avoid shared mutable state; if synchronization is needed, use safe primitives consistently.
- Keep channel and synchronization usage simple; avoid intricate patterns that are hard to reason about.
- Use timeouts and cancellation for external calls and long-running work.

Section: Project layout

- Organize binaries under [cmd/](./cmd), one subdirectory per executable.
- Place application packages under [internal/](./internal) to prevent unintended external use.
- Use [pkg/](./pkg) only if you intentionally want to allow import by external projects; prefer internal when in doubt.
- Keep packages focused and cohesive; avoid cyclic dependencies and deep nesting.
- Provide a top-level [README.md](./README.md) describing build, run, test, configuration, and operational notes.

Section: API and package design

- Keep interfaces small and focused; accept interfaces and return concrete types.
- Avoid stutter in names; keep package and type names concise and informative.
- Prefer constructors that return fully initialized values; make zero values useful where possible.
- Do not expose global state; pass dependencies explicitly through parameters or lightweight initializers.

Section: Testing and quality

- Write table-driven tests; include negative cases and edge conditions.
- Use subtests for structure and readability; keep tests deterministic and fast by default.
- Include examples that double as documentation where helpful.
- Run static analysis, vetting, formatting, and tests in continuous integration; enforce the race detector in CI for relevant packages.
- Keep test fixtures under a dedicated directory, such as testdata, and avoid network or time dependencies without isolation.

Section: Observability and logging

- Prefer structured logging; define a single logging abstraction at the application boundary and inject it where needed.
- Avoid logging sensitive data; redact or hash where appropriate.
- Include correlation identifiers in logs for distributed workflows when available.
- Expose metrics and tracing hooks where appropriate for services; document how to enable or scrape them.

Section: Configuration and CLI

- Support configuration via command-line flags and environment variables with clear precedence; provide sane defaults.
- Store secrets only in external secret managers or environment variables; never commit secrets to the repository.
- Validate configuration at startup and fail fast with a clear message when invalid.

Section: Security

- Validate and sanitize all external inputs; use safe defaults for file permissions and network access.
- Prefer cryptographically secure randomness for secrets and tokens.
- Keep dependencies updated and monitor advisories; remediate promptly.
- Minimize the attack surface: least privilege for processes, credentials, and network exposure.

Section: Performance and reliability

- Measure before optimizing; profile with standard tooling and capture representative workloads.
- Prefer allocation-free hot paths only when profiling justifies the change; favor clarity first.
- Use backoff and retry with jitter for transient failures when interacting with external systems.
- Implement graceful shutdown with time-bounded drains for in-flight work.

Section: Style and documentation

- Enforce formatting and imports using standard tools in CI.
- Follow community naming conventions; write clear doc comments for exported packages, types, and functions.
- Keep comments accurate and up to date; avoid redundant or restated comments.

Section: Tooling and CI

- Automate linting, formatting, vetting, unit tests, and security checks in CI.
- Keep reproducible builds; document required versions of the toolchain.
- Ensure go mod tidy runs in CI to keep module files consistent.
- Provide developer workflows via scripts or a [Makefile](./Makefile) or Taskfile; keep commands idempotent.

Section: Policies for AI-assisted changes

- No placeholders or incomplete scaffolding; generated code must compile and include basic tests where applicable.
- Prefer the standard library and established, well-maintained dependencies; explain any new dependency in the pull request description.
- Include rationale and references to these rules in pull requests made by AI agents.
- Do not introduce or copy secrets, keys, or proprietary content; sanitize prompts and training examples.
- Keep prompt templates, specifications, and generated artifacts under version control separate from hand-written code where useful.
- Require a human review gate for any change that touches security, cryptography, authentication, authorization, or data retention.

Operational checklist

- Format, vet, and lint the codebase; ensure module files are tidy.
- Run unit tests locally; run with the race detector where it adds value.
- Verify dependency updates and license compliance.
- Validate configuration and environment; verify graceful shutdown paths.
- Update documentation and changelog entries when behavior changes.

Notes

- These rules apply across services and libraries in this repository.
- When a rule must be bent for good reason, document the exception at the point of change and in the pull request.

References

- Effective Go: https://go.dev/doc/effective_go
- Go Code Review Comments: https://github.com/golang/go/wiki/CodeReviewComments
- Uber Go Style Guide: https://github.com/uber-go/guide
- Go Modules Reference: https://go.dev/ref/mod
- Go Memory Model: https://go.dev/ref/mem
- Go Security: https://go.dev/security
- Error handling in Go: https://go.dev/blog/error-handling-and-go
- Context article: https://go.dev/blog/context
- Table-driven tests reference: https://dave.cheney.net/2019/05/07/prefer-table-driven-tests
- Concurrency patterns talk: https://go.dev/talks/2012/concurrency.slide
