package proxmox

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIURL        string
	TokenID       string
	TokenSecret   string
	Node          string
	StoragePool   string
	NetworkBridge string
	VLANID        int
	Profiles      map[string]CloneProfile
	SkipTLSVerify bool
}

type CloneProfile struct {
	TemplateVMID int               `yaml:"template_vmid"`
	CloneMode    string            `yaml:"clone_mode"`
	Config       map[string]string `yaml:"config"`
}

type cloneProfilesFile struct {
	Profiles map[string]CloneProfile `yaml:"profiles"`
}

func LoadCloneProfiles(path string) (map[string]CloneProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading proxmox profiles file: %w", err)
	}

	var file cloneProfilesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing proxmox profiles file: %w", err)
	}

	if len(file.Profiles) == 0 {
		return nil, fmt.Errorf("proxmox profiles file must define at least one profile")
	}

	for name, profile := range file.Profiles {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("proxmox profile name cannot be empty")
		}
		if profile.TemplateVMID <= 0 {
			return nil, fmt.Errorf("proxmox profile %q requires template_vmid greater than 0", name)
		}
		if profile.CloneMode == "" {
			profile.CloneMode = "linked"
		}
		if profile.CloneMode != "full" && profile.CloneMode != "linked" {
			return nil, fmt.Errorf("proxmox profile %q has invalid clone_mode %q; must be 'full' or 'linked'", name, profile.CloneMode)
		}
		if err := validateProfileConfig(name, profile.Config); err != nil {
			return nil, err
		}
		if profile.Config == nil {
			profile.Config = map[string]string{}
		}
		file.Profiles[name] = profile
	}

	return file.Profiles, nil
}

func validateProfileConfig(profileName string, cfg map[string]string) error {
	for key, value := range cfg {
		switch key {
		case "cipublickey":
			return fmt.Errorf("proxmox profile %q uses unsupported config key %q; use %q instead", profileName, key, "sshkeys")
		case "ciname":
			return fmt.Errorf("proxmox profile %q uses unsupported config key %q; VM hostname is set during clone", profileName, key)
		}

		if strings.HasPrefix(key, "ipconfig") && strings.Contains(value, "bridge=") {
			return fmt.Errorf("proxmox profile %q has invalid %s value %q; bridge belongs in net0/template NIC config, not %s", profileName, key, value, key)
		}
		if err := validatePlaceholders(profileName, key, value); err != nil {
			return err
		}
	}
	return nil
}

func validatePlaceholders(profileName, key, value string) error {
	remaining := value
	for {
		start := strings.Index(remaining, "{{")
		if start == -1 {
			return nil
		}
		end := strings.Index(remaining[start+2:], "}}")
		if end == -1 {
			return fmt.Errorf("proxmox profile %q config key %q has unterminated placeholder in value %q", profileName, key, value)
		}

		placeholder := strings.TrimSpace(remaining[start+2 : start+2+end])
		if placeholder != "ssh_public_key" && placeholder != "hostname" {
			return fmt.Errorf("proxmox profile %q config key %q uses unsupported placeholder %q; supported placeholders are {{ ssh_public_key }} and {{ hostname }}", profileName, key, "{{ "+placeholder+" }}")
		}
		remaining = remaining[start+2+end+2:]
	}
}
