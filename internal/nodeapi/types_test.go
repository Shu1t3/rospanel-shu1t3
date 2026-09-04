package nodeapi

import (
	"testing"
)

func TestNormalizedComponents(t *testing.T) {
	tests := []struct {
		name           string
		req            SyncRequest
		awgConfigured  bool
		wantComponents []ComponentStatus
	}{
		{
			name: "forward compatible: explicit components preserve existing list",
			req: SyncRequest{
				Components: []ComponentStatus{
					{Name: ComponentXray, Running: true, Status: StatusHealthy, Version: "1.8.0"},
					{Name: ComponentAWG, Running: false, Status: StatusDegraded, Error: "peer timeout"},
				},
			},
			awgConfigured: true,
			wantComponents: []ComponentStatus{
				{Name: ComponentXray, Running: true, Status: StatusHealthy, Version: "1.8.0"},
				{Name: ComponentAWG, Running: false, Status: StatusDegraded, Error: "peer timeout"},
			},
		},
		{
			name: "backward compatible: legacy agent with Xray running and AWG configured and running",
			req: SyncRequest{
				XrayRunning: true,
				XrayVersion: "1.8.0",
				AWGRunning:  true,
			},
			awgConfigured: true,
			wantComponents: []ComponentStatus{
				{Name: ComponentXray, Running: true, Status: StatusHealthy, Version: "1.8.0"},
				{Name: ComponentAWG, Running: true, Status: StatusHealthy},
			},
		},
		{
			name: "backward compatible: legacy agent with Xray stopped",
			req: SyncRequest{
				XrayRunning: false,
				AWGRunning:  true,
			},
			awgConfigured: true,
			wantComponents: []ComponentStatus{
				{Name: ComponentXray, Running: false, Status: StatusUnhealthy},
				{Name: ComponentAWG, Running: true, Status: StatusHealthy},
			},
		},
		{
			name: "backward compatible: legacy agent with AWG down",
			req: SyncRequest{
				XrayRunning: true,
				AWGRunning:  false,
			},
			awgConfigured: true,
			wantComponents: []ComponentStatus{
				{Name: ComponentXray, Running: true, Status: StatusHealthy},
				{Name: ComponentAWG, Running: false, Status: StatusUnhealthy},
			},
		},
		{
			name: "backward compatible: legacy agent with AWG error",
			req: SyncRequest{
				XrayRunning: true,
				AWGRunning:  false,
				AWGError:    "failed to bind port",
			},
			awgConfigured: true,
			wantComponents: []ComponentStatus{
				{Name: ComponentXray, Running: true, Status: StatusHealthy},
				{Name: ComponentAWG, Running: false, Status: StatusUnhealthy, Error: "failed to bind port"},
			},
		},
		{
			name: "backward compatible: legacy agent with AWG unconfigured on node",
			req: SyncRequest{
				XrayRunning: true,
				AWGRunning:  false,
			},
			awgConfigured: false,
			wantComponents: []ComponentStatus{
				{Name: ComponentXray, Running: true, Status: StatusHealthy},
				{Name: ComponentAWG, Running: false, Status: StatusDisabled},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.req.NormalizedComponents(tc.awgConfigured)
			if len(got) != len(tc.wantComponents) {
				t.Fatalf("NormalizedComponents len = %d, want %d", len(got), len(tc.wantComponents))
			}
			for i := range got {
				if got[i].Name != tc.wantComponents[i].Name {
					t.Errorf("[%d] Name = %q, want %q", i, got[i].Name, tc.wantComponents[i].Name)
				}
				if got[i].Running != tc.wantComponents[i].Running {
					t.Errorf("[%d] Running = %v, want %v", i, got[i].Running, tc.wantComponents[i].Running)
				}
				if got[i].Status != tc.wantComponents[i].Status {
					t.Errorf("[%d] Status = %q, want %q", i, got[i].Status, tc.wantComponents[i].Status)
				}
				if got[i].Error != tc.wantComponents[i].Error {
					t.Errorf("[%d] Error = %q, want %q", i, got[i].Error, tc.wantComponents[i].Error)
				}
				if got[i].Version != tc.wantComponents[i].Version {
					t.Errorf("[%d] Version = %q, want %q", i, got[i].Version, tc.wantComponents[i].Version)
				}
			}
		})
	}
}
