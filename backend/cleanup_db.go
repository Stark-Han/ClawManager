//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"

	"clawreef/internal/config"
	"clawreef/internal/db"
	"clawreef/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Initialize(cfg.Database)
	if err != nil {
		fmt.Printf("Failed to init DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	instanceRepo := repository.NewInstanceRepository(database)

	// Get all instances
	instances, err := instanceRepo.GetByUserID(2, 0, 100)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Current instances:")
	for _, inst := range instances {
		fmt.Printf("ID=%d, Name=%s, Status=%s\n", inst.ID, inst.Name, inst.Status)
		if inst.Status == "creating" {
			fmt.Printf("  -> Deleting failed instance %d\n", inst.ID)
			instanceRepo.Delete(inst.ID)
		}
	}

	fmt.Println("\nCleanup complete!")
}
