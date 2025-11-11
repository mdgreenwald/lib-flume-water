package flumewater

import (
	"encoding/json"
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

// createTestJWT creates a test JWT token with the given user_id
func createTestJWT(userID string) (string, error) {
	token := jwt.New()
	if err := token.Set("user_id", userID); err != nil {
		return "", err
	}
	if err := token.Set("type", "user"); err != nil {
		return "", err
	}
	if err := token.Set(jwt.IssuedAtKey, time.Now().Unix()); err != nil {
		return "", err
	}
	if err := token.Set(jwt.ExpirationKey, time.Now().Add(24*time.Hour).Unix()); err != nil {
		return "", err
	}

	// For testing, we'll create an unsigned token (alg: none)
	serialized, err := jwt.NewSerializer().Serialize(token)
	if err != nil {
		return "", err
	}

	return string(serialized), nil
}

// createTestJWTWithNumericUserID creates a test JWT token with a numeric user_id
func createTestJWTWithNumericUserID(userID int64) (string, error) {
	token := jwt.New()
	if err := token.Set("user_id", userID); err != nil {
		return "", err
	}
	if err := token.Set("type", "user"); err != nil {
		return "", err
	}
	if err := token.Set(jwt.IssuedAtKey, time.Now().Unix()); err != nil {
		return "", err
	}
	if err := token.Set(jwt.ExpirationKey, time.Now().Add(24*time.Hour).Unix()); err != nil {
		return "", err
	}

	serialized, err := jwt.NewSerializer().Serialize(token)
	if err != nil {
		return "", err
	}

	return string(serialized), nil
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version is empty")
	}

	// Version should follow semantic versioning format (X.Y.Z)
	if !strings.Contains(Version, ".") {
		t.Errorf("Version %s does not appear to follow semantic versioning", Version)
	}

	// Current version should be 0.1.0
	if Version != "0.1.0" {
		t.Errorf("Version = %s, want 0.1.0", Version)
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
	testAccessToken, err := createTestJWT(testUserID)
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
					UserID:       testUserID,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test authentication
	result, err := client.Authenticate("test_client_id", "test_client_secret", "test@example.com", "test_password")
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
}

func TestAuthenticate_NumericUserID(t *testing.T) {
	// Create test JWT token with numeric user_id
	testUserID := int64(12345)
	testAccessToken, err := createTestJWTWithNumericUserID(testUserID)
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
					UserID:       testUserID,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	result, err := client.Authenticate("test_client_id", "test_client_secret", "test@example.com", "test_password")
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
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.Authenticate("bad_client_id", "bad_client_secret", "test@example.com", "wrong_password")
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
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.Authenticate("test_client_id", "test_client_secret", "test@example.com", "test_password")
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
		if _, err := w.Write([]byte("invalid json")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.Authenticate("test_client_id", "test_client_secret", "test@example.com", "test_password")
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
					UserID:       "12345",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.Authenticate("test_client_id", "test_client_secret", "test@example.com", "test_password")
	if err == nil {
		t.Fatal("Expected error for invalid JWT, got nil")
	}
}

func TestAuthenticate_MissingUserID(t *testing.T) {
	// Create JWT without user_id claim
	token := jwt.New()
	if err := token.Set("type", "user"); err != nil {
		t.Fatalf("failed to set type claim: %v", err)
	}
	if err := token.Set(jwt.IssuedAtKey, time.Now().Unix()); err != nil {
		t.Fatalf("failed to set issued at claim: %v", err)
	}
	if err := token.Set(jwt.ExpirationKey, time.Now().Add(24*time.Hour).Unix()); err != nil {
		t.Fatalf("failed to set expiration claim: %v", err)
	}

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
					UserID:       "12345",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err = client.Authenticate("test_client_id", "test_client_secret", "test@example.com", "test_password")
	if err == nil {
		t.Fatal("Expected error for missing user_id in JWT, got nil")
	}

	expectedError := "user_id not found in JWT token"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Error = %s, want it to contain %s", err.Error(), expectedError)
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
	os.Unsetenv("FLUME_CLIENT_ID")
	os.Unsetenv("FLUME_CLIENT_SECRET")
	os.Unsetenv("FLUME_USER_EMAIL")
	os.Unsetenv("FLUME_USER_PASSWORD")

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

func TestLoadCredentialsFromEnv_MissingFile(t *testing.T) {
	// Clear any existing environment variables
	os.Unsetenv("FLUME_CLIENT_ID")
	os.Unsetenv("FLUME_CLIENT_SECRET")
	os.Unsetenv("FLUME_USER_EMAIL")
	os.Unsetenv("FLUME_USER_PASSWORD")

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

	os.Unsetenv("FLUME_CLIENT_ID")

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

	os.Unsetenv("FLUME_CLIENT_SECRET")

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

	os.Unsetenv("FLUME_USER_EMAIL")

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

	os.Unsetenv("FLUME_USER_PASSWORD")

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
	testAccessToken, err := createTestJWT(testUserID)
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
	os.Unsetenv("FLUME_CLIENT_ID")
	os.Unsetenv("FLUME_CLIENT_SECRET")
	os.Unsetenv("FLUME_USER_EMAIL")
	os.Unsetenv("FLUME_USER_PASSWORD")

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
					UserID:       testUserID,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test AuthenticateFromEnv
	result, err := client.AuthenticateFromEnv(envPath)
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

	os.Unsetenv("FLUME_CLIENT_ID")
	os.Unsetenv("FLUME_CLIENT_SECRET")
	os.Unsetenv("FLUME_USER_EMAIL")
	os.Unsetenv("FLUME_USER_PASSWORD")

	_, err := client.AuthenticateFromEnv("/nonexistent/path/.env")
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
					ID:         float64(1),
					Type:       2,
					ProductID:  float64(101),
					LocationID: float64(201),
					UserID:     float64(12345),
				},
				{
					ID:         float64(2),
					Type:       1,
					ProductID:  float64(102),
					LocationID: float64(201),
					UserID:     float64(12345),
				},
			},
			Count:         2,
			StatusCode:    200,
			StatusMessage: "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test GetDevices
	devices, err := client.GetDevices("test_access_token", "12345")
	if err != nil {
		t.Fatalf("GetDevices() error = %v", err)
	}

	if len(devices) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(devices))
	}

	if devices[0].GetIDString() != "1" {
		t.Errorf("Device[0].ID = %s, want 1", devices[0].GetIDString())
	}

	if devices[0].Type != 2 {
		t.Errorf("Device[0].Type = %d, want 2", devices[0].Type)
	}

	if devices[1].GetIDString() != "2" {
		t.Errorf("Device[1].ID = %s, want 2", devices[1].GetIDString())
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
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetDevices("invalid_token", "12345")
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
		if _, err := w.Write([]byte("Internal Server Error")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetDevices("test_token", "12345")
	if err == nil {
		t.Fatal("Expected error for HTTP error, got nil")
	}
}

func TestGetDevices_InvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("invalid json")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetDevices("test_token", "12345")
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestGetDevicesByLocation_Success(t *testing.T) {
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
					ID:         float64(1),
					Type:       2,
					ProductID:  float64(101),
					LocationID: float64(201),
					UserID:     float64(12345),
				},
			},
			Count:         1,
			StatusCode:    200,
			StatusMessage: "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test GetDevicesByLocation
	devices, err := client.GetDevicesByLocation("test_access_token", "12345", "loc1")
	if err != nil {
		t.Fatalf("GetDevicesByLocation() error = %v", err)
	}

	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}

	if devices[0].GetIDString() != "1" {
		t.Errorf("Device[0].ID = %s, want 1", devices[0].GetIDString())
	}

	if devices[0].GetLocationIDString() != "201" {
		t.Errorf("Device[0].LocationID = %s, want 201", devices[0].GetLocationIDString())
	}
}

