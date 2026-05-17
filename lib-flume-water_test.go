package flumewater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwt"
)

// createTestJWT creates a test JWT token with the given user_id. The returned
// expiresAt is the JWT's exp claim, truncated to second precision (matching
// what JWT serialization preserves).
func createTestJWT(userID string) (token string, expiresAt time.Time, err error) {
	return buildTestJWT(userID)
}

// createTestJWTWithNumericUserID creates a test JWT token with a numeric user_id.
func createTestJWTWithNumericUserID(userID int64) (token string, expiresAt time.Time, err error) {
	return buildTestJWT(userID)
}

// mustTime parses a space-separated datetime for test use.
func mustTime(t *testing.T, s string) Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		t.Fatalf("mustTime(%q): %v", s, err)
	}
	return Time{Time: parsed}
}

func buildTestJWT(userID any) (string, time.Time, error) {
	tok := jwt.New()
	if err := tok.Set("user_id", userID); err != nil {
		return "", time.Time{}, err
	}
	if err := tok.Set("type", "user"); err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	exp := now.Add(24 * time.Hour).Truncate(time.Second)
	if err := tok.Set(jwt.IssuedAtKey, now.Unix()); err != nil {
		return "", time.Time{}, err
	}
	if err := tok.Set(jwt.ExpirationKey, exp.Unix()); err != nil {
		return "", time.Time{}, err
	}
	serialized, err := jwt.NewSerializer().Serialize(tok)
	if err != nil {
		return "", time.Time{}, err
	}
	return string(serialized), exp, nil
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version is empty")
	}

	// Version should follow semantic versioning format (X.Y.Z)
	if !strings.Contains(Version, ".") {
		t.Errorf("Version %s does not appear to follow semantic versioning", Version)
	}

	// Current version should be 1.3.0
	if Version != "1.3.0" {
		t.Errorf("Version = %s, want 1.3.0", Version)
	}
}

func TestTime_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{"rfc3339_ms_utc", `"2026-05-17T14:36:31.000Z"`, time.Date(2026, 5, 17, 14, 36, 31, 0, time.UTC), false},
		{"rfc3339_no_ms", `"2026-05-17T14:36:31Z"`, time.Date(2026, 5, 17, 14, 36, 31, 0, time.UTC), false},
		{"space_format", `"2026-05-10 00:00:00"`, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), false},
		{"null", `null`, time.Time{}, false},
		{"empty_string", `""`, time.Time{}, false},
		{"garbage", `"not-a-date"`, time.Time{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got Time
			err := json.Unmarshal([]byte(c.in), &got)
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", c.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%q): %v", c.in, err)
			}
			if !got.Equal(c.want) {
				t.Errorf("got %v, want %v", got.Time, c.want)
			}
		})
	}
}

func TestTime_MarshalJSON(t *testing.T) {
	t.Run("non_zero_emits_space_format", func(t *testing.T) {
		tm := Time{Time: time.Date(2026, 5, 17, 14, 36, 31, 0, time.UTC)}
		got, err := json.Marshal(tm)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(got) != `"2026-05-17 14:36:31"` {
			t.Errorf("got %s, want %q", got, "2026-05-17 14:36:31")
		}
	})
	t.Run("zero_marshals_null", func(t *testing.T) {
		got, err := json.Marshal(Time{})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(got) != "null" {
			t.Errorf("got %s, want null", got)
		}
	})
}

func TestQuery_ZeroUntilDatetime_OmittedFromRequest(t *testing.T) {
	// With omitzero on UntilDatetime, a zero Time should be elided from the
	// JSON request body so the API doesn't reject a null bound.
	q := Query{
		RequestID:     "x",
		Bucket:        "DAY",
		SinceDatetime: mustTime(t, "2025-11-01 00:00:00"),
	}
	got, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(got), "until_datetime") {
		t.Errorf("expected until_datetime to be omitted, got %s", got)
	}
}

func TestQueryData_DecodesSpaceFormat(t *testing.T) {
	raw := []byte(`{"datetime":"2026-05-10 00:00:00","value":204.67982556}`)
	var qd QueryData
	if err := json.Unmarshal(raw, &qd); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	if !qd.Datetime.Equal(want) {
		t.Errorf("Datetime = %v, want %v", qd.Datetime.Time, want)
	}
	if qd.Value != 204.67982556 {
		t.Errorf("Value = %v", qd.Value)
	}
}

func TestID_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want ID
	}{
		{"number", `12345`, 12345},
		{"large_string", `"6919448433101715210"`, 6919448433101715210},
		{"null", `null`, 0},
		{"empty_string", `""`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var id ID
			if err := json.Unmarshal([]byte(c.in), &id); err != nil {
				t.Fatalf("Unmarshal(%q): %v", c.in, err)
			}
			if id != c.want {
				t.Errorf("got %d, want %d", id, c.want)
			}
		})
	}
}

