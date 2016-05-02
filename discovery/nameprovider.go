package discovery

type NameProvider interface {
	// Returns a name for the service or an empty string,
	// if no name could be determined.
	GetName(host string, port uint16) string
}
