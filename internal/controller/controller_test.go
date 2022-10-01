package controller_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yohamta/dagu/internal/controller"
	"github.com/yohamta/dagu/internal/dag"
	"github.com/yohamta/dagu/internal/database"
	"github.com/yohamta/dagu/internal/models"
	"github.com/yohamta/dagu/internal/scheduler"
	"github.com/yohamta/dagu/internal/settings"
	"github.com/yohamta/dagu/internal/sock"
	"github.com/yohamta/dagu/internal/utils"
)

var (
	testdataDir = filepath.Join(utils.MustGetwd(), "./testdata")
)

func TestMain(m *testing.M) {
	tempDir := utils.MustTempDir("controller_test")
	settings.ChangeHomeDir(tempDir)
	code := m.Run()
	os.RemoveAll(tempDir)
	os.Exit(code)
}

func TestGetStatusRunningAndDone(t *testing.T) {
	file := testDAG("get_status.yaml")

	dr := controller.NewDAGStatusReader()
	ds, err := dr.ReadStatus(file, false)
	require.NoError(t, err)

	dc := controller.NewDAGController(ds.DAG)

	socketServer, _ := sock.NewServer(
		&sock.Config{
			Addr: ds.DAG.SockAddr(),
			HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
				status := models.NewStatus(
					ds.DAG, []*scheduler.Node{},
					scheduler.SchedulerStatus_Running, 0, nil, nil)
				w.WriteHeader(http.StatusOK)
				b, _ := status.ToJson()
				w.Write(b)
			},
		})

	go func() {
		socketServer.Serve(nil)
	}()

	defer socketServer.Shutdown()

	time.Sleep(time.Millisecond * 100)
	st, err := dc.GetStatus()
	require.NoError(t, err)
	require.Equal(t, scheduler.SchedulerStatus_Running, st.Status)

	socketServer.Shutdown()

	st, err = dc.GetStatus()
	require.NoError(t, err)
	require.Equal(t, scheduler.SchedulerStatus_None, st.Status)
}

func TestGrepDAGs(t *testing.T) {
	ret, _, err := controller.GrepDAG(testdataDir, "aabbcc")
	require.NoError(t, err)
	require.Equal(t, 1, len(ret))

	ret, _, err = controller.GrepDAG(testdataDir, "steps")
	require.NoError(t, err)
	require.Greater(t, len(ret), 1)
}

func TestUpdateStatus(t *testing.T) {
	var (
		file      = testDAG("update_status.yaml")
		requestId = "test-update-status"
		now       = time.Now()
		dr        = controller.NewDAGStatusReader()
		db        = &database.Database{Config: database.DefaultConfig()}
	)

	dag, err := dr.ReadStatus(file, false)
	require.NoError(t, err)

	dc := controller.NewDAGController(dag.DAG)

	w, _, _ := db.NewWriter(dag.DAG.Location, now, requestId)
	err = w.Open()
	require.NoError(t, err)

	st := testNewStatus(dag.DAG, requestId,
		scheduler.SchedulerStatus_Success, scheduler.NodeStatus_Success)

	err = w.Write(st)
	require.NoError(t, err)
	w.Close()

	time.Sleep(time.Millisecond * 100)

	st, err = dc.GetStatusByRequestId(requestId)
	require.NoError(t, err)
	require.Equal(t, scheduler.NodeStatus_Success, st.Nodes[0].Status)

	newStatus := scheduler.NodeStatus_Error
	st.Nodes[0].Status = newStatus

	err = dc.UpdateStatus(st)
	require.NoError(t, err)

	statusByRequestId, err := dc.GetStatusByRequestId(requestId)
	require.NoError(t, err)

	require.Equal(t, 1, len(st.Nodes))
	require.Equal(t, newStatus, statusByRequestId.Nodes[0].Status)
}

