package address

type awsAssigner struct {
}

func (a *awsAssigner) Assign(_, _ string, _ []string, _ string) error {
	return nil
}
