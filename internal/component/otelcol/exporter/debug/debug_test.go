package debug_test

import (
	"fmt"
	"testing"

	"github.com/grafana/alloy/internal/component/otelcol/exporter/debug"
	"github.com/grafana/alloy/syntax"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	"go.opentelemetry.io/collector/exporter/debugexporter"
)

func Test(t *testing.T) {
	tests := []struct {
		testName       string
		args           string
		expectedReturn debugexporter.Config
		errorMsg       string
	}{
		{
			testName: "defaultConfig",
			args:     ``,
			expectedReturn: debugexporter.Config{
				Verbosity:          configtelemetry.LevelBasic,
				SamplingInitial:    2,
				SamplingThereafter: 1,
				UseInternalLogger:  true,
			},
		},

		{
			testName: "validConfig",
			args: ` 
				verbosity = "detailed"
				sampling_initial = 5
				sampling_thereafter = 20
				use_internal_logger = false
			`,
			expectedReturn: debugexporter.Config{
				Verbosity:          configtelemetry.LevelDetailed,
				SamplingInitial:    5,
				SamplingThereafter: 20,
				UseInternalLogger:  false,
			},
		},

		{
			testName: "invalidConfig",
			args: `
				verbosity = "test"
				sampling_initial = 5
				sampling_thereafter = 20
			`,
			errorMsg: "error in conversion to config arguments",
		},
	}

	for _, tc := range tests {
		t.Run(tc.testName, func(t *testing.T) {
			var args debug.Arguments
			err := syntax.Unmarshal([]byte(tc.args), &args)
			require.NoError(t, err)

			actualPtr, err := args.Convert()
			if tc.errorMsg != "" {
				require.ErrorContains(t, err, tc.errorMsg)
				return
			}

			require.NoError(t, err)

			actual := actualPtr.(*debugexporter.Config)
			fmt.Printf("Passed conversion")

			require.NoError(t, xconfmap.Validate(actual))

			require.Equal(t, tc.expectedReturn, *actual)
		})
	}
}