func TestGetDevicesByLocation_APIError(t *testing.T) {
	// Create test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DevicesResponse{
			Success: false,
			Code:    404,
			Message: "Location not found",
			Data:    []Device{},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetDevicesByLocation("test_token", "12345", "invalid_loc")
	if err == nil {
		t.Fatal("Expected error for API error response, got nil")
	}

	expectedError := "API request failed: Location not found (code: 404)"
	if err.Error() != expectedError {
		t.Errorf("Error = %s, want %s", err.Error(), expectedError)
	}
}

func TestGetDevicesByLocation_HTTPError(t *testing.T) {
	// Create test server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("Internal Server Error")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetDevicesByLocation("test_token", "12345", "loc1")
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
					ID:          float64(201),
					Name:        "Home",
					Address:     "123 Main St",
					City:        "San Francisco",
					State:       "CA",
					PostalCode:  "94102",
					Country:     "US",
					Timezone:    "America/Los_Angeles",
					UserID:      float64(12345),
					UtilityType: "water",
				},
				{
					ID:          float64(202),
					Name:        "Office",
					Address:     "456 Market St",
					City:        "San Francisco",
					State:       "CA",
					PostalCode:  "94103",
					Country:     "US",
					Timezone:    "America/Los_Angeles",
					UserID:      float64(12345),
					UtilityType: "water",
				},
			},
			Count:         2,
			StatusCode:    200,
			StatusMessage: "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	// Test GetLocations
	locations, err := client.GetLocations("test_access_token", "12345")
	if err != nil {
		t.Fatalf("GetLocations() error = %v", err)
	}

	if len(locations) != 2 {
		t.Errorf("Expected 2 locations, got %d", len(locations))
	}

	if locations[0].GetIDString() != "201" {
		t.Errorf("Location[0].ID = %s, want 201", locations[0].GetIDString())
	}

	if locations[0].Name != "Home" {
		t.Errorf("Location[0].Name = %s, want Home", locations[0].Name)
	}

	if locations[1].GetIDString() != "202" {
		t.Errorf("Location[1].ID = %s, want 202", locations[1].GetIDString())
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
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetLocations("invalid_token", "12345")
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
		if _, err := w.Write([]byte("Bad Request")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetLocations("test_token", "12345")
	if err == nil {
		t.Fatal("Expected error for HTTP error, got nil")
	}
}

func TestGetLocations_InvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("not valid json")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	_, err := client.GetLocations("test_token", "12345")
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

		// Send success response
		response := QueryResponse{
			Success: true,
			Code:    200,
			Message: "Success",
			Data: []QueryResult{
				{
					RequestID: "daily_usage",
					Bucket:    "DAY",
					Data: []QueryData{
						{Datetime: "2025-11-01 00:00:00", Value: 123.45},
						{Datetime: "2025-11-02 00:00:00", Value: 156.78},
					},
				},
				{
					RequestID: "monthly_usage",
					Bucket:    "MON",
					Data: []QueryData{
						{Datetime: "2025-11-01 00:00:00", Value: 3450.12},
					},
				},
			},
			StatusCode:    200,
			StatusMessage: "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
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
			SinceDatetime: "2025-11-01 00:00:00",
			UntilDatetime: "2025-11-03 00:00:00",
		},
		{
			RequestID:     "monthly_usage",
			Bucket:        "MON",
			SinceDatetime: "2025-11-01 00:00:00",
		},
	}

	results, err := client.QueryDevice("test_access_token", "12345", "device1", queries)
	if err != nil {
		t.Fatalf("QueryDevice() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if results[0].RequestID != "daily_usage" {
		t.Errorf("Result[0].RequestID = %s, want daily_usage", results[0].RequestID)
	}

	if results[0].Bucket != "DAY" {
		t.Errorf("Result[0].Bucket = %s, want DAY", results[0].Bucket)
	}

	if len(results[0].Data) != 2 {
		t.Errorf("Expected 2 data points in result[0], got %d", len(results[0].Data))
	}

	if results[0].Data[0].Value != 123.45 {
		t.Errorf("Result[0].Data[0].Value = %f, want 123.45", results[0].Data[0].Value)
	}

	if results[1].RequestID != "monthly_usage" {
		t.Errorf("Result[1].RequestID = %s, want monthly_usage", results[1].RequestID)
	}
}

