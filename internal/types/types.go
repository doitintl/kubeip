package types

// Instance GKE Instance VM
type Instance struct {
	ProjectID string
	Name      string
	Zone      string
	Pool      string
}

// IPAddress GKE IP
type IPAddress struct {
	IP     string
	Name   string
	Labels map[string]string
}
