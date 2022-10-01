package dag

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yohamta/dagu/internal/settings"
	"github.com/yohamta/dagu/internal/utils"
)

var (
	testdataDir = filepath.Join(utils.MustGetwd(), "testdata")
	testHomeDir = filepath.Join(utils.MustGetwd(), "testdata/home")
	testEnv     = []string{}
)

func TestMain(m *testing.M) {
	settings.ChangeHomeDir(testHomeDir)
	testEnv = []string{
		fmt.Sprintf("LOG_DIR=%s", filepath.Join(testHomeDir, "/logs")),
		fmt.Sprintf("PATH=%s", os.ExpandEnv("${PATH}")),
	}
	code := m.Run()
	os.Exit(code)
}

func TestToString(t *testing.T) {
	l := &Loader{}

	d, err := l.Load(filepath.Join(testdataDir, "default.yaml"), "")
	require.NoError(t, err)

	ret := d.String()
	require.Contains(t, ret, "Name: default")
}

func TestReadingFile(t *testing.T) {
	tmpDir := utils.MustTempDir("read-config-test")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tmpFile := filepath.Join(tmpDir, "DAG.yaml")
	input := `
steps:
  - name: step 1
    command: echo test
`
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	require.NoError(t, err)

	ret, err := ReadFile(tmpFile)
	require.NoError(t, err)
	require.Equal(t, input, ret)
}
