package dag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yohamta/dagu/internal/settings"
)

func TestBuildErrors(t *testing.T) {
	tests := []struct {
		input         string
		expectedError string
	}{
		{
			input: `
steps:
  - command: echo 1`,
			expectedError: "step name must be specified",
		},
		{
			input: `
steps:
  - name: step 1`,
			expectedError: "step command must be specified",
		},
		{
			input: fmt.Sprintf(`
env: 
  - VAR: %q`, "`invalid`"),
			expectedError: `exec: "invalid": executable file not found in $PATH`,
		},
		{
			input:         fmt.Sprintf(`params: %q`, "`invalid`"),
			expectedError: `exec: "invalid": executable file not found in $PATH`,
		},
		{
			input:         `schedule: "1"`,
			expectedError: "invalid schedule: expected exactly 5 fields",
		},
	}

	for i, tt := range tests {
		l := &Loader{}
		d, err := l.unmarshalData([]byte(tt.input))
		require.NoError(t, err)

		def, err := l.decode(d)
		require.NoError(t, err)

		b := &builder{}

		_, err = b.buildFromDefinition(def, nil)
		require.Error(t, err)
		if !strings.Contains(err.Error(), tt.expectedError) {
			t.Errorf("test %d: expected error %q, got %q", i, tt.expectedError, err.Error())
		}
	}
}

func TestBuildingEnvs(t *testing.T) {
	tests := []struct {
		input    string
		expected map[string]string
	}{
		{
			input: `
env: 
  VAR: "` + "`echo 1`" + `"
`,
			expected: map[string]string{"VAR": "1"},
		},
		{
			input: `
env: 
  "1": "123"
`,
			expected: map[string]string{"1": "123"},
		},
		{
			input: `
env: 
  - "FOO": "BAR"
  - "FOO": "${FOO}:BAZ"
  - "FOO": "${FOO}:BAR"
  - "FOO": "${FOO}:FOO"
`,
			expected: map[string]string{"FOO": "BAR:BAZ:BAR:FOO"},
		},
	}

	for _, tt := range tests {
		l := &Loader{}
		d, err := l.unmarshalData([]byte(tt.input))
		require.NoError(t, err)

		def, err := l.decode(d)
		require.NoError(t, err)

		b := &builder{}
		_, err = b.buildFromDefinition(def, nil)
		require.NoError(t, err)

		for k, v := range tt.expected {
			require.Equal(t, v, os.Getenv(k))
		}
	}
}

func TestBuildingParameters(t *testing.T) {
	tests := []struct {
		params   string
		env      string
		expected map[string]string
	}{
		{
			params: "x",
			expected: map[string]string{
				"1": "x",
			},
		},
		{
			params: "x y",
			expected: map[string]string{
				"1": "x",
				"2": "y",
			},
		},
		{
			params: "x yy zzz",
			expected: map[string]string{
				"1": "x",
				"2": "yy",
				"3": "zzz",
			},
		},
		{
			params: "x $1",
			expected: map[string]string{
				"1": "x",
				"2": "x",
			},
		},
		{
			params: "first P1=foo P2=${FOO} P3=`/bin/echo ${P2}` X=bar Y=${P1} Z=\"A B C\"",
			env:    "FOO: BAR",
			expected: map[string]string{
				"P1": "foo",
				"P2": "BAR",
				"P3": "BAR",
				"X":  "bar",
				"Y":  "foo",
				"Z":  "A B C",
				"1":  "first",
				"2":  "P1=foo",
				"3":  "P2=BAR",
				"4":  "P3=BAR",
				"5":  "X=bar",
				"6":  "Y=foo",
				"7":  "Z=A B C",
			},
		},
	}

	for _, tt := range tests {
		l := &Loader{}
		d, err := l.unmarshalData([]byte(fmt.Sprintf(`
env:
  - %s
params: %s
  	`, tt.env, tt.params)))
		require.NoError(t, err)

		def, err := l.decode(d)
		require.NoError(t, err)

		b := &builder{}
		_, err = b.buildFromDefinition(def, nil)
		require.NoError(t, err)

		for k, v := range tt.expected {
			require.Equal(t, v, os.Getenv(k))
		}
	}
}

func TestExpandingEnvs(t *testing.T) {
	b := &builder{}
	os.Setenv("FOO", "BAR")
	require.Equal(t, b.expandEnv("${FOO}"), "BAR")

	b = &builder{
		BuildDAGOptions: BuildDAGOptions{
			noEval: true,
		},
	}
	require.Equal(t, b.expandEnv("${FOO}"), "${FOO}")
}

func TestBuildingTags(t *testing.T) {
	input := `tags: Daily, Monthly`
	expected := []string{"daily", "monthly"}

	l := &Loader{}
	m, err := l.unmarshalData([]byte(input))
	require.NoError(t, err)

	def, err := l.decode(m)
	require.NoError(t, err)

	b := &builder{}
	d, err := b.buildFromDefinition(def, nil)
	require.NoError(t, err)

	for _, tag := range expected {
		require.True(t, d.HasTag(tag))
	}

	require.False(t, d.HasTag("weekly"))
}

