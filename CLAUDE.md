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
4. **Device types** — `Device`, `DevicesResponse`, `DeviceListParams`
5. **Location types** — `Location`, `LocationsResponse`, `LocationListParams`
6. **Query types** — `Query`, `QueryRequest`, `QueryData`, `QueryResult`, `queryResponse` (unexported)

All functions and methods follow after the type definitions.

### API Methods

All methods are receivers on `*Client`:
- `Authenticate()` / `AuthenticateFromEnv()` — OAuth2 password grant, returns `AuthResult`
- `GetDevices(accessToken, userID, *DeviceListParams)` — pass `nil` for defaults
- `GetLocations(accessToken, userID, *LocationListParams)` — pass `nil` for defaults
- `QueryDevice(accessToken, userID, deviceID, []Query)` — water usage data

`DefaultDeviceListParams()` and `DefaultLocationListParams()` return sensible defaults. Override specific fields as needed.

### Query Response Handling

The Flume API returns query results in a non-standard format where `data` is `[{"request_id": [{datetime, value}, ...]}]` — a single-element array containing an object keyed by request ID. The `queryResponse` struct (unexported) uses `[]map[string]json.RawMessage` to parse this, then `QueryDevice` converts it into `[]QueryResult` for callers.

### ID Handling

The API returns IDs as either strings or numbers. All ID fields use `interface{}` with `GetIDString()` methods and an `interfaceToString()` helper for consistent conversion.

## Testing Patterns

Tests use `httptest.NewServer()` mock servers. Standard pattern:
1. Create test JWT with `createTestJWT()` or `createTestJWTWithNumericUserID()`
2. Set up mock server handler that validates request and returns mock response
3. Create client with `BaseURL` pointing to test server
4. Assert on parsed response

Query tests use `map[string]interface{}` for mock responses (matching the real API format) and order-independent assertions via map lookups.

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
