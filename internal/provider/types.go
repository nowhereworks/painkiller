package provider

type NetworkProfile struct {
	Name string
}

type VMRequest struct {
	Hostname      string
	Role          string
	Template      string
	Network       NetworkProfile
	SSHPublicKey  string
	CloudInitData map[string]string
	Tags          map[string]string
}

type VMResult struct {
	ProviderVMID string
	IPAddress    string
	Hostname     string
}

type ClusterRequest struct {
	Name  string
	Nodes []VMRequest
}

type EnvironmentSpec struct {
	Workstation VMRequest
	Clusters    []ClusterRequest
}

type ClusterResult struct {
	Name  string
	Nodes []VMResult
}

type EnvironmentResult struct {
	Workstation VMResult
	Clusters    []ClusterResult
}