func TestBuildingSchedules(t *testing.T) {
	tests := []struct {
		input    string
		isErr    bool
		expected map[string][]string
	}{
		{
			input: `
schedule:
  start: "0 1 * * *"
  stop: "0 2 * * *"
`,
			expected: map[string][]string{
				"start": {"0 1 * * *"},
				"stop":  {"0 2 * * *"},
			},
		},
		{
			input: `
schedule:
  start: "0 1 * * *"
`,
			expected: map[string][]string{
				"start": {"0 1 * * *"},
			},
		},
		{
			input: `schedule:
  stop: "0 1 * * *"
`,
			expected: map[string][]string{
				"stop": {"0 1 * * *"},
			},
		},
		{
			input: `
schedule:
  start: 
    - "0 1 * * *"
    - "0 18 * * *"
  stop:
    - "0 2 * * *"
    - "0 20 * * *"
`,
			expected: map[string][]string{
				"start": {"0 1 * * *", "0 18 * * *"},
				"stop":  {"0 2 * * *", "0 20 * * *"},
			},
		},
		{
			input: `
schedule:
  start: "0 8 * * *"
  restart: "0 12 * * *"
  stop: "0 20 * * *"
`,
			expected: map[string][]string{
				"start":   {"0 8 * * *"},
				"restart": {"0 12 * * *"},
				"stop":    {"0 20 * * *"},
			},
		},
		{
			input: `
schedule:
  stop: "* * * * * * *"
`,
			isErr: true,
		},
		{
			input: `
schedule:
  invalid: "* * * * * * *"
`,
			isErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := &Loader{}
			m, err := l.unmarshalData([]byte(tt.input))
			require.NoError(t, err)

			def, err := l.decode(m)
			require.NoError(t, err)

			b := &builder{}
			d, err := b.buildFromDefinition(def, nil)

			if tt.isErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			for k, v := range tt.expected {
				var actual []*Schedule
				switch k {
				case "start":
					actual = d.Schedule
				case "stop":
					actual = d.StopSchedule
				case "restart":
					actual = d.RestartSchedule
				}

				if len(actual) != len(v) {
					t.Errorf("expected %d schedules, got %d", len(v), len(actual))
				}

				for i, s := range actual {
					if s.Expression != v[i] {
						t.Errorf("expected %s, got %s", v[i], s.Expression)
					}
				}
			}
		})
	}
}

func TestGeneratingSockAddr(t *testing.T) {
	d := &DAG{Location: "testdata/testDag.yml"}
	require.Regexp(t, `^/tmp/@dagu-testDag-[0-9a-f]+\.sock$`, d.SockAddr())
}

func TestOverwriteGlobalConfig(t *testing.T) {
	l := &Loader{BaseConfig: settings.MustGet(settings.SETTING__BASE_CONFIG)}

	d, err := l.Load(filepath.Join(testdataDir, "overwrite.yaml"), "")
	require.NoError(t, err)

	require.Equal(t, &MailOn{Failure: false, Success: false}, d.MailOn)
	require.Equal(t, d.HistRetentionDays, 7)

	d, err = l.Load(filepath.Join(testdataDir, "no_overwrite.yaml"), "")
	require.NoError(t, err)

	require.Equal(t, &MailOn{Failure: true, Success: false}, d.MailOn)
	require.Equal(t, d.HistRetentionDays, 30)
}

func TestBulidingExecutor(t *testing.T) {
	tests := []struct {
		input          string
		expectedExec   string
		expectedConfig map[string]interface{}
	}{
		{
			input: `
steps:
  - name: S1
    command: echo 1
    executor: http
`,
			expectedExec:   "http",
			expectedConfig: nil,
		},
		{
			input: `
steps:
  - name: S1
    command: echo 1
    executor:
      type: http
      config: some option 
`,
			expectedExec: "http",
			expectedConfig: map[string]interface{}{
				"config": "some option",
			},
		},
	}

	for _, tt := range tests {
		l := &Loader{}
		d, err := l.unmarshalData([]byte(tt.input))
		require.NoError(t, err)

		def, err := l.decode(d)
		require.NoError(t, err)

		b := &builder{}
		dag, err := b.buildFromDefinition(def, nil)
		require.NoError(t, err)

		if len(dag.Steps) != 1 {
			t.Errorf("expected 1 step, got %d", len(dag.Steps))
		}

		require.Equal(t, tt.expectedExec, dag.Steps[0].ExecutorConfig.Type)
		if tt.expectedConfig != nil {
			require.Equal(t, tt.expectedConfig, dag.Steps[0].ExecutorConfig.Config)
		}
	}
}

func TestBuildingSignalOnStop(t *testing.T) {
	for _, tc := range []struct {
		sig  string
		want string
		err  bool
	}{
		{
			sig:  "SIGINT",
			want: "SIGINT",
			err:  false,
		},
		{
			sig: "2000",
			err: true,
		},
	} {
		dat := fmt.Sprintf(`name: test DAG
steps:
  - name: "1"
    command: "true"
    signalOnStop: "%s"
`, tc.sig)
		l := &Loader{}
		ret, err := l.LoadData([]byte(dat))
		if tc.err {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)

		step := ret.Steps[0]
		require.Equal(t, step.SignalOnStop, tc.want)
	}
}
