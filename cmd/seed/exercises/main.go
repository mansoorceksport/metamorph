package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mansoorceksport/metamorph/internal/config"
	"github.com/mansoorceksport/metamorph/internal/domain"
	"github.com/mansoorceksport/metamorph/internal/repository"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Connect to Mongo
	// Hardcoding for seed script or loading from env.
	cfg, err := config.Load()
	if err != nil {
		// Log warning but proceed with defaults if possible OR fatal?
		// Since Validate runs in Load, it might fail if env missing.
		// For seeding usually we want it to work.
		log.Printf("Config load error: %v, proceeding (might fail if mongo defaults incorrect)", err)
		// Or just ignore if it returns partial config?
		// Looking at config.go, Load returns defaults BEFORE validation, but validation error aborts return.
		// Wait, Load() returns (*Config, error). If error, config might be nil.
		// Let's assume we need to fix env vars if it fails.
		// But for now, let's just log.Fatalf
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoDB.URI))
	if err != nil {
		log.Fatalf("Failed to connect to Mongo: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(cfg.MongoDB.Database)
	repo := repository.NewMongoExerciseRepository(db)

	exercises := []domain.Exercise{
		// Legs
		{Name: "Barbell Squat", MuscleGroup: "Legs", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=SW_C1A-rejs"},
		{Name: "Leg Press", MuscleGroup: "Legs", Equipment: "Machine", VideoURL: "https://www.youtube.com/watch?v=IZxyjW7MPJQ"},
		{Name: "Walking Lunge", MuscleGroup: "Legs", Equipment: "Bodyweight/Dumbbell", VideoURL: "https://www.youtube.com/watch?v=D7KaRcUTQeE"},
		{Name: "Leg Extension", MuscleGroup: "Legs", Equipment: "Machine", VideoURL: "https://www.youtube.com/watch?v=YyvSfVLYZqo"},
		{Name: "Lying Leg Curl", MuscleGroup: "Legs", Equipment: "Machine", VideoURL: "https://www.youtube.com/watch?v=1Tq3QdYUuHs"},
		{Name: "Romanian Deadlift", MuscleGroup: "Legs (Hamstrings)", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=JCXUYuzwZ_M"},
		{Name: "Calf Raise", MuscleGroup: "Legs (Calves)", Equipment: "Machine", VideoURL: "https://www.youtube.com/watch?v=3UWi44yN-wM"},
		{Name: "Goblet Squat", MuscleGroup: "Legs", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=MeIiGibT6X0"},
		{Name: "Bulgarian Split Squat", MuscleGroup: "Legs", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=9FOMyxA3Lw4"},
		{Name: "Glute Bridge", MuscleGroup: "Legs (Glutes)", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=vOvRFsGMMqo"},

		// Chest
		{Name: "Barbell Bench Press", MuscleGroup: "Chest", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=EUjh50tLlBo"},
		{Name: "Incline Dumbbell Press", MuscleGroup: "Chest", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=8iPEnn-ltC8"},
		{Name: "Push Up", MuscleGroup: "Chest", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=IODxDxX7oi4"},
		{Name: "Cable Fly", MuscleGroup: "Chest", Equipment: "Cable", VideoURL: "https://www.youtube.com/watch?v=I-Ue34qLxc4"},
		{Name: "Dips", MuscleGroup: "Chest/Triceps", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=SwDers3SMZ4"},
		{Name: "Machine Chest Press", MuscleGroup: "Chest", Equipment: "Machine", VideoURL: "https://www.youtube.com/watch?v=x0X6V1-lVqM"},
		{Name: "Pec Deck", MuscleGroup: "Chest", Equipment: "Machine", VideoURL: "https://www.youtube.com/watch?v=O-5G_Kk9tI4"},
		{Name: "Decline Bench Press", MuscleGroup: "Chest", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=n1uA2MEAPIU"},
		{Name: "Svend Press", MuscleGroup: "Chest", Equipment: "Plate", VideoURL: "https://www.youtube.com/watch?v=tC3v9W4Gf3Y"},
		{Name: "Landmine Press", MuscleGroup: "Chest", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=TAsJgY2P7o8"},

		// Back
		{Name: "Pull Up", MuscleGroup: "Back", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=eGo4IYlbE5g"},
		{Name: "Lat Pulldown", MuscleGroup: "Back", Equipment: "Cable", VideoURL: "https://www.youtube.com/watch?v=CAwf7n6Luuc"},
		{Name: "Barbell Row", MuscleGroup: "Back", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=DgyslsszCQ0"},
		{Name: "Seated Cable Row", MuscleGroup: "Back", Equipment: "Cable", VideoURL: "https://www.youtube.com/watch?v=GZbfZ033f74"},
		{Name: "Single Arm Dumbbell Row", MuscleGroup: "Back", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=dFzUjzuWss0"},
		{Name: "Deadlift", MuscleGroup: "Back/Legs", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=U1H1VG9Uh50"},
		{Name: "Face Pull", MuscleGroup: "Back (Rear Delts)", Equipment: "Cable", VideoURL: "https://www.youtube.com/watch?v=ntBwG1E3Pzs"},
		{Name: "T-Bar Row", MuscleGroup: "Back", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=j3Igk5nyZE4"},
		{Name: "Hyperextension", MuscleGroup: "Back (Lower)", Equipment: "Machine", VideoURL: "https://www.youtube.com/watch?v=5_Ej9mH-K6E"},
		{Name: "Straight Arm Pulldown", MuscleGroup: "Back", Equipment: "Cable", VideoURL: "https://www.youtube.com/watch?v=vV_uD6X8fMc"},

		// Shoulders
		{Name: "Overhead Press", MuscleGroup: "Shoulders", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=HzIiInu578Q"},
		{Name: "Dumbbell Shoulder Press", MuscleGroup: "Shoulders", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=1jYq9QQEWqE"},
		{Name: "Lateral Raise", MuscleGroup: "Shoulders", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=3VcKaXpzqRo"},
		{Name: "Front Raise", MuscleGroup: "Shoulders", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=CH9JzDStL3U"},
		{Name: "Reverse Fly", MuscleGroup: "Shoulders (Rear)", Equipment: "Machine", VideoURL: "https://www.youtube.com/watch?v=C7E-O3-KId4"},
		{Name: "Arnold Press", MuscleGroup: "Shoulders", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=fFyrgCWTIaI"},
		{Name: "Upright Row", MuscleGroup: "Shoulders/Traps", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=amCU-ziHITM"},

		// Arms
		{Name: "Barbell Curl", MuscleGroup: "Biceps", Equipment: "Barbell", VideoURL: "https://www.youtube.com/watch?v=aEscWJ3dS3w"},
		{Name: "Hammer Curl", MuscleGroup: "Biceps", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=obovFxPjXSM"},
		{Name: "Preacher Curl", MuscleGroup: "Biceps", Equipment: "Machine/EZ Bar", VideoURL: "https://www.youtube.com/watch?v=fIWP-FRFNU0"},
		{Name: "Tricep Pushdown", MuscleGroup: "Triceps", Equipment: "Cable", VideoURL: "https://www.youtube.com/watch?v=2-LAMcpzHLU"},
		{Name: "Skullcrusher", MuscleGroup: "Triceps", Equipment: "EZ Bar", VideoURL: "https://www.youtube.com/watch?v=l3rHYPtMUo8"},
		{Name: "Overhead Tricep Extension", MuscleGroup: "Triceps", Equipment: "Dumbbell", VideoURL: "https://www.youtube.com/watch?v=6SS6K3lAw_o"},

		// Core
		{Name: "Plank", MuscleGroup: "Core", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=pSHjTRCQxIw"},
		{Name: "Crunch", MuscleGroup: "Core", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=cQ5JKgEZCU4"},
		{Name: "Leg Raise", MuscleGroup: "Core", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=jbLpAteP_t4"},
		{Name: "Russian Twist", MuscleGroup: "Core", Equipment: "Bodyweight/Weight", VideoURL: "https://www.youtube.com/watch?v=wkD8rjk6OGI"},
		{Name: "Ab Wheel Rollout", MuscleGroup: "Core", Equipment: "Ab Wheel", VideoURL: "https://www.youtube.com/watch?v=_BHKT60P6bc"},
		{Name: "Mountain Climber", MuscleGroup: "Core", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=nmwgirgXLYM"},
		{Name: "Bicycle Crunch", MuscleGroup: "Core", Equipment: "Bodyweight", VideoURL: "https://www.youtube.com/watch?v=eqg47ZuGZXQ"},
	}

	for _, ex := range exercises {
		if err := repo.Create(context.Background(), &ex); err != nil {
			if err == domain.ErrDuplicateExercise {
				fmt.Printf("Skipping duplicate: %s\n", ex.Name)
			} else {
				log.Printf("Error creating %s: %v\n", ex.Name, err)
			}
		} else {
			fmt.Printf("Created: %s\n", ex.Name)
		}
	}
	fmt.Println("Seeding Exercises Complete.")
}
