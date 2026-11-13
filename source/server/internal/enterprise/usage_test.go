package enterprise

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReportUsageAuthenticatedScopeAndResponse(t *testing.T) {
	const org = "11111111-1111-1111-1111-111111111111"
	const hostID = "22222222-2222-2222-2222-222222222222"
	called := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/organizations/"+org+"/hosts/"+hostID+"/usage-reports" || r.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Errorf("wrong report target or authorization")
		}
		var body struct {
			SchemaVersion        string            `json:"schema_version"`
			MeterSchema          string            `json:"meter_schema"`
			BatchID              string            `json:"batch_id"`
			ClientInstallationID string            `json:"client_installation_id"`
			Events               []json.RawMessage `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.SchemaVersion != "cercano.usage-report.v2" || body.MeterSchema != "cercano.meters.v1" || body.BatchID != "batch" || body.ClientInstallationID != hostID || len(body.Events) != 1 {
			t.Errorf("wrong report envelope: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":"cercano.usage-report.v2","batch_id":"batch","acknowledgments":[{"index":0,"attempt_id":"attempt","revision":1,"status":"accepted"}]}`))
	}))
	defer server.Close()
	manager := &Manager{client: server.Client(), credentials: &Credentials{Server: server.URL, OrganizationID: org, HostID: hostID, Access: "synthetic-token", ExpiresAt: time.Now().Add(time.Hour)}}
	h := &Host{managed: true, manager: manager}
	if _, err := h.ReportUsage(context.Background(), org, "foreign", "batch", []json.RawMessage{json.RawMessage(`{"attempt_id":"attempt"}`)}); err != ErrDenied || called != 0 {
		t.Fatalf("foreign host attempted upload: %v %d", err, called)
	}
	response, err := h.ReportUsage(context.Background(), org, hostID, "batch", []json.RawMessage{json.RawMessage(`{"attempt_id":"attempt"}`)})
	if err != nil || called != 1 || len(response.Acknowledgments) != 1 {
		t.Fatalf("valid report: %v %+v calls=%d", err, response, called)
	}
}
