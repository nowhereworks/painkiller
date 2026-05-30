package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestNextVMIDParsesStringAndNumber(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "string", body: `{"data":"1234"}`},
		{name: "number", body: `{"data":1234}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api2/json/cluster/nextid" {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := NewClient(Config{APIURL: server.URL})
			got, err := client.NextVMID(context.Background())
			if err != nil {
				t.Fatalf("NextVMID failed: %v", err)
			}
			if got != 1234 {
				t.Fatalf("NextVMID = %d, want 1234", got)
			}
		})
	}
}

func TestCloneVMIncludesAllocatedNewID(t *testing.T) {
	cloneIDs := []int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/nextid":
			_, _ = w.Write([]byte(`{"data":"1234"}`))

		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve1/qemu/900/clone":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode clone body: %v", err)
			}
			newID, ok := body["newid"].(float64)
			if !ok || int(newID) != 1234 {
				t.Fatalf("newid = %#v, want 1234", body["newid"])
			}
			if body["name"] != "workstation-test" {
				t.Fatalf("name = %#v, want workstation-test", body["name"])
			}
			if body["storage"] != "local-lvm" {
				t.Fatalf("storage = %#v, want local-lvm", body["storage"])
			}
			full, ok := body["full"].(float64)
			if !ok || int(full) != 1 {
				t.Fatalf("full = %#v, want 1", body["full"])
			}
			cloneIDs = append(cloneIDs, int(newID))
			_, _ = w.Write([]byte(`{"data":"UPID:pve1:00000000:00000000:00000000:qmclone:1234:root@pam:"}`))

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve1/tasks/"):
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIURL:      server.URL,
		Node:        "pve1",
		StoragePool: "local-lvm",
	})
	got, err := client.CloneVM(context.Background(), 900, "workstation-test", true)
	if err != nil {
		t.Fatalf("CloneVM failed: %v", err)
	}
	if got != 1234 {
		t.Fatalf("CloneVM = %d, want 1234", got)
	}
	if !reflect.DeepEqual(cloneIDs, []int{1234}) {
		t.Fatalf("clone IDs = %v, want [1234]", cloneIDs)
	}
}

func TestCloneVMRetriesOnVMIDConflict(t *testing.T) {
	nextIDs := []string{"1234", "1235"}
	cloneIDs := []int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/nextid":
			if len(nextIDs) == 0 {
				t.Fatal("unexpected nextid request")
			}
			id := nextIDs[0]
			nextIDs = nextIDs[1:]
			_, _ = w.Write([]byte(`{"data":"` + id + `"}`))

		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve1/qemu/900/clone":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode clone body: %v", err)
			}
			newID := int(body["newid"].(float64))
			cloneIDs = append(cloneIDs, newID)
			if newID == 1234 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"data":null,"message":"VM 1234 already exists"}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":"UPID:pve1:00000000:00000000:00000000:qmclone:1235:root@pam:"}`))

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve1/tasks/"):
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{APIURL: server.URL, Node: "pve1", StoragePool: "local-lvm"})
	got, err := client.CloneVM(context.Background(), 900, "workstation-test", true)
	if err != nil {
		t.Fatalf("CloneVM failed: %v", err)
	}
	if got != 1235 {
		t.Fatalf("CloneVM = %d, want 1235", got)
	}
	if !reflect.DeepEqual(cloneIDs, []int{1234, 1235}) {
		t.Fatalf("clone IDs = %v, want [1234 1235]", cloneIDs)
	}
}

func TestCloneVMDoesNotRetryNonConflict(t *testing.T) {
	cloneAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/nextid":
			_, _ = w.Write([]byte(`{"data":"1234"}`))

		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve1/qemu/900/clone":
			cloneAttempts++
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"data":null,"message":"permission denied"}`))

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{APIURL: server.URL, Node: "pve1", StoragePool: "local-lvm"})
	_, err := client.CloneVM(context.Background(), 900, "workstation-test", true)
	if err == nil {
		t.Fatal("expected CloneVM error")
	}
	if cloneAttempts != 1 {
		t.Fatalf("clone attempts = %d, want 1", cloneAttempts)
	}
}

func TestCloneVMLinkedMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/nextid":
			_, _ = w.Write([]byte(`{"data":"1234"}`))

		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve1/qemu/900/clone":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode clone body: %v", err)
			}
			full, ok := body["full"].(float64)
			if !ok || int(full) != 0 {
				t.Fatalf("full = %#v, want 0 for linked clone", body["full"])
			}
			if _, ok := body["storage"]; ok {
				t.Fatalf("storage should not be set for linked clone, got %#v", body["storage"])
			}
			_, _ = w.Write([]byte(`{"data":"UPID:pve1:00000000:00000000:00000000:qmclone:1234:root@pam:"}`))

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api2/json/nodes/pve1/tasks/"):
			_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		APIURL:      server.URL,
		Node:        "pve1",
		StoragePool: "local-lvm",
	})
	got, err := client.CloneVM(context.Background(), 900, "workstation-test", false)
	if err != nil {
		t.Fatalf("CloneVM failed: %v", err)
	}
	if got != 1234 {
		t.Fatalf("CloneVM = %d, want 1234", got)
	}
}
