package types

type Address struct {
	IP   string
	Type string
}

type Node struct {
	Name      string
	Cluster   string
	Pool      string
	Spot      bool
	Region    string
	Zone      string
	Addresses []string
}
