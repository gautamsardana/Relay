package store

import (
	"reflect"
	"testing"
)

func TestCSVRoundTrip(t *testing.T) {
	cases := [][]string{
		{"golang"},
		{"golang", "kubernetes"},
	}
	for _, c := range cases {
		if got := splitCSV(joinCSV(c)); !reflect.DeepEqual(got, c) {
			t.Fatalf("round trip: in=%v out=%v", c, got)
		}
	}
	if got := splitCSV(joinCSV(nil)); got != nil {
		t.Fatalf("empty should be nil, got %v", got)
	}
	if got := splitCSV("a, b ,,c"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("splitCSV trim/empty handling: %v", got)
	}
}
