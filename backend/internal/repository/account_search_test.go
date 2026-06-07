//go:build unit

package repository

import "testing"

func TestParseAccountSearchID(t *testing.T) {
	tests := []struct {
		name   string
		search string
		wantID int64
		wantOK bool
	}{
		{name: "plain id", search: "2190", wantID: 2190, wantOK: true},
		{name: "hash id", search: "#2190", wantID: 2190, wantOK: true},
		{name: "trim spaces", search: " 2190 ", wantID: 2190, wantOK: true},
		{name: "empty", search: "", wantOK: false},
		{name: "negative", search: "-1", wantOK: false},
		{name: "name", search: "findcg", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := parseAccountSearchID(tt.search)
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Fatalf("parseAccountSearchID(%q) = (%d, %v), want (%d, %v)", tt.search, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}
