# Manual Testing Example

This example program demonstrates how to use the Flume Water API library and allows you to manually test it with real API credentials.

## Setup

1. **Copy the example .env file and add your credentials:**
   ```bash
   cd /Users/mdgreenwald/dev/lib-flume-water
   cp .env.example .env
   ```

2. **Edit the .env file with your Flume Water API credentials:**
   ```bash
   # Open .env in your editor and fill in:
   FLUME_CLIENT_ID=your_actual_client_id
   FLUME_CLIENT_SECRET=your_actual_client_secret
   FLUME_USER_EMAIL=your_email@example.com
   FLUME_USER_PASSWORD=your_actual_password
   ```

   > **Note:** You can get your API credentials from the Flume Water portal. The .env file is already in .gitignore so your credentials won't be committed.

## Running the Manual Test

From the library root directory, run:

```bash
go run example/main.go
```

## What the Test Does

The example program will:

1. ✓ Authenticate with the Flume API using your credentials
2. ✓ Fetch all locations associated with your account
3. ✓ Fetch all devices associated with your account
4. ✓ Fetch devices filtered by the first location (if you have locations)

## Expected Output

```
=== Flume Water API Manual Test ===

1. Authenticating...
✓ Authentication successful!
  User ID: 12345
  Access Token: eyJhbGciOiJIUzI1NiIs...

2. Fetching locations...
✓ Found 1 location(s):
  [1] Home (ID: loc123)
      Address: 123 Main St, San Francisco, CA 94102
      Timezone: America/Los_Angeles

3. Fetching all devices...
✓ Found 2 device(s):
  [1] Bridge (ID: dev123)
      Product ID: prod123
      Location ID: loc123
      Last Seen: 2024-01-15T10:30:00Z
  [2] Sensor (ID: dev456)
      Product ID: prod456
      Location ID: loc123
      Last Seen: 2024-01-15T10:31:00Z

4. Fetching devices for location 'Home' (ID: loc123)...
✓ Found 2 device(s) at this location:
  [1] Bridge (ID: dev123)
  [2] Sensor (ID: dev456)

=== All tests completed successfully! ===
```

## Troubleshooting

- **Authentication failed:** Check your credentials in the .env file
- **No devices found:** Make sure you have Flume devices registered to your account
- **Connection errors:** Verify your internet connection and that the Flume API is accessible
