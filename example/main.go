package main

import (
	"context"
	"fmt"
	"log"
	"time"

	flumewater "github.com/mdgreenwald/lib-flume-water"
)

func main() {
	fmt.Println("=== Flume Water API Manual Test ===")
	fmt.Printf("Library Version: %s\n", flumewater.Version)
	fmt.Println()

	ctx := context.Background()

	// Create a new client
	client := flumewater.NewClient()

	// Authenticate using .env file from current directory
	fmt.Println("1. Authenticating...")
	authResult, err := client.AuthenticateFromEnv(ctx, ".env")
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	fmt.Printf("✓ Authentication successful!\n")
	fmt.Printf("  User ID: %s\n\n", authResult.UserID)

	// Get locations
	fmt.Println("2. Fetching locations...")
	locations, err := client.GetLocations(ctx, authResult.AccessToken, authResult.UserID, nil)
	if err != nil {
		log.Fatalf("Failed to get locations: %v", err)
	}
	fmt.Printf("✓ Found %d location(s):\n", len(locations))
	for i, loc := range locations {
		fmt.Printf("  [%d] %s (ID: %s)\n", i+1, loc.Name, loc.ID)
		fmt.Printf("      Address: %s, %s, %s %s\n", loc.Address, loc.City, loc.State, loc.PostalCode)
		fmt.Printf("      Timezone: %s\n", loc.Timezone)
	}
	fmt.Println()

	// Get all devices
	fmt.Println("3. Fetching all devices...")
	devices, err := client.GetDevices(ctx, authResult.AccessToken, authResult.UserID, nil)
	if err != nil {
		log.Fatalf("Failed to get devices: %v", err)
	}
	fmt.Printf("✓ Found %d device(s):\n", len(devices))
	for i, dev := range devices {
		fmt.Printf("  [%d] %s (ID: %s)\n", i+1, dev.Type, dev.ID)
		fmt.Printf("      Product: %s\n", dev.Product)
		if dev.Location != nil {
			fmt.Printf("      Location ID: %s\n", dev.Location.ID)
		}
		fmt.Printf("      Last Seen: %s\n", dev.LastSeen)
	}
	fmt.Println()

	// Get devices by location (if we have locations)
	if len(locations) > 0 {
		firstLocationID := locations[0].ID.String()
		fmt.Printf("4. Fetching devices for location '%s' (ID: %s)...\n", locations[0].Name, firstLocationID)
		params := flumewater.DefaultDeviceListParams()
		params.LocationID = firstLocationID
		locationDevices, err := client.GetDevices(ctx, authResult.AccessToken, authResult.UserID, &params)
		if err != nil {
			log.Fatalf("Failed to get devices by location: %v", err)
		}
		fmt.Printf("✓ Found %d device(s) at this location:\n", len(locationDevices))
		for i, dev := range locationDevices {
			fmt.Printf("  [%d] %s (ID: %s)\n", i+1, dev.Type, dev.ID)
		}
		fmt.Println()
	}

	// Query device for water usage (if we have devices)
	if len(devices) > 0 {
		// Use the first sensor device we find
		var sensorDevice *flumewater.Device
		for i := range devices {
			if devices[i].Type == flumewater.DeviceTypeSensor {
				sensorDevice = &devices[i]
				break
			}
		}

		if sensorDevice != nil {
			deviceID := sensorDevice.ID.String()
			fmt.Printf("5. Querying water usage for device %s...\n", deviceID)

			// Query last 30 days of daily usage
			now := time.Now()
			since := now.AddDate(0, 0, -30)
			queries := []flumewater.Query{
				{
					RequestID:     "last_30_days",
					Bucket:        "DAY",
					SinceDatetime: flumewater.Time{Time: since},
					UntilDatetime: flumewater.Time{Time: now},
				},
			}

			results, err := client.QueryDevice(ctx, authResult.AccessToken, authResult.UserID, deviceID, queries)
			if err != nil {
				log.Printf("Warning: Failed to query device: %v", err)
			} else {
				fmt.Printf("✓ Query results:\n")
				for _, result := range results {
					fmt.Printf("  Request ID: %s\n", result.RequestID)
					if len(result.Data) > 0 {
						fmt.Printf("  Found %d data points:\n", len(result.Data))
						for _, dataPoint := range result.Data {
							fmt.Printf("    - %s: %9.2f gallons\n", dataPoint.Datetime.Format("2006-01-02"), dataPoint.Value)
						}
					} else {
						fmt.Printf("  No data points found for this period\n")
					}
				}
				fmt.Println()
			}
		} else {
			fmt.Println("5. Skipping query test - no sensor devices found")
			fmt.Println()
		}
	}

	fmt.Println("=== All tests completed successfully! ===")
}
