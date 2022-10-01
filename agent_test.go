package dagu

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yohamta/dagu/internal/controller"
	"github.com/yohamta/dagu/internal/dag"
	"github.com/yohamta/dagu/internal/models"
	"github.com/yohamta/dagu/internal/scheduler"
	"github.com/yohamta/dagu/internal/settings"
	"github.com/yohamta/dagu/internal/utils"
)

var testdataDir = filepath.Join(utils.MustGetwd(), "testdata")

func TestMain(m *testing.M) {
	testHomeDir := utils.MustTempDir("agent_test")
	settings.ChangeHomeDir(testHomeDir)
	code := m.Run()
	os.RemoveAll(testHomeDir)
	os.Exit(code)
}

func TestRunDAG(t *testing.T) {
	d := testLoadDAG(t, "run.yaml")
	a := &Agent{AgentConfig: &AgentConfig{DAG: d}}

	status, _ := controller.NewDAGController(d).GetLastStatus()
	require.Equal(t, scheduler.SchedulerStatus_None, status.Status)

	go func() {
		err := a.Run()
		require.NoError(t, err)
	}()

	time.Sleep(100 * time.Millisecond)

	require.Eventually(t, func() bool {
		status, err := controller.NewDAGController(d).GetLastStatus()
		require.NoError(t, err)
		return status.Status == scheduler.SchedulerStatus_Success
	}, time.Second*2, time.Millisecond*100)

	// check deletion of expired history files
	d.HistRetentionDays = 0
	a = &Agent{AgentConfig: &AgentConfig{DAG: d}}
	err := a.Run()
	require.NoError(t, err)
	statusList := controller.NewDAGController(d).GetRecentStatuses(100)
	require.Equal(t, 1, len(statusList))
}

func TestCheckRunning(t *testing.T) {
	d := testLoadDAG(t, "is_running.yaml")

	a := &Agent{AgentConfig: &AgentConfig{DAG: d}}

	go func() {
		a.Run()
	}()

	time.Sleep(time.Millisecond * 30)

	status := a.Status()
	require.NotNil(t, status)
	require.Equal(t, status.Status, scheduler.SchedulerStatus_Running)

	_, err := testDAG(t, d)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is already running")
}

func TestDryRun(t *testing.T) {
	a := &Agent{AgentConfig: &AgentConfig{
		DAG: testLoadDAG(t, "dry.yaml"),
		Dry: true,
	}}
	err := a.Run()
	require.NoError(t, err)

	status := a.Status()
	require.NoError(t, err)

	require.Equal(t, scheduler.SchedulerStatus_Success, status.Status)
}

func TestCancelDAG(t *testing.T) {
	for _, abort := range []func(*Agent){
		func(a *Agent) { a.Signal(syscall.SIGTERM) },
	} {
		a, d := testDAGAsync(t, "sleep.yaml")
		time.Sleep(time.Millisecond * 100)
		abort(a)
		time.Sleep(time.Millisecond * 500)
		status, err := controller.NewDAGController(d).GetLastStatus()
		require.NoError(t, err)
		require.Equal(t, scheduler.SchedulerStatus_Cancel, status.Status)
	}
}

func TestPreConditionInvalid(t *testing.T) {
	d := testLoadDAG(t, "multiple_steps.yaml")
	d.Preconditions = []*dag.Condition{
		{
			Condition: "`echo 1`",
			Expected:  "0",
		},
	}

	status, err := testDAG(t, d)
	require.Error(t, err)

	require.Equal(t, scheduler.SchedulerStatus_Cancel, status.Status)
	require.Equal(t, scheduler.NodeStatus_None, status.Nodes[0].Status)
	require.Equal(t, scheduler.NodeStatus_None, status.Nodes[1].Status)
}

func TestPreConditionValid(t *testing.T) {
	d := testLoadDAG(t, "with_params.yaml")

	d.Preconditions = []*dag.Condition{
		{
			Condition: "`echo 1`",
			Expected:  "1",
		},
	}
	status, err := testDAG(t, d)
	require.NoError(t, err)

	require.Equal(t, scheduler.SchedulerStatus_Success, status.Status)
	for _, s := range status.Nodes {
		require.Equal(t, scheduler.NodeStatus_Success, s.Status)
	}
}