func TestID_UnmarshalJSON_InvalidString(t *testing.T) {
	var id ID
	if err := json.Unmarshal([]byte(`"not-a-number"`), &id); err == nil {
		t.Error("expected error for non-numeric string, got nil")
	}
}

func TestID_MarshalJSON(t *testing.T) {
	type wrapper struct {
		X ID `json:"x"`
	}
	got, err := json.Marshal(wrapper{X: 6919448433101715210})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"x":6919448433101715210}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDevice_UnmarshalRealResponse(t *testing.T) {
	// Shape matches a real /devices response (sensor with stringified id and
	// nested bridge_id, location, user).
	raw := []byte(`{
		"id": "6919448433101715210",
		"type": 2,
		"bridge_id": "6916596620381398904",
		"oriented": true,
		"last_seen": "2026-05-17T14:45:45.000Z",
		"connected": true,
		"battery_level": "high",
		"product": "flume2",
		"user": {"id": 70760, "email_address": "u@example.com"},
		"location": {"id": 92632, "name": "Home", "tz": "America/New_York"}
	}`)
	var d Device
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.ID != 6919448433101715210 {
		t.Errorf("ID = %d, want 6919448433101715210", d.ID)
	}
	if d.Type != DeviceTypeSensor {
		t.Errorf("Type = %v, want DeviceTypeSensor", d.Type)
	}
	if d.BridgeID == nil || *d.BridgeID != 6916596620381398904 {
		t.Errorf("BridgeID = %v, want 6916596620381398904", d.BridgeID)
	}
	if !d.Connected || !d.Oriented {
		t.Errorf("Connected=%v Oriented=%v, want true,true", d.Connected, d.Oriented)
	}
	if d.BatteryLevel != "high" {
		t.Errorf("BatteryLevel = %q, want high", d.BatteryLevel)
	}
	if d.Product != "flume2" {
		t.Errorf("Product = %q, want flume2", d.Product)
	}
	if d.User == nil || d.User.ID != 70760 {
		t.Errorf("User.ID = %v, want 70760", d.User)
	}
	if d.Location == nil || d.Location.ID != 92632 {
		t.Errorf("Location.ID = %v, want 92632", d.Location)
	}
	wantSeen := time.Date(2026, 5, 17, 14, 45, 45, 0, time.UTC)
	if !d.LastSeen.Equal(wantSeen) {
		t.Errorf("LastSeen = %v, want %v", d.LastSeen.Time, wantSeen)
	}
}

func TestDevice_UnmarshalBridge_NullBridgeID(t *testing.T) {
	raw := []byte(`{"id":"6916596620381398904","type":1,"bridge_id":null,"connected":true,"product":"flume2"}`)
	var d Device
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.Type != DeviceTypeBridge {
		t.Errorf("Type = %v, want DeviceTypeBridge", d.Type)
	}
	if d.BridgeID != nil {
		t.Errorf("BridgeID = %v, want nil for a bridge device", d.BridgeID)
	}
}

func TestLocation_UnmarshalRealResponse(t *testing.T) {
	raw := []byte(`{
		"id": 92632,
		"user_id": 70760,
		"name": "Chester",
		"primary_location": true,
		"address": "56 E Chester Street",
		"address_2": "",
		"city": "Kingston",
		"state": "NY",
		"postal_code": "12401",
		"country": "United States",
		"tz": "America/New_York",
		"installation": "DONE",
		"insurer_id": 19,
		"building_type": "SINGLE_FAMILY_HOME",
		"away_mode": false
	}`)
	var l Location
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if l.ID != 92632 {
		t.Errorf("ID = %d, want 92632", l.ID)
	}
	if l.UserID != 70760 {
		t.Errorf("UserID = %d, want 70760", l.UserID)
	}
	if !l.PrimaryLocation {
		t.Error("PrimaryLocation should be true")
	}
	if l.Installation != "DONE" {
		t.Errorf("Installation = %q, want DONE", l.Installation)
	}
	if l.InsurerID != 19 {
		t.Errorf("InsurerID = %d, want 19", l.InsurerID)
	}
	if l.BuildingType != "SINGLE_FAMILY_HOME" {
		t.Errorf("BuildingType = %q, want SINGLE_FAMILY_HOME", l.BuildingType)
	}
	if l.AwayMode {
		t.Error("AwayMode should be false")
	}
}

func TestContextCancellation(t *testing.T) {
	// Server that never responds — request should be cancelled by the context.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	client := &Client{HTTPClient: http.DefaultClient, BaseURL: server.URL}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // already cancelled
	_, err := client.GetDevices(ctx, "tok", "12345", nil)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want a context.Canceled wrapper", err)
	}
}

func TestDeviceType_String(t *testing.T) {
	cases := []struct {
		in   DeviceType
		want string
	}{
		{DeviceTypeBridge, "Bridge"},
		{DeviceTypeSensor, "Sensor"},
		{DeviceType(99), "Unknown(99)"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("DeviceType(%d).String() = %q, want %q", int(c.in), got, c.want)
		}
	}
}

