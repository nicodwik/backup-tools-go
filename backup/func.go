package backup

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type backup struct {
	SourcePath       string
	OutputPath       string
	CompressionLevel int
}

func New(sourcePath, outputPath string, compressionLevel int) *backup {
	return &backup{
		SourcePath:       sourcePath,
		OutputPath:       outputPath,
		CompressionLevel: compressionLevel,
	}
}

// zipDirectory zips the contents of sourceDir into a new zip file at destZipPath.
// It now accepts a compressionLevel (e.g., flate.DefaultCompression, flate.BestSpeed, flate.BestCompression, or 1-9).
func (b *backup) ZipDirectory(sourcePath, destZipPath string) error {
	zipFile, err := os.Create(destZipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file %q: %w", destZipPath, err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Register a custom Deflate compressor with the specified compression level
	zipWriter.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, b.CompressionLevel)
	})
	if err != nil {
		return fmt.Errorf("failed to register compressor for zip writer: %w", err)
	}

	fmt.Printf("Zipping contents of %q to %q with level %d...\n", sourcePath, destZipPath, b.CompressionLevel)

	err = filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the base directory itself if we don't want it as the root entry in the zip
		// If you want the folder name as the root inside the zip, adjust this logic.
		// For consistency with typical zip tool behavior, we usually include the base dir.
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q: %w", path, err)
		}

		if relPath == "." {
			return nil
		}
		// Determine the name to use inside the zip file
		zipEntryName := relPath
		// if relPath == "." { // This is the root directory being zipped
		// 	zipEntryName = filepath.Base(sourcePath) // Use the base name of the source directory
		// }

		info, _ := d.Info()

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("failed to create file info header for %q: %w", path, err)
		}

		header.Name = zipEntryName
		if d.IsDir() {
			header.Name += "/"        // Add trailing slash for directories
			header.Method = zip.Store // Directories are usually stored, not compressed
		} else {
			header.Method = zip.Deflate // Use Deflate for files, which will use our registered compressor
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("failed to create zip header for %q: %w", header.Name, err)
		}

		if !d.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %q: %w", path, err)
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			if err != nil {
				return fmt.Errorf("failed to copy file contents %q to zip: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory for zipping %q: %w", sourcePath, err)
	}

	return nil
}

// DirectoryEntry represents a single directory in the flat JSON array.
type directoryEntry struct {
	Name     string           `json:"name"` // Will be the full relative path
	Type     string           `json:"type"` // "file" or "directory"
	ModTime  time.Time        `json:"mod_time"`
	Children []directoryEntry `json:"children,omitempty"` // Only for directories
	ZipPath  string           `json:"zip_path,omitempty"` // New: Path to the generated zip file
}

// collectAllDescendantDirectoriesFlat walks a given directory (targetPath)
// and collects all its subdirectories (children, grandchildren, etc.) into a flat slice.
// The 'name' field in the returned entries will be relative to 'targetPath'.
func (b *backup) collectAllDescendantDirectoriesFlat(targetPath string) ([]directoryEntry, error) {
	var descendants []directoryEntry

	err := filepath.WalkDir(targetPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path %q: %v\n", path, err)
			return err
		}

		// Skip the targetPath itself (the parent) as it will be handled at the top level.
		// We are only interested in its descendants.
		if path == targetPath {
			return nil
		}

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				fmt.Printf("Error getting info for directory %q: %v\n", path, err)
				return err
			}

			// The 'name' for children should be relative to their direct parent.
			// However, your example shows "child_2/grandchild_1", implying it's relative to the *top-level parent*.
			// So, let's make it relative to the 'targetPath' itself.
			relPathFromTarget, err := filepath.Rel(targetPath, path)
			if err != nil {
				return fmt.Errorf("error getting relative path for %q from %q: %w", path, targetPath, err)
			}

			descendants = append(descendants, directoryEntry{
				Name:    relPathFromTarget, // This includes parent names like "child_2/grandchild_1"
				Type:    "directory",
				ModTime: info.ModTime(),
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return descendants, nil
}

// buildHybridOneLevelNestedJSON creates the specific hybrid structure requested.
// It lists parent directories and then a flat list of all their descendants.
func (b *backup) BuildHybridOneLevelNestedJSON() ([]directoryEntry, error) {
	var result []directoryEntry

	// Read immediate entries within the rootPath
	entries, err := os.ReadDir(b.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("error reading root directory %q: %w", b.SourcePath, err)
	}

	for _, entry := range entries {
		// Only consider immediate directories at the root level as "parents"
		if entry.IsDir() {
			parentFullPath := filepath.Join(b.SourcePath, entry.Name())
			parentInfo, err := entry.Info()
			if err != nil {
				fmt.Printf("Warning: Could not get info for parent directory %q: %v\n", parentFullPath, err)
				continue
			}

			parentEntry := directoryEntry{
				Name:    entry.Name(), // Use base name for the top-level parent
				Type:    "directory",
				ModTime: parentInfo.ModTime(),
			}

			// Collect all descendants (children, grandchildren, etc.) for this parent
			descendants, err := b.collectAllDescendantDirectoriesFlat(parentFullPath)
			if err != nil {
				fmt.Printf("Warning: Could not collect descendants for %q: %v\n", parentFullPath, err)
				// Continue without populating children for this specific parent
			} else {
				// Assign the flat list of descendants to the Children field
				parentEntry.Children = descendants
			}

			result = append(result, parentEntry)
		}
	}

	return result, nil
}
