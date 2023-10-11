package address

import "context"

type azureAssigner struct {
}

func (a *azureAssigner) Assign(_ context.Context, _, _ string, _ []string, _ string) error {
	return nil
}
