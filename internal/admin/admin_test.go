package admin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yohamta/dagu/internal/settings"
	"github.com/yohamta/dagu/internal/utils"
)

var testdataDir = filepath.Join(utils.MustGetwd(), "testdata")
var testHomeDir string

func TestMain(m *testing.M) {
	testHomeDir = utils.MustTempDir("dagu-admin-test")
	os.Setenv("HOST", "localhost")
	settings.ChangeHomeDir(testdataDir)
	code := m.Run()
	_ = os.RemoveAll(testHomeDir)
	os.Exit(code)
}
