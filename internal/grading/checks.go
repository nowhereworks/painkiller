package grading

import (
	"time"
)

type CheckResult struct {
	CheckID       string
	Passed        bool
	Stdout        string
	Stderr        string
	ExitCode      int
	PointsAwarded int
	PointsPossible int
	RanAt         time.Time
}
