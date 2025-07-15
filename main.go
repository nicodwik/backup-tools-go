package main

import (
	"encoding/json"
	"errors"
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

var (
	sourcePath       = "/data"
	backupOutputPath = "/backups"
	jkt, _           = time.LoadLocation("Asia/Jakarta")
)

func main() {
	cronExpression := os.Getenv("CRON_EXPRESSION")
	if cronExpression == "" {
		cronExpression = "0 15 * * * *"
	}
	cr := cron.New()

	cr.AddFunc(cronExpression, func() {
		fmt.Println("Backup s running at:", time.Now().In(jkt).Format(time.DateTime))
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
	compressionLevel, _ := strconv.Atoi(os.Getenv("COMPRESSION_LEVEL"))

	backup := backup.New(sourcePath, backupOutputPath, compressionLevel)

	fileSystemTree, err := backup.BuildHybridOneLevelNestedJSON() // Use the recursive builder
	if err != nil {
		fmt.Printf("Error building file system JSON: %v\n", err)
		return err
	}

	oldManifest, err := openManifest()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			for _, fst := range fileSystemTree {
				fst.ModTime = time.Now().In(jkt).Format(time.RFC3339)
				for _, c := range fst.Children {
					c.ModTime = time.Now().In(jkt).Format(time.RFC3339)
				}
			}
			if err := saveManifest(fileSystemTree); err != nil {
				return fmt.Errorf("ERROR when saving manifest: %s", err.Error())
			}
			fmt.Println("First manifest created")
			return nil
		}

		return fmt.Errorf("ERROR when opening manifest: %s", err.Error())
	}

	processedBackup := 0
	// Iterate through the parent directories in the JSON response
	// and create a zip file for each.
	wg := new(sync.WaitGroup)
	for _, ft := range fileSystemTree {
		for _, of := range oldManifest {
			if ft.Name == of.Name && of.ModTime != ft.ModTime {

				isChildDifferent := false
				if len(ft.Children) == 0 {
					isChildDifferent = true
				} else {
					for _, ftc := range ft.Children {
						for _, ofc := range of.Children {
							if ftc.Name == ofc.Name && ftc.ModTime != ofc.ModTime {
								isChildDifferent = true
								break
							}
						}
					}
				}

				if isChildDifferent {

					processedBackup++
					wg.Add(1)

					go func() {
						defer wg.Done()
						parent := ft // Get a pointer to modify the original struct in the slice
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
			}
		}
	}
	wg.Wait()

	if processedBackup == 0 {
		fmt.Println("There's nothing to backup")
	} else {
		fmt.Println("Total processed backups:", processedBackup)
	}

	if err := saveManifest(fileSystemTree); err != nil {
		return fmt.Errorf("ERROR when saving manifest: %s", err.Error())
	}

	fmt.Println()

	return nil
}

func saveManifest(fileSystemTree []*backup.DirectoryEntry) error {
	m, _ := json.MarshalIndent(fileSystemTree, "", "\t")
	newManifest, err := os.Create(filepath.Join(backupOutputPath, "manifest.json"))
	if err != nil {
		return err
	}

	newManifest.Write(m)
	defer newManifest.Close()

	return nil
}

func openManifest() ([]*backup.DirectoryEntry, error) {
	var fileSystemTree []*backup.DirectoryEntry

	m, err := os.Open(filepath.Join(backupOutputPath, "manifest.json"))
	if err != nil {
		return nil, err
	}

	if err := json.NewDecoder(m).Decode(&fileSystemTree); err != nil {
		return nil, err
	}

	return fileSystemTree, nil
}