func TestUpdateStatusError(t *testing.T) {
	var (
		file      = testDAG("update_status_failed.yaml")
		requestId = "test-update-status-failure"
		dr        = controller.NewDAGStatusReader()
	)

	dag, err := dr.ReadStatus(file, false)
	require.NoError(t, err)

	dc := controller.NewDAGController(dag.DAG)

	status := testNewStatus(dag.DAG, requestId,
		scheduler.SchedulerStatus_Error, scheduler.NodeStatus_Error)

	err = dc.UpdateStatus(status)
	require.Error(t, err)

	// update with invalid request id
	status.RequestId = "invalid-request-id"
	err = dc.UpdateStatus(status)
	require.Error(t, err)
}

func TestStart(t *testing.T) {
	var (
		file = testDAG("start.yaml")
		dr   = controller.NewDAGStatusReader()
	)

	dag, err := dr.ReadStatus(file, false)
	require.NoError(t, err)

	dc := controller.NewDAGController(dag.DAG)
	err = dc.Start(filepath.Join(utils.MustGetwd(), "../../bin/dagu"), "", "")
	require.Error(t, err)

	status, err := dc.GetLastStatus()
	require.NoError(t, err)
	require.Equal(t, scheduler.SchedulerStatus_Error, status.Status)
}

func TestStop(t *testing.T) {
	var (
		file = testDAG("stop.yaml")
		dr   = controller.NewDAGStatusReader()
	)

	dag, err := dr.ReadStatus(file, false)
	require.NoError(t, err)

	dc := controller.NewDAGController(dag.DAG)
	dc.StartAsync(filepath.Join(utils.MustGetwd(), "../../bin/dagu"), "", "")

	require.Eventually(t, func() bool {
		st, _ := dc.GetStatus()
		return st.Status == scheduler.SchedulerStatus_Running
	}, time.Millisecond*1500, time.Millisecond*100)

	dc.Stop()

	require.Eventually(t, func() bool {
		st, _ := dc.GetLastStatus()
		return st.Status == scheduler.SchedulerStatus_Cancel
	}, time.Millisecond*1500, time.Millisecond*100)
}

func TestRestart(t *testing.T) {
	var (
		file = testDAG("restart.yaml")
		dr   = controller.NewDAGStatusReader()
	)

	dag, err := dr.ReadStatus(file, false)
	require.NoError(t, err)

	dc := controller.NewDAGController(dag.DAG)
	err = dc.Restart(filepath.Join(utils.MustGetwd(), "../../bin/dagu"), "")
	require.NoError(t, err)

	status, err := dc.GetLastStatus()
	require.NoError(t, err)
	require.Equal(t, scheduler.SchedulerStatus_Success, status.Status)
}

func TestRetry(t *testing.T) {
	var (
		file = testDAG("retry.yaml")
		dr   = controller.NewDAGStatusReader()
	)

	dag, err := dr.ReadStatus(file, false)
	require.NoError(t, err)

	dc := controller.NewDAGController(dag.DAG)
	err = dc.Start(filepath.Join(utils.MustGetwd(), "../../bin/dagu"), "", "x y z")
	require.NoError(t, err)

	status, err := dc.GetLastStatus()
	require.NoError(t, err)
	require.Equal(t, scheduler.SchedulerStatus_Success, status.Status)

	requestId := status.RequestId
	params := status.Params

	err = dc.Retry(filepath.Join(utils.MustGetwd(), "../../bin/dagu"), "", requestId)
	require.NoError(t, err)
	status, err = dc.GetLastStatus()
	require.NoError(t, err)

	require.Equal(t, scheduler.SchedulerStatus_Success, status.Status)
	require.Equal(t, params, status.Params)

	statusByRequestId, err := dc.GetStatusByRequestId(status.RequestId)
	require.NoError(t, err)
	require.Equal(t, status, statusByRequestId)

	recentStatuses := dc.GetRecentStatuses(1)
	require.Equal(t, status, recentStatuses[0].Status)
}

