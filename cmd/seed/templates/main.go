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
	cfg, err := config.Load()
	if err != nil {
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
	exRepo := repository.NewMongoExerciseRepository(db) // Need list method or find by name
	tplRepo := repository.NewMongoTemplateRepository(db)

	// Helper to get IDs
	getIDs := func(names []string) []string {
		var ids []string
		for _, name := range names {
			// Using List with filter since we don't have GetByName exposed explicitly
			exs, err := exRepo.List(ctx, map[string]interface{}{"name": name})
			if err == nil && len(exs) > 0 {
				ids = append(ids, exs[0].ID)
			} else {
				fmt.Printf("Warning: Exercise not found: %s\n", name)
			}
		}
		return ids
	}

	templates := []struct {
		Name          string
		Gender        string
		ExerciseNames []string
	}{
		{
			Name:   "Upper Body",
			Gender: "All",
			ExerciseNames: []string{
				"Barbell Bench Press", "Overhead Press", "Lat Pulldown", "Barbell Row",
				"Barbell Curl", "Tricep Pushdown",
			},
		},
		{
			Name:   "Lower Body",
			Gender: "All",
			ExerciseNames: []string{
				"Barbell Squat", "Deadlift", "Leg Press", "Walking Lunge",
				"Leg Extension", "Lying Leg Curl", "Calf Raise",
			},
		},
		{
			Name:   "Full Body - Beginner",
			Gender: "All",
			ExerciseNames: []string{
				"Goblet Squat", "Push Up", "Seated Cable Row",
				"Dumbbell Shoulder Press", "Plank",
			},
		},
	}

	for _, tpl := range templates {
		// Check exists?
		// We don't have GetByName for templates in repo interface but we can list and check
		// Or just create and ignore dupes if we had unique index (we don't on template name yet)

		ids := getIDs(tpl.ExerciseNames)
		newTpl := &domain.WorkoutTemplate{
			Name:        tpl.Name,
			Gender:      tpl.Gender,
			ExerciseIDs: ids,
		}

		if err := tplRepo.Create(ctx, newTpl); err != nil {
			log.Printf("Error creating template %s: %v\n", tpl.Name, err)
		} else {
			fmt.Printf("Created Template: %s with %d exercises\n", tpl.Name, len(ids))
		}
	}
}
