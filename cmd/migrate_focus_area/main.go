package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Pattern matching rules for inferring focus area from session_goal
var focusPatterns = map[string]*regexp.Regexp{
	"LEG_DAY":    regexp.MustCompile(`(?i)(leg|squat|lunge|calf|quad|hamstring|glute)`),
	"UPPER_BODY": regexp.MustCompile(`(?i)(upper|shoulder|arm|bicep|tricep)`),
	"BACK_DAY":   regexp.MustCompile(`(?i)(back|lat|row|pull|deadlift)`),
	"CHEST_DAY":  regexp.MustCompile(`(?i)(chest|bench|push.?up|pec)`),
	"FULL_BODY":  regexp.MustCompile(`(?i)(full.?body|total.?body|circuit)`),
	"FUNCTIONAL": regexp.MustCompile(`(?i)(functional|cardio|hiit|conditioning|endurance)`),
	"CORE":       regexp.MustCompile(`(?i)(core|abs|plank|crunch)`),
}

func inferFocusArea(sessionGoal string) string {
	if sessionGoal == "" {
		return ""
	}

	for focus, pattern := range focusPatterns {
		if pattern.MatchString(sessionGoal) {
			return focus
		}
	}
	return "" // No match, leave empty
}

func main() {
	// Parse command line flags
	mongoURI := flag.String("mongo", "", "MongoDB URI (required)")
	dbName := flag.String("db", "homgym", "Database name")
	dryRun := flag.Bool("dry-run", true, "Preview changes without writing (default: true)")
	flag.Parse()

	if *mongoURI == "" {
		// Try environment variable
		*mongoURI = os.Getenv("MONGO_URI")
		if *mongoURI == "" {
			log.Fatal("MongoDB URI is required. Use -mongo flag or MONGO_URI env var")
		}
	}

	ctx := context.Background()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(*dbName)
	schedulesCol := db.Collection("schedules")
	volumesCol := db.Collection("daily_volumes")

	fmt.Println("=== Focus Area Migration ===")
	fmt.Printf("Database: %s\n", *dbName)
	fmt.Printf("Dry Run: %v\n\n", *dryRun)

	// --- Step 1: Migrate Schedules ---
	fmt.Println("--- Migrating Schedules ---")

	// Find schedules with session_goal but no focus_area
	filter := bson.M{
		"session_goal": bson.M{"$exists": true, "$ne": ""},
		"$or": []bson.M{
			{"focus_area": bson.M{"$exists": false}},
			{"focus_area": ""},
		},
	}

	cursor, err := schedulesCol.Find(ctx, filter)
	if err != nil {
		log.Fatalf("Failed to query schedules: %v", err)
	}
	defer cursor.Close(ctx)

	var scheduleUpdates, scheduleMatched, scheduleSkipped int
	for cursor.Next(ctx) {
		scheduleMatched++
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		sessionGoal := doc["session_goal"].(string)
		inferredFocus := inferFocusArea(sessionGoal)

		if inferredFocus == "" {
			scheduleSkipped++
			continue
		}

		scheduleID := doc["_id"]
		fmt.Printf("  Schedule %v: \"%s\" -> %s\n",
			scheduleID,
			truncate(sessionGoal, 40),
			inferredFocus,
		)

		if !*dryRun {
			_, err := schedulesCol.UpdateByID(ctx, scheduleID, bson.M{
				"$set": bson.M{"focus_area": inferredFocus},
			})
			if err != nil {
				log.Printf("  ERROR updating schedule %v: %v", scheduleID, err)
				continue
			}
		}
		scheduleUpdates++
	}

	fmt.Printf("\nSchedules: %d matched, %d updated, %d skipped (no pattern match)\n\n",
		scheduleMatched, scheduleUpdates, scheduleSkipped)

	// --- Step 2: Migrate Daily Volumes ---
	fmt.Println("--- Migrating Daily Volumes ---")

	// For each schedule that has focus_area, update the corresponding volume
	scheduleCursor, err := schedulesCol.Find(ctx, bson.M{
		"focus_area": bson.M{"$exists": true, "$ne": ""},
	})
	if err != nil {
		log.Fatalf("Failed to query schedules with focus_area: %v", err)
	}
	defer scheduleCursor.Close(ctx)

	var volumeUpdates int
	for scheduleCursor.Next(ctx) {
		var doc bson.M
		if err := scheduleCursor.Decode(&doc); err != nil {
			continue
		}

		scheduleID := doc["_id"]
		focusArea := doc["focus_area"].(string)

		// Find and update the corresponding volume
		volumeFilter := bson.M{
			"schedule_id": scheduleID,
			"$or": []bson.M{
				{"focus_area": bson.M{"$exists": false}},
				{"focus_area": ""},
			},
		}

		if *dryRun {
			count, _ := volumesCol.CountDocuments(ctx, volumeFilter)
			if count > 0 {
				fmt.Printf("  Volume for schedule %v -> %s\n", scheduleID, focusArea)
				volumeUpdates += int(count)
			}
		} else {
			result, err := volumesCol.UpdateMany(ctx, volumeFilter, bson.M{
				"$set": bson.M{"focus_area": focusArea},
			})
			if err != nil {
				log.Printf("  ERROR updating volume for schedule %v: %v", scheduleID, err)
				continue
			}
			volumeUpdates += int(result.ModifiedCount)
		}
	}

	fmt.Printf("\nVolumes: %d updated\n\n", volumeUpdates)

	// --- Summary ---
	fmt.Println("=== Migration Summary ===")
	fmt.Printf("Schedules updated: %d\n", scheduleUpdates)
	fmt.Printf("Volumes updated: %d\n", volumeUpdates)

	if *dryRun {
		fmt.Println("\n⚠️  This was a DRY RUN. No data was modified.")
		fmt.Println("Run with -dry-run=false to apply changes.")
	} else {
		fmt.Println("\n✅ Migration complete!")
	}
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
