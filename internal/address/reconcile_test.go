package address

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/doitintl/kubeip/internal/cloud"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

func TestGCPRestoreAddress(t *testing.T) {
	for _, scenario := range []string{"steady", "replace", "missing access config", "occupied", "claimed before deletion", "deletion wait fails", "add fails"} {
		t.Run(scenario, func(t *testing.T) {
			var deletes, adds atomic.Int32
			current := "203.0.113.99"
			if scenario == "steady" {
				current = "203.0.113.10"
			}
			if scenario == "missing access config" {
				current = ""
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				write := func(value any) { require.NoError(t, json.NewEncoder(w).Encode(value)) }
				switch {
				case strings.HasSuffix(r.URL.Path, "/instances/instance"):
					nic := &compute.NetworkInterface{Name: "nic0", Fingerprint: "fresh"}
					if current != "" {
						nic.AccessConfigs = []*compute.AccessConfig{{Name: "External IP", Type: "ONE_TO_ONE_NAT", NatIP: current}}
					}
					write(&compute.Instance{Name: "instance", NetworkInterfaces: []*compute.NetworkInterface{nic}})
				case strings.HasSuffix(r.URL.Path, "/addresses"):
					require.Contains(t, r.URL.Query().Get("filter"), `(address="203.0.113.10")`)
					require.Contains(t, r.URL.Query().Get("filter"), "(labels.pool=test)")
					if scenario == "occupied" {
						write(&compute.AddressList{})
						return
					}
					write(&compute.AddressList{Items: []*compute.Address{{Name: "desired", Address: "203.0.113.10", Status: "RESERVED"}}})
				case strings.HasSuffix(r.URL.Path, "/addresses/desired"):
					status := "RESERVED"
					if scenario == "claimed before deletion" {
						status = "IN_USE"
					}
					write(&compute.Address{Name: "desired", Address: "203.0.113.10", Status: status})
				case strings.HasSuffix(r.URL.Path, "/deleteAccessConfig"):
					deletes.Add(1)
					current = ""
					status := "DONE"
					if scenario == "deletion wait fails" {
						status = "PENDING"
					}
					write(&compute.Operation{Name: "delete", Status: status})
				case strings.HasSuffix(r.URL.Path, "/operations/delete/wait"):
					http.Error(w, "wait failed", http.StatusInternalServerError)
				case strings.HasSuffix(r.URL.Path, "/addAccessConfig"):
					adds.Add(1)
					var access compute.AccessConfig
					require.NoError(t, json.NewDecoder(r.Body).Decode(&access))
					require.Equal(t, "203.0.113.10", access.NatIP)
					if scenario == "add fails" {
						http.Error(w, "add failed", http.StatusForbidden)
						return
					}
					current = access.NatIP
					write(&compute.Operation{Name: "add", Status: "DONE"})
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			svc, err := compute.NewService(context.Background(), option.WithEndpoint(server.URL), option.WithoutAuthentication())
			require.NoError(t, err)
			a := &gcpAssigner{lister: cloud.NewLister(svc), instanceGetter: cloud.NewInstanceGetter(svc), addressManager: cloud.NewAddressManager(svc, false), waiter: cloud.NewZoneWaiter(svc), project: "test", region: "region", logger: logrus.NewEntry(logrus.New())}
			err = a.RestoreAddress(context.Background(), "instance", "zone", "203.0.113.10", []string{"labels.pool=test"})
			switch scenario {
			case "steady":
				require.NoError(t, err)
				require.Zero(t, deletes.Load())
				require.Zero(t, adds.Load())
			case "replace":
				require.NoError(t, err)
				require.EqualValues(t, 1, deletes.Load())
				require.EqualValues(t, 1, adds.Load())
			case "missing access config":
				require.NoError(t, err)
				require.Zero(t, deletes.Load())
				require.EqualValues(t, 1, adds.Load())
			case "occupied", "claimed before deletion":
				require.Error(t, err)
				require.Zero(t, deletes.Load())
				require.Zero(t, adds.Load())
			case "deletion wait fails":
				require.Error(t, err)
				require.EqualValues(t, 1, deletes.Load())
				require.Zero(t, adds.Load())
			case "add fails":
				require.Error(t, err)
				require.EqualValues(t, 1, deletes.Load())
				require.EqualValues(t, 1, adds.Load())
			}
		})
	}
}
