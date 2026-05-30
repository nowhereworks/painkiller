package proxmox

type Config struct {
	APIURL        string
	TokenID       string
	TokenSecret   string
	Node          string
	StoragePool   string
	NetworkBridge string
	VLANID        int
	Templates     map[string]int
	SkipTLSVerify bool
}
