package controller

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yohamta/dagu/internal/dag"
	"github.com/yohamta/dagu/internal/database"
	"github.com/yohamta/dagu/internal/models"
	"github.com/yohamta/dagu/internal/scheduler"
	"github.com/yohamta/dagu/internal/sock"
	"github.com/yohamta/dagu/internal/utils"
	"github.com/yohamta/grep"
)

// GrepResult is a result of grep.
type GrepResult struct {
	Name    string
	DAG     *dag.DAG
	Matches []*grep.Match
}

// GrepDAG returns all DAGs that contain the given string.
func GrepDAG(dir string, pattern string) (ret []*GrepResult, errs []string, err error) {
	ret = []*GrepResult{}
	errs = []string{}
	if !utils.FileExists(dir) {
		if err = os.MkdirAll(dir, 0755); err != nil {
			errs = append(errs, err.Error())
			return
		}
	}
	fis, err := os.ReadDir(dir)
	dl := &dag.Loader{}
	opts := &grep.Options{
		IsRegexp: true,
		Before:   2,
		After:    2,
	}
	utils.LogErr("read DAGs directory", err)
	for _, fi := range fis {
		if utils.MatchExtension(fi.Name(), dag.EXTENSIONS) {
			fn := filepath.Join(dir, fi.Name())
			utils.LogErr("read DAG file", err)
			m, err := grep.Grep(fn, fmt.Sprintf("(?i)%s", pattern), opts)
			if err != nil {
				continue
			} else if err != nil {
				errs = append(errs, fmt.Sprintf("grep %s failed: %s", fi.Name(), err))
				continue
			}
			dag, err := dl.LoadHeadOnly(fn)
			if err != nil {
				errs = append(errs, fmt.Sprintf("check %s failed: %s", fi.Name(), err))
				continue
			}
			ret = append(ret, &GrepResult{
				Name:    strings.TrimSuffix(fi.Name(), filepath.Ext(fi.Name())),
				DAG:     dag,
				Matches: m,
			})
		}
	}
	return ret, errs, nil
}

const (
	_DAGTemplate = `steps:
  - name: step1
    command: echo hello
`
)

// CreateDAG creates a new DAG.
func CreateDAG(file string) error {
	if err := validateLocation(file); err != nil {
		return err
	}
	if utils.FileExists(file) {
		return fmt.Errorf("the config file %s already exists", file)
	}
	return os.WriteFile(file, []byte(_DAGTemplate), 0644)
}

// MoveDAG moves the DAG file.
func MoveDAG(oldDAGPath, newDAGPath string) error {
	if err := validateLocation(newDAGPath); err != nil {
		return err
	}
	if err := os.Rename(oldDAGPath, newDAGPath); err != nil {
		return err
	}
	db := database.New()
	return db.MoveData(oldDAGPath, newDAGPath)
}

// DAGController is a object to interact with a DAG.
type DAGController struct {
	*dag.DAG
}

func NewDAGController(d *dag.DAG) *DAGController {
	return &DAGController{
		DAG: d,
	}
}

func (dc *DAGController) Stop() error {
	client := sock.Client{Addr: dc.SockAddr()}
	_, err := client.Request("POST", "/stop")
	return err
}

func (dc *DAGController) StartAsync(binPath string, workDir string, params string) {
	go func() {
		err := dc.Start(binPath, workDir, params)
		utils.LogErr("starting a DAG", err)
	}()
}

func (dc *DAGController) Start(binPath string, workDir string, params string) error {
	args := []string{"start"}
	if params != "" {
		args = append(args, fmt.Sprintf("--params=\"%s\"", params))
	}
	args = append(args, dc.Location)
	cmd := exec.Command(binPath, args...)
	cmd.SysProcAttr = utils.SysProcAttrForSetpgid()
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	err := cmd.Start()
	if err != nil {
		return err
	}
	return cmd.Wait()
}

func (dc *DAGController) Retry(binPath string, workDir string, reqId string) (err error) {
	go func() {
		args := []string{"retry"}
		args = append(args, fmt.Sprintf("--req=%s", reqId))
		args = append(args, dc.Location)
		cmd := exec.Command(binPath, args...)
		cmd.SysProcAttr = utils.SysProcAttrForSetpgid()
		cmd.Dir = workDir
		cmd.Env = os.Environ()
		defer cmd.Wait()
		err = cmd.Start()
		utils.LogErr("retry a DAG", err)
	}()
	time.Sleep(time.Millisecond * 500)
	return
}

