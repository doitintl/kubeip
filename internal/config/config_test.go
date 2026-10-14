package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestEmptyFilterEnvironmentForIPAM(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        []string
	}{
		{name: "empty Helm environment", value: "", want: nil},
		{name: "blank entries", value: " ; labels.pool=test; ", want: []string{"labels.pool=test"}},
		{name: "AWS values retain commas", value: "Name=tag:kubeip,Values=reserved,public;Name=tag:env,Values=test", want: []string{"Name=tag:kubeip,Values=reserved,public", "Name=tag:env,Values=test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FILTER", tc.value)
			app := &cli.App{SliceFlagSeparator: ";", Flags: []cli.Flag{&cli.StringSliceFlag{Name: "filter", EnvVars: []string{"FILTER"}}}, Action: func(c *cli.Context) error {
				cfg := NewConfig(c)
				require.Equal(t, tc.want, cfg.Filter)
				return nil
			}}
			require.NoError(t, app.Run([]string{"kubeip"}))
		})
	}
}
