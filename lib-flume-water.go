package flumewater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// ---------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------

const (
	// Version is the current version of the library
	Version = "1.3.0"

	// FlumeAPIURL is the base URL for the Flume API
	FlumeAPIURL = "https://api.flumewater.com"
)

// ---------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------

// Client represents a Flume API client
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

// Credentials represents the credentials loaded from .env file
type Credentials struct {
	ClientID     string
	ClientSecret string
	UserEmail    string
	UserPassword string
}

// ---------------------------------------------------------------------
// Authentication types
// ---------------------------------------------------------------------

// AuthRequest represents the authentication request payload
type AuthRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

// refreshRequest represents the refresh-token request payload
type refreshRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// TokenData represents the token data in the response. The Flume API does
// not include a user_id field in the JSON response; the canonical UserID
// comes from parsing the JWT in AuthResult.
type TokenData struct {
	TokenType    string `json:"token_type"`
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// AuthResponse represents the authentication response
type AuthResponse struct {
	Success bool        `json:"success"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    []TokenData `json:"data"`
}

// AuthResult contains the authentication result
type AuthResult struct {
	AccessToken  string
	RefreshToken string
	UserID       string
	// ExpiresAt is the absolute time at which AccessToken expires, computed
	// from the response's expires_in at the moment the response was received.
	ExpiresAt time.Time
}

// ---------------------------------------------------------------------
// Shared types
// ---------------------------------------------------------------------

// ID represents a Flume resource identifier. The Flume API returns IDs as
// either JSON numbers (locations, users) or JSON strings (devices, which are
// 64-bit integers stringified to avoid JS precision loss). ID accepts both
// forms when unmarshaling and exposes the value as int64.
type ID int64

// UnmarshalJSON parses an ID from a JSON number or a JSON string containing
// a base-10 integer. JSON null produces a zero ID.
func (id *ID) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*id = 0
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("id: %w", err)
		}
		if s == "" {
			*id = 0
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("id: parse %q: %w", s, err)
		}
		*id = ID(n)
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("id: %w", err)
	}
	*id = ID(n)
	return nil
}

// MarshalJSON emits the ID as a JSON number.
func (id ID) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(id), 10)), nil
}

// String returns the ID as a base-10 string.
func (id ID) String() string {
	return strconv.FormatInt(int64(id), 10)
}

// Time wraps time.Time to handle the Flume API's two datetime formats:
// RFC3339 with milliseconds and Z timezone (used in /devices, /locations,
// /oauth/token) and a naive space-separated format (used in /query). On
// UnmarshalJSON both formats are accepted. On MarshalJSON the space format
// is emitted, which is what /query accepts as input.
//
// Time embeds time.Time, so consumers can call standard time methods
// directly (e.g. d.LastSeen.IsZero(), d.LastSeen.Format(time.RFC3339)).
type Time struct {
	time.Time
}

const flumeTimeSpaceFormat = "2006-01-02 15:04:05"

// UnmarshalJSON parses RFC3339 (with or without fractional seconds, with or
// without timezone) and the API's space-separated format. JSON null and
// empty strings produce a zero Time.
func (t *Time) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" || s == `""` || s == "" {
		t.Time = time.Time{}
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("time: %w", err)
	}
	if raw == "" {
		t.Time = time.Time{}
		return nil
	}
	// Try RFC3339 variants first (the resource endpoints use this).
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			t.Time = parsed
			return nil
		}
	}
	// Then the naive space-separated format used by /query.
	if parsed, err := time.Parse(flumeTimeSpaceFormat, raw); err == nil {
		t.Time = parsed
		return nil
	}
	return fmt.Errorf("time: cannot parse %q in any known Flume format", raw)
}

// MarshalJSON emits the space-separated format ("2006-01-02 15:04:05")
// expected by the /query request. A zero Time marshals to JSON null.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.Format(flumeTimeSpaceFormat) + `"`), nil
}

// User represents a Flume user, returned nested inside Device.User and
// Location.User. Most consumers only need User.ID.
type User struct {
	ID                 ID     `json:"id"`
	EmailAddress       string `json:"email_address,omitempty"`
	FirstName          string `json:"first_name,omitempty"`
	LastName           string `json:"last_name,omitempty"`
	Phone              string `json:"phone,omitempty"`
	Status             string `json:"status,omitempty"`
	Type               string `json:"type,omitempty"`
	SignupDatetime     Time   `json:"signup_datetime,omitzero"`
	InvalidateDatetime Time   `json:"invalidate_datetime,omitzero"`
}

// ---------------------------------------------------------------------
// Device types
// ---------------------------------------------------------------------

// DeviceType identifies the kind of Flume device.
type DeviceType int

const (
	// DeviceTypeBridge is the Flume Bridge — the Wi-Fi gateway that relays
	// data from sensors to the cloud.
	DeviceTypeBridge DeviceType = 1
	// DeviceTypeSensor is the Flume Sensor attached to the water meter.
	DeviceTypeSensor DeviceType = 2
)

// String returns a human-readable name for the DeviceType.
func (t DeviceType) String() string {
	switch t {
	case DeviceTypeBridge:
		return "Bridge"
	case DeviceTypeSensor:
		return "Sensor"
	default:
		return fmt.Sprintf("Unknown(%d)", int(t))
	}
}

// Device represents a Flume device. Fields BridgeID, Oriented, and
// BatteryLevel are only meaningful for sensors (Type == DeviceTypeSensor).
type Device struct {
	ID           ID         `json:"id"`
	Type         DeviceType `json:"type"`
	BridgeID     *ID        `json:"bridge_id,omitempty"`
	Connected    bool       `json:"connected"`
	Oriented     bool       `json:"oriented,omitempty"`
	Product      string     `json:"product,omitempty"`
	BatteryLevel string     `json:"battery_level,omitempty"`
	LastSeen     Time       `json:"last_seen,omitzero"`
	User         *User      `json:"user,omitempty"`
	Location     *Location  `json:"location,omitempty"`
}

// DevicesResponse represents the API response for devices
type DevicesResponse struct {
	Success       bool     `json:"success"`
	Code          int      `json:"code"`
	Message       string   `json:"message"`
	Data          []Device `json:"data"`
	Count         int      `json:"count"`
	StatusCode    int      `json:"status_code"`
	StatusMessage string   `json:"status_message"`
}

// DeviceListParams controls query parameters for device listing.
// Use DefaultDeviceListParams() and override fields as needed.
type DeviceListParams struct {
	Limit           int
	Offset          int
	SortField       string
	SortDirection   string
	User            bool
	Location        bool
	ListShared      bool
	PrimaryLocation bool
	LocationID      string // optional filter by location
}

// ---------------------------------------------------------------------
// Location types
// ---------------------------------------------------------------------

// Location represents a Flume install location. Both the top-level
// /locations endpoint and the nested Device.Location field decode into this
// type. UserID is populated when returned from /locations; User is populated
// when nested in a Device response. Only one is present at a time.
type Location struct {
	ID              ID     `json:"id"`
	Name            string `json:"name,omitempty"`
	PrimaryLocation bool   `json:"primary_location,omitempty"`
	Address         string `json:"address,omitempty"`
	Address2        string `json:"address_2,omitempty"`
	City            string `json:"city,omitempty"`
	State           string `json:"state,omitempty"`
	PostalCode      string `json:"postal_code,omitempty"`
	Country         string `json:"country,omitempty"`
	Timezone        string `json:"tz,omitempty"`
	Installation    string `json:"installation,omitempty"`
	InsurerID       int    `json:"insurer_id,omitempty"`
	BuildingType    string `json:"building_type,omitempty"`
	UtilityType     string `json:"utility_type,omitempty"`
	AwayMode        bool   `json:"away_mode,omitempty"`
	UserID          ID     `json:"user_id,omitempty"`
	User            *User  `json:"user,omitempty"`
}

// LocationsResponse represents the API response for locations
type LocationsResponse struct {
	Success       bool       `json:"success"`
	Code          int        `json:"code"`
	Message       string     `json:"message"`
	Data          []Location `json:"data"`
	Count         int        `json:"count"`
	StatusCode    int        `json:"status_code"`
	StatusMessage string     `json:"status_message"`
}

// LocationListParams controls query parameters for location listing.
// Use DefaultLocationListParams() and override fields as needed.
type LocationListParams struct {
	Limit         int
	Offset        int
	SortField     string
	SortDirection string
	ListShared    bool
}

// ---------------------------------------------------------------------
// Query types
// ---------------------------------------------------------------------

// Query represents a single query in a device query request. SinceDatetime
// and UntilDatetime are emitted in the API's space-separated format by Time's
// MarshalJSON. A zero UntilDatetime is omitted from the request body.
type Query struct {
	RequestID       string `json:"request_id"`
	Bucket          string `json:"bucket"`
	SinceDatetime   Time   `json:"since_datetime"`
	UntilDatetime   Time   `json:"until_datetime,omitzero"`
	GroupMultiplier int    `json:"group_multiplier,omitempty"`
}

// QueryRequest represents the request body for querying a device
type QueryRequest struct {
	Queries []Query `json:"queries"`
}

// QueryData represents the data points returned for a query
type QueryData struct {
	Datetime Time    `json:"datetime"`
	Value    float64 `json:"value"`
}

// QueryResult represents the result of a single query
type QueryResult struct {
	RequestID string      `json:"request_id"`
	Bucket    string      `json:"bucket"`
	Data      []QueryData `json:"data"`
}

// queryResponse represents the raw API response for device queries.
// The Data field is an array containing a single object where keys are
// request IDs and values are arrays of QueryData.
type queryResponse struct {
	Success       bool                         `json:"success"`
	Code          int                          `json:"code"`
	Message       string                       `json:"message"`
	Data          []map[string]json.RawMessage `json:"data"`
	StatusCode    int                          `json:"status_code"`
	StatusMessage string                       `json:"status_message"`
}

// ---------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------

// NewClient creates a new Flume API client
func NewClient() *Client {
	return &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    FlumeAPIURL,
	}
}

