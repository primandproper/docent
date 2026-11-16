package tour

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/primandproper/docent/internal/orderedjson"
)

// The functions here edit a tour's raw document in place. The CLI calls them,
// and so does the app for the one edit it makes: recording a review.

// SetStamp records the commit a repo's part of the tour was written against.
func SetStamp(raw *orderedjson.Value, repo, sha string) {
	raw.Get("repos").Get(repo).Set("stamp", orderedjson.String(sha))
}

// SetHash records a step's contentHash, or removes it when hash is empty.
func SetHash(raw *orderedjson.Value, step int, hash string) {
	steps := raw.Get("steps")
	if steps == nil || steps.Kind != orderedjson.KindArray || step >= len(steps.Elems) {
		return
	}

	if hash == "" {
		steps.Elems[step].Delete("contentHash")
		return
	}

	steps.Elems[step].Set("contentHash", orderedjson.String(hash))
}

// AppendReview adds an entry to `reviewedBy`.
func AppendReview(raw *orderedjson.Value, review *Review) {
	entry := orderedjson.Object(
		orderedjson.Member{Key: "name", Value: orderedjson.String(review.Name)},
		orderedjson.Member{Key: "at", Value: orderedjson.String(review.At)},
	)
	if review.Ref != "" {
		entry.Set("ref", orderedjson.String(review.Ref))
	}

	list := raw.Get("reviewedBy")
	if list == nil || list.Kind != orderedjson.KindArray {
		list = &orderedjson.Value{Kind: orderedjson.KindArray}
		raw.Set("reviewedBy", list)
	}

	list.Elems = append(list.Elems, entry)
}

// Write formats a tour's raw document canonically and replaces the file with
// it. The write goes through a temporary file, so a crash can't leave half a
// tour behind.
func Write(path string, raw *orderedjson.Value) error {
	formatted, err := raw.Format()
	if err != nil {
		return fmt.Errorf("formatting %s: %w", path, err)
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	defer os.Remove(tmp.Name()) //nolint:errcheck // gone already once the rename succeeds

	if _, err = tmp.Write(formatted); err == nil {
		err = tmp.Chmod(mode)
	}

	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}

	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}

	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// SetAnchorHash records the contentHash of one of a step's extra anchors, or
// removes it when hash is empty.
func SetAnchorHash(raw *orderedjson.Value, step, anchor int, hash string) {
	steps := raw.Get("steps")
	if steps == nil || steps.Kind != orderedjson.KindArray || step >= len(steps.Elems) {
		return
	}

	anchors := steps.Elems[step].Get("anchors")
	if anchors == nil || anchors.Kind != orderedjson.KindArray || anchor >= len(anchors.Elems) {
		return
	}

	if hash == "" {
		anchors.Elems[anchor].Delete("contentHash")
		return
	}

	anchors.Elems[anchor].Set("contentHash", orderedjson.String(hash))
}
