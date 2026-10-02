package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// The four outcomes a covered file can have.
const (
	fileEqual = iota
	fileDiffers
	fileOnlyAtPin
	fileOnlyInTree
)

type fileResult struct {
	outcome  int
	pinPath  string // full SemStreams path; empty when only in the tree
	treePath string // path in the tree; empty when only at the pin
	pinText  []byte // after the rewrite
	treeText []byte // after gofmt
}

type entryResult struct {
	entry       entry
	notCompared string // the reason; empty when the entry was compared
	files       []fileResult
}

// counts returns the number of files on both sides, differing, only at the pin and only in the
// tree.
func (r entryResult) counts() (both, differ, onlyPin, onlyTree int) {
	for _, f := range r.files {
		switch f.outcome {
		case fileEqual:
			both++
		case fileDiffers:
			both++
			differ++
		case fileOnlyAtPin:
			onlyPin++
		case fileOnlyInTree:
			onlyTree++
		}
	}
	return both, differ, onlyPin, onlyTree
}

// compareEntry compares one entry's covered files with the pin. An error means the pin could not
// be read; everything about the entry itself is in the result.
func compareEntry(ctx context.Context, root string, store pinStore, e entry, moved map[string]string) (entryResult, error) {
	result := entryResult{entry: e}
	side := resolveDestination(root, e.Destination)
	if side.abs == "" {
		result.notCompared = side.problem
		return result, nil
	}
	pinObj, err := store.stat(ctx, e.SourceSHA, e.SourcePath)
	if err != nil {
		return result, err
	}
	switch {
	case pinObj.kind == "":
		result.notCompared = "source_path does not exist at " + e.SourceSHA
		return result, nil
	case pinObj.kind == "blob" && side.isDir:
		result.notCompared = "source_path is a file at the pin and the destination path is a directory"
		return result, nil
	case pinObj.kind == "tree" && !side.isDir:
		result.notCompared = "source_path is a directory at the pin and the destination path is a file"
		return result, nil
	case pinObj.kind != "blob" && pinObj.kind != "tree":
		result.notCompared = "source_path is a " + pinObj.kind + " at the pin, neither a file nor a directory"
		return result, nil
	}

	// A file entry covers one file, keyed by the empty relative path.
	pinFiles := map[string]pinObject{"": pinObj}
	treeFiles := map[string]bool{"": true}
	if pinObj.kind == "tree" {
		if pinFiles, err = store.covered(ctx, e.SourceSHA, e.SourcePath); err != nil {
			return result, err
		}
		if treeFiles, err = coveredInTree(side.abs); err != nil {
			result.notCompared = "destination path " + side.path + " could not be read: " + err.Error()
			return result, nil
		}
	}

	rels := make([]string, 0, len(pinFiles)+len(treeFiles))
	for rel := range pinFiles {
		rels = append(rels, rel)
	}
	for rel := range treeFiles {
		if _, ok := pinFiles[rel]; !ok {
			rels = append(rels, rel)
		}
	}
	if len(rels) == 0 {
		// An entry that compared nothing has not been shown to match.
		result.notCompared = "no covered file at the pin or in the tree"
		return result, nil
	}
	slices.Sort(rels)
	for _, rel := range rels {
		f, err := compareFile(ctx, store, side, rel, pinFiles, treeFiles, moved)
		var unreadable treeReadError
		if errors.As(err, &unreadable) {
			result.notCompared, result.files = unreadable.Error(), nil
			return result, nil
		}
		if err != nil {
			return result, err
		}
		result.files = append(result.files, f)
	}
	return result, nil
}

func compareFile(ctx context.Context, store pinStore, side treeSide, rel string, pinFiles map[string]pinObject, treeFiles map[string]bool, moved map[string]string) (fileResult, error) {
	var f fileResult
	o, atPin := pinFiles[rel]
	if atPin {
		f.pinPath = o.path
	}
	if treeFiles[rel] {
		f.treePath = path.Join(side.path, rel)
	}
	switch {
	case !treeFiles[rel]:
		f.outcome = fileOnlyAtPin
		return f, nil
	case !atPin:
		f.outcome = fileOnlyInTree
		return f, nil
	}
	pinText, err := store.read(ctx, o)
	if err != nil {
		return f, err
	}
	treeAbs := side.abs
	if rel != "" {
		treeAbs = filepath.Join(side.abs, filepath.FromSlash(rel))
	}
	treeText, err := os.ReadFile(treeAbs)
	if err != nil {
		return f, treeReadError{err}
	}
	if rewritten(f.pinPath) {
		pinText, treeText = rewrite(pinText, moved), gofmt(treeText)
	}
	f.pinText, f.treeText = pinText, treeText
	f.outcome = fileEqual
	if string(pinText) != string(treeText) {
		f.outcome = fileDiffers
	}
	return f, nil
}

// treeReadError is a covered tree file that could not be read: the entry is not compared.
type treeReadError struct{ err error }

func (e treeReadError) Error() string { return "a covered file could not be read: " + e.err.Error() }

// coveredName reports whether a file directly in a directory entry is covered: a .go file, test
// files included. Files under testdata are covered whatever their name.
func coveredName(name string) bool {
	return strings.HasSuffix(name, ".go")
}

// rewritten reports whether a pin file is rewritten and formatted before comparing: a .go file
// outside testdata. Every other covered file is compared byte for byte.
func rewritten(pinPath string) bool {
	return strings.HasSuffix(pinPath, ".go") && !slices.Contains(strings.Split(pinPath, "/"), "testdata")
}

// coveredInTree lists the files a destination directory holds that the entry covers, by path
// relative to it: the .go files directly in it and every file under its testdata directory.
func coveredInTree(dir string) (map[string]bool, error) {
	files := map[string]bool{}
	direct, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, d := range direct {
		if !coveredName(d.Name()) {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, d.Name()))
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			files[d.Name()] = true
		}
	}
	testdata := filepath.Join(dir, "testdata")
	info, err := os.Stat(testdata)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return files, nil
	}
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(testdata, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	return files, err
}
