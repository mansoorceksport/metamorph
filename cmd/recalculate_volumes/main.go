package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Simplified domain structs for the script
type Schedule struct {
	ID        string    `bson:"_id"`
	MemberID  string    `bson:"member_id"`
	TenantID  string    `bson:"tenant_id"`
	Status    string    `bson:"status"`
	StartTime time.Time `bson:"start_time"`
}

type SetLog struct {
	ID         string  `bson:"_id"`
	ScheduleID string  `bson:"schedule_id"`
	MemberID   string  `bson:"member_id"`
	ExerciseID string  `bson:"exercise_id"`
	Weight     float64 `bson:"weight"`
	Reps       int     `bson:"reps"`
	Completed  bool    `bson:"completed"`
}

type DailyVolume struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	TenantID      string             `bson:"tenant_id"`
	MemberID      string             `bson:"member_id"`
	ScheduleID    string             `bson:"schedule_id"`
	Date          time.Time          `bson:"date"`
	TotalVolume   float64            `bson:"total_volume"`
	TotalSets     int                `bson:"total_sets"`
	TotalReps     int                `bson:"total_reps"`
	TotalWeight   float64            `bson:"total_weight"`
	ExerciseCount int                `bson:"exercise_count"`
	CreatedAt     time.Time          `bson:"created_at"`
}

func main() {
	// Command line flags
	memberID := flag.String("member", "", "Member ID to recalculate volumes for (required)")
	mongoURI := flag.String("mongo", "mongodb://localhost:27017", "MongoDB connection URI")
	dbName := flag.String("db", "homgym", "Database name")
	dryRun := flag.Bool("dry-run", false, "Show what would be done without making changes")
	flag.Parse()

	if *memberID == "" {
		fmt.Println("Usage: recalculate_volumes -member <MEMBER_ID> [-mongo <URI>] [-db <NAME>] [-dry-run]")
		fmt.Println("\nThis script recalculates workout volume aggregations for a member.")
		fmt.Println("It finds all completed schedules and regenerates the daily_volumes collection.")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(*dbName)
	schedulesCol := db.Collection("schedules")
	setLogsCol := db.Collection("set_logs")
	volumesCol := db.Collection("daily_volumes")

	fmt.Printf("🔍 Finding completed schedules for member: %s\n", *memberID)

	// Find all completed schedules for this member
	cursor, err := schedulesCol.Find(ctx, bson.M{
		"member_id": *memberID,
		"status":    "Completed", // Match domain.ScheduleStatusCompleted
	})
	if err != nil {
		log.Fatalf("Failed to query schedules: %v", err)
	}
	defer cursor.Close(ctx)

	var schedules []Schedule
	if err := cursor.All(ctx, &schedules); err != nil {
		log.Fatalf("Failed to decode schedules: %v", err)
	}

	fmt.Printf("📋 Found %d completed schedules\n\n", len(schedules))

	if len(schedules) == 0 {
		fmt.Println("No completed schedules found for this member.")
		os.Exit(0)
	}

	// Delete existing volumes for this member
	if !*dryRun {
		result, err := volumesCol.DeleteMany(ctx, bson.M{"member_id": *memberID})
		if err != nil {
			log.Fatalf("Failed to delete existing volumes: %v", err)
		}
		fmt.Printf("🗑️  Deleted %d existing volume records\n\n", result.DeletedCount)
	} else {
		fmt.Println("🏃 DRY RUN - Would delete existing volume records\n")
	}

	// Process each schedule
	var totalVolumesCreated int
	var grandTotalVolume float64

	for _, schedule := range schedules {
		fmt.Printf("📅 Processing schedule: %s (Date: %s)\n", schedule.ID, schedule.StartTime.Format("2006-01-02"))

		// Fetch set logs for this schedule
		setCursor, err := setLogsCol.Find(ctx, bson.M{"schedule_id": schedule.ID})
		if err != nil {
			fmt.Printf("   ⚠️  Failed to fetch set logs: %v\n", err)
			continue
		}

		var setLogs []SetLog
		if err := setCursor.All(ctx, &setLogs); err != nil {
			fmt.Printf("   ⚠️  Failed to decode set logs: %v\n", err)
			setCursor.Close(ctx)
			continue
		}
		setCursor.Close(ctx)

		// Calculate aggregates
		var totalVolume float64
		var totalWeight float64
		var totalReps int
		var totalSets int
		exerciseIDs := make(map[string]bool)

		for _, log := range setLogs {
			if log.Completed && log.Weight > 0 && log.Reps > 0 {
				volume := log.Weight * float64(log.Reps)
				totalVolume += volume
				totalWeight += log.Weight
				totalReps += log.Reps
				totalSets++
				exerciseIDs[log.ExerciseID] = true
			}
		}

		fmt.Printf("   📊 Sets: %d, Reps: %d, Volume: %.0f kg\n", totalSets, totalReps, totalVolume)

		if totalSets == 0 {
			fmt.Printf("   ⏭️  No completed sets, skipping\n\n")
			continue
		}

		// Create DailyVolume record
		dailyVolume := DailyVolume{
			TenantID:      schedule.TenantID,
			MemberID:      schedule.MemberID,
			ScheduleID:    schedule.ID,
			Date:          schedule.StartTime,
			TotalVolume:   totalVolume,
			TotalSets:     totalSets,
			TotalReps:     totalReps,
			TotalWeight:   totalWeight,
			ExerciseCount: len(exerciseIDs),
			CreatedAt:     time.Now(),
		}

		if !*dryRun {
			_, err := volumesCol.InsertOne(ctx, dailyVolume)
			if err != nil {
				fmt.Printf("   ❌ Failed to insert volume: %v\n\n", err)
				continue
			}
			fmt.Printf("   ✅ Created volume record\n\n")
		} else {
			fmt.Printf("   🏃 DRY RUN - Would create volume record\n\n")
		}

		totalVolumesCreated++
		grandTotalVolume += totalVolume
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("✅ Summary:\n")
	fmt.Printf("   Schedules processed: %d\n", len(schedules))
	fmt.Printf("   Volume records created: %d\n", totalVolumesCreated)
	fmt.Printf("   Grand total volume: %.0f kg\n", grandTotalVolume)

	if *dryRun {
		fmt.Println("\n⚠️  This was a dry run. No changes were made.")
		fmt.Println("   Run without -dry-run to apply changes.")
	}
}
