package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"plenti/common"
	"strings"
	"time"
)

// EjectCopy does a direct copy of any ejectable js files needed in spa build dir.
func EjectCopy(buildPath string, tempBuildDir string, ejectedDir string) error {

	defer Benchmark(time.Now(), "Copying ejectable core files for build")

	Log("\nCopying ejectable core files to their destination:")
	// Make list of files not to copy to build.
	// Make list of files not to copy to build.
	excludedFiles := map[string]bool{
		ejectedDir + "/build.js": true,
	}
	copiedSourceCounter := 0
	if common.UseMemFS {

		// Check if the current file is in the excluded list.
		for k, v := range common.Iter() {
			if strings.HasPrefix(k, ejectedDir) && filepath.Ext(k) == ".js" && !excludedFiles[k] {

				destPath := buildPath + "/spa/"
				common.Set(destPath+strings.TrimPrefix(k, tempBuildDir), v)
				copiedSourceCounter++

			}

		}

	} else {
		ejectedFilesErr := filepath.Walk(ejectedDir, func(ejectPath string, ejectFileInfo os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("can't stat '%s': %w", ejectPath, err)
			}

			// Check if the current file is in the excluded list.
			if excludedFiles[ejectPath] {
				return nil
			}

			// If the file is already in .js format just copy it straight over to build dir.
			if filepath.Ext(ejectPath) == ".js" {

				destPath := buildPath + "/spa/"

				if err := os.MkdirAll(destPath+strings.TrimPrefix(ejectedDir, tempBuildDir), os.ModePerm); err != nil {
					return err
				}

				from, err := os.Open(ejectPath)
				if err != nil {
					return fmt.Errorf("Could not open source .js file for copying: %w%s", err, common.Caller())
				}
				defer from.Close()

				to, err := os.Create(destPath + strings.TrimPrefix(ejectPath, tempBuildDir))
				if err != nil {
					return fmt.Errorf("Could not create destination .js file for copying: %w%s", err, common.Caller())
				}
				defer to.Close()

				_, err = io.Copy(to, from)
				if err != nil {
					return fmt.Errorf("Could not copy .js from source to destination: %w%s", err, common.Caller())
				}

				copiedSourceCounter++
			}
			return nil
		})
		if ejectedFilesErr != nil {
			return fmt.Errorf("Could not get ejectable file: %w%s", ejectedFilesErr, common.Caller())
		}
	}

	Log(fmt.Sprintf("Number of ejectable core files copied: %d\n", copiedSourceCounter))
	return nil

}
