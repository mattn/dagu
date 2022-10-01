package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"github.com/yohamta/dagu/internal/constants"
	"github.com/yohamta/dagu/internal/settings"
	"github.com/yohamta/dagu/internal/utils"
)

type appTest struct {
	args        []string
	errored     bool
	output      []string
	errMessage  []string
	exactOutput string
	stdin       io.ReadCloser
}

var (
	testdataDir = filepath.Join(utils.MustGetwd(), "testdata")
	testHomeDir = ""
)

func TestMain(m *testing.M) {
	testHomeDir = utils.MustTempDir("dagu_test")
	settings.ChangeHomeDir(testHomeDir)
	code := m.Run()
	os.RemoveAll(testHomeDir)
	os.Exit(code)
}

func TestSetVersion(t *testing.T) {
	version = "0.0.1"
	setVersion()
	require.Equal(t, version, constants.Version)
}

func testConfig(name string) string {
	return filepath.Join(testdataDir, name)
}

func runAppTestOutput(app *cli.App, test appTest, t *testing.T) {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	log.SetOutput(w)

	defer func() {
		os.Stdout = origStdout
		log.SetOutput(origStdout)
	}()

	if test.stdin != nil {
		origStdin := stdin
		stdin = test.stdin
		defer func() {
			stdin = origStdin
		}()
	}

	err = app.Run(test.args)
	os.Stdout = origStdout
	w.Close()

	if err != nil && !test.errored {
		t.Fatalf("failed unexpectedly %v", err)
		return
	}

	if err != nil && len(test.errMessage) > 0 {
		for _, v := range test.errMessage {
			require.Contains(t, err.Error(), v)
		}
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	s := buf.String()
	if len(test.output) > 0 {
		for _, v := range test.output {
			require.Contains(t, s, v)
		}
	}

	if test.exactOutput != "" {
		require.Equal(t, test.exactOutput, s)
	}
}

func runAppTest(app *cli.App, test appTest, t *testing.T) {
	err := app.Run(test.args)

	if err != nil && !test.errored {
		t.Fatalf("failed unexpectedly %v", err)
		return
	}
}
