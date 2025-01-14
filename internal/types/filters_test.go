package types

import (
	"testing"

	"github.com/pkg/errors"
)

func Test_types_CheckFreeformTagFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter OCIFilters
		target map[string]string
		want   bool
	}{
		{
			name:   "nil filter",
			filter: OCIFilters{FreeformTags: nil},
			target: map[string]string{"key1": "value1"},
			want:   true,
		},
		{
			name:   "nil target",
			filter: OCIFilters{FreeformTags: map[string]string{"key1": "value1"}},
			target: nil,
			want:   false,
		},
		{
			name:   "matching filter",
			filter: OCIFilters{FreeformTags: map[string]string{"key1": "value1"}},
			target: map[string]string{"key1": "value1"},
			want:   true,
		},
		{
			name:   "non-matching filter",
			filter: OCIFilters{FreeformTags: map[string]string{"key1": "value1"}},
			target: map[string]string{"key1": "value2"},
			want:   false,
		},
		{
			name:   "partial match",
			filter: OCIFilters{FreeformTags: map[string]string{"key1": "value1", "key2": "value2"}},
			target: map[string]string{"key1": "value1"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.CheckFreeformTagFilter(tt.target); got != tt.want {
				t.Errorf("CheckFreeformTagFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_types_ParseFreeformTagFilter(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		wantKey string
		wantVal string
		wantErr error
	}{
		{
			name:    "valid filter",
			filter:  "freeformTags.key=value",
			wantKey: "key",
			wantVal: "value",
			wantErr: nil,
		},
		{
			name:    "invalid filter format",
			filter:  "freeformTags.keyvalue",
			wantKey: "",
			wantVal: "",
			wantErr: errors.New("invalid filter format for freeform tags, should be in format freeformTags.key=value, found: freeformTags.keyvalue"),
		},
		{
			name:    "missing prefix",
			filter:  "key=value",
			wantKey: "",
			wantVal: "",
			wantErr: errors.New("invalid filter format for freeform tags, should be in format freeformTags.key=value, found: key=value"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotVal, err := ParseFreeformTagFilter(tt.filter)
			if gotKey != tt.wantKey || gotVal != tt.wantVal || (err != nil && err.Error() != tt.wantErr.Error()) {
				t.Errorf("ParseFreeformTagFilter() = (%v, %v, %v), want (%v, %v, %v)", gotKey, gotVal, err, tt.wantKey, tt.wantVal, tt.wantErr)
			}
		})
	}
}