func TestDeviceType_JSONRoundTrip(t *testing.T) {
	raw := []byte(`{"type": 2}`)
	var d Device
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.Type != DeviceTypeSensor {
		t.Errorf("Type = %v, want DeviceTypeSensor", d.Type)
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient()

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	if client.HTTPClient == nil {
		t.Error("HTTPClient is nil")
	}

	if client.BaseURL != FlumeAPIURL {
		t.Errorf("BaseURL = %s, want %s", client.BaseURL, FlumeAPIURL)
	}
}

func TestAuthenticate_Success(t *testing.T) {
	// Create test JWT token
	testUserID := "12345"
	testAccessToken, expectedExp, err := createTestJWT(testUserID)
	if err != nil {
		t.Fatalf("failed to create test JWT: %v", err)
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/oauth/token" {
			t.Errorf("Expected path /oauth/token, got %s", r.URL.Path)
		}

		// Verify headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Verify request body
		var authReq AuthRequest
		if err := json.NewDecoder(r.Body).Decode(&authReq); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if authReq.GrantType != "password" {
			t.Errorf("Expected grant_type password, got %s", authReq.GrantType)
		}

		// Send success response
		response := AuthResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []TokenData{
				{
					TokenType:    "bearer",
					AccessToken:  testAccessToken,
					ExpiresIn:    86400,
					RefreshToken: "test_refresh_token",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	result, err := client.Authenticate(t.Context(), "test_client_id", "test_client_secret", "test@example.com", "test_password")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	if result.AccessToken != testAccessToken {
		t.Errorf("AccessToken = %s, want %s", result.AccessToken, testAccessToken)
	}

	if result.RefreshToken != "test_refresh_token" {
		t.Errorf("RefreshToken = %s, want %s", result.RefreshToken, "test_refresh_token")
	}

	if result.UserID != testUserID {
		t.Errorf("UserID = %s, want %s", result.UserID, testUserID)
	}

	// ExpiresAt is derived from the JWT's exp claim.
	if !result.ExpiresAt.Equal(expectedExp) {
		t.Errorf("ExpiresAt = %v, want %v (from JWT exp)", result.ExpiresAt, expectedExp)
	}
}

func TestAuthenticate_NumericUserID(t *testing.T) {
	// Create test JWT token with numeric user_id
	testUserID := int64(12345)
	testAccessToken, _, err := createTestJWTWithNumericUserID(testUserID)
	if err != nil {
		t.Fatalf("failed to create test JWT: %v", err)
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []TokenData{
				{
					TokenType:    "bearer",
					AccessToken:  testAccessToken,
					ExpiresIn:    86400,
					RefreshToken: "test_refresh_token",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	result, err := client.Authenticate(t.Context(), "test_client_id", "test_client_secret", "test@example.com", "test_password")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	expectedUserID := fmt.Sprintf("%d", testUserID)
	if result.UserID != expectedUserID {
		t.Errorf("UserID = %s, want %s", result.UserID, expectedUserID)
	}
}

func TestAuthenticate_FailedAuth(t *testing.T) {
	// Create test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: false,
			Code:    401,
			Message: "Invalid credentials",
			Data:    []TokenData{},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.Authenticate(t.Context(), "bad_client_id", "bad_client_secret", "test@example.com", "wrong_password")
	if err == nil {
		t.Fatal("Expected error for failed authentication, got nil")
	}

	expectedError := "authentication failed: Invalid credentials (code: 401)"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestAuthenticate_EmptyData(t *testing.T) {
	// Create test server that returns empty data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data:    []TokenData{},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.Authenticate(t.Context(), "test_client_id", "test_client_secret", "test@example.com", "test_password")
	if err == nil {
		t.Fatal("Expected error for empty data, got nil")
	}

	expectedError := "no authentication data returned"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestAuthenticate_InvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.Authenticate(t.Context(), "test_client_id", "test_client_secret", "test@example.com", "test_password")
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestAuthenticate_InvalidJWT(t *testing.T) {
	// Create test server that returns an invalid JWT token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []TokenData{
				{
					TokenType:    "bearer",
					AccessToken:  "invalid.jwt.token",
					ExpiresIn:    86400,
					RefreshToken: "test_refresh_token",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.Authenticate(t.Context(), "test_client_id", "test_client_secret", "test@example.com", "test_password")
	if err == nil {
		t.Fatal("Expected error for invalid JWT, got nil")
	}
}

func TestAuthenticate_MissingUserID(t *testing.T) {
	// Create JWT without user_id claim
	token := jwt.New()
	_ = token.Set("type", "user")
	_ = token.Set(jwt.IssuedAtKey, time.Now().Unix())
	_ = token.Set(jwt.ExpirationKey, time.Now().Add(24*time.Hour).Unix())

	serialized, err := jwt.NewSerializer().Serialize(token)
	if err != nil {
		t.Fatalf("failed to create test JWT: %v", err)
	}

	testAccessToken := string(serialized)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []TokenData{
				{
					TokenType:    "bearer",
					AccessToken:  testAccessToken,
					ExpiresIn:    86400,
					RefreshToken: "test_refresh_token",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err = client.Authenticate(t.Context(), "test_client_id", "test_client_secret", "test@example.com", "test_password")
	if err == nil {
		t.Fatal("Expected error for missing user_id in JWT, got nil")
	}

	expectedError := "user_id not found in JWT token"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Error = %s, want it to contain %s", err.Error(), expectedError)
	}
}

func TestRefreshToken_Success(t *testing.T) {
	testUserID := "12345"
	testAccessToken, expectedExp, err := createTestJWT(testUserID)
	if err != nil {
		t.Fatalf("failed to create test JWT: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/oauth/token" {
			t.Errorf("Expected path /oauth/token, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}
		if body["grant_type"] != "refresh_token" {
			t.Errorf("Expected grant_type refresh_token, got %s", body["grant_type"])
		}
		if body["refresh_token"] != "old_refresh_token" {
			t.Errorf("Expected refresh_token old_refresh_token, got %s", body["refresh_token"])
		}
		if body["client_id"] != "test_client_id" {
			t.Errorf("Expected client_id test_client_id, got %s", body["client_id"])
		}
		if body["client_secret"] != "test_client_secret" {
			t.Errorf("Expected client_secret test_client_secret, got %s", body["client_secret"])
		}

		response := AuthResponse{
			Success: true,
			Code:    602,
			Message: "Request OK",
			Data: []TokenData{
				{
					TokenType:    "bearer",
					AccessToken:  testAccessToken,
					ExpiresIn:    604800,
					RefreshToken: "new_refresh_token",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	result, err := client.RefreshToken(t.Context(), "test_client_id", "test_client_secret", "old_refresh_token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}

	if result.AccessToken != testAccessToken {
		t.Errorf("AccessToken = %s, want %s", result.AccessToken, testAccessToken)
	}
	if result.RefreshToken != "new_refresh_token" {
		t.Errorf("RefreshToken = %s, want new_refresh_token", result.RefreshToken)
	}
	if result.UserID != testUserID {
		t.Errorf("UserID = %s, want %s", result.UserID, testUserID)
	}

	if !result.ExpiresAt.Equal(expectedExp) {
		t.Errorf("ExpiresAt = %v, want %v (from JWT exp)", result.ExpiresAt, expectedExp)
	}
}

func TestRefreshToken_FailedRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: false,
			Code:    400,
			Message: "invalid_client",
			Data:    []TokenData{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.RefreshToken(t.Context(), "bad_client", "bad_secret", "stale_refresh_token")
	if err == nil {
		t.Fatal("Expected error for failed refresh, got nil")
	}

	expectedError := "authentication failed: invalid_client (code: 400)"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestRefreshToken_EmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: true,
			Code:    602,
			Message: "Request OK",
			Data:    []TokenData{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.RefreshToken(t.Context(), "test_client_id", "test_client_secret", "refresh_token")
	if err == nil {
		t.Fatal("Expected error for empty data, got nil")
	}

	expectedError := "no authentication data returned"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestRefreshToken_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.RefreshToken(t.Context(), "test_client_id", "test_client_secret", "refresh_token")
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestRefreshToken_InvalidJWT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: true,
			Code:    602,
			Message: "Request OK",
			Data: []TokenData{
				{
					TokenType:    "bearer",
					AccessToken:  "invalid.jwt.token",
					ExpiresIn:    604800,
					RefreshToken: "new_refresh_token",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.RefreshToken(t.Context(), "test_client_id", "test_client_secret", "refresh_token")
	if err == nil {
		t.Fatal("Expected error for invalid JWT, got nil")
	}
}

func TestLoadCredentialsFromEnv_Success(t *testing.T) {
	// Create a temporary .env file
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `FLUME_CLIENT_ID=test_client_id
FLUME_CLIENT_SECRET=test_client_secret
FLUME_USER_EMAIL=test@example.com
FLUME_USER_PASSWORD=test_password
`

	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	// Clear any existing environment variables
	_ = os.Unsetenv("FLUME_CLIENT_ID")
	_ = os.Unsetenv("FLUME_CLIENT_SECRET")
	_ = os.Unsetenv("FLUME_USER_EMAIL")
	_ = os.Unsetenv("FLUME_USER_PASSWORD")

	// Load credentials
	creds, err := LoadCredentialsFromEnv(envPath)
	if err != nil {
		t.Fatalf("LoadCredentialsFromEnv() error = %v", err)
	}

	// Verify credentials
	if creds.ClientID != "test_client_id" {
		t.Errorf("ClientID = %s, want test_client_id", creds.ClientID)
	}
	if creds.ClientSecret != "test_client_secret" {
		t.Errorf("ClientSecret = %s, want test_client_secret", creds.ClientSecret)
	}
	if creds.UserEmail != "test@example.com" {
		t.Errorf("UserEmail = %s, want test@example.com", creds.UserEmail)
	}
	if creds.UserPassword != "test_password" {
		t.Errorf("UserPassword = %s, want test_password", creds.UserPassword)
	}
}

func TestLoadCredentialsFromEnv_NoProcessEnvMutation(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `FLUME_CLIENT_ID=from_file
FLUME_CLIENT_SECRET=from_file
FLUME_USER_EMAIL=from_file@example.com
FLUME_USER_PASSWORD=from_file
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	// Clear the env so we can detect if Load wrote to it.
	for _, k := range []string{"FLUME_CLIENT_ID", "FLUME_CLIENT_SECRET", "FLUME_USER_EMAIL", "FLUME_USER_PASSWORD"} {
		_ = os.Unsetenv(k)
	}

	if _, err := LoadCredentialsFromEnv(envPath); err != nil {
		t.Fatalf("LoadCredentialsFromEnv() error = %v", err)
	}

	for _, k := range []string{"FLUME_CLIENT_ID", "FLUME_CLIENT_SECRET", "FLUME_USER_EMAIL", "FLUME_USER_PASSWORD"} {
		if v, ok := os.LookupEnv(k); ok {
			t.Errorf("LoadCredentialsFromEnv set process env %s=%q; expected no mutation", k, v)
		}
	}
}

func TestLoadCredentialsFromEnv_FilePrecedenceOverOsEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `FLUME_CLIENT_ID=from_file
FLUME_CLIENT_SECRET=from_file
FLUME_USER_EMAIL=from_file@example.com
FLUME_USER_PASSWORD=from_file
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	t.Setenv("FLUME_CLIENT_ID", "from_os_env")

	creds, err := LoadCredentialsFromEnv(envPath)
	if err != nil {
		t.Fatalf("LoadCredentialsFromEnv() error = %v", err)
	}
	if creds.ClientID != "from_file" {
		t.Errorf("ClientID = %q, want %q (.env file should win over os.Getenv)", creds.ClientID, "from_file")
	}
}

func TestLoadCredentialsFromEnv_OsEnvFallback(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	// File missing FLUME_USER_PASSWORD; should fall back to os env.
	envContent := `FLUME_CLIENT_ID=from_file
FLUME_CLIENT_SECRET=from_file
FLUME_USER_EMAIL=from_file@example.com
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	t.Setenv("FLUME_USER_PASSWORD", "from_os_env")

	creds, err := LoadCredentialsFromEnv(envPath)
	if err != nil {
		t.Fatalf("LoadCredentialsFromEnv() error = %v", err)
	}
	if creds.UserPassword != "from_os_env" {
		t.Errorf("UserPassword = %q, want %q (os.Getenv fallback)", creds.UserPassword, "from_os_env")
	}
}

func TestLoadCredentialsFromEnv_MissingFile(t *testing.T) {
	// Clear any existing environment variables
	_ = os.Unsetenv("FLUME_CLIENT_ID")
	_ = os.Unsetenv("FLUME_CLIENT_SECRET")
	_ = os.Unsetenv("FLUME_USER_EMAIL")
	_ = os.Unsetenv("FLUME_USER_PASSWORD")

	// Try to load from non-existent file
	_, err := LoadCredentialsFromEnv("/nonexistent/path/.env")
	if err == nil {
		t.Fatal("Expected error for missing .env file, got nil")
	}

	expectedError := "failed to load .env file"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Error = %s, want it to contain %s", err.Error(), expectedError)
	}
}

func TestLoadCredentialsFromEnv_MissingClientID(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `FLUME_CLIENT_SECRET=test_client_secret
FLUME_USER_EMAIL=test@example.com
FLUME_USER_PASSWORD=test_password
`

	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	_ = os.Unsetenv("FLUME_CLIENT_ID")

	_, err := LoadCredentialsFromEnv(envPath)
	if err == nil {
		t.Fatal("Expected error for missing FLUME_CLIENT_ID, got nil")
	}

	expectedError := "FLUME_CLIENT_ID not found in environment"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestLoadCredentialsFromEnv_MissingClientSecret(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `FLUME_CLIENT_ID=test_client_id
FLUME_USER_EMAIL=test@example.com
FLUME_USER_PASSWORD=test_password
`

	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	_ = os.Unsetenv("FLUME_CLIENT_SECRET")

	_, err := LoadCredentialsFromEnv(envPath)
	if err == nil {
		t.Fatal("Expected error for missing FLUME_CLIENT_SECRET, got nil")
	}

	expectedError := "FLUME_CLIENT_SECRET not found in environment"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestLoadCredentialsFromEnv_MissingUserEmail(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `FLUME_CLIENT_ID=test_client_id
FLUME_CLIENT_SECRET=test_client_secret
FLUME_USER_PASSWORD=test_password
`

	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	_ = os.Unsetenv("FLUME_USER_EMAIL")

	_, err := LoadCredentialsFromEnv(envPath)
	if err == nil {
		t.Fatal("Expected error for missing FLUME_USER_EMAIL, got nil")
	}

	expectedError := "FLUME_USER_EMAIL not found in environment"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestLoadCredentialsFromEnv_MissingUserPassword(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `FLUME_CLIENT_ID=test_client_id
FLUME_CLIENT_SECRET=test_client_secret
FLUME_USER_EMAIL=test@example.com
`

	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	_ = os.Unsetenv("FLUME_USER_PASSWORD")

	_, err := LoadCredentialsFromEnv(envPath)
	if err == nil {
		t.Fatal("Expected error for missing FLUME_USER_PASSWORD, got nil")
	}

	expectedError := "FLUME_USER_PASSWORD not found in environment"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestAuthenticateFromEnv_Success(t *testing.T) {
	// Create test JWT token
	testUserID := "12345"
	testAccessToken, _, err := createTestJWT(testUserID)
	if err != nil {
		t.Fatalf("failed to create test JWT: %v", err)
	}

	// Create temporary .env file
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `FLUME_CLIENT_ID=test_client_id
FLUME_CLIENT_SECRET=test_client_secret
FLUME_USER_EMAIL=test@example.com
FLUME_USER_PASSWORD=test_password
`

	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	// Clear any existing environment variables
	_ = os.Unsetenv("FLUME_CLIENT_ID")
	_ = os.Unsetenv("FLUME_CLIENT_SECRET")
	_ = os.Unsetenv("FLUME_USER_EMAIL")
	_ = os.Unsetenv("FLUME_USER_PASSWORD")

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AuthResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []TokenData{
				{
					TokenType:    "bearer",
					AccessToken:  testAccessToken,
					ExpiresIn:    86400,
					RefreshToken: "test_refresh_token",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test AuthenticateFromEnv
	result, err := client.AuthenticateFromEnv(t.Context(), envPath)
	if err != nil {
		t.Fatalf("AuthenticateFromEnv() error = %v", err)
	}

	if result.AccessToken != testAccessToken {
		t.Errorf("AccessToken = %s, want %s", result.AccessToken, testAccessToken)
	}

	if result.RefreshToken != "test_refresh_token" {
		t.Errorf("RefreshToken = %s, want %s", result.RefreshToken, "test_refresh_token")
	}

	if result.UserID != testUserID {
		t.Errorf("UserID = %s, want %s", result.UserID, testUserID)
	}
}

func TestAuthenticateFromEnv_InvalidEnvFile(t *testing.T) {
	client := NewClient()

	_ = os.Unsetenv("FLUME_CLIENT_ID")
	_ = os.Unsetenv("FLUME_CLIENT_SECRET")
	_ = os.Unsetenv("FLUME_USER_EMAIL")
	_ = os.Unsetenv("FLUME_USER_PASSWORD")

	_, err := client.AuthenticateFromEnv(t.Context(), "/nonexistent/path/.env")
	if err == nil {
		t.Fatal("Expected error for invalid .env file, got nil")
	}
}

func TestGetDevices_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/users/") || !strings.HasSuffix(r.URL.Path, "/devices") {
			t.Errorf("Expected path /users/{user_id}/devices, got %s", r.URL.Path)
		}

		// Verify headers
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept application/json, got %s", r.Header.Get("Accept"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("Expected Authorization header with Bearer token")
		}

		// Send success response
		response := DevicesResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []Device{
				{
					ID:       1,
					Type:     DeviceTypeSensor,
					Product:  "flume2",
					Location: &Location{ID: 201},
					User:     &User{ID: 12345},
				},
				{
					ID:       2,
					Type:     DeviceTypeBridge,
					Product:  "flume2",
					Location: &Location{ID: 201},
					User:     &User{ID: 12345},
				},
			},
			Count:         2,
			StatusCode:    200,
			StatusMessage: "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test GetDevices
	devices, err := client.GetDevices(t.Context(), "test_access_token", "12345", nil)
	if err != nil {
		t.Fatalf("GetDevices() error = %v", err)
	}

	if len(devices) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(devices))
	}

	if devices[0].ID != 1 {
		t.Errorf("Device[0].ID = %d, want 1", devices[0].ID)
	}

	if devices[0].Type != DeviceTypeSensor {
		t.Errorf("Device[0].Type = %v, want DeviceTypeSensor", devices[0].Type)
	}

	if devices[1].ID != 2 {
		t.Errorf("Device[1].ID = %d, want 2", devices[1].ID)
	}
}

func TestGetDevices_APIError(t *testing.T) {
	// Create test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DevicesResponse{
			Success: false,
			Code:    401,
			Message: "Unauthorized",
			Data:    []Device{},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetDevices(t.Context(), "invalid_token", "12345", nil)
	if err == nil {
		t.Fatal("Expected error for API error response, got nil")
	}

	expectedError := "API request failed: Unauthorized (code: 401)"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestGetDevices_HTTPError(t *testing.T) {
	// Create test server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetDevices(t.Context(), "test_token", "12345", nil)
	if err == nil {
		t.Fatal("Expected error for HTTP error, got nil")
	}
}

func TestGetDevices_InvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetDevices(t.Context(), "test_token", "12345", nil)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestGetDevices_WithLocationFilter_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/users/") || !strings.HasSuffix(r.URL.Path, "/devices") {
			t.Errorf("Expected path /users/{user_id}/devices, got %s", r.URL.Path)
		}

		// Verify location_id query parameter
		locationID := r.URL.Query().Get("location_id")
		if locationID != "loc1" {
			t.Errorf("Expected location_id=loc1, got %s", locationID)
		}

		// Verify headers
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept application/json, got %s", r.Header.Get("Accept"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("Expected Authorization header with Bearer token")
		}

		// Send success response with devices from location loc1
		response := DevicesResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []Device{
				{
					ID:       1,
					Type:     DeviceTypeSensor,
					Product:  "flume2",
					Location: &Location{ID: 201},
					User:     &User{ID: 12345},
				},
			},
			Count:         1,
			StatusCode:    200,
			StatusMessage: "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test GetDevices with LocationID filter
	params := DefaultDeviceListParams()
	params.LocationID = "loc1"
	devices, err := client.GetDevices(t.Context(), "test_access_token", "12345", &params)
	if err != nil {
		t.Fatalf("GetDevicesByLocation() error = %v", err)
	}

	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}

	if devices[0].ID != 1 {
		t.Errorf("Device[0].ID = %d, want 1", devices[0].ID)
	}

	if devices[0].Location == nil || devices[0].Location.ID != 201 {
		t.Errorf("Device[0].Location.ID = %v, want 201", devices[0].Location)
	}
}

func TestGetDevices_WithLocationFilter_APIError(t *testing.T) {
	// Create test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DevicesResponse{
			Success: false,
			Code:    404,
			Message: "Location not found",
			Data:    []Device{},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	params := DefaultDeviceListParams()
	params.LocationID = "invalid_loc"
	_, err := client.GetDevices(t.Context(), "test_token", "12345", &params)
	if err == nil {
		t.Fatal("Expected error for API error response, got nil")
	}

	expectedError := "API request failed: Location not found (code: 404)"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestGetDevices_WithLocationFilter_HTTPError(t *testing.T) {
	// Create test server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	params := DefaultDeviceListParams()
	params.LocationID = "loc1"
	_, err := client.GetDevices(t.Context(), "test_token", "12345", &params)
	if err == nil {
		t.Fatal("Expected error for HTTP error, got nil")
	}
}

func TestGetLocations_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/users/") || !strings.HasSuffix(r.URL.Path, "/locations") {
			t.Errorf("Expected path /users/{user_id}/locations, got %s", r.URL.Path)
		}

		// Verify headers
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept application/json, got %s", r.Header.Get("Accept"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("Expected Authorization header with Bearer token")
		}

		// Send success response
		response := LocationsResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []Location{
				{
					ID:          201,
					Name:        "Home",
					Address:     "123 Main St",
					City:        "San Francisco",
					State:       "CA",
					PostalCode:  "94102",
					Country:     "US",
					Timezone:    "America/Los_Angeles",
					UserID:      12345,
					UtilityType: "water",
				},
				{
					ID:          202,
					Name:        "Office",
					Address:     "456 Market St",
					City:        "San Francisco",
					State:       "CA",
					PostalCode:  "94103",
					Country:     "US",
					Timezone:    "America/Los_Angeles",
					UserID:      12345,
					UtilityType: "water",
				},
			},
			Count:         2,
			StatusCode:    200,
			StatusMessage: "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test GetLocations
	locations, err := client.GetLocations(t.Context(), "test_access_token", "12345", nil)
	if err != nil {
		t.Fatalf("GetLocations() error = %v", err)
	}

	if len(locations) != 2 {
		t.Errorf("Expected 2 locations, got %d", len(locations))
	}

	if locations[0].ID != 201 {
		t.Errorf("Location[0].ID = %d, want 201", locations[0].ID)
	}

	if locations[0].Name != "Home" {
		t.Errorf("Location[0].Name = %s, want Home", locations[0].Name)
	}

	if locations[1].ID != 202 {
		t.Errorf("Location[1].ID = %d, want 202", locations[1].ID)
	}

	if locations[1].Name != "Office" {
		t.Errorf("Location[1].Name = %s, want Office", locations[1].Name)
	}
}

func TestGetLocations_APIError(t *testing.T) {
	// Create test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := LocationsResponse{
			Success: false,
			Code:    403,
			Message: "Forbidden",
			Data:    []Location{},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetLocations(t.Context(), "invalid_token", "12345", nil)
	if err == nil {
		t.Fatal("Expected error for API error response, got nil")
	}

	expectedError := "API request failed: Forbidden (code: 403)"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestGetLocations_HTTPError(t *testing.T) {
	// Create test server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad Request"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetLocations(t.Context(), "test_token", "12345", nil)
	if err == nil {
		t.Fatal("Expected error for HTTP error, got nil")
	}
}

func TestGetLocations_InvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetLocations(t.Context(), "test_token", "12345", nil)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestQueryDevice_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/devices/") || !strings.HasSuffix(r.URL.Path, "/query") {
			t.Errorf("Expected path /users/{user_id}/devices/{device_id}/query, got %s", r.URL.Path)
		}

		// Verify headers
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept application/json, got %s", r.Header.Get("Accept"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("Expected Authorization header with Bearer token")
		}

		// Verify request body
		var queryReq QueryRequest
		if err := json.NewDecoder(r.Body).Decode(&queryReq); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if len(queryReq.Queries) != 2 {
			t.Errorf("Expected 2 queries, got %d", len(queryReq.Queries))
		}

		// Send success response in actual Flume API format:
		// data is an array with one object, keyed by request_id
		response := map[string]interface{}{
			"success": true,
			"code":    200,
			"message": "Success",
			"data": []map[string]interface{}{
				{
					"daily_usage": []map[string]interface{}{
						{"datetime": "2025-11-01 00:00:00", "value": 123.45},
						{"datetime": "2025-11-02 00:00:00", "value": 156.78},
					},
					"monthly_usage": []map[string]interface{}{
						{"datetime": "2025-11-01 00:00:00", "value": 3450.12},
					},
				},
			},
			"status_code":    200,
			"status_message": "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test QueryDevice
	queries := []Query{
		{
			RequestID:     "daily_usage",
			Bucket:        "DAY",
			SinceDatetime: mustTime(t, "2025-11-01 00:00:00"),
			UntilDatetime: mustTime(t, "2025-11-03 00:00:00"),
		},
		{
			RequestID:     "monthly_usage",
			Bucket:        "MON",
			SinceDatetime: mustTime(t, "2025-11-01 00:00:00"),
		},
	}

	results, err := client.QueryDevice(t.Context(), "test_access_token", "12345", "device1", queries)
	if err != nil {
		t.Fatalf("QueryDevice() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// Build a map for order-independent assertions (map iteration is unordered)
	resultMap := make(map[string]QueryResult)
	for _, r := range results {
		resultMap[r.RequestID] = r
	}

	daily, ok := resultMap["daily_usage"]
	if !ok {
		t.Fatal("Expected result with RequestID 'daily_usage'")
	}
	if len(daily.Data) != 2 {
		t.Errorf("Expected 2 data points for daily_usage, got %d", len(daily.Data))
	}
	if daily.Data[0].Value != 123.45 {
		t.Errorf("daily_usage Data[0].Value = %f, want 123.45", daily.Data[0].Value)
	}
	if daily.Bucket != "DAY" {
		t.Errorf("daily_usage Bucket = %q, want %q", daily.Bucket, "DAY")
	}

	monthly, ok := resultMap["monthly_usage"]
	if !ok {
		t.Fatal("Expected result with RequestID 'monthly_usage'")
	}
	if len(monthly.Data) != 1 {
		t.Errorf("Expected 1 data point for monthly_usage, got %d", len(monthly.Data))
	}
	if monthly.Bucket != "MON" {
		t.Errorf("monthly_usage Bucket = %q, want %q", monthly.Bucket, "MON")
	}
}

func TestQueryDevice_APIError(t *testing.T) {
	// Create test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"success": false,
			"code":    400,
			"message": "Invalid query parameters",
			"data":    []interface{}{},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	queries := []Query{
		{
			RequestID:     "test",
			Bucket:        "DAY",
			SinceDatetime: mustTime(t, "2025-11-01 00:00:00"),
		},
	}

	_, err := client.QueryDevice(t.Context(), "test_token", "12345", "device1", queries)
	if err == nil {
		t.Fatal("Expected error for API error response, got nil")
	}

	expectedError := "API request failed: Invalid query parameters (code: 400)"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestQueryDevice_HTTPError(t *testing.T) {
	// Create test server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	queries := []Query{
		{
			RequestID:     "test",
			Bucket:        "DAY",
			SinceDatetime: mustTime(t, "2025-11-01 00:00:00"),
		},
	}

	_, err := client.QueryDevice(t.Context(), "invalid_token", "12345", "device1", queries)
	if err == nil {
		t.Fatal("Expected error for HTTP error, got nil")
	}
}

func TestQueryDevice_InvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json response"))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	queries := []Query{
		{
			RequestID:     "test",
			Bucket:        "DAY",
			SinceDatetime: mustTime(t, "2025-11-01 00:00:00"),
		},
	}

	_, err := client.QueryDevice(t.Context(), "test_token", "12345", "device1", queries)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestQueryDevice_EmptyQueries(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"success":        true,
			"code":           200,
			"message":        "Success",
			"data":           []interface{}{},
			"status_code":    200,
			"status_message": "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	results, err := client.QueryDevice(t.Context(), "test_token", "12345", "device1", []Query{})
	if err != nil {
		t.Fatalf("QueryDevice() with empty queries error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}
