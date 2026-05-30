package attempts

import (
	"testing"

	"painkiller-shell/internal/models"
)

func TestValidTransitions(t *testing.T) {
	cases := []struct {
		from models.AttemptStatus
		to   models.AttemptStatus
		want bool
	}{
		{models.AttemptStatusPurchased, models.AttemptStatusAvailable, true},
		{models.AttemptStatusPurchased, models.AttemptStatusExpiredBeforeStart, true},
		{models.AttemptStatusPurchased, models.AttemptStatusRunning, false},
		{models.AttemptStatusAvailable, models.AttemptStatusAttemptRequested, true},
		{models.AttemptStatusAvailable, models.AttemptStatusExpiredBeforeStart, true},
		{models.AttemptStatusAvailable, models.AttemptStatusScored, false},
		{models.AttemptStatusAttemptRequested, models.AttemptStatusEnvironmentProvisioning, true},
		{models.AttemptStatusAttemptRequested, models.AttemptStatusProvisionFailed, true},
		{models.AttemptStatusEnvironmentProvisioning, models.AttemptStatusEnvironmentReady, true},
		{models.AttemptStatusEnvironmentProvisioning, models.AttemptStatusProvisionFailed, true},
		{models.AttemptStatusEnvironmentReady, models.AttemptStatusTerminalOpened, true},
		{models.AttemptStatusEnvironmentReady, models.AttemptStatusExpiredBeforeStart, true},
		{models.AttemptStatusTerminalOpened, models.AttemptStatusRunning, true},
		{models.AttemptStatusTerminalOpened, models.AttemptStatusSubmitted, false},
		{models.AttemptStatusRunning, models.AttemptStatusSubmitted, true},
		{models.AttemptStatusRunning, models.AttemptStatusExpired, true},
		{models.AttemptStatusRunning, models.AttemptStatusExpiredRunning, true},
		{models.AttemptStatusRunning, models.AttemptStatusScored, false},
		{models.AttemptStatusSubmitted, models.AttemptStatusGrading, true},
		{models.AttemptStatusExpired, models.AttemptStatusGrading, true},
		{models.AttemptStatusExpiredRunning, models.AttemptStatusGrading, true},
		{models.AttemptStatusGrading, models.AttemptStatusScored, true},
		{models.AttemptStatusScored, models.AttemptStatusCleanupPending, true},
		{models.AttemptStatusCleanupPending, models.AttemptStatusDestroyed, true},
		{models.AttemptStatusCleanupPending, models.AttemptStatusCleanupFailed, true},
		{models.AttemptStatusProvisionFailed, models.AttemptStatusCleanupPending, true},
		{models.AttemptStatusProvisionFailed, models.AttemptStatusAttemptRequested, true},
		{models.AttemptStatusExpiredBeforeStart, models.AttemptStatusCleanupPending, true},
		{models.AttemptStatusCleanupFailed, models.AttemptStatusCleanupPending, true},
		{models.AttemptStatusDestroyed, models.AttemptStatusPurchased, false},
		{models.AttemptStatusScored, models.AttemptStatusRunning, false},
	}

	for _, tc := range cases {
		got := ValidTransition(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("ValidTransition(%s -> %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestCanTransition(t *testing.T) {
	status, ok := CanTransition(
		models.AttemptStatusRunning,
		models.AttemptStatusScored,
		models.AttemptStatusSubmitted,
	)
	if !ok {
		t.Fatal("expected CanTransition to find a valid target")
	}
	if status != models.AttemptStatusSubmitted {
		t.Errorf("expected submitted, got %s", status)
	}
}

func TestCanTransitionNoMatch(t *testing.T) {
	_, ok := CanTransition(
		models.AttemptStatusRunning,
		models.AttemptStatusScored,
		models.AttemptStatusPurchased,
	)
	if ok {
		t.Fatal("expected CanTransition to find no valid target")
	}
}

func TestValidTransitionUnknownStatus(t *testing.T) {
	if ValidTransition("nonexistent", models.AttemptStatusRunning) {
		t.Error("expected false for unknown source status")
	}
}

func TestFullHappyPathLifecycle(t *testing.T) {
	path := []models.AttemptStatus{
		models.AttemptStatusPurchased,
		models.AttemptStatusAvailable,
		models.AttemptStatusAttemptRequested,
		models.AttemptStatusEnvironmentProvisioning,
		models.AttemptStatusEnvironmentReady,
		models.AttemptStatusTerminalOpened,
		models.AttemptStatusRunning,
		models.AttemptStatusSubmitted,
		models.AttemptStatusGrading,
		models.AttemptStatusScored,
		models.AttemptStatusCleanupPending,
		models.AttemptStatusDestroyed,
	}

	for i := 0; i < len(path)-1; i++ {
		if !ValidTransition(path[i], path[i+1]) {
			t.Errorf("expected valid transition from %s to %s", path[i], path[i+1])
		}
	}
}