// DefaultDeviceListParams returns DeviceListParams with sensible defaults.
func DefaultDeviceListParams() DeviceListParams {
	return DeviceListParams{
		Limit:           50,
		Offset:          0,
		SortField:       "id",
		SortDirection:   "ASC",
		User:            true,
		Location:        true,
		ListShared:      true,
		PrimaryLocation: true,
	}
}

func (p DeviceListParams) encode() string {
	v := url.Values{}
	v.Set("limit", strconv.Itoa(p.Limit))
	v.Set("offset", strconv.Itoa(p.Offset))
	v.Set("sort_field", p.SortField)
	v.Set("sort_direction", p.SortDirection)
	v.Set("user", strconv.FormatBool(p.User))
	v.Set("location", strconv.FormatBool(p.Location))
	v.Set("list_shared", strconv.FormatBool(p.ListShared))
	v.Set("primary_location", strconv.FormatBool(p.PrimaryLocation))
	if p.LocationID != "" {
		v.Set("location_id", p.LocationID)
	}
	return v.Encode()
}

// DefaultLocationListParams returns LocationListParams with sensible defaults.
func DefaultLocationListParams() LocationListParams {
	return LocationListParams{
		Limit:         50,
		Offset:        0,
		SortField:     "id",
		SortDirection: "ASC",
		ListShared:    true,
	}
}

