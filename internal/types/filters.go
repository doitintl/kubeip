package types

import (
	"strings"

	"github.com/pkg/errors"
)

// OCIFilters is a struct that holds the filters for the OCI.
type OCIFilters struct {
	FreeformTags map[string]string
	DefinedTags  map[string]map[string]interface{}
}

// CheckFreeformTagFilter checks if the target contains all the filter keys and values.
func (f *OCIFilters) CheckFreeformTagFilter(target map[string]string) bool {
	// If the filter is nil, return true, since there is no filter to apply
	if f.FreeformTags == nil {
		return true
	}

	// If the target is nil, return false, since filter cannot be applied
	if target == nil {
		return false
	}

	// Loop through the filter map and check if the target map contains all the filter keys and values
	for key, value := range f.FreeformTags {
		if val, ok := target[key]; !ok || val != value {
			return false
		}
	}
	return true
}

// ParseFreeformTagFilter parses the filter string for freeform tags.
// Filter should be in following format:
//   - "freeformTags.key=value"
func ParseFreeformTagFilter(filter string) (string, string, error) {
	f := filter
	if strings.HasPrefix(f, "freeformTags.") {
		f = strings.TrimPrefix(f, "freeformTags.")
		if split := strings.Split(f, "="); len(split) == 2 { //nolint:gomnd
			return split[0], split[1], nil
		}
	}

	return "", "", errors.New("invalid filter format for freeform tags, should be in format freeformTags.key=value, found: " + filter)
}

// ParseDefinedTagFilter parses the filter string for defined tags.
// Filter should be in following format:
//   - "definedTags.Namespace.key=value"
//
// TODO: Add filter support for DefinedTags
func ParseDefinedTagFilter(_ string) (string, string, string, error) {
	return "", "", "", nil
}

// CheckDefinedTagFilter checks if the target contains all the filter keys and values.
// TODO: Add filter support for DefinedTags
func (f *OCIFilters) CheckDefinedTagFilter(_ map[string]map[string]interface{}) bool {
	return true
}
