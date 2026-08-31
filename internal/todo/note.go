// Note disposition: write-time frontmatter classification for clear-time.
package todo

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/polymorcodeus/park/schema"
)

// Disposition classifies a task's companion note by its frontmatter so
// clear-time tooling can decide whether to preserve, delete, or float it.
type Disposition string

const (
	// DispositionPark marks a record note carrying park's native frontmatter.
	DispositionPark Disposition = "park"
	// DispositionWorkOrder marks a disposable note stamped kind: work-order.
	DispositionWorkOrder Disposition = "work-order"
	// DispositionFloat marks a note with no recognized disposition.
	DispositionFloat Disposition = "float"
)

// defaultRecordSource is stamped into a record's source field when none is
// supplied. A todo note originates from the repo's task list.
const defaultRecordSource = "repo"

// NoteDisposition reads the leading frontmatter block of a note and classifies
// it: park when a category field is present, work-order when a kind: work-order
// marker is present, and float when neither is found. A note without any
// frontmatter block is float, never an error.
func NoteDisposition(path string) (Disposition, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	//nolint:errcheck // read-only file; close error not meaningful here
	defer f.Close()

	var hasCategory, hasWorkOrder bool
	scanner := bufio.NewScanner(f)
	if scanner.Scan() && strings.TrimSpace(scanner.Text()) == "---" {
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "---" {
				break
			}
			key, val, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "category":
				hasCategory = true
			case "kind":
				if strings.TrimSpace(val) == "work-order" {
					hasWorkOrder = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	if hasCategory {
		return DispositionPark, nil
	}
	if hasWorkOrder {
		return DispositionWorkOrder, nil
	}
	return DispositionFloat, nil
}

// buildNoteContent prefixes body with the note's disposition frontmatter.
// kind "work-order" stamps the disposable marker; otherwise a park record
// frontmatter is stamped, defaulting category to areas and synopsis/source to
// the task summary and the repo, respectively. created is the note's write date.
func buildNoteContent(body, kind, category, synopsis, source, fallbackSynopsis, created string) (string, error) {
	if kind != "" {
		if kind != "work-order" {
			return "", fmt.Errorf("invalid kind %q: want work-order", kind)
		}
		return "---\nkind: work-order\n---\n\n" + body, nil
	}

	if category == "" {
		category = string(schema.CategoryAreas)
	} else if !isCategory(category) {
		return "", fmt.Errorf("invalid category %q: want one of %s", category, strings.Join(schema.Categories(), ", "))
	}
	if synopsis == "" {
		synopsis = fallbackSynopsis
	}
	if source == "" {
		source = defaultRecordSource
	}

	return schema.Render(schema.Frontmatter{
		Category: category,
		Created:  created,
		Source:   source,
		Synopsis: synopsis,
	}, body), nil
}

// isCategory reports whether s is a canonical park category.
func isCategory(s string) bool {
	return slices.Contains(schema.Categories(), s)
}
