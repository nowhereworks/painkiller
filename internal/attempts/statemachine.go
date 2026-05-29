package attempts

import (
	"painkiller-shell/internal/models"
)

var validTransitions = map[models.AttemptStatus][]models.AttemptStatus{
	models.AttemptStatusPurchased: {
		models.AttemptStatusAvailable,
		models.AttemptStatusExpiredBeforeStart,
	},
	models.AttemptStatusAvailable: {
		models.AttemptStatusAttemptRequested,
		models.AttemptStatusExpiredBeforeStart,
	},
	models.AttemptStatusAttemptRequested: {
		models.AttemptStatusEnvironmentProvisioning,
		models.AttemptStatusProvisionFailed,
	},
	models.AttemptStatusEnvironmentProvisioning: {
		models.AttemptStatusEnvironmentReady,
		models.AttemptStatusProvisionFailed,
	},
	models.AttemptStatusEnvironmentReady: {
		models.AttemptStatusTerminalOpened,
		models.AttemptStatusExpiredBeforeStart,
	},
	models.AttemptStatusTerminalOpened: {
		models.AttemptStatusRunning,
	},
	models.AttemptStatusRunning: {
		models.AttemptStatusSubmitted,
		models.AttemptStatusExpired,
		models.AttemptStatusExpiredRunning,
	},
	models.AttemptStatusSubmitted: {
		models.AttemptStatusGrading,
	},
	models.AttemptStatusExpired: {
		models.AttemptStatusGrading,
	},
	models.AttemptStatusExpiredRunning: {
		models.AttemptStatusGrading,
	},
	models.AttemptStatusGrading: {
		models.AttemptStatusScored,
	},
	models.AttemptStatusScored: {
		models.AttemptStatusCleanupPending,
	},
	models.AttemptStatusCleanupPending: {
		models.AttemptStatusDestroyed,
		models.AttemptStatusCleanupFailed,
	},
	models.AttemptStatusProvisionFailed: {
		models.AttemptStatusCleanupPending,
		models.AttemptStatusAttemptRequested,
	},
	models.AttemptStatusExpiredBeforeStart: {
		models.AttemptStatusCleanupPending,
	},
	models.AttemptStatusCleanupFailed: {
		models.AttemptStatusCleanupPending,
	},
}

func ValidTransition(from, to models.AttemptStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, status := range allowed {
		if status == to {
			return true
		}
	}
	return false
}

func CanTransition(from models.AttemptStatus, to ...models.AttemptStatus) (models.AttemptStatus, bool) {
	for _, target := range to {
		if ValidTransition(from, target) {
			return target, true
		}
	}
	return "", false
}