func (p LocationListParams) encode() string {
	v := url.Values{}
	v.Set("limit", strconv.Itoa(p.Limit))
	v.Set("offset", strconv.Itoa(p.Offset))
	v.Set("sort_field", p.SortField)
	v.Set("sort_direction", p.SortDirection)
	v.Set("list_shared", strconv.FormatBool(p.ListShared))
	return v.Encode()
}

// Authenticate authenticates with the Flume API and returns tokens and user ID.
func (c *Client) Authenticate(ctx context.Context, clientID, clientSecret, userEmail, userPassword string) (*AuthResult, error) {
	return c.postTokenRequest(ctx, AuthRequest{
		GrantType:    "password",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Username:     userEmail,
		Password:     userPassword,
	})
}

// RefreshToken exchanges a refresh token for a new access token. The returned
// AuthResult contains a fresh AccessToken, the refresh_token returned by the
// server (which callers should persist in place of the prior one), the UserID
// parsed from the new access token, and ExpiresAt.
func (c *Client) RefreshToken(ctx context.Context, clientID, clientSecret, refreshToken string) (*AuthResult, error) {
	return c.postTokenRequest(ctx, refreshRequest{
		GrantType:    "refresh_token",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
	})
}

// postTokenRequest sends a token request payload to /oauth/token, parses the
// envelope and JWT, and returns an AuthResult. Shared by Authenticate and
// RefreshToken.
func (c *Client) postTokenRequest(ctx context.Context, payload any) (*AuthResult, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth request: %w", err)
	}

	reqURL := fmt.Sprintf("%s/oauth/token?envelope=true", c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var authResp AuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !authResp.Success {
		return nil, fmt.Errorf("authentication failed: %s (code: %d)", authResp.Message, authResp.Code)
	}

	if len(authResp.Data) == 0 {
		return nil, fmt.Errorf("no authentication data returned")
	}

	tokenData := authResp.Data[0]

	token, err := jwt.ParseString(tokenData.AccessToken, jwt.WithVerify(false))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	// Prefer the JWT's exp claim — an absolute time that doesn't drift with
	// request latency. Fall back to expires_in if exp is absent (shouldn't
	// happen for Flume's tokens, but cheap to guard).
	expiresAt, ok := token.Expiration()
	if !ok || expiresAt.IsZero() {
		expiresAt = time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
	}

	var userIDClaim interface{}
	if err := token.Get("user_id", &userIDClaim); err != nil {
		return nil, fmt.Errorf("user_id not found in JWT token: %w", err)
	}

	var userID string
	switch v := userIDClaim.(type) {
	case string:
		userID = v
	case float64:
		userID = fmt.Sprintf("%.0f", v)
	case int:
		userID = fmt.Sprintf("%d", v)
	case int64:
		userID = fmt.Sprintf("%d", v)
	default:
		return nil, fmt.Errorf("unexpected user_id type: %T", userIDClaim)
	}

	return &AuthResult{
		AccessToken:  tokenData.AccessToken,
		RefreshToken: tokenData.RefreshToken,
		UserID:       userID,
		ExpiresAt:    expiresAt,
	}, nil
}

