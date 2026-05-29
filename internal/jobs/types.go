package jobs

type JobKind string

const (
	JobKindProvisionEnvironment JobKind = "provision_environment"
	JobKindGradeAttempt         JobKind = "grade_attempt"
	JobKindCleanupEnvironment   JobKind = "cleanup_environment"
	JobKindExpireAttempt        JobKind = "expire_attempt"
)
