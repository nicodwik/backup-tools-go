package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/nicodwik/backup-tools-go/backup"
	"github.com/robfig/cron"
)

func main() {
	cronExpression := os.Getenv("CRON_EXPRESSION")
	if cronExpression == "" {
		cronExpression = "0 15 * * * *"
	}
	cr := cron.New()

	cr.AddFunc(cronExpression, func() {
		fmt.Println("Backup s running at:", time.Now().Format(time.DateTime))
		if err := doBackup(); err != nil {
			log.Fatalf("ERROR when doing backup: %s", err.Error())
			return
		}
	})

	cr.Start()

	fmt.Println("CRON STARTED")

	select {}
}

func doBackup() error {
	sourcePath := "/data"
	backupOutputPath := "/backups"
	compressionLevel, _ := strconv.Atoi(os.Getenv("COMPRESSION_LEVEL"))

	backup := backup.New(sourcePath, backupOutputPath, compressionLevel)

	fileSystemTree, err := backup.BuildHybridOneLevelNestedJSON() // Use the recursive builder
	if err != nil {
		fmt.Printf("Error building file system JSON: %v\n", err)
		return err
	}

	// Iterate through the parent directories in the JSON response
	// and create a zip file for each.
	wg := new(sync.WaitGroup)
	for i := range fileSystemTree {
		wg.Add(1)

		go func() {
			defer wg.Done()
			parent := &fileSystemTree[i] // Get a pointer to modify the original struct in the slice
			parentDirFullPath := filepath.Join(sourcePath, parent.Name)
			zipFileName := parent.Name + ".zip"
			destZipPath := filepath.Join(backupOutputPath, zipFileName)
			sourcePath := filepath.Join(sourcePath, parent.Name)

			err := backup.ZipDirectory(sourcePath, destZipPath)
			if err != nil {
				fmt.Printf("Failed to zip directory %q: %v\n", parentDirFullPath, err)
				return
			}

			fmt.Printf("Successfully zipped %q to %q\n", parentDirFullPath, destZipPath)
			parent.ZipPath = destZipPath // Add zip path to JSON response
		}()
	}
	wg.Wait()

	return nil
}
