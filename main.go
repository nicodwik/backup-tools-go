package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nicodwik/backup-tools-go/backup"
	"github.com/robfig/cron"
)

func main() {

	cr := cron.New()

	cr.AddFunc("0 * * * *", func() {
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
	for i := range fileSystemTree {
		parent := &fileSystemTree[i] // Get a pointer to modify the original struct in the slice
		parentDirFullPath := filepath.Join(sourcePath, parent.Name)
		zipFileName := parent.Name + ".zip"
		destZipPath := filepath.Join(backupOutputPath, zipFileName)

		err := backup.ZipDirectory(destZipPath)
		if err != nil {
			fmt.Printf("Failed to zip directory %q: %v\n", parentDirFullPath, err)
			return err
		}

		fmt.Printf("Successfully zipped %q to %q\n", parentDirFullPath, destZipPath)
		parent.ZipPath = destZipPath // Add zip path to JSON response
	}

	return nil
}
