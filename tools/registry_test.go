package tools

import (
	"sort"
	"testing"
)

// registerOutOfOrder builds a registry by inserting tools in an order that
// does not match their sorted name order, so a stable result from All/Names
// cannot be an accident of insertion order.
func registerOutOfOrder() *Registry {
	r := NewRegistry()
	r.Register(NewWebSearchTool())
	r.Register(NewBashTool())
	r.Register(NewGlobTool())
	r.Register(NewFileReadTool())
	r.Register(NewGrepTool())
	return r
}

func TestRegistryAllSorted(t *testing.T) {
	r := registerOutOfOrder()

	got := r.All()
	names := make([]string, len(got))
	for i, tool := range got {
		names[i] = tool.Name()
	}

	want := append([]string(nil), names...)
	sort.Strings(want)

	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("All() not sorted by name: got %v, want %v", names, want)
		}
	}
}

func TestRegistryNamesSorted(t *testing.T) {
	r := registerOutOfOrder()

	got := r.Names()
	want := append([]string(nil), got...)
	sort.Strings(want)

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() not sorted: got %v, want %v", got, want)
		}
	}
}

func TestRegistryOrderStableAcrossCalls(t *testing.T) {
	r := registerOutOfOrder()

	firstAll := r.All()
	firstNames := r.Names()

	for i := 0; i < 20; i++ {
		all := r.All()
		if len(all) != len(firstAll) {
			t.Fatalf("All() length changed: got %d, want %d", len(all), len(firstAll))
		}
		for j := range all {
			if all[j].Name() != firstAll[j].Name() {
				t.Fatalf("All() order changed on call %d at index %d: got %q, want %q",
					i, j, all[j].Name(), firstAll[j].Name())
			}
		}

		names := r.Names()
		if len(names) != len(firstNames) {
			t.Fatalf("Names() length changed: got %d, want %d", len(names), len(firstNames))
		}
		for j := range names {
			if names[j] != firstNames[j] {
				t.Fatalf("Names() order changed on call %d at index %d: got %q, want %q",
					i, j, names[j], firstNames[j])
			}
		}
	}
}
