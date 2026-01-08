# Volume Recalculation Script

This utility script recalculates the "Strength Volume Mountain" data (aggregated Daily Volumes) for a specific member. It is useful when:
1.  **Backfilling Data:** You have existing workout history created before the volume aggregation feature was implemented.
2.  **Fixing Data:** Volume data is missing or incorrect for some workouts.

## How it Works
1.  Finds all `completed` schedules for the specified member.
2.  **Deletes** all existing `daily_volumes` records for that member (to avoid duplicates).
3.  Iterates through each completed schedule, fetches the associated `set_logs`, and recalculates:
    *   Total Volume (Weight × Reps)
    *   Total Sets
    *   Total Reps
    *   Total Weight Lifted
    *   Exercise Count
4.  Inserts new `DailyVolume` records into the `daily_volumes` collection.

## Usage

### 1. Build or Run directly
You can run the script using `go run`:

```bash
# Run from the project root
go run cmd/recalculate_volumes/main.go [flags]
```

### 2. Command Flags

| Flag | Description | Default | Required |
|------|-------------|---------|:--------:|
| `-member` | The MongoDB ID of the member to recalculate. | (none) | Yes |
| `-dry-run` | Preview actions without deleting or creating data. | `false` | No |
| `-mongo` | MongoDB connection URI. | `mongodb://localhost:27017` | No |
| `-db` | Database name. | `metamorph` | No |

### 3. Examples

**Dry Run (Safe Mode):**
Check what would happen without modifying the database.
```bash
go run cmd/recalculate_volumes/main.go -member 6950f9e282b9634944041d0c -dry-run
```

**Recalculate for a Member:**
Refreshes all volume history for the user.
```bash
go run cmd/recalculate_volumes/main.go -member 6950f9e282b9634944041d0c
```

**Custom Database Connection:**
```bash
go run cmd/recalculate_volumes/main.go -member 6950f9e282b9634944041d0c -mongo "mongodb://user:pass@remote-host:27017" -db "production_db"
```
