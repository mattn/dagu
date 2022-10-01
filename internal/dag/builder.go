package dag

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-shellwords"
	"github.com/robfig/cron/v3"
	"github.com/yohamta/dagu/internal/constants"
	"github.com/yohamta/dagu/internal/utils"
)

var EXTENSIONS = []string{".yaml", ".yml"}

type BuildDAGOptions struct {
	headOnly   bool
	parameters string
	noEval     bool
	noSetenv   bool
	defaultEnv map[string]string
}

type builder struct {
	BuildDAGOptions
	baseConfig *DAG
}

type buildStep struct {
	BuildFn  func(def *configDefinition, d *DAG) error
	Headline bool
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func (b *builder) buildFromDefinition(def *configDefinition, baseConfig *DAG) (d *DAG, err error) {
	b.baseConfig = baseConfig

	d = &DAG{}
	d.Init()

	d.Name = def.Name
	if def.Name != "" {
		d.Name = def.Name
	}
	d.Group = def.Group
	d.Description = def.Description
	if def.MailOn != nil {
		d.MailOn = &MailOn{
			Failure: def.MailOn.Failure,
			Success: def.MailOn.Success,
		}
	}
	d.Delay = time.Second * time.Duration(def.DelaySec)
	d.RestartWait = time.Second * time.Duration(def.RestartWaitSec)
	d.Tags = parseTags(def.Tags)

	for _, bs := range []buildStep{
		{
			BuildFn:  b.buildSchedule,
			Headline: true,
		},
		{
			BuildFn: b.buildEnvVariables,
		},
		{
			BuildFn: b.buildLogdir,
		},
		{
			BuildFn: b.buildParameters,
		},
		{
			BuildFn: b.buildStepsFromDefinition,
		},
		{
			BuildFn: b.buildHandlers,
		},
		{
			BuildFn: b.buildConfig,
		},
		{
			BuildFn: buildSmtpConfigFromDefinition,
		},
		{
			BuildFn: buildErrorMailConfig,
		},
		{
			BuildFn: buildInfoMailConfig,
		},
	} {
		if (b.headOnly && bs.Headline) || !b.headOnly {
			if err = bs.BuildFn(def, d); err != nil {
				return
			}
		}
	}

	return d, nil
}

const (
	scheduleStart   = "start"
	scheduleStop    = "stop"
	scheduleRestart = "restart"
)

func (b *builder) buildSchedule(def *configDefinition, d *DAG) error {
	starts := []string{}
	stops := []string{}
	restarts := []string{}

	switch (def.Schedule).(type) {
	case string:
		starts = append(starts, def.Schedule.(string))
	case []interface{}:
		for _, s := range def.Schedule.([]interface{}) {
			if ss, ok := s.(string); ok {
				starts = append(starts, ss)
			} else {
				return fmt.Errorf("schedule must be a string or an array of strings")
			}
		}
	case map[interface{}]interface{}:
		for k, v := range def.Schedule.(map[interface{}]interface{}) {
			if _, ok := k.(string); !ok {
				return fmt.Errorf("schedule key must be a string")
			}
			kk := k.(string)
			switch kk {
			case scheduleStart, scheduleStop, scheduleRestart:
				switch (v).(type) {
				case string:
					switch kk {
					case scheduleStart:
						starts = append(starts, v.(string))
					case scheduleStop:
						stops = append(stops, v.(string))
					case scheduleRestart:
						restarts = append(restarts, v.(string))
					}
				case []interface{}:
					for _, vv := range v.([]interface{}) {
						if vvv, ok := vv.(string); ok {
							switch kk {
							case scheduleStart:
								starts = append(starts, vvv)
							case scheduleStop:
								stops = append(stops, vvv)
							case scheduleRestart:
								restarts = append(restarts, vvv)
							}
						} else {
							return fmt.Errorf("schedule must be a string or an array of strings")
						}
					}
				default:
					return fmt.Errorf("schedule must be a string or an array of strings")
				}
			default:
				return fmt.Errorf("schedule key must be start or stop")
			}
		}
	case nil:
	default:
		return fmt.Errorf("invalid schedule type: %T", def.Schedule)
	}
	var err error
	d.Schedule, err = parseSchedule(starts)
	if err != nil {
		return err
	}
	d.StopSchedule, err = parseSchedule(stops)
	if err != nil {
		return err
	}
	d.RestartSchedule, err = parseSchedule(restarts)
	return err
}

func (b *builder) buildEnvVariables(def *configDefinition, d *DAG) (err error) {
	var env map[string]string
	env, err = b.loadVariables(def.Env, b.defaultEnv)
	if err == nil {
		d.Env = buildConfigEnv(env)
		if b.baseConfig != nil {
			for _, e := range b.baseConfig.Env {
				key := strings.SplitN(e, "=", 2)[0]
				if _, ok := env[key]; !ok {
					d.Env = append(d.Env, e)
				}
			}
		}
	}
	return
}

func (b *builder) buildLogdir(def *configDefinition, d *DAG) (err error) {
	d.LogDir, err = utils.ParseVariable(def.LogDir)
	return err
}

func (b *builder) buildParameters(def *configDefinition, d *DAG) (err error) {
	d.DefaultParams = def.Params
	p := d.DefaultParams
	if b.parameters != "" {
		p = b.parameters
	}
	var envs []string
	d.Params, envs, err = b.parseParameters(p, !b.noEval)
	if err == nil {
		d.Env = append(d.Env, envs...)
	}
	return
}

func (b *builder) buildHandlers(def *configDefinition, d *DAG) (err error) {
	if def.HandlerOn.Exit != nil {
		def.HandlerOn.Exit.Name = constants.OnExit
		if d.HandlerOn.Exit, err = b.buildStep(d.Env, def.HandlerOn.Exit); err != nil {
			return err
		}
	}

	if def.HandlerOn.Success != nil {
		def.HandlerOn.Success.Name = constants.OnSuccess
		if d.HandlerOn.Success, err = b.buildStep(d.Env, def.HandlerOn.Success); err != nil {
			return
		}
	}

	if def.HandlerOn.Failure != nil {
		def.HandlerOn.Failure.Name = constants.OnFailure
		if d.HandlerOn.Failure, err = b.buildStep(d.Env, def.HandlerOn.Failure); err != nil {
			return
		}
	}

	if def.HandlerOn.Cancel != nil {
		def.HandlerOn.Cancel.Name = constants.OnCancel
		if d.HandlerOn.Cancel, err = b.buildStep(d.Env, def.HandlerOn.Cancel); err != nil {
			return
		}
	}
	return nil
}

func (b *builder) buildConfig(def *configDefinition, d *DAG) (err error) {
	if def.HistRetentionDays != nil {
		d.HistRetentionDays = *def.HistRetentionDays
	}
	d.Preconditions = loadPreCondition(def.Preconditions)
	d.MaxActiveRuns = def.MaxActiveRuns

	if def.MaxCleanUpTimeSec != nil {
		d.MaxCleanUpTime = time.Second * time.Duration(*def.MaxCleanUpTimeSec)
	}
	return nil
}

func (b *builder) parseParameters(value string, eval bool) (
	params []string,
	envs []string,
	err error,
) {
	parser := shellwords.NewParser()
	parser.ParseBacktick = false
	parser.ParseEnv = false

	var parsed []string
	parsed, err = parser.Parse(value)
	if err != nil {
		return
	}

	ret := []string{}
	for i, v := range parsed {
		if eval {
			v, err = utils.ParseCommand(os.ExpandEnv(v))
			if err != nil {
				return nil, nil, err
			}
		}
		if !b.noSetenv {
			if strings.Contains(v, "=") {
				parts := strings.SplitN(v, "=", 2)
				os.Setenv(parts[0], parts[1])
				envs = append(envs, v)
			}
			err = os.Setenv(strconv.Itoa(i+1), v)
			if err != nil {
				return nil, nil, err
			}
		}
		ret = append(ret, v)
	}
	return ret, envs, nil
}

type envVariable struct {
	key string
	val string
}

func (b *builder) loadVariables(strVariables interface{}, defaults map[string]string) (
	map[string]string, error,
) {
	var vals []*envVariable = []*envVariable{}
	for k, v := range defaults {
		vals = append(vals, &envVariable{k, v})
	}

	loadFn := func(a []*envVariable, m map[interface{}]interface{}) ([]*envVariable, error) {
		for k, v := range m {
			if ks, ok := k.(string); ok {
				if vs, ok := v.(string); ok {
					a = append(a, &envVariable{ks, vs})
				} else {
					return a, fmt.Errorf("invalid value for env %s", ks)
				}
			}
		}
		return a, nil
	}

	var err error
	if a, ok := strVariables.(map[interface{}]interface{}); ok {
		vals, err = loadFn(vals, a)
		if err != nil {
			return nil, err
		}
	}

	if a, ok := strVariables.([]interface{}); ok {
		for _, v := range a {
			if aa, ok := v.(map[interface{}]interface{}); ok {
				vals, err = loadFn(vals, aa)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	vars := map[string]string{}
	for _, v := range vals {
		parsed, err := utils.ParseVariable(v.val)
		if err != nil {
			return nil, err
		}
		vars[v.key] = parsed
		if !b.noSetenv {
			err = os.Setenv(v.key, parsed)
			if err != nil {
				return nil, err
			}
		}
	}
	return vars, nil
}

func (b *builder) buildStepsFromDefinition(def *configDefinition, d *DAG) error {
	ret := []*Step{}
	for _, stepDef := range def.Steps {
		step, err := b.buildStep(d.Env, stepDef)
		if err != nil {
			return err
		}
		ret = append(ret, step)
	}
	d.Steps = ret

	return nil
}

func (b *builder) buildStep(variables []string, def *stepDef) (*Step, error) {
	if err := assertStepDef(def); err != nil {
		return nil, err
	}
	step := &Step{}
	step.Name = def.Name
	step.Description = def.Description
	step.CmdWithArgs = def.Command
	step.Command, step.Args = utils.SplitCommand(step.CmdWithArgs, false)
	step.Script = def.Script
	step.Stdout = b.expandEnv(def.Stdout)
	step.Stderr = b.expandEnv(def.Stderr)
	step.Output = def.Output
	step.Dir = b.expandEnv(def.Dir)
	step.ExecutorConfig.Config = map[string]interface{}{}
	if def.Executor != nil {
		switch val := (def.Executor).(type) {
		case string:
			step.ExecutorConfig.Type = val
		case map[interface{}]interface{}:
			for k, v := range val {
				if k == "type" {
					step.ExecutorConfig.Type = v.(string)
				} else {
					step.ExecutorConfig.Config[k.(string)] = v
				}
			}
		default:
			return nil, fmt.Errorf("invalid executor config")
		}
	}
	// TODO: validate executor config
	step.Variables = variables
	step.Depends = def.Depends
	if def.ContinueOn != nil {
		step.ContinueOn.Skipped = def.ContinueOn.Skipped
		step.ContinueOn.Failure = def.ContinueOn.Failure
	}
	if def.RetryPolicy != nil {
		step.RetryPolicy = &RetryPolicy{
			Limit:    def.RetryPolicy.Limit,
			Interval: time.Second * time.Duration(def.RetryPolicy.IntervalSec),
		}
	}
	if def.RepeatPolicy != nil {
		step.RepeatPolicy.Repeat = def.RepeatPolicy.Repeat
		step.RepeatPolicy.Interval = time.Second * time.Duration(def.RepeatPolicy.IntervalSec)
	}
	if def.SignalOnStop != nil {
		sigDef := *def.SignalOnStop
		_, err := utils.SignalNum(sigDef)
		if err != nil {
			return nil, err
		}
		step.SignalOnStop = sigDef
	}
	step.MailOnError = def.MailOnError
	step.Preconditions = loadPreCondition(def.Preconditions)
	return step, nil
}

func (b *builder) expandEnv(val string) string {
	if b.noEval {
		return val
	}
	return os.ExpandEnv(val)
}

func buildSmtpConfigFromDefinition(def *configDefinition, d *DAG) (err error) {
	smtp := &SmtpConfig{}
	smtp.Host = def.Smtp.Host
	smtp.Port = def.Smtp.Port
	smtp.Username = def.Smtp.Username
	smtp.Password = def.Smtp.Password
	d.Smtp = smtp
	return nil
}

func buildErrorMailConfig(def *configDefinition, d *DAG) (err error) {
	d.ErrorMail, err = buildMailConfigFromDefinition(def.ErrorMail)
	return
}

func buildInfoMailConfig(def *configDefinition, d *DAG) (err error) {
	d.InfoMail, err = buildMailConfigFromDefinition(def.InfoMail)
	return
}

func buildMailConfigFromDefinition(def mailConfigDef) (*MailConfig, error) {
	d := &MailConfig{}
	d.From = def.From
	d.To = def.To
	d.Prefix = def.Prefix
	return d, nil
}

func buildConfigEnv(vars map[string]string) []string {
	ret := []string{}
	for k, v := range vars {
		ret = append(ret, fmt.Sprintf("%s=%s", k, v))
	}
	return ret
}

func loadPreCondition(cond []*conditionDef) []*Condition {
	ret := []*Condition{}
	for _, v := range cond {
		ret = append(ret, &Condition{
			Condition: v.Condition,
			Expected:  v.Expected,
		})
	}
	return ret
}

func parseTags(value string) []string {
	values := strings.Split(value, ",")
	ret := []string{}
	for _, v := range values {
		tag := strings.ToLower(strings.TrimSpace(v))
		if tag != "" {
			ret = append(ret, tag)
		}
	}
	return ret
}

func parseSchedule(values []string) ([]*Schedule, error) {
	ret := []*Schedule{}
	for _, v := range values {
		paresed, err := cronParser.Parse(v)
		if err != nil {
			return nil, fmt.Errorf("invalid schedule: %s", err)
		}
		ret = append(ret, &Schedule{
			Expression: v,
			Parsed:     paresed,
		})
	}
	return ret, nil
}

func assertStepDef(def *stepDef) error {
	if def.Name == "" {
		return fmt.Errorf("step name must be specified")
	}
	if def.Command == "" {
		return fmt.Errorf("step command must be specified")
	}
	return nil
}
