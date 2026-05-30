package ansible

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"painkiller-shell/internal/provisioner"
)

func GenerateVars(spec provisioner.EnvironmentProvisionSpec) (string, error) {
	vars := map[string]interface{}{
		"workstation_ip": spec.WorkstationIP,
		"clusters":       make([]map[string]interface{}, 0, len(spec.Clusters)),
	}

	if spec.ProxyAddr != "" {
		proxyURL := fmt.Sprintf("http://%s", spec.ProxyAddr)
		vars["http_proxy"] = proxyURL
		vars["https_proxy"] = proxyURL
		vars["no_proxy"] = "localhost,127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	}

	if spec.ProxyIPTScript != "" {
		vars["proxy_iptables_script"] = spec.ProxyIPTScript
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
