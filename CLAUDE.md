# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go library (`github.com/mdgreenwald/lib-flume-water`) for the Flume Water API. Provides authentication, device management, location retrieval, and water usage querying for Flume smart water monitoring systems.

**Key characteristics:**
- Library, not an application — no binaries are built
- Single-file implementation: all code in `lib-flume-water.go`, all tests in `lib-flume-water_test.go`
- File is organized with all types/structs/constants at the top, all functions below
- Uses JWT tokens for authentication; all API responses use a standard envelope format

## Development Commands

### Mise Tasks (preferred)
```bash
mise run fmt    # Format Go source files (gofmt -w .)
mise run test   # Run tests (go test -v)
mise run lint   # Run linter (golangci-lint run)
mise run ci     # Run pre-commit on all files
```

### Direct Commands
```bash
go test -v                              # Run tests with verbose output
go test -v -race -coverprofile=coverage.out -covermode=atomic $(go list ./... | grep -v /example)  # CI test command
go vet ./...                            # Static analysis
golangci-lint run                       # Linter
gofmt -w .                              # Format all files
pre-commit run --all-files              # Run all pre-commit hooks
```

### Manual Testing
```bash
# Requires .env file with credentials in example/ directory
cd example && go run .
```

## Architecture

### File Organization (`lib-flume-water.go`)

Types and constants are grouped at the top in this order:
1. **Constants** — `Version`, `FlumeAPIURL`
2. **Client types** — `Client`, `Credentials`
3. **Authentication types** — `AuthRequest`, `TokenData`, `AuthResponse`, `AuthResult`
4. **Shared types** — `ID`, `Time`, `User`
5. **Device types** — `DeviceType` + constants, `Device`, `DevicesResponse`, `DeviceListParams`
6. **Location types** — `Location`, `LocationsResponse`, `LocationListParams`
7. **Query types** — `Query`, `QueryRequest`, `QueryData`, `QueryResult`, `queryResponse` (unexported)

All functions and methods follow after the type definitions.

### API Methods

All client I/O methods take a `context.Context` as their first argument:
- `Authenticate(ctx, ...)` / `AuthenticateFromEnv(ctx, envPath)` — OAuth2 password grant, returns `*AuthResult`
- `RefreshToken(ctx, clientID, clientSecret, refreshToken)` — exchanges refresh token for a new access token
- `GetDevices(ctx, accessToken, userID, *DeviceListParams)` — pass `nil` for defaults
- `GetLocations(ctx, accessToken, userID, *LocationListParams)` — pass `nil` for defaults
- `QueryDevice(ctx, accessToken, userID, deviceID, []Query)` — water usage data

`DefaultDeviceListParams()` and `DefaultLocationListParams()` return sensible defaults. Override specific fields as needed.

`AuthResult.ExpiresAt` is derived from the JWT's `exp` claim (with a fallback to `time.Now() + expires_in` if the claim is absent).

### Query Response Handling

The Flume API returns query results in a non-standard format where `data` is `[{"request_id": [{datetime, value}, ...]}]` — a single-element array containing an object keyed by request ID. The `queryResponse` struct (unexported) uses `[]map[string]json.RawMessage` to parse this, then `QueryDevice` converts it into `[]QueryResult` for callers, populating `Bucket` by matching `RequestID` back to the input `Query`.

### ID Handling

All resource IDs use the `ID` type (alias of `int64`). The API returns IDs as either JSON numbers (locations, users) or JSON strings (devices, which are 64-bit values stringified to avoid JS precision loss). `ID.UnmarshalJSON` accepts both forms; `ID.String()` returns the base-10 representation.

### Datetime Handling

All datetime fields use the `Time` type, which embeds `time.Time`. Its `UnmarshalJSON` accepts the two formats the API uses: RFC3339 with milliseconds and `Z` timezone (`/devices`, `/locations`) and a naive space-separated format (`/query`). `MarshalJSON` emits the space format (the request side); zero values marshal to `null` and `omitzero` JSON tags elide them from request bodies.

### Nested Resources

`Device.User` and `Device.Location` are nested pointer fields populated by `/devices`. The top-level `/locations` response uses `Location.UserID` (a flat ID) instead of the nested `User`; both fields use `omitempty` so only the populated one shows up.

## Testing Patterns

Tests use `httptest.NewServer()` mock servers. Standard pattern:
1. Create test JWT with `createTestJWT(userID)` (returns `token, expiresAt, error` — the JWT's `exp` claim is the source of truth for `AuthResult.ExpiresAt`)
2. Set up mock server handler that validates request and returns mock response
3. Create client with `BaseURL` pointing to test server
4. Pass `t.Context()` as the first arg to client methods
5. Assert on parsed response

Query tests use `map[string]interface{}` for mock responses (matching the real API format) and order-independent assertions via map lookups. Use the `mustTime(t, "2006-01-02 15:04:05")` helper to build `Time` values for `Query` literals.

The `example/` directory is excluded from test coverage.

## Version Management

Version is stored as `const Version` in `lib-flume-water.go` with a matching assertion in `TestVersion`.

**To bump:** update both the constant and test, commit as `chore: bump version to X.Y.Z`, create annotated tag `git tag -a vX.Y.Z -m "Release vX.Y.Z"`, push tag to trigger release workflow.

## Commit Conventions

Conventional commits for changelog generation:
- `feat:`, `fix:`, `perf:`, `refactor:`, `deps:` — appear in changelog
- `docs:`, `test:`, `chore:` — excluded from changelog

## CI/CD

- **Test workflow** (`.github/workflows/test.yml`): Runs on push/PR to main/master/develop. Steps: go vet, tests with race detection + coverage, gofmt check, build verification, golangci-lint (separate job), Codecov upload.
- **Release workflow** (`.github/workflows/release.yml`): Triggered by `v*.*.*` tags. Runs tests, then GoReleaser for GitHub releases.
- **Pre-commit** (`.pre-commit-config.yaml`): check-yaml, end-of-file-fixer, trailing-whitespace, go-vet, go-fmt, golangci-lint.

## Dependencies

- `github.com/joho/godotenv` — loading `.env` files
- `github.com/lestrrat-go/jwx/v3` — JWT parsing (extracts `user_id` from access tokens without verification)

## Environment Variables

Required in `.env` for `AuthenticateFromEnv()`:
- `FLUME_CLIENT_ID`, `FLUME_CLIENT_SECRET`, `FLUME_USER_EMAIL`, `FLUME_USER_PASSWORD`

`LoadCredentialsFromEnv` reads the file with `godotenv.Read` (no process env mutation). Values in the `.env` file take precedence; missing keys fall back to `os.Getenv`.
