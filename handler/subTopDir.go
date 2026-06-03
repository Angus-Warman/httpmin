package handler

import "io/fs"

// If root contains exactly one top-level dir and nothing else, substitute it. Recursive.
func substituteTopLevelDir(root fs.FS) fs.FS {
	entries, err := fs.ReadDir(root, ".")

	if err != nil {
		return root
	}

	if len(entries) != 1 {
		return root
	}

	entry := entries[0]

	if !entry.IsDir() {
		return root
	}

	newRoot, err := fs.Sub(root, entry.Name())

	if err != nil {
		return root
	}

	newRoot = substituteTopLevelDir(newRoot)

	return newRoot
}
