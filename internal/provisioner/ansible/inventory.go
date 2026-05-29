package ansible

import (
	"fmt"
	"strings"

	"painkiller-shell/internal/provisioner"
)

func GenerateInventory(spec provisioner.EnvironmentProvisionSpec) string {
	var b strings.Builder

	b.WriteString("[workstation]\n")
	b.WriteString(fmt.Sprintf("%s ansible_user=root\n", spec.WorkstationIP))
	b.WriteString("\n")

	for _, cluster := range spec.Clusters {
		groupName := strings.ReplaceAll(cluster.Name, "-", "_")

		b.WriteString(fmt.Sprintf("[%s_control_plane]\n", groupName))
		for _, node := range cluster.Nodes {
			if node.Role == "control-plane" {
				b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_user=root\n", node.Hostname, node.IP))
			}
		}
		b.WriteString("\n")

		b.WriteString(fmt.Sprintf("[%s_workers]\n", groupName))
		for _, node := range cluster.Nodes {
			if node.Role == "worker" {
				b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_user=root\n", node.Hostname, node.IP))
			}
		}
		b.WriteString("\n")

		b.WriteString(fmt.Sprintf("[%s:children]\n", groupName))
		b.WriteString(fmt.Sprintf("%s_control_plane\n", groupName))
		b.WriteString(fmt.Sprintf("%s_workers\n", groupName))
		b.WriteString("\n")
	}

	b.WriteString("[all:vars]\n")
	b.WriteString("ansible_ssh_private_key_file=/tmp/painkiller-ssh-key\n")
	b.WriteString("ansible_ssh_common_args='-o StrictHostKeyChecking=no'\n")

	return b.String()
}
