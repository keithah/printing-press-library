package cli

import (
	"reflect"
	"testing"
)

func TestDefaultSyncResourcesIncludesLibrary(t *testing.T) {
	if got, want := defaultSyncResources(), []string{"library"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("defaultSyncResources() = %v, want %v", got, want)
	}
}
