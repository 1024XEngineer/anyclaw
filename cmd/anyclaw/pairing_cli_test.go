package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

type pairingTestFrame struct {
	Type   string         `json:"type"`
	ID     string         `json:"id,omitempty"`
	Method string         `json:"method,omitempty"`
	Event  string         `json:"event,omitempty"`
	Params map[string]any `json:"params,omitempty"`
	Data   any            `json:"data,omitempty"`
	OK     bool           `json:"ok,omitempty"`
	Error  string         `json:"error,omitempty"`
}

type pairingRequestRecord struct {
	Method string
	Params map[string]any
	Auth   string
}

func TestRunAnyClawCLIRoutesPairingUsage(t *testing.T) {
	clearModelsCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"pairing"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI pairing: %v", err)
	}
	for _, want := range []string{
		"AnyClaw pairing commands:",
		"anyclaw pairing generate",
		"anyclaw pairing list",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunPairingCommandsUseGatewayWebSocket(t *testing.T) {
	clearModelsCLIEnv(t)

	serverURL, records := newPairingMockGateway(t)
	configPath := writeStatusCLIConfig(t, serverURL, "pairing-token")

	tests := []struct {
		name       string
		args       []string
		wantOut    string
		wantMethod string
	}{
		{
			name:       "generate",
			args:       []string{"pairing", "generate", "--config", configPath, "--name", "Laptop", "--type", "desktop"},
			wantOut:    "Pairing Code Generated",
			wantMethod: "device.pairing.generate",
		},
		{
			name:       "list",
			args:       []string{"pairing", "list", "--config", configPath},
			wantOut:    "Paired Devices",
			wantMethod: "device.pairing.list",
		},
		{
			name:       "status",
			args:       []string{"pairing", "status", "--config", configPath},
			wantOut:    "Device Pairing Status",
			wantMethod: "device.pairing.status",
		},
		{
			name:       "renew",
			args:       []string{"pairing", "renew", "--config", configPath, "--device", "dev-1"},
			wantOut:    "Renewed pairing: dev-1",
			wantMethod: "device.pairing.renew",
		},
		{
			name:       "unpair",
			args:       []string{"pairing", "unpair", "--config", configPath, "--device", "dev-1"},
			wantOut:    "Unpaired device: dev-1",
			wantMethod: "device.pairing.unpair",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := captureCLIOutput(t, func() error {
				return runAnyClawCLI(tc.args)
			})
			if err != nil {
				t.Fatalf("runAnyClawCLI %v: %v", tc.args, err)
			}
			if !strings.Contains(stdout, tc.wantOut) {
				t.Fatalf("expected %q in output, got %q", tc.wantOut, stdout)
			}
		})
	}

	got := records.snapshot()
	if len(got) != len(tests) {
		t.Fatalf("expected %d gateway requests, got %#v", len(tests), got)
	}
	for i, tc := range tests {
		if got[i].Method != tc.wantMethod {
			t.Fatalf("request %d: expected method %q, got %#v", i, tc.wantMethod, got[i])
		}
		if got[i].Auth != "Bearer pairing-token" {
			t.Fatalf("request %d: expected bearer token, got %q", i, got[i].Auth)
		}
	}
	if got[0].Params["device_name"] != "Laptop" || got[0].Params["device_type"] != "desktop" {
		t.Fatalf("generate params were not forwarded: %#v", got[0].Params)
	}
	if got[3].Params["device_id"] != "dev-1" || got[4].Params["device_id"] != "dev-1" {
		t.Fatalf("device params were not forwarded: renew=%#v unpair=%#v", got[3].Params, got[4].Params)
	}
}

func TestRunPairingDeviceCommandsRequireDeviceID(t *testing.T) {
	clearModelsCLIEnv(t)

	for _, args := range [][]string{
		{"pairing", "renew"},
		{"pairing", "unpair"},
	} {
		_, _, err := captureCLIOutput(t, func() error {
			return runAnyClawCLI(args)
		})
		if err == nil || !strings.Contains(err.Error(), "device ID is required") {
			t.Fatalf("expected device validation error for %v, got %v", args, err)
		}
	}
}

type pairingRecords struct {
	mu      sync.Mutex
	records []pairingRequestRecord
}

func (r *pairingRecords) append(record pairingRequestRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
}

func (r *pairingRecords) snapshot() []pairingRequestRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]pairingRequestRecord, len(r.records))
	copy(out, r.records)
	return out
}

func newPairingMockGateway(t *testing.T) (string, *pairingRecords) {
	t.Helper()

	records := &pairingRecords{}
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_ = conn.WriteJSON(pairingTestFrame{Type: "event", Event: "connect.challenge", Data: map[string]any{"nonce": "challenge-token"}})
		var connectReq pairingTestFrame
		if err := conn.ReadJSON(&connectReq); err != nil {
			return
		}
		_ = conn.WriteJSON(pairingTestFrame{Type: "res", ID: connectReq.ID, OK: true, Data: map[string]any{"connected": true}})

		var frame pairingTestFrame
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		records.append(pairingRequestRecord{
			Method: frame.Method,
			Params: clonePairingParams(frame.Params),
			Auth:   r.Header.Get("Authorization"),
		})

		resp := pairingTestFrame{Type: "res", ID: frame.ID, OK: true, Data: pairingResponseData(frame.Method)}
		_ = conn.WriteJSON(resp)
	}))
	t.Cleanup(server.Close)
	return server.URL, records
}

func pairingResponseData(method string) any {
	switch method {
	case "device.pairing.generate":
		return map[string]any{"code": "abcd1234", "expires": "soon", "device": "Laptop", "type": "desktop"}
	case "device.pairing.list":
		return map[string]any{"devices": []any{map[string]any{"device_id": "dev-1", "device_name": "Laptop", "device_type": "desktop", "status": "paired"}}}
	case "device.pairing.status":
		return map[string]any{"enabled": true, "max_devices": float64(5), "paired": float64(1), "active": float64(1), "expired": float64(0), "codes": float64(1)}
	case "device.pairing.renew":
		return map[string]any{"device_id": "dev-1", "status": "paired"}
	case "device.pairing.unpair":
		return map[string]any{"ok": true}
	default:
		return map[string]any{}
	}
}

func clonePairingParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return nil
	}
	data, _ := json.Marshal(params)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}