func (dc *DAGController) Restart(bin string, workDir string) error {
	args := []string{"restart", dc.Location}
	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = utils.SysProcAttrForSetpgid()
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	err := cmd.Start()
	if err != nil {
		return err
	}
	return cmd.Wait()
}

func (dc *DAGController) GetStatus() (*models.Status, error) {
	client := sock.Client{Addr: dc.SockAddr()}
	ret, err := client.Request("GET", "/status")
	if err != nil {
		if errors.Is(err, sock.ErrTimeout) {
			return nil, err
		} else {
			return defaultStatus(dc.DAG), nil
		}
	}
	return models.StatusFromJson(ret)
}

func (dc *DAGController) GetLastStatus() (*models.Status, error) {
	client := sock.Client{Addr: dc.SockAddr()}
	ret, err := client.Request("GET", "/status")
	if err == nil {
		return models.StatusFromJson(ret)
	}

	if err == nil || !errors.Is(err, sock.ErrTimeout) {
		db := database.New()
		status, err := db.ReadStatusToday(dc.Location)
		if err != nil {
			var readErr error = nil
			if err != database.ErrNoStatusDataToday && err != database.ErrNoStatusData {
				fmt.Printf("read status failed : %s", err)
				readErr = err
			}
			return defaultStatus(dc.DAG), readErr
		}
		// it is wrong status if the status is running
		status.CorrectRunningStatus()
		return status, nil
	}
	return nil, err
}

func (dc *DAGController) GetStatusByRequestId(requestId string) (*models.Status, error) {
	db := database.New()
	ret, err := db.FindByRequestId(dc.Location, requestId)
	if err != nil {
		return nil, err
	}
	status, _ := dc.GetStatus()
	if status != nil && status.RequestId != requestId {
		// if the request id is not matched then correct the status
		ret.Status.CorrectRunningStatus()
	}
	return ret.Status, err
}

func (dc *DAGController) GetRecentStatuses(n int) []*models.StatusFile {
	db := database.New()
	ret := db.ReadStatusHist(dc.Location, n)
	return ret
}

func (dc *DAGController) UpdateStatus(status *models.Status) error {
	client := sock.Client{Addr: dc.SockAddr()}
	res, err := client.Request("GET", "/status")
	if err != nil {
		if errors.Is(err, sock.ErrTimeout) {
			return err
		}
	} else {
		ss, _ := models.StatusFromJson(res)
		if ss != nil && ss.RequestId == status.RequestId &&
			ss.Status == scheduler.SchedulerStatus_Running {
			return fmt.Errorf("the DAG is running")
		}
	}
	db := database.New()
	toUpdate, err := db.FindByRequestId(dc.Location, status.RequestId)
	if err != nil {
		return err
	}
	w := &database.Writer{Target: toUpdate.File}
	if err := w.Open(); err != nil {
		return err
	}
	defer w.Close()
	return w.Write(status)
}

func (dc *DAGController) UpdateDAGSpec(value string) error {
	// validate
	cl := dag.Loader{}
	_, err := cl.LoadData([]byte(value))
	if err != nil {
		return err
	}
	if !utils.FileExists(dc.Location) {
		return fmt.Errorf("the config file %s does not exist", dc.Location)
	}
	err = os.WriteFile(dc.Location, []byte(value), 0755)
	return err
}

func (dc *DAGController) DeleteDAG() error {
	db := database.New()
	err := db.RemoveAll(dc.Location)
	if err != nil {
		return err
	}
	return os.Remove(dc.Location)
}

func validateLocation(dagLocation string) error {
	if filepath.Ext(dagLocation) != ".yaml" {
		return fmt.Errorf("the config file must be a yaml file with .yaml extension")
	}
	return nil
}

func defaultStatus(d *dag.DAG) *models.Status {
	return models.NewStatus(
		d,
		nil,
		scheduler.SchedulerStatus_None,
		int(models.PidNotRunning), nil, nil)
}
