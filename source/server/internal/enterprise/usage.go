package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"
)

// UsageReceipt identifies one durable client event. The server decides whether
// the event was accepted; this type carries no prompt or provider credentials.
type UsageReceipt struct {
	Index     int    `json:"index"`
	AttemptID string `json:"attempt_id"`
	Revision  uint64 `json:"revision"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
}

type UsageResponse struct {
	SchemaVersion   string         `json:"schema_version"`
	BatchID         string         `json:"batch_id"`
	Acknowledgments []UsageReceipt `json:"acknowledgments"`
}

// ReportUsage uses the existing verified native connection. The caller must
// persist every event before invoking it and retire events only after checking
// the response against the exact submitted batch.
func (h *Host) ReportUsage(ctx context.Context, org, host, batchID string, events []json.RawMessage) (UsageResponse, error) {
	h.mu.Lock()
	manager, blocked := h.manager, !h.managed || h.changing || h.closed
	h.mu.Unlock()
	if blocked || manager == nil || org == "" || host == "" || batchID == "" || len(events) == 0 || len(events) > 64 {
		return UsageResponse{}, ErrNotConnected
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.credentials == nil || manager.credentials.OrganizationID != org || manager.credentials.HostID != host {
		return UsageResponse{}, ErrDenied
	}
	if err := manager.refresh(ctx); err != nil {
		return UsageResponse{}, err
	}
	if manager.credentials == nil || manager.credentials.OrganizationID != org || manager.credentials.HostID != host {
		return UsageResponse{}, ErrDenied
	}
	request := struct {
		SchemaVersion        string            `json:"schema_version"`
		MeterSchema          string            `json:"meter_schema"`
		BatchID              string            `json:"batch_id"`
		ClientInstallationID string            `json:"client_installation_id"`
		Events               []json.RawMessage `json:"events"`
	}{"cercano.usage-report.v2", "cercano.meters.v1", batchID, host, events}
	path := "/v1/organizations/" + url.PathEscape(org) + "/hosts/" + url.PathEscape(host) + "/usage-reports"
	var response UsageResponse
	if err := manager.request(ctx, manager.credentials.Server, "POST", path, manager.credentials.Access, request, &response, 1<<20); err != nil {
		return UsageResponse{}, err
	}
	if response.SchemaVersion != request.SchemaVersion || response.BatchID != batchID || len(response.Acknowledgments) != len(events) {
		return UsageResponse{}, errors.New("enterprise usage response does not match batch")
	}
	return response, nil
}

// Reporting uses a short timeout independently of the host's policy poll.
const UsageReportTimeout = 5 * time.Second