func TestQueryDevice_APIError(t *testing.T) {
	// Create test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := QueryResponse{
			Success: false,
			Code:    400,
			Message: "Invalid query parameters",
			Data:    []QueryResult{},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
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
			SinceDatetime: "invalid_date",
		},
	}

	_, err := client.QueryDevice("test_token", "12345", "device1", queries)
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
		if _, err := w.Write([]byte("Unauthorized")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
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
			SinceDatetime: "2025-11-01 00:00:00",
		},
	}

	_, err := client.QueryDevice("invalid_token", "12345", "device1", queries)
	if err == nil {
		t.Fatal("Expected error for HTTP error, got nil")
	}
}

func TestQueryDevice_InvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("invalid json response")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
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
			SinceDatetime: "2025-11-01 00:00:00",
		},
	}

	_, err := client.QueryDevice("test_token", "12345", "device1", queries)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestQueryDevice_EmptyQueries(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := QueryResponse{
			Success:       true,
			Code:          200,
			Message:       "Success",
			Data:          []QueryResult{},
			StatusCode:    200,
			StatusMessage: "OK",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    server.URL,
	}

	results, err := client.QueryDevice("test_token", "12345", "device1", []Query{})
	if err != nil {
		t.Fatalf("QueryDevice() with empty queries error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}
