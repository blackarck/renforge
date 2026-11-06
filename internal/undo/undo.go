// Package undo reads and writes rename logs (CSV) and reverts them.
package undo

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"FileRenUtil/internal/rename"
)

var header = []string{"old_path", "new_path", "old_name", "new_name", "status", "reason"}

// Write writes items as a CSV log.
func Write(w io.Writer, items []rename.Item) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, it := range items {
		rec := []string{it.OldPath, it.NewPath, it.OldName, it.NewName, string(it.Status), it.Reason}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteFile writes the log to path, replacing any existing file.
func WriteFile(path string, items []rename.Item) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := Write(f, items); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Read parses a CSV log written by Write (or by earlier RenForge versions).
func Read(r io.Reader) ([]rename.Item, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows[0]) < 2 || rows[0][0] != header[0] || rows[0][1] != header[1] {
		return nil, errors.New("not a RenForge undo log (unexpected CSV header)")
	}
	var items []rename.Item
	for _, row := range rows[1:] {
		if len(row) < 5 {
			continue
		}
		it := rename.Item{
			OldPath: row[0], NewPath: row[1], OldName: row[2], NewName: row[3],
			Status: rename.Status(row[4]),
		}
		if len(row) > 5 {
			it.Reason = row[5]
		}
		items = append(items, it)
	}
	return items, nil
}

// ReadFile reads the log at path.
func ReadFile(path string) ([]rename.Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

// RevertSummary describes what reverting a log will do.
type RevertSummary struct {
	Total   int      // entries in the log that were actually renamed
	Ready   int      // can be restored
	Missing []string // renamed file no longer exists at its new path
	Blocked []string // original name is occupied again
}

// BuildRevertPlan turns a log into a plan that restores the original names.
// Only entries with status "renamed" are considered (dry-run logs revert nothing).
// The returned plan can be executed with rename.Execute.
func BuildRevertPlan(entries []rename.Item) ([]rename.Item, RevertSummary) {
	var sum RevertSummary
	var plan []rename.Item

	for _, e := range entries {
		if e.Status != rename.StatusRenamed {
			continue
		}
		sum.Total++
		it := rename.Item{
			OldPath: e.NewPath,
			NewPath: e.OldPath,
			OldName: filepath.Base(e.NewPath),
			NewName: filepath.Base(e.OldPath),
			Status:  rename.StatusOK,
		}
		if _, err := os.Lstat(it.OldPath); err != nil {
			sum.Missing = append(sum.Missing, it.OldName)
			it.Status, it.Reason = rename.StatusSkip, "missing: file no longer exists"
		}
		plan = append(plan, it)
	}
	// A restore target held by another file that is also being restored (frame
	// shifts, swaps) is fine; only genuinely occupied names are skipped.
	rename.ResolveTargets(plan, nil)
	for _, it := range plan {
		switch {
		case it.Status == rename.StatusOK:
			sum.Ready++
		case strings.HasPrefix(it.Reason, "conflict"):
			sum.Blocked = append(sum.Blocked, fmt.Sprintf("%s → %s", it.OldName, it.NewName))
		}
	}
	return plan, sum
}
