package provisioner

type NodeSpec struct {
	Hostname string
	IP       string
	Role     string
}

type ClusterSpec struct {
	Name        string
	KubeContext string
	Nodes       []NodeSpec
}

type EnvironmentProvisionSpec struct {
	WorkstationIP  string
	SSHPrivateKey  []byte
	Clusters       []ClusterSpec
	PlaybookPath   string
	ScenarioVars   map[string]interface{}
	ProxyAddr      string
	ProxyIPTScript string
}

type ProvisionResult struct {
	Ready bool
}