// LoadCredentialsFromEnv loads credentials from a .env file without mutating
// the process environment. Values in the .env file take precedence over
// existing environment variables; any key absent from the file falls back to
// os.Getenv. If envPath is empty, it looks for .env in the current directory.
func LoadCredentialsFromEnv(envPath string) (*Credentials, error) {
	if envPath == "" {
		envPath = ".env"
	}

	fileEnv, err := godotenv.Read(envPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load .env file: %w", err)
	}

	get := func(key string) string {
		if v, ok := fileEnv[key]; ok {
			return v
		}
		return os.Getenv(key)
	}

	clientID := get("FLUME_CLIENT_ID")
	clientSecret := get("FLUME_CLIENT_SECRET")
	userEmail := get("FLUME_USER_EMAIL")
	userPassword := get("FLUME_USER_PASSWORD")

	// Validate required fields
	if clientID == "" {
		return nil, fmt.Errorf("FLUME_CLIENT_ID not found in environment")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("FLUME_CLIENT_SECRET not found in environment")
	}
	if userEmail == "" {
		return nil, fmt.Errorf("FLUME_USER_EMAIL not found in environment")
	}
	if userPassword == "" {
		return nil, fmt.Errorf("FLUME_USER_PASSWORD not found in environment")
	}

	return &Credentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		UserEmail:    userEmail,
		UserPassword: userPassword,
	}, nil
}

// AuthenticateFromEnv authenticates using credentials from a .env file.
// If envPath is empty, it looks for .env in the current directory.
func (c *Client) AuthenticateFromEnv(ctx context.Context, envPath string) (*AuthResult, error) {
	creds, err := LoadCredentialsFromEnv(envPath)
	if err != nil {
		return nil, err
	}

	return c.Authenticate(ctx, creds.ClientID, creds.ClientSecret, creds.UserEmail, creds.UserPassword)
}

// GetDevices retrieves devices for a user. Pass nil to use default parameters.
func (c *Client) GetDevices(ctx context.Context, accessToken, userID string, params *DeviceListParams) ([]Device, error) {
	p := DefaultDeviceListParams()
	if params != nil {
		p = *params
	}

	reqURL := fmt.Sprintf("%s/users/%s/devices?%s", c.BaseURL, userID, p.encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var devicesResp DevicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&devicesResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !devicesResp.Success {
		return nil, fmt.Errorf("API request failed: %s (code: %d)", devicesResp.Message, devicesResp.Code)
	}

	return devicesResp.Data, nil
}

// GetLocations retrieves locations for a user. Pass nil to use default parameters.
func (c *Client) GetLocations(ctx context.Context, accessToken, userID string, params *LocationListParams) ([]Location, error) {
	p := DefaultLocationListParams()
	if params != nil {
		p = *params
	}

	reqURL := fmt.Sprintf("%s/users/%s/locations?%s", c.BaseURL, userID, p.encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var locationsResp LocationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&locationsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !locationsResp.Success {
		return nil, fmt.Errorf("API request failed: %s (code: %d)", locationsResp.Message, locationsResp.Code)
	}

	return locationsResp.Data, nil
}

// QueryDevice queries a device for water usage data.
func (c *Client) QueryDevice(ctx context.Context, accessToken, userID, deviceID string, queries []Query) ([]QueryResult, error) {
	url := fmt.Sprintf("%s/users/%s/devices/%s/query", c.BaseURL, userID, deviceID)

	// Prepare request body
	queryReq := QueryRequest{
		Queries: queries,
	}

	payloadBytes, err := json.Marshal(queryReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var queryResp queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&queryResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !queryResp.Success {
		return nil, fmt.Errorf("API request failed: %s (code: %d)", queryResp.Message, queryResp.Code)
	}

	// Map request_id -> bucket so we can populate QueryResult.Bucket from
	// the response, which only includes request_id as the key.
	bucketByRequestID := make(map[string]string, len(queries))
	for _, q := range queries {
		bucketByRequestID[q.RequestID] = q.Bucket
	}

	// The API returns data as [{"request_id_1": [...], "request_id_2": [...]}]
	// Convert this into []QueryResult for a cleaner interface.
	var results []QueryResult
	if len(queryResp.Data) > 0 {
		for key, raw := range queryResp.Data[0] {
			var dataPoints []QueryData
			if err := json.Unmarshal(raw, &dataPoints); err != nil {
				return nil, fmt.Errorf("failed to decode query data for %q: %w", key, err)
			}
			results = append(results, QueryResult{
				RequestID: key,
				Bucket:    bucketByRequestID[key],
				Data:      dataPoints,
			})
		}
	}

	return results, nil
}
