package address

type azureAssigner struct {
}

func (a *azureAssigner) Assign(_, _ string, _ []string, _ string) error {
	return nil
}
