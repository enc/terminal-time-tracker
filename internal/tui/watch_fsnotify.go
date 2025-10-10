package tui

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FSNotifyJournalWatch implements JournalWatch using fsnotify.
// It watches ~/.tt/journal (or a provided root) recursively and coalesces
// filesystem activity into debounced change notifications.
type FSNotifyJournalWatch struct {
	root     string
	debounce time.Duration
}

// NewFSNotifyJournalWatch creates a new filesystem watcher rooted at the
// provided path. If root is empty, it defaults to ~/.tt/journal.
// Debounce controls how quickly bursty events are coalesced into a single
// notification (default 250ms if <= 0).
func NewFSNotifyJournalWatch(root string, debounce time.Duration) *FSNotifyJournalWatch {
	if root == "" {
		root = DefaultJournalRoot()
	}
	if debounce <= 0 {
		debounce = 250 * time.Millisecond
	}
	return &FSNotifyJournalWatch{
		root:     root,
		debounce: debounce,
	}
}

// DefaultJournalRoot returns the default journal root under the user's home.
func DefaultJournalRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".tt", "journal")
}

// Changes starts watching the journal root recursively and returns a channel
// that emits a signal whenever relevant file activity is detected.
// The channel closes when the provided context is canceled or the watcher fails.
//
// Notes:
//   - Directory creation is handled dynamically: new year/month directories added
//     under the journal root are watched automatically.
//   - File events are debounced to avoid spamming the UI on bursts of writes.
func (w *FSNotifyJournalWatch) Changes(ctx context.Context) <-chan struct{} {
	out := make(chan struct{}, 1)

	go func() {
		defer close(out)

		// Ensure the root exists to allow recursive watching.
		if err := os.MkdirAll(w.root, 0o755); err != nil {
			log.Printf("journal watch: unable to ensure root %s: %v", w.root, err)
			return
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Printf("journal watch: new watcher error: %v", err)
			return
		}
		defer watcher.Close()

		// Add all existing directories under the root.
		if err := addWatchRecursive(watcher, w.root); err != nil {
			log.Printf("journal watch: initial add recursive error: %v", err)
			// Continue; we can still receive events for the paths that were added.
		}

		// Debounce logic: coalesce multiple events into a single notification.
		var (
			timer   *time.Timer
			pending bool
		)
		trigger := func() {
			// Initialize or reset the timer
			if timer == nil {
				timer = time.NewTimer(w.debounce)
				pending = true
				return
			}
			if !timer.Stop() {
				// Drain if needed to avoid leaking a tick
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounce)
			pending = true
		}
		notify := func() {
			// Non-blocking send, coalesce if receiver is slow.
			select {
			case out <- struct{}{}:
			default:
			}
		}

		for {
			select {
			case <-ctx.Done():
				return

			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Handle newly created directories: add watcher.
				if ev.Op&fsnotify.Create == fsnotify.Create {
					// If a directory is created, start watching it (and subdirs).
					if isDir(ev.Name) {
						if err := addWatchRecursive(watcher, ev.Name); err != nil {
							log.Printf("journal watch: add recursive on create %s: %v", ev.Name, err)
						}
						// No need to trigger for directory create itself.
						continue
					}
				}

				// For file changes, any kind of write/rename/remove/chmod -> trigger debounce.
				if ev.Op&(fsnotify.Write|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod|fsnotify.Create) != 0 {
					trigger()
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("journal watch: watcher error: %v", err)

			case <-func() <-chan time.Time {
				if timer == nil {
					// disabled case
					return nil
				}
				return timer.C
			}():
				// Debounced notification
				if pending {
					notify()
					pending = false
				}
			}
		}
	}()

	return out
}

// addWatchRecursive walks the directory tree and adds a watch for each directory.
func addWatchRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip paths we can't stat.
			return nil
		}
		if d.IsDir() {
			if err := w.Add(path); err != nil {
				// Continue walking despite errors on some directories.
				log.Printf("journal watch: add %s error: %v", path, err)
			}
		}
		return nil
	})
}

// isDir returns true if the given path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