func TestUpdate(t *testing.T) {
	tmpDir := utils.MustTempDir("controller-test-save")
	defer os.RemoveAll(tmpDir)

	loc := filepath.Join(tmpDir, "test.yaml")
	d := &dag.DAG{
		Name:     "test",
		Location: loc,
	}
	dc := controller.NewDAGController(d)

	// invalid DAG
	invalidDAG := `name: test DAG`
	err := dc.UpdateDAGSpec(invalidDAG)
	require.Error(t, err)

	// valid DAG
	validDAG := `name: test DAG
steps:
  - name: "1"
    command: "true"
`
	// Update Error: the DAG does not exist
	err = dc.UpdateDAGSpec(validDAG)
	require.Error(t, err)

	// create a new DAG file
	newFile, _ := utils.CreateFile(loc)
	defer newFile.Close()

	// Update the DAG
	err = dc.UpdateDAGSpec(validDAG)
	require.NoError(t, err)

	// Check the content of the DAG file
	updatedFile, _ := os.Open(loc)
	defer updatedFile.Close()
	b, err := io.ReadAll(updatedFile)
	require.NoError(t, err)
	require.Equal(t, validDAG, string(b))
}

func TestRemove(t *testing.T) {
	tmpDir := utils.MustTempDir("controller-test-remove")
	defer os.RemoveAll(tmpDir)

	loc := filepath.Join(tmpDir, "test.yaml")
	d := &dag.DAG{
		Name:     "test",
		Location: loc,
	}

	dc := controller.NewDAGController(d)
	dagSpec := `name: test DAG
steps:
  - name: "1"
    command: "true"
`
	// create file
	newFile, _ := utils.CreateFile(loc)
	defer newFile.Close()

	err := dc.UpdateDAGSpec(dagSpec)
	require.NoError(t, err)

	// check file
	saved, _ := os.Open(loc)
	defer saved.Close()
	b, err := io.ReadAll(saved)
	require.NoError(t, err)
	require.Equal(t, dagSpec, string(b))

	// remove file
	err = dc.DeleteDAG()
	require.NoError(t, err)
	require.NoFileExists(t, loc)
}

func TestCreateNewDAG(t *testing.T) {
	tmpDir := utils.MustTempDir("controller-test-save")
	defer os.RemoveAll(tmpDir)

	// invalid filename
	filename := filepath.Join(tmpDir, "test")
	err := controller.CreateDAG(filename)
	require.Error(t, err)

	// valid filename
	filename = filepath.Join(tmpDir, "test.yaml")
	err = controller.CreateDAG(filename)
	require.NoError(t, err)

	// check file is created
	cl := &dag.Loader{}

	d, err := cl.Load(filename, "")
	require.NoError(t, err)
	require.Equal(t, "test", d.Name)

	steps := d.Steps[0]
	require.Equal(t, "step1", steps.Name)
	require.Equal(t, "echo", steps.Command)
	require.Equal(t, []string{"hello"}, steps.Args)
}

func TestRenameDAG(t *testing.T) {
	tmpDir := utils.MustTempDir("controller-test-rename")
	defer os.RemoveAll(tmpDir)

	oldName := filepath.Join(tmpDir, "rename_dag.yaml")
	newName := filepath.Join(tmpDir, "rename_dag_renamed.yaml")

	err := controller.CreateDAG(oldName)
	require.NoError(t, err)

	err = controller.MoveDAG(oldName, "invalid-config-name")
	require.Error(t, err)

	err = controller.MoveDAG(oldName, newName)
	require.NoError(t, err)
	require.FileExists(t, newName)
}

func testDAG(name string) string {
	return filepath.Join(testdataDir, name)
}

func testNewStatus(d *dag.DAG, reqId string, status scheduler.SchedulerStatus, nodeStatus scheduler.NodeStatus) *models.Status {
	now := time.Now()
	ret := models.NewStatus(
		d, []*scheduler.Node{{NodeState: scheduler.NodeState{Status: nodeStatus}}},
		status, 0, &now, nil)
	ret.RequestId = reqId
	return ret
}