func TestStartError(t *testing.T) {
	d := testLoadDAG(t, "error.yaml")
	status, err := testDAG(t, d)
	require.Error(t, err)

	require.Equal(t, scheduler.SchedulerStatus_Error, status.Status)
}

func TestOnExit(t *testing.T) {
	d := testLoadDAG(t, "on_exit.yaml")
	status, err := testDAG(t, d)
	require.NoError(t, err)

	require.Equal(t, scheduler.SchedulerStatus_Success, status.Status)
	for _, s := range status.Nodes {
		require.Equal(t, scheduler.NodeStatus_Success, s.Status)
	}
	require.Equal(t, scheduler.NodeStatus_Success, status.OnExit.Status)
}

func TestRetry(t *testing.T) {
	d := testLoadDAG(t, "retry.yaml")

	status, err := testDAG(t, d)
	require.Error(t, err)
	require.Equal(t, scheduler.SchedulerStatus_Error, status.Status)

	for _, n := range status.Nodes {
		n.CmdWithArgs = "true"
	}
	a := &Agent{
		AgentConfig: &AgentConfig{
			DAG: d,
		},
		RetryConfig: &RetryConfig{
			Status: status,
		},
	}
	err = a.Run()
	status = a.Status()
	require.NoError(t, err)
	require.Equal(t, scheduler.SchedulerStatus_Success, status.Status)

	for _, n := range status.Nodes {
		if n.Status != scheduler.NodeStatus_Success &&
			n.Status != scheduler.NodeStatus_Skipped {
			t.Errorf("invalid status: %s", n.Status.String())
		}
	}
}

func TestHandleHTTP(t *testing.T) {
	d := testLoadDAG(t, "handle_http.yaml")

	a := &Agent{AgentConfig: &AgentConfig{
		DAG: d,
	}}

	go func() {
		err := a.Run()
		require.NoError(t, err)
	}()

	<-time.After(time.Millisecond * 50)

	var mockResponseWriter = mockResponseWriter{}

	// status
	r := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "/status",
		},
	}

	a.handleHTTP(&mockResponseWriter, r)
	require.Equal(t, http.StatusOK, mockResponseWriter.status)

	status, err := models.StatusFromJson(mockResponseWriter.body)
	require.NoError(t, err)
	require.Equal(t, scheduler.SchedulerStatus_Running, status.Status)

	// invalid path
	r = &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "/invalid-path",
		},
	}
	a.handleHTTP(&mockResponseWriter, r)
	require.Equal(t, http.StatusNotFound, mockResponseWriter.status)

	// cancel
	r = &http.Request{
		Method: "POST",
		URL: &url.URL{
			Path: "/stop",
		},
	}
	a.handleHTTP(&mockResponseWriter, r)
	require.Equal(t, http.StatusOK, mockResponseWriter.status)
	require.Equal(t, "OK", mockResponseWriter.body)

	<-time.After(time.Millisecond * 50)

	status = a.Status()
	require.Equal(t, status.Status, scheduler.SchedulerStatus_Cancel)
}

type mockResponseWriter struct {
	status int
	body   string
	header *http.Header
}

var _ (http.ResponseWriter) = (*mockResponseWriter)(nil)

func (h *mockResponseWriter) Header() http.Header {
	if h.header == nil {
		h.header = &http.Header{}
	}
	return *h.header
}

func (h *mockResponseWriter) Write(body []byte) (int, error) {
	h.body = string(body)
	return 0, nil
}

func (h *mockResponseWriter) WriteHeader(statusCode int) {
	h.status = statusCode
}

func testDAG(t *testing.T, d *dag.DAG) (*models.Status, error) {
	t.Helper()
	a := &Agent{AgentConfig: &AgentConfig{
		DAG: d,
	}}
	err := a.Run()
	return a.Status(), err
}

func testLoadDAG(t *testing.T, name string) *dag.DAG {
	file := filepath.Join(testdataDir, name)
	cl := &dag.Loader{}
	d, err := cl.Load(file, "")
	require.NoError(t, err)
	return d
}

func testDAGAsync(t *testing.T, file string) (*Agent, *dag.DAG) {
	t.Helper()

	d := testLoadDAG(t, file)
	a := &Agent{AgentConfig: &AgentConfig{
		DAG: d,
	}}

	go func() {
		a.Run()
	}()

	return a, d
}
