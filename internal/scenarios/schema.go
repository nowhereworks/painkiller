package scenarios

type Scenario struct {
	ID                string   `yaml:"id"`
	Title             string   `yaml:"title"`
	DurationMinutes   int      `yaml:"duration_minutes"`
	AccessWindowHours int      `yaml:"access_window_hours"`
	AttemptsAllowed   int      `yaml:"attempts_allowed"`
	Topology          Topology `yaml:"topology"`
	Tasks             []Task   `yaml:"tasks"`
}

type Topology struct {
	Clusters []Cluster `yaml:"clusters"`
}

type Cluster struct {
	ID          string `yaml:"id"`
	DisplayName string `yaml:"display_name"`
	KubeContext string `yaml:"kube_context"`
	Nodes       []Node `yaml:"nodes"`
}

type Node struct {
	Name     string `yaml:"name"`
	Role     string `yaml:"role"`
	Template string `yaml:"template"`
}

type Task struct {
	ID          string `yaml:"id"`
	ClusterID   string `yaml:"cluster_id"`
	KubeContext string `yaml:"kube_context"`
	Points      int    `yaml:"points"`
	PromptFile  string `yaml:"prompt_file"`
}

type Check struct {
	ID        string `yaml:"id"`
	TaskID    string `yaml:"task_id"`
	ClusterID string `yaml:"cluster_id"`
	Type      string `yaml:"type"`
	Command   string `yaml:"command"`
	Points    int    `yaml:"points"`
}

type ChecksFile struct {
	Checks []Check `yaml:"checks"`
}
