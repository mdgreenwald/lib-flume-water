package flumewater

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// ---------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------

const (
	// Version is the current version of the library
	Version = "0.1.0"

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

// TokenData represents the token data in the response
type TokenData struct {
	TokenType    string      `json:"token_type"`
	AccessToken  string      `json:"access_token"`
	ExpiresIn    int         `json:"expires_in"`
	RefreshToken string      `json:"refresh_token"`
	UserID       interface{} `json:"user_id"` // Can be string or int
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
}

// ---------------------------------------------------------------------
// Device types
// ---------------------------------------------------------------------

// Device represents a Flume device
type Device struct {
	ID                interface{} `json:"id"`
	Type              int         `json:"type"` // 1 = Bridge, 2 = Sensor
	ProductID         interface{} `json:"product_id"`
	LocationID        interface{} `json:"location_id"`
	UserID            interface{} `json:"user_id"`
	ConnectedDevice   interface{} `json:"connected_device"`
	BatteryLevel      interface{} `json:"battery_level"`
	LastSeen          string      `json:"last_seen"`
	ConnectedDatetime string      `json:"connected_datetime"`
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

// Location represents a Flume location
type Location struct {
	ID          interface{} `json:"id"`
	Name        string      `json:"name"`
	Address     string      `json:"address"`
	Address2    string      `json:"address2"`
	City        string      `json:"city"`
	State       string      `json:"state"`
	PostalCode  string      `json:"postal_code"`
	Country     string      `json:"country"`
	Timezone    string      `json:"tz"`
	UserID      interface{} `json:"user_id"`
	UtilityType string      `json:"utility_type"`
	AwayMode    interface{} `json:"away_mode"`
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

// Query represents a single query in a device query request
type Query struct {
	RequestID       string `json:"request_id"`
	Bucket          string `json:"bucket"`
	SinceDatetime   string `json:"since_datetime"`
	UntilDatetime   string `json:"until_datetime,omitempty"`
	GroupMultiplier int    `json:"group_multiplier,omitempty"`
}

// QueryRequest represents the request body for querying a device
type QueryRequest struct {
	Queries []Query `json:"queries"`
}

// QueryData represents the data points returned for a query
type QueryData struct {
	Datetime string  `json:"datetime"`
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

// interfaceToString converts an interface{} value to string
// Handles string, int, int64, float64 types
func interfaceToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetIDString returns the ID as a string
func (d *Device) GetIDString() string {
	return interfaceToString(d.ID)
}

// GetLocationIDString returns the LocationID as a string
func (d *Device) GetLocationIDString() string {
	return interfaceToString(d.LocationID)
}

// GetIDString returns the ID as a string
func (l *Location) GetIDString() string {
	return interfaceToString(l.ID)
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

// Authenticate authenticates with the Flume API and returns tokens and user ID
func (c *Client) Authenticate(clientID, clientSecret, userEmail, userPassword string) (*AuthResult, error) {
	// Prepare request payload
	authReq := AuthRequest{
		GrantType:    "password",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Username:     userEmail,
		Password:     userPassword,
	}

	payloadBytes, err := json.Marshal(authReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/oauth/token?envelope=true", c.BaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response
	var authResp AuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check for errors
	if !authResp.Success {
		return nil, fmt.Errorf("authentication failed: %s (code: %d)", authResp.Message, authResp.Code)
	}

	if len(authResp.Data) == 0 {
		return nil, fmt.Errorf("no authentication data returned")
	}

	tokenData := authResp.Data[0]

	// Parse JWT to extract user_id using lestrrat-go/jwx
	token, err := jwt.ParseString(tokenData.AccessToken, jwt.WithVerify(false))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	// Extract user_id from token claims
	var userIDClaim interface{}
	if err := token.Get("user_id", &userIDClaim); err != nil {
		return nil, fmt.Errorf("user_id not found in JWT token: %w", err)
	}

	// Convert user_id to string (handle both string and numeric types)
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
	}, nil
}

// LoadCredentialsFromEnv loads credentials from a .env file
// If envPath is empty, it looks for .env in the current directory
func LoadCredentialsFromEnv(envPath string) (*Credentials, error) {
	// Load .env file
	if envPath == "" {
		envPath = ".env"
	}

	if err := godotenv.Load(envPath); err != nil {
		return nil, fmt.Errorf("failed to load .env file: %w", err)
	}

	// Read environment variables
	clientID := os.Getenv("FLUME_CLIENT_ID")
	clientSecret := os.Getenv("FLUME_CLIENT_SECRET")
	userEmail := os.Getenv("FLUME_USER_EMAIL")
	userPassword := os.Getenv("FLUME_USER_PASSWORD")

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

// AuthenticateFromEnv authenticates using credentials from a .env file
// If envPath is empty, it looks for .env in the current directory
func (c *Client) AuthenticateFromEnv(envPath string) (*AuthResult, error) {
	creds, err := LoadCredentialsFromEnv(envPath)
	if err != nil {
		return nil, err
	}

	return c.Authenticate(creds.ClientID, creds.ClientSecret, creds.UserEmail, creds.UserPassword)
}

// GetDevices retrieves devices for a user. Pass nil to use default parameters.
func (c *Client) GetDevices(accessToken, userID string, params *DeviceListParams) ([]Device, error) {
	p := DefaultDeviceListParams()
	if params != nil {
		p = *params
	}

	reqURL := fmt.Sprintf("%s/users/%s/devices?%s", c.BaseURL, userID, p.encode())

	req, err := http.NewRequest("GET", reqURL, nil)
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
func (c *Client) GetLocations(accessToken, userID string, params *LocationListParams) ([]Location, error) {
	p := DefaultLocationListParams()
	if params != nil {
		p = *params
	}

	reqURL := fmt.Sprintf("%s/users/%s/locations?%s", c.BaseURL, userID, p.encode())

	req, err := http.NewRequest("GET", reqURL, nil)
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

// QueryDevice queries a device for water usage data
func (c *Client) QueryDevice(accessToken, userID, deviceID string, queries []Query) ([]QueryResult, error) {
	url := fmt.Sprintf("%s/users/%s/devices/%s/query", c.BaseURL, userID, deviceID)

	// Prepare request body
	queryReq := QueryRequest{
		Queries: queries,
	}

	payloadBytes, err := json.Marshal(queryReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
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
				Data:      dataPoints,
			})
		}
	}

	return results, nil
}
