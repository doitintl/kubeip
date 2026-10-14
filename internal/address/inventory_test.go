package address

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/doitintl/kubeip/internal/cloud"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

func TestGCPInventoryPaginationAndFilters(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		filter := r.URL.Query().Get("filter")
		require.Contains(t, filter, "(labels.pool=lab)")
		require.Contains(t, filter, "(addressType=EXTERNAL)")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(filter, "status=RESERVED") && r.URL.Query().Get("pageToken") == "":
			_, _ = w.Write([]byte(`{"items":[{"address":"203.0.113.1"}],"nextPageToken":"next"}`))
		case strings.Contains(filter, "status=RESERVED"):
			_, _ = w.Write([]byte(`{"items":[{"address":"203.0.113.2"}]}`))
		default:
			_, _ = w.Write([]byte(`{"items":[{"address":"203.0.113.3"}]}`))
		}
	}))
	defer server.Close()
	svc, err := compute.NewService(context.Background(), option.WithEndpoint(server.URL), option.WithoutAuthentication())
	require.NoError(t, err)
	a := &gcpAssigner{lister: cloud.NewLister(svc), project: "lab", region: "us-central1"}
	available, assigned, err := a.Inventory(context.Background(), []string{"labels.pool=lab"})
	require.NoError(t, err)
	require.Equal(t, 2, available)
	require.Equal(t, 1, assigned)
	require.Equal(t, 3, requests)
}
func TestAWSIPAMInventoryCountsReservations(t *testing.T) {
	t.Parallel()
	a, f := newIPAMFixture(t)
	_, err := a.Assign(context.Background(), "i-test", "", nil, "")
	require.NoError(t, err)
	available, assigned, err := a.Inventory(context.Background(), nil)
	require.NoError(t, err)
	require.Zero(t, available)
	require.Equal(t, 1, assigned)
	require.NoError(t, f.Unassign(context.Background(), "eipassoc-test"))
	available, assigned, err = a.Inventory(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, available)
	require.Zero(t, assigned)
}
