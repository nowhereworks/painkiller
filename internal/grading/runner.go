package grading

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
	"painkiller-shell/internal/models"
)

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) RunCheck(ctx context.Context, check models.Check, workstationIP string, sshKey []byte) (*CheckResult, error) {
	signer, err := ssh.ParsePrivateKey(sshKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", workstationIP+":22", config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial SSH: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	exitCode := 0
	if err := session.Run(check.Command); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return nil, fmt.Errorf("failed to run command: %w", err)
		}
	}

	passed := exitCode == 0
	pointsAwarded := 0
	if passed {
		pointsAwarded = check.Points
	}

	return &CheckResult{
		CheckID:        check.ID.String(),
		Passed:         passed,
		Stdout:         stdout.String(),
		Stderr:         stderr.String(),
		ExitCode:       exitCode,
		PointsAwarded:  pointsAwarded,
		PointsPossible: check.Points,
		RanAt:          time.Now(),
	}, nil
}
