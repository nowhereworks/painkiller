package models

type AttemptStatus string

const (
	AttemptStatusPurchased              AttemptStatus = "purchased"
	AttemptStatusAvailable              AttemptStatus = "available"
	AttemptStatusAttemptRequested       AttemptStatus = "attempt_requested"
	AttemptStatusEnvironmentProvisioning AttemptStatus = "environment_provisioning"
	AttemptStatusEnvironmentReady       AttemptStatus = "environment_ready"
	AttemptStatusTerminalOpened         AttemptStatus = "terminal_opened"
	AttemptStatusRunning                AttemptStatus = "running"
	AttemptStatusSubmitted              AttemptStatus = "submitted"
	AttemptStatusExpired                AttemptStatus = "expired"
	AttemptStatusGrading                AttemptStatus = "grading"
	AttemptStatusScored                 AttemptStatus = "scored"
	AttemptStatusCleanupPending         AttemptStatus = "cleanup_pending"
	AttemptStatusDestroyed              AttemptStatus = "destroyed"
	AttemptStatusProvisionFailed        AttemptStatus = "provision_failed"
	AttemptStatusExpiredBeforeStart     AttemptStatus = "expired_before_start"
	AttemptStatusExpiredRunning         AttemptStatus = "expired_running"
	AttemptStatusCleanupFailed          AttemptStatus = "cleanup_failed"
)

type EnvironmentStatus string

const (
	EnvironmentStatusProvisioning EnvironmentStatus = "provisioning"
	EnvironmentStatusReady      EnvironmentStatus = "ready"
	EnvironmentStatusActive     EnvironmentStatus = "active"
	EnvironmentStatusDestroying EnvironmentStatus = "destroying"
	EnvironmentStatusDestroyed  EnvironmentStatus = "destroyed"
	EnvironmentStatusFailed     EnvironmentStatus = "failed"
)

type NodeRole string

const (
	NodeRoleControlPlane NodeRole = "control-plane"
	NodeRoleWorker       NodeRole = "worker"
)

type CheckType string

const (
	CheckTypeKubectl CheckType = "kubectl"
	CheckTypeScript  CheckType = "script"
)
