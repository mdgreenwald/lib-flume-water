# lib-flume-water

[![Tests](https://github.com/mdgreenwald/lib-flume-water/actions/workflows/test.yml/badge.svg)](https://github.com/mdgreenwald/lib-flume-water/actions/workflows/test.yml)
[![Release](https://github.com/mdgreenwald/lib-flume-water/actions/workflows/release.yml/badge.svg)](https://github.com/mdgreenwald/lib-flume-water/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/mdgreenwald/lib-flume-water)](https://goreportcard.com/report/github.com/mdgreenwald/lib-flume-water)
[![codecov](https://codecov.io/gh/mdgreenwald/lib-flume-water/branch/main/graph/badge.svg)](https://codecov.io/gh/mdgreenwald/lib-flume-water)
[![Go Reference](https://pkg.go.dev/badge/github.com/mdgreenwald/lib-flume-water.svg)](https://pkg.go.dev/github.com/mdgreenwald/lib-flume-water)

A Go library for interacting with the Flume Water API. This library provides a simple, idiomatic Go interface for authenticating with the Flume API and retrieving information about your water monitoring devices and locations.

## What is Flume Water?

Flume is a smart home water monitoring system that helps you track water usage, detect leaks, and understand your water consumption patterns. This library allows developers to programmatically access their Flume account data.

### Use the library

```go
package main

import (
    "fmt"
    "log"

    flumewater "github.com/mdgreenwald/lib-flume-water"
)

func main() {
    // Create a new client
    client := flumewater.NewClient()

    // Authenticate
    authResult, err := client.AuthenticateFromEnv(".env")
    if err != nil {
        log.Fatal(err)
    }

    // Get locations
    locations, err := client.GetLocations(authResult.AccessToken, authResult.UserID)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Found %d locations\n", len(locations))
    for _, loc := range locations {
        fmt.Printf("- %s (ID: %s)\n", loc.Name, loc.GetIDString())
    }

    // Get devices
    devices, err := client.GetDevices(authResult.AccessToken, authResult.UserID)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Found %d devices\n", len(devices))
}
```

## API Reference

### Client

#### `NewClient() *Client`

Creates a new Flume API client with default settings.

```go
client := flumewater.NewClient()
```

### Authentication

#### `Authenticate(clientID, clientSecret, userEmail, userPassword string) (*AuthResult, error)`

Authenticates with the Flume API using explicit credentials.

```go
authResult, err := client.Authenticate(
    "client_id",
    "client_secret",
    "user@example.com",
    "password",
)
```

#### `AuthenticateFromEnv(envPath string) (*AuthResult, error)`

Authenticates using credentials from a `.env` file. Pass an empty string to use `.env` in the current directory.

```go
authResult, err := client.AuthenticateFromEnv(".env")
```

#### `RefreshToken(clientID, clientSecret, refreshToken string) (*AuthResult, error)`

Exchanges a refresh token for a new access token. Use `ExpiresAt` on the
returned result to decide when to refresh next. Always persist the
`RefreshToken` from the returned result in place of the prior one — the Flume
API may rotate it.

```go
newAuth, err := client.RefreshToken(
    "client_id",
    "client_secret",
    storedAuth.RefreshToken,
)
```

**Returns**: `AuthResult` containing:
- `AccessToken` - OAuth2 access token for API requests
- `RefreshToken` - Token for refreshing access (overwrite any previously stored value)
- `UserID` - The authenticated user's ID
- `ExpiresAt` - Absolute `time.Time` at which `AccessToken` expires

### Handling token refresh

The library is stateless: it does not store tokens or refresh them
automatically. Consumers own that policy. A minimal wrapper that refreshes
ahead of expiry:

```go
type Session struct {
    client       *flumewater.Client
    clientID     string
    clientSecret string
    auth         *flumewater.AuthResult
}

// Token returns a non-expired access token, refreshing if needed.
func (s *Session) Token() (string, error) {
    // Refresh 60s before expiry to absorb clock skew and request latency.
    if time.Now().Add(60 * time.Second).Before(s.auth.ExpiresAt) {
        return s.auth.AccessToken, nil
    }
    next, err := s.client.RefreshToken(s.clientID, s.clientSecret, s.auth.RefreshToken)
    if err != nil {
        return "", err
    }
    s.auth = next // persist next.RefreshToken alongside; Flume may rotate it
    return s.auth.AccessToken, nil
}
```

### Devices

#### `GetDevices(accessToken, userID string) ([]Device, error)`

Retrieves all devices associated with the user account.

```go
devices, err := client.GetDevices(authResult.AccessToken, authResult.UserID)
```

#### `GetDevicesByLocation(accessToken, userID, locationID string) ([]Device, error)`

Retrieves devices filtered by a specific location ID.

```go
devices, err := client.GetDevicesByLocation(
    authResult.AccessToken,
    authResult.UserID,
    "92632",
)
```

**Device Types**:
- `Type = 1`: Bridge device
- `Type = 2`: Sensor device

**Device Methods**:
- `GetIDString()` - Returns device ID as a string
- `GetLocationIDString()` - Returns location ID as a string

### Locations

#### `GetLocations(accessToken, userID string) ([]Location, error)`

Retrieves all locations associated with the user account.

```go
locations, err := client.GetLocations(authResult.AccessToken, authResult.UserID)
```

**Location Methods**:
- `GetIDString()` - Returns location ID as a string

### Version

The library version can be accessed via the exported constant:

```go
fmt.Println(flumewater.Version) // Output: 0.1.0
```

## Running Tests

### Run all tests

```bash
go test
```

### Run tests with verbose output

```bash
go test -v
```

### Run tests with coverage

```bash
go test -cover
```

### Generate coverage report

```bash
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Manual Testing

A complete manual testing example is provided in the `example/` directory.

### Setup

1. Copy your `.env` file to the example directory:
   ```bash
   cp .env example/.env
   ```

2. Run the example:
   ```bash
   cd example
   go run .
   ```

The example will:
- Authenticate with your credentials
- Fetch and display all locations
- Fetch and display all devices
- Filter devices by location

See `example/README.md` for more details.

## Data Types

### Device

```go
type Device struct {
    ID               interface{} // Device ID (numeric or string)
    Type             int         // 1 = Bridge, 2 = Sensor
    ProductID        interface{} // Product identifier
    LocationID       interface{} // Associated location ID
    UserID           interface{} // Owner user ID
    ConnectedDevice  interface{} // Connected device info
    BatteryLevel     interface{} // Battery level (if applicable)
    LastSeen         string      // Last seen timestamp
    ConnectedDatetime string     // Connection timestamp
}
```

### Location

```go
type Location struct {
    ID          interface{} // Location ID (numeric or string)
    Name        string      // Location name
    Address     string      // Street address
    Address2    string      // Address line 2
    City        string      // City
    State       string      // State/province
    PostalCode  string      // ZIP/postal code
    Country     string      // Country code
    Timezone    string      // IANA timezone
    UserID      interface{} // Owner user ID
    UtilityType string      // Utility type (e.g., "water")
    AwayMode    interface{} // Away mode settings
}
```

## Error Handling

All API methods return errors that should be checked:

```go
devices, err := client.GetDevices(accessToken, userID)
if err != nil {
    // Handle error
    log.Printf("Failed to get devices: %v", err)
    return
}
```

Common error scenarios:
- Authentication failures (invalid credentials)
- Network errors
- API rate limiting
- Invalid or expired access tokens
- Malformed API responses

## Requirements

- Go 1.26 or higher
- Valid [Flume Water API credentials](https://flumetech.readme.io/docs/authentication)

## Dependencies

- `github.com/joho/godotenv` - Environment variable loading
- `github.com/lestrrat-go/jwx/v3` - JWT token parsing

## API Documentation

For complete Flume API documentation, visit: https://flumetech.readme.io/reference

## Contributing

This library follows the patterns outlined in `CLAUDE.md`. When adding new methods:

1. Implement the method in `lib-flume-water.go`
2. Add corresponding tests in `lib-flume-water_test.go`
3. Follow existing patterns for error handling and response parsing
4. Ensure all tests pass with `go test`
5. Use conventional commit messages (see `.github/WORKFLOWS.md`)

### Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes and add tests
4. Run tests: `go test -v -race ./...`
5. Run linter: `golangci-lint run`
6. Commit your changes (`git commit -m 'feat: add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

All pull requests trigger automated testing via GitHub Actions.

## Releasing

Releases are automated using GitHub Actions and GoReleaser. See `.github/WORKFLOWS.md` for detailed instructions.

**Quick Release Steps:**

1. Update version in `lib-flume-water.go` and `lib-flume-water_test.go`
2. Commit: `git commit -m "chore: bump version to 0.2.0"`
3. Tag: `git tag -a v0.2.0 -m "Release v0.2.0"`
4. Push: `git push origin main && git push origin v0.2.0`

The release workflow will automatically create a GitHub release with changelog and artifacts.

## License

See LICENSE file for details.

## Support

For issues with this library, please open a GitHub issue.

For Flume API support, contact Flume Water support or consult their [API documentation](https://flumetech.readme.io/reference/accessing-the-api).
