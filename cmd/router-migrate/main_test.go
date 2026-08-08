package main

import (
	"strings"
	"testing"

	"smart-llmrouter/internal/router"
)

func TestRequireServingCompatible(t *testing.T) {
	tests := []struct {
		name   string
		status router.MigrationStatus
		want   bool
	}{
		{name: "pending ledger", status: router.MigrationStatus{Compatible: true, State: "pending"}},
		{name: "incompatible ledger", status: router.MigrationStatus{Compatible: false, State: "current"}},
		{name: "unvalidated job", status: router.MigrationStatus{Compatible: true, State: "current", Jobs: []router.MigrationDataJobStatus{{State: "pending"}}}},
		{name: "current validated ledger", status: router.MigrationStatus{Compatible: true, State: "current", Jobs: []router.MigrationDataJobStatus{{State: "validated"}}}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireServingCompatible(tc.status)
			if (err == nil) != tc.want {
				t.Fatalf("requireServingCompatible(%+v) error = %v, want success %t", tc.status, err, tc.want)
			}
			if err != nil && !strings.Contains(err.Error(), "migration verify-serving requires current compatible ledger and validated data jobs") {
				t.Fatalf("error = %q", err)
			}
		})
	}
}
