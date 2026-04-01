package nameserver

// NameString exposes the canonical nameserver name string for helper adapters.
func (ns Nameserver) NameString() string {
	return ns.Name.String()
}

// AddressString exposes the canonical nameserver IP string for helper adapters.
func (ns Nameserver) AddressString() string {
	return ns.Address.String()
}
