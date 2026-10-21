package api

import (
	"testing"

	"lodestar/internal/models"
)

func jobs(statuses ...string) []models.DeployJob {
	out := make([]models.DeployJob, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, models.DeployJob{Status: status})
	}
	return out
}

func TestAggregateStatus(t *testing.T) {
	cases := []struct {
		name     string
		statuses []string
		want     string
	}{
		{"no targets", nil, "no_targets"},
		{"all success", []string{"success", "success"}, "success"},
		{"all failed", []string{"failed", "failed"}, "failed"},
		{"mixed", []string{"success", "failed"}, "partial"},
		{"canceled counts as failed", []string{"success", "canceled"}, "partial"},
		{"pending wins", []string{"success", "pending"}, "running"},
		{"running wins", []string{"failed", "running"}, "running"},
		{"empty status is pending", []string{"success", ""}, "running"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := aggregateStatus(jobs(tc.statuses...)); got != tc.want {
				t.Fatalf("aggregateStatus(%v) = %q, want %q", tc.statuses, got, tc.want)
			}
		})
	}
}
