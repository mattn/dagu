package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yohamta/dagu/internal/admin"
	"github.com/yohamta/dagu/internal/controller"
	"github.com/yohamta/dagu/internal/dag"
	"github.com/yohamta/dagu/internal/scheduler"
	"github.com/yohamta/dagu/internal/utils"
)

func TestAgent(t *testing.T) {
	tmpDir := utils.MustTempDir("runner_agent_test")
	defer func() {
		os.RemoveAll(tmpDir)
	}()

	now := time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC)
	agent := NewAgent(
		&admin.Config{
			DAGs:    testdataDir,
			Command: testBin,
			LogDir:  filepath.Join(tmpDir, "log"),
		})
	utils.FixedTime = now

	go func() {
		err := agent.Start()
		require.NoError(t, err)
	}()

	pathToDAG := filepath.Join(testdataDir, "scheduled_job.yaml")
	loader := &dag.Loader{}
	dag, err := loader.LoadHeadOnly(pathToDAG)
	require.NoError(t, err)
	c := controller.NewDAGController(dag)

	require.Eventually(t, func() bool {
		status, err := c.GetLastStatus()
		return err == nil && status.Status == scheduler.SchedulerStatus_Success
	}, time.Second*1, time.Millisecond*100)

	agent.Stop()
}

func TestAgentForStop(t *testing.T) {
	tmpDir := utils.MustTempDir("runner_agent_test_for_stop")
	defer func() {
		os.RemoveAll(tmpDir)
	}()

	now := time.Date(2020, 1, 1, 1, 1, 0, 0, time.UTC)
	agent := NewAgent(
		&admin.Config{
			DAGs:    testdataDir,
			Command: testBin,
			LogDir:  filepath.Join(tmpDir, "log"),
		})
	utils.FixedTime = now

	// read the test DAG
	file := filepath.Join(testdataDir, "start_stop.yaml")
	dr := controller.NewDAGStatusReader()
	dag, _ := dr.ReadStatus(file, false)
	c := controller.NewDAGController(dag.DAG)

	j := &job{
		DAG:    dag.DAG,
		Config: testConfig,
		Next:   time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC),
	}

	// start the test job
	go func() {
		_ = j.Start()
	}()

	time.Sleep(time.Millisecond * 100)

	// confirm the job is running
	status, err := c.GetLastStatus()
	require.NoError(t, err)
	require.Equal(t, scheduler.SchedulerStatus_Running, status.Status)

	// start the agent
	go func() {
		err := agent.Start()
		require.NoError(t, err)
	}()

	time.Sleep(time.Millisecond * 100)

	// confirm the test job is canceled
	require.Eventually(t, func() bool {
		s, err := c.GetLastStatus()
		return err == nil && s.Status == scheduler.SchedulerStatus_Cancel
	}, time.Second*1, time.Millisecond*100)

	// stop the agent
	agent.Stop()
}
