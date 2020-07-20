package main

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	dryRun = flag.Bool("dry_run", false, "If set, do not rename files, just print what renames would occur.")
)

func main() {
	flag.Parse()
	if len(flag.Args()) == 0 {
		die("Usage: hashrename [--dry_run] globs")
	}

	// Find files to rename.
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
	fmt.Printf("Renaming %d file(s)\n", len(files))

	// Compute sha1sums.
	// TODO: do this in parallel? (may not be helpful; typically, this loop is IO-bound)
	hash := sha1.New()
	for fn := range files {
		hash.Reset()
		f, err := os.Open(fn)
		if err != nil {
			die("Couldn't open %q: %v", fn, err)
		}
		if _, err := io.Copy(hash, f); err != nil {
			die("Couldn't read %q: %v", fn, err)
		}
		if err := f.Close(); err != nil {
			die("Couldn't close %q: %v", fn, err)
		}

		newFn := hex.EncodeToString(hash.Sum(nil))
		ext := filepath.Ext(fn)
		if ext != "" {
			newFn = fmt.Sprintf("%s%s", newFn, ext)
		}
		newFn = filepath.Join(filepath.Dir(fn), newFn)
		fmt.Printf("%s -> %s\n", fn, newFn)
		if !*dryRun {
			if err := os.Rename(fn, newFn); err != nil {
				fmt.Fprintf(os.Stderr, "Couldn't rename %q to %q: %v\n", fn, newFn, err)
			}
		}
	}
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
