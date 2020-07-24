package main

import (
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	dryRun              = flag.Bool("dry_run", false, "If set, do not rename files, just print what renames would occur.")
	concurrency         = flag.Int("concurrency", 0, "The number of files to process at once. If unset, a reasonable value will be chosen automatically.")
	hashName            = flag.String("hash", "sha512_256", "The hash to use. Supported values include `sha1` & `sha512_256`.")
	skipHashedFilenames = flag.Bool("skip_hashed_filenames", true, "If set, skip files whose names appear to already be a hash. (Does not check that the hash is correct.)")
)

func main() {
	// Parse & validate flags.
	flag.Parse()
	if len(flag.Args()) == 0 {
		die("Usage: hashrename [--dry_run] [--concurrency=N] [--hash=sha512_256] globs")
	}
	switch {
	case *concurrency == 0:
		*concurrency = runtime.GOMAXPROCS(0)
	case *concurrency < 0:
		die("The --concurrency flag must be non-negative.")
	}
	var newHash func() hash.Hash
	switch *hashName {
	case "sha1":
		newHash = sha1.New
	case "sha512_256":
		newHash = sha512.New512_256
	default:
		die("Unknown --hash value %q", *hashName)
	}

	fnFilter := func(string) bool { return true }
	if *skipHashedFilenames {
		// Build & hash some data to figure out how big a filename hash will be.
		hLen := 2 * newHash().Size() // times 2 to account for hex-encoding
		r, err := regexp.Compile(fmt.Sprintf(`^[0-9a-f]{%d}(\..*)?$`, hLen))
		if err != nil {
			die("Couldn't compile filter regex: %v", err)
		}

		fnFilter = func(fn string) bool { return !r.MatchString(filepath.Base(fn)) }
	}

	// Start per-file workers.
	var wg sync.WaitGroup
	var errCount int64
	ch := make(chan string)
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := newHash()
			for fn := range ch {
				if err := func() error {
					// Check filter.
					if !fnFilter(fn) {
						return nil
					}

					// Hash file.
					f, err := os.Open(fn)
					if err != nil {
						return fmt.Errorf("couldn't open: %w", err)
					}
					defer f.Close()
					h.Reset()
					if _, err := io.Copy(h, f); err != nil {
						return fmt.Errorf("couldn't read: %w", err)
					}
					if err := f.Close(); err != nil {
						return fmt.Errorf("couldn't close: %w", err)
					}

					// Move file to new filename based on hash.
					newFn := hex.EncodeToString(h.Sum(nil))
					ext := filepath.Ext(fn)
					if ext != "" {
						newFn = fmt.Sprintf("%s%s", newFn, ext)
					}
					newFn = filepath.Join(filepath.Dir(fn), newFn)
					fmt.Printf("%s -> %s\n", fn, newFn)
					if !*dryRun {
						if err := os.Rename(fn, newFn); err != nil {
							return fmt.Errorf("couldn't rename: %w")
						}
					}
					return nil
				}(); err != nil {
					atomic.AddInt64(&errCount, 1)
					fmt.Fprintf(os.Stderr, "Couldn't handle %q: %v\n", fn, err)
				}
			}
		}()
	}

	// Find files to rename. (find all files before renaming anything to ensure we handle each file only once)
	files := map[string]struct{}{}
	for _, glob := range flag.Args() {
		fns, err := filepath.Glob(glob)
		if err != nil {
			die("Bad glob %q: %v", glob, err)
		}
		for _, fn := range fns {
			files[fn] = struct{}{}
		}
	}
	fmt.Printf("Processing %d file(s)\n", len(files))
	for fn := range files {
		ch <- fn
	}
	close(ch)
	wg.Wait()
	if errCount > 0 {
		die("Encountered %d errors", errCount)
	}
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
