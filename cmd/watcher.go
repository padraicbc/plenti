package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"plenti/cmd/build"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

type watcher struct {
	*fsnotify.Watcher
}

func gowatch(buildPath string) {
	// Creates a new file watcher.
	wtch, err := fsnotify.NewWatcher()
	// stop here as nothing will be watched
	if err != nil {
		log.Fatalf("couldn't create 'fsnotify.Watcher'")
	}
	go func() {

		// this can error
		defer wtch.Close()
		w := &watcher{wtch}
		w.watch(buildPath)
	}()

}

// will act like a mutex but no panic worries like trying to unlock and already unlocked mutex.
// concurrent builds of the same files are probably unsafe anyway so this locking is good outside of just reloading logic..
var buildLock uint32
var isBuilding = make(chan struct{}, 1)

// Watch looks for updates to filesystem to prompt a site rebuild.
func (w *watcher) watch(buildPath string) {
	// die on any error or will loop infinitely
	// Watch specific directories for changes (only if they exist).
	if _, err := os.Stat("content"); !os.IsNotExist(err) {
		if err := filepath.Walk("content", w.watchDir(buildPath)); err != nil {
			log.Fatalf("Error watching 'content/' folder for changes: %v\n", err)
		}
	}
	if _, err := os.Stat("layout"); !os.IsNotExist(err) {
		if err := filepath.Walk("layout", w.watchDir(buildPath)); err != nil {
			log.Fatalf("Error watching 'layout/' folder for changes: %v\n", err)
		}
	}
	if _, err := os.Stat("assets"); !os.IsNotExist(err) {
		if err := filepath.Walk("assets", w.watchDir(buildPath)); err != nil {
			log.Fatalf("Error watching 'assets/' folder for changes: %v\n", err)
		}
	}
	if err := w.Add("plenti.json"); err != nil {
		log.Fatalf("couldn't add 'plenti.json' to watcher")

	}
	if err := w.Add("package.json"); err != nil {
		log.Fatalf("couldn't add 'package.json' to watcher")

	}

	done := make(chan bool)

	// Set delay for batching events.
	ticker := time.NewTicker(300 * time.Millisecond)
	// use a map for double firing events (happens when saving files in some text editors).
	events := map[string]fsnotify.Event{}

	go func() {
		for {
			select {
			// Watch for events.
			case event := <-w.Events:
				// Don't rebuild when build dir is added or deleted.
				if event.Name != "./"+buildPath {
					// Add current event to array for batching.
					events[event.Name] = event
				}
			case <-ticker.C:
				// Checks on set interval if there are events.
				// only build if there was an event.
				if len(events) > 0 {
					// Try aquire "mutex/lock",
					// builds will queue so ctrl-s back to back will still work fine
					// this just helps with reloading and stops concurrent/parallel access to the same files.
					// may need to change the logic when  build logic goes into go separate routines.
					for {

						if !atomic.CompareAndSwapUint32(&buildLock, 0, 1) {
							continue
						}
						Build()
						// will be unlocked when we receive loaded message from ws in window.onload
						if build.Doreload {
							reloadC <- struct{}{}
						} else {
							// not reloading sdo just unlock
							atomic.StoreUint32(&buildLock, 0)
						}
						break
					}

				}

				// Display messages for each events in batch.
				for _, event := range events {
					if event.Op&fsnotify.Create == fsnotify.Create {
						build.Log("File create detected: " + event.String())
						//common.CheckErr(w.Add(event.Name))
						// TODO: Checking error breaks server on Ubuntu.
						w.Add(event.Name)
						build.Log("Now watching " + event.Name)
					}
					if event.Op&fsnotify.Write == fsnotify.Write {
						build.Log("File write detected: " + event.String())

					}
					if event.Op&fsnotify.Remove == fsnotify.Remove {
						build.Log("File delete detected: " + event.String())
					}
					if event.Op&fsnotify.Rename == fsnotify.Rename {
						build.Log("File rename detected: " + event.String())
					}
					// optimised since 1.11 to reuse map
					for k := range events {
						delete(events, k)
					}

				}

			// Watch for errors.
			case err := <-w.Errors:
				if err != nil {
					fmt.Printf("\nFile watching error: %s\n", err)
				}
			}
		}
	}()

	<-done
}

// Closure that enables passing buildPath as arg to callback.
func (w *watcher) watchDir(buildPath string) filepath.WalkFunc {
	// Callback for walk func: searches for directories to add watchers to.
	return func(path string, fi os.FileInfo, err error) error {
		// Skip the "public" build dir to avoid infinite loops.
		if fi.IsDir() && fi.Name() == buildPath {
			return filepath.SkipDir
		}
		// Add watchers only to nested directory.
		if fi.Mode().IsDir() {
			return w.Add(path)
		}
		return nil
	}
}
