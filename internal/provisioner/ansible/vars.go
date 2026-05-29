package ansible

import (
	"strings"

	"gopkg.in/yaml.v3"
	"painkiller-shell/internal/provisioner"
)

func GenerateVars(spec provisioner.EnvironmentProvisionSpec) (string, error) {
	vars := map[string]interface{}{
		"workstation_ip": spec.WorkstationIP,
		"clusters":       make([]map[string]interface{}, 0, len(spec.Clusters)),
	}

	for _, cluster := range spec.Clusters {
		c := map[string]interface{}{
			"name":         cluster.Name,
			"kube_context": cluster.KubeContext,
			"nodes":        make([]map[string]string, 0, len(cluster.Nodes)),
		}
		for _, node := range cluster.Nodes {
			c["nodes"] = append(c["nodes"].([]map[string]string), map[string]string{
				"hostname": node.Hostname,
				"ip":       node.IP,
				"role":     node.Role,
			})
		}
		vars["clusters"] = append(vars["clusters"].([]map[string]interface{}), c)
	}

	for k, v := range spec.ScenarioVars {
		vars[k] = v
	}

	data, err := yaml.Marshal(vars)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

var _ = strings.Builder{}
