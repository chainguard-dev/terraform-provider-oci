package testing

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
)

// SetupRegistry starts a local registry for testing.
//
// If TF_OCI_REGISTRY is set, it will be used instead.
func SetupRegistry(t *testing.T) (name.Registry, func()) {
	t.Helper()
	if got := os.Getenv("TF_OCI_REGISTRY"); got != "" {
		reg, err := name.NewRegistry(got)
		if err != nil {
			t.Fatalf("failed to parse TF_OCI_REGISTRY: %v", err)
		}
		return reg, func() {}
	}
	srv := httptest.NewServer(registry.New())
	t.Logf("Started registry: %s", srv.URL)
	reg, err := name.NewRegistry(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("failed to parse test registry: %v", err)
	}
	return reg, srv.Close
}

// SetupRegistry starts a local registry for testing and returns a repository within that registry.
//
// If TF_OCI_REGISTRY is set, that registry will be used instead.
func SetupRepository(t *testing.T, repo string) (name.Repository, func()) {
	reg, cleanup := SetupRegistry(t)
	// TODO: use reg.Repo after https://github.com/google/go-containerregistry/pull/1671
	r, err := name.NewRepository(reg.RegistryStr() + "/" + repo)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	return r, cleanup
}
