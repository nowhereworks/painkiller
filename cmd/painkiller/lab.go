package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// labSpec is trusted exam metadata. Only topology is published in the catalog.
type labSpec struct {
	Controllers        int    `json:"controllers"`
	Workers            int    `json:"workers"`
	InitialControllers int    `json:"initial_controllers"`
	Mode               string `json:"provision_mode"`
	CNI                string `json:"cni"`
	Endpoint           string `json:"endpoint,omitempty"`
	MemoryMB           int    `json:"memory_mb"`
	Template           string `json:"template_id,omitempty"`
	Prepare            string `json:"prepare,omitempty"`
}

// Snapshot every operational input, including empty optional values, so a
// restart or environment change cannot redirect teardown or guest SSH.
type labConfig struct {
	TestID          string            `json:"test_id"`
	CreationStarted bool              `json:"creation_started,omitempty"`
	Env             map[string]string `json:"env"`
}

var labEnvKeys = []string{
	"LAB_PROXMOX_HOST", "LAB_PROVISION_MODE", "LAB_TEMPLATE_ID", "LAB_VM_START", "LAB_VM_END",
	"LAB_CONTROLLERS", "LAB_INITIAL_CONTROLLERS", "LAB_MARKER", "LAB_IP_PREFIX", "LAB_READY_TIMEOUT",
	"LAB_SSH_USER", "LAB_VM_MEMORY", "LAB_KNOWN_HOSTS", "LAB_KUBERNETES_VERSION",
	"LAB_CNI_PLUGIN", "LAB_CONTROL_PLANE_ENDPOINT",
}

func trustedExamFile(exam examDefinition, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) || filepath.Clean(name) != name ||
		!regexp.MustCompile(`^[a-zA-Z0-9_/-]+\.sh$`).MatchString(name) ||
		strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("%s: unsafe preparation path %q", exam.ID, name)
	}
	root, err := filepath.EvalSymlinks(exam.dir)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: preparation path escapes exam", exam.ID)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: preparation must be a regular file", exam.ID)
	}
	return filepath.Abs(path)
}

func validateLabSpec(exam examDefinition) error {
	s := exam.Lab
	if s == nil {
		return nil
	}
	if s.Controllers < 1 || s.Workers < 0 || s.Controllers+s.Workers > 20 ||
		s.InitialControllers < 1 || s.InitialControllers > s.Controllers || s.MemoryMB < 2048 ||
		(s.Mode != "legacy" && s.Mode != "prebaked") ||
		(s.CNI != "flannel" && s.CNI != "calico") {
		return fmt.Errorf("%s: invalid lab specification", exam.ID)
	}
	if s.Endpoint != "" {
		host, port, err := net.SplitHostPort(s.Endpoint)
		p, parseErr := strconv.Atoi(port)
		if err != nil || parseErr != nil || !validEndpointHost(host) || p < 1 || p > 65535 {
			return fmt.Errorf("%s: invalid lab endpoint", exam.ID)
		}
	}
	if s.Mode == "prebaked" {
		// Prebaked supports two images: the single-controller Flannel image and
		// the three-controller Calico HA image behind a stable endpoint.
		single := s.Controllers == 1 && s.CNI == "flannel" && s.Endpoint == ""
		ha := s.Controllers == 3 && s.CNI == "calico" && s.Endpoint != ""
		if !single && !ha {
			return fmt.Errorf("%s: incompatible prebaked topology (single-controller Flannel, or three-controller Calico HA)", exam.ID)
		}
		if s.Controllers > 1 && s.Template == "" {
			return fmt.Errorf("%s: prebaked multi-controller topology requires template_id", exam.ID)
		}
	}
	if s.Template != "" {
		id, err := strconv.Atoi(s.Template)
		if err != nil || id < 9100 || id > 9200 {
			return fmt.Errorf("%s: invalid template_id %q", exam.ID, s.Template)
		}
	}
	if s.Prepare != "" {
		_, err := trustedExamFile(exam, s.Prepare)
		return err
	}
	return nil
}

