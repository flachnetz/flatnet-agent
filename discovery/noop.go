package discovery

// Instances of this type always return constant value.
type constantNameProvider string

func NewNoopNameProvider() NameProvider {
	return NewConstantNameProvider("")
}

func NewConstantNameProvider(name string) NameProvider {
	result := constantNameProvider(name)
	return &result
}

func (val *constantNameProvider) GetName(host string, port uint16) string {
	if val == nil {
		return ""
	} else {
		return string(*val)
	}
}
