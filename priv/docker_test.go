package priv_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moby/moby/client"

	"github.com/buildpacks/lifecycle/priv"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

// TestDockerClient_SupportsOlderAPIVersions is a regression test for
// https://github.com/buildpacks/lifecycle/issues/1607, where a dependency bump
// implicitly raised the minimum supported Docker Engine API version to v1.44,
// breaking compatibility with all released versions of podman (which top out at v1.41).
func TestDockerClient_SupportsOlderAPIVersions(t *testing.T) {
	// podman (as of v5.7) reports API version 1.41 on ping.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", "1.41")
		w.Header().Set("Docker-Experimental", "false")
		w.Header().Set("Ostype", "linux")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("DOCKER_HOST", server.URL)

	cli, err := priv.DockerClient()
	h.AssertNil(t, err)

	result, err := cli.Ping(context.Background(), client.PingOptions{NegotiateAPIVersion: true})
	h.AssertNil(t, err)
	h.AssertEq(t, result.APIVersion, "1.41")
}