// kubeadm accepts either an IP address or a DNS name for controlPlaneEndpoint.
func validEndpointHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func resolveLab(exam examDefinition) (*labConfig, error) {
	c := &labConfig{TestID: exam.ID, Env: make(map[string]string)}
	defaults := map[string]string{
		"LAB_PROXMOX_HOST": "root@prox-k12", "LAB_PROVISION_MODE": "prebaked",
		"LAB_VM_START": "3000", "LAB_VM_END": "3002", "LAB_CONTROLLERS": "1",
		"LAB_MARKER": "painkiller-mvp", "LAB_IP_PREFIX": "192.168.254.",
		"LAB_READY_TIMEOUT": "600", "LAB_SSH_USER": "milkman", "LAB_VM_MEMORY": "3072",
		"LAB_KNOWN_HOSTS": "data/lab_known_hosts", "LAB_KUBERNETES_VERSION": "1.36.1", "LAB_CNI_PLUGIN": "flannel",
	}
	for _, key := range labEnvKeys {
		c.Env[key] = env(key, defaults[key])
	}
	if s := exam.Lab; s != nil {
		start, err := strconv.Atoi(c.Env["LAB_VM_START"])
		if err != nil {
			return nil, fmt.Errorf("invalid LAB_VM_START")
		}
		required := map[string]string{
			"LAB_CONTROLLERS": strconv.Itoa(s.Controllers), "LAB_INITIAL_CONTROLLERS": strconv.Itoa(s.InitialControllers),
			"LAB_PROVISION_MODE": s.Mode, "LAB_CNI_PLUGIN": s.CNI,
			"LAB_CONTROL_PLANE_ENDPOINT": s.Endpoint, "LAB_VM_END": strconv.Itoa(start + s.Controllers + s.Workers - 1),
		}
		for key, want := range required {
			if got := os.Getenv(key); got != "" && got != want {
				return nil, fmt.Errorf("%s requires %s=%s; explicit %s is incompatible", exam.ID, key, want, got)
			}
			c.Env[key] = want
		}
		memory, err := strconv.Atoi(env("LAB_VM_MEMORY", strconv.Itoa(s.MemoryMB)))
		if err != nil || memory < s.MemoryMB {
			return nil, fmt.Errorf("%s requires LAB_VM_MEMORY >= %d", exam.ID, s.MemoryMB)
		}
		c.Env["LAB_VM_MEMORY"] = strconv.Itoa(memory)
	}
	template := "9100"
	if c.Env["LAB_PROVISION_MODE"] == "legacy" {
		template = "9010"
	}
	if exam.Lab != nil && exam.Lab.Template != "" {
		template = exam.Lab.Template
	}
	c.Env["LAB_TEMPLATE_ID"] = env("LAB_TEMPLATE_ID", template)
	if c.Env["LAB_INITIAL_CONTROLLERS"] == "" {
		c.Env["LAB_INITIAL_CONTROLLERS"] = c.Env["LAB_CONTROLLERS"]
	}
	start, e1 := strconv.Atoi(c.Env["LAB_VM_START"])
	end, e2 := strconv.Atoi(c.Env["LAB_VM_END"])
	controllers, e3 := strconv.Atoi(c.Env["LAB_CONTROLLERS"])
	if e1 != nil || e2 != nil || e3 != nil || start < 3000 || end > 3999 || end < start ||
		controllers < 1 || controllers > end-start+1 {
		return nil, fmt.Errorf("invalid LAB_VM_START/LAB_VM_END or LAB_CONTROLLERS")
	}
	// Pin relative paths now, not relative to a later server working directory.
	path, err := filepath.Abs(c.Env["LAB_KNOWN_HOSTS"])
	if err != nil {
		return nil, err
	}
	c.Env["LAB_KNOWN_HOSTS"] = path
	return c, nil
}

func (a *app) labValue(key, fallback string) string {
	if a.state.Lab != nil {
		if v, ok := a.state.Lab.Env[key]; ok {
			return v
		}
	}
	return env(key, fallback)
}

func (a *app) labCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	if a.state.Lab == nil {
		return cmd // legacy saved attempts retain their original environment behavior
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, pinned := a.state.Lab.Env[key]; !pinned {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	for key, value := range a.state.Lab.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return cmd
}

func (a *app) labSSHArgs() []string {
	return []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=yes",
		"-o", "GlobalKnownHostsFile=/dev/null", "-o", "UserKnownHostsFile=" + a.labValue("LAB_KNOWN_HOSTS", "data/lab_known_hosts")}
}
