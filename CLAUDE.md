# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go library (`github.com/mdgreenwald/lib-flume-water`) for interacting with the Flume Water API. It provides authentication, device management, and location retrieval functionality for Flume smart water monitoring systems.

**Key characteristics:**
- This is a library, not an application - no binaries are built
- Single-file implementation pattern: all code in `lib-flume-water.go`, all tests in `lib-flume-water_test.go`
- API client follows standard Go HTTP client patterns with struct-based responses
- Uses JWT tokens for authentication with the Flume API
- All API responses follow a standard envelope format with `success`, `code`, `message`, and `data` fields

## Development Commands

### Testing
```bash
# Run all tests (excludes example/ directory)
go test

# Run tests with verbose output
go test -v

# Run tests with race detection and coverage (CI command)
go test -v -race -coverprofile=coverage.out -covermode=atomic $(go list ./... | grep -v /example)

# Generate HTML coverage report
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Code Quality
```bash
# Run static analysis
go vet ./...

# Run linter (used in CI)
golangci-lint run

# Verify dependencies
go mod verify

# Update dependencies
go mod tidy
```

### Manual Testing
```bash
# Run the example (requires .env file with credentials)
cd example
go run .
```

## Architecture and Patterns

### Client Structure
The library uses a single `Client` struct that holds:
- `HTTPClient`: Standard `*http.Client` for making requests
- `BaseURL`: API base URL (defaults to `https://api.flumewater.com`)

All API methods are receiver methods on `*Client`.

### Authentication Flow
1. User calls `Authenticate()` or `AuthenticateFromEnv()`
2. Library sends OAuth2 password grant request to `/oauth/token?envelope=true`
3. Response contains JWT access token
4. JWT is parsed (without verification) to extract `user_id` from claims
5. Returns `AuthResult` with `AccessToken`, `RefreshToken`, and `UserID`

**Important:** The `user_id` can be either a string or numeric type in the JWT. The `interfaceToString()` helper handles this conversion consistently.

### API Response Pattern
All API endpoints return responses in this envelope format:
```go
{
  "success": bool,
  "code": int,
  "message": string,
  "data": [...],  // Array of results
  "count": int,
  "status_code": int,
  "status_message": string
}
```

When implementing new API methods:
1. Create a request struct (if needed for POST/PUT)
2. Create a data struct representing the resource (e.g., `Device`, `Location`)
3. Create a response struct with the envelope fields and `Data []YourStruct`
4. Follow the pattern in `GetDevices()` or `GetLocations()`

### ID Handling
The Flume API returns IDs as either strings or numbers. All structs with IDs use `interface{}` for ID fields and provide `GetIDString()` methods for consistent string conversion.

### Error Handling
- All methods return `error` as the second return value
- Errors are wrapped with context using `fmt.Errorf("context: %w", err)`
- API failures check both HTTP status codes and the `success` field in responses
- Error messages include both the API message and error code

## Testing Patterns

### Mock Server Pattern
Tests use `httptest.NewServer()` to create mock API servers. See `TestAuthenticate_Success` for the standard pattern:
1. Create a test JWT token using `createTestJWT()` or `createTestJWTWithNumericUserID()`
2. Set up an `httptest.Server` with a handler that returns mock responses
3. Create a client with `BaseURL` set to the test server URL
4. Verify request format and response parsing

### Test Coverage
- Tests should cover success cases, error cases, and edge cases (like numeric vs string IDs)
- The example directory is explicitly excluded from test coverage
- Current tests use table-driven patterns for multiple scenarios

## Version Management

The library version is stored as a constant in `lib-flume-water.go`:
```go
const Version = "0.1.0"
```

**When bumping versions:**
1. Update `Version` constant in `lib-flume-water.go`
2. Update the version check in `TestVersion` in `lib-flume-water_test.go`
3. Commit with message: `chore: bump version to X.Y.Z`
4. Create annotated git tag: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
5. Push tag to trigger release workflow: `git push origin vX.Y.Z`

Follow semantic versioning:
- MAJOR: Breaking API changes
- MINOR: New features (backwards compatible)
- PATCH: Bug fixes (backwards compatible)

## Commit Conventions

Use conventional commits for changelog generation:
- `feat:` - New features (appears in changelog)
- `fix:` - Bug fixes (appears in changelog)
- `perf:` - Performance improvements (appears in changelog)
- `refactor:` - Code refactoring (appears in changelog)
- `deps:` - Dependency updates (appears in changelog)
- `docs:` - Documentation only (excluded from changelog)
- `test:` - Test changes only (excluded from changelog)
- `chore:` - Maintenance tasks (excluded from changelog)

## Adding New API Methods

When adding support for new Flume API endpoints:

1. **Define the data structure** for the resource (e.g., `type UsageData struct`)
2. **Define the response structure** following the envelope pattern
3. **Add the method to `*Client`** following existing patterns:
   - Use `fmt.Sprintf()` to build URLs with user-provided IDs
   - Set required headers: `Accept: application/json`, `Authorization: Bearer {token}`
   - Check HTTP status code before parsing response
   - Check the `Success` field in the response envelope
   - Return the `Data` field from the response
4. **Add comprehensive tests** in `lib-flume-water_test.go`:
   - Test with mock server
   - Test success and error cases
   - Test ID type variations if applicable
5. **Update README.md** with the new method documentation

## CI/CD

### GitHub Actions Workflows
- **Test workflow** (`.github/workflows/test.yml`): Runs on push/PR to main/master/develop
  - Executes `go vet` and tests with race detection
  - Runs `golangci-lint` for code quality
  - Uploads coverage to Codecov (requires `CODECOV_TOKEN` secret)

- **Release workflow** (`.github/workflows/release.yml`): Runs when version tags are pushed
  - Triggered by tags matching `v*.*.*`
  - Runs all tests
  - Uses GoReleaser to create GitHub releases with changelog and source archives
  - Requires repository workflow permissions set to "Read and write"

### Pre-commit Hooks
If using pre-commit, this command can be run:
```bash
pre-commit run
```

## Dependencies

The library has minimal dependencies:
- `github.com/joho/godotenv` - Loading credentials from `.env` files
- `github.com/lestrrat-go/jwx/v3` - JWT token parsing (used for extracting user_id from access tokens)

## Environment Variables

When using `AuthenticateFromEnv()`, these variables are required in the `.env` file:
- `FLUME_CLIENT_ID` - OAuth2 client ID
- `FLUME_CLIENT_SECRET` - OAuth2 client secret
- `FLUME_USER_EMAIL` - User email address
- `FLUME_USER_PASSWORD` - User password

The `.env` file should never be committed to the repository.
