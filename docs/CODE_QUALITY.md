# Code quality

Install frontend development dependencies once with `npm --prefix frontend ci`.
Go downloads its pinned tools on first use. Use Go 1.26.3 or newer, Node 24 (as in
Docker/CI), Bash and make. Node 22.17.1 also works for the current tools.

```sh
make fmt          # Format Go imports/code and frontend JS, JSX, JSON and CSS
make lint         # Go vet + Staticcheck + ESLint; warnings fail the check
make fmt-check    # Check formatting without modifying files
make check        # Lint, formatting checks and Go race tests
```

The individual `lint-go`, `lint-frontend`, `fmt-go`, `fmt-frontend`,
`fmt-check-go` and `fmt-check-frontend` targets are available for smaller changes.
For safe frontend lint fixes, run `npm --prefix frontend run lint:fix`, then
`make fmt-frontend`. Review fixes as ordinary code changes.

Go uses goimports (which includes gofmt) to separate standard-library,
third-party and project imports. Staticcheck catches correctness problems,
unused code and idiomatic simplifications. Both tools are tracked in `go.mod`
and `go.sum` and invoked with `go tool`; no global installation is required.

Frontend ESLint uses Next.js Core Web Vitals rules, React Hooks checks and
explicit equality/constant-declaration rules. Prettier owns formatting;
eslint-config-prettier prevents conflicting style rules. Tool versions and
transitive dependencies are pinned through `package.json` and its lockfile.
Generated output, dependencies and browser artifacts are excluded, while source,
configuration and browser tests are checked. `.editorconfig` provides consistent
indentation, UTF-8, line endings and final newlines for compatible editors.

## Validation beyond linting

```sh
# Dedicated test database required; tests create and drop their own schemas.
TEST_DATABASE_URL='postgres://postgres:YOUR_TEST_PASSWORD@localhost:55432/hosting_test?sslmode=disable' make integration
go build ./cmd/...
npm --prefix frontend run build
npm --prefix frontend test
```

Go database integration tests skip when `TEST_DATABASE_URL` is absent, so a plain
`make check` does not prove database behavior. Browser tests require a production
build and Chromium (`cd frontend && npx playwright install chromium`).

`.github/workflows/quality.yml` runs on pushes and pull requests once this project
is hosted on GitHub. Separate jobs enforce backend lint/format/build/race tests
with PostgreSQL, and frontend lint/format/build/browser tests. The workflow uses
only a disposable database password and needs no Panel, payment or production secrets.
Linting is a repeatable baseline; business invariants still need integration tests
and code review. Go targets explicitly cover `cmd`, `internal` and `migrations`
so Go snippets shipped inside frontend dependencies are not linted or formatted.

`make test-compose` uses Python 3 and Docker Compose to check deployment credential
separation, host mounts and pinned Panel source without starting any containers.
CI runs it with the backend checks.

The Next.js 16.4 lint plugins currently declare compatibility with ESLint 9,
so this setup pins ESLint 9 rather than forcing incompatible ESLint 10 peers.
The upstream `braces` dependency in the Next.js lint plugin has an unresolved
[stack-exhaustion advisory](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm).
`npm audit` reports five related development dependency entries; there is no
patched `braces` release available at setup time. Runtime dependencies pass
`npm audit --omit=dev`. Update the lint toolchain when compatible fixes become
available; do not use `npm audit fix --force` to downgrade the Next.js config.

Tool references: [Staticcheck](https://staticcheck.dev/docs/),
[Next.js ESLint configuration](https://nextjs.org/docs/app/api-reference/config/eslint).
