package services

import (
	"clawreef/internal/models"
	"clawreef/internal/repository"
	"context"
	"errors"
	"fmt"
	"github.com/upper/db/v4"
	"sort"
	"time"
)

type TeamLifecycleControl interface {
	RequestTeamPower(context.Context, int, int, string) (*models.TeamRuntimeState, error)
	TeamPower(context.Context, int) (*models.TeamRuntimeState, error)
}

type teamLifecycleCoordinator struct {
	store     *repository.TeamLifecycleRepository
	teams     *teamService
	instances *instanceService
	auto      *InstanceAutoStopService
	kick      chan struct{}
}

func ConfigureTeamLifecycle(sess db.Session, teams TeamService, auto *InstanceAutoStopService) {
	t, ok := teams.(*teamService)
	if !ok || auto == nil || auto.lifecycle == nil {
		return
	}
	c := &teamLifecycleCoordinator{store: repository.NewTeamLifecycleRepository(sess), teams: t, instances: auto.lifecycle, auto: auto, kick: make(chan struct{}, 1)}
	t.power = c
	auto.teamPower = c
}

func (s *teamService) TeamPower(ctx context.Context, id int) (*models.TeamRuntimeState, error) {
	if s.power == nil {
		return &models.TeamRuntimeState{State: "running", DesiredState: "running"}, nil
	}
	v, err := s.power.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return &models.TeamRuntimeState{State: "running", DesiredState: "running", CanStop: true}, nil
	}
	return &v.TeamRuntimeState, nil
}
func (s *teamService) RequestTeamPower(ctx context.Context, user, id int, target string) (*models.TeamRuntimeState, error) {
	if _, err := s.requireOwnedTeam(user, id); err != nil {
		return nil, err
	}
	if s.power == nil {
		return nil, fmt.Errorf("团队启停服务尚未就绪")
	}
	if target != "running" && target != "stopped" {
		return nil, fmt.Errorf("invalid Team power target")
	}
	v, err := s.power.store.Request(ctx, id, target, "manual", 0)
	if err != nil {
		return nil, err
	}
	select {
	case s.power.kick <- struct{}{}:
	default:
	}
	return &v.TeamRuntimeState, nil
}
func (s *teamService) requireTeamPowerRunning(ctx context.Context, id int) error {
	if s.power == nil {
		return nil
	}
	v, err := s.TeamPower(ctx, id)
	if err != nil {
		return err
	}
	if v.State != "running" {
		return repository.ErrTeamNotRunning
	}
	return nil
}

func (c *teamLifecycleCoordinator) reconcile(ctx context.Context) error {
	if err := c.store.Initialize(ctx); err != nil {
		return err
	}
	ids, err := c.store.Pending(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		op, e := c.store.Get(ctx, id)
		if e != nil || op == nil {
			continue
		}
		claimed, e := c.store.Claim(ctx, op)
		if e != nil || !claimed {
			continue
		}
		progress, e := c.execute(ctx, op)
		if writeErr := c.store.Result(context.WithoutCancel(ctx), op, progress, e); writeErr != nil {
			return writeErr
		}
	}
	return nil
}
func (c *teamLifecycleCoordinator) scan(ctx context.Context, cfg models.InstanceAutoStopSettings) error {
	ids, err := c.store.Candidates(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, _ = c.store.Request(ctx, id, "stopped", "idle_timeout", cfg.IdleTimeoutDays)
	}
	return nil
}

func (c *teamLifecycleCoordinator) preflight(ctx context.Context, op *models.TeamLifecycleOperation) error {
	if err := c.store.Busy(ctx, op.TeamID); err != nil {
		return err
	}
	client, ok := c.instances.agentClient.(RuntimeIdleStopClient)
	if !ok {
		return fmt.Errorf("Runtime 尚不支持可确认的空闲停止")
	}
	for _, m := range op.Members {
		i, err := c.instances.instanceRepo.GetByID(m.InstanceID)
		if err != nil {
			return err
		}
		if i == nil || i.WorkspacePath == nil || *i.WorkspacePath != m.Workspace || i.RuntimeGeneration != m.Generation {
			return fmt.Errorf("成员身份或工作区已变化")
		}
		a, err := c.auto.store.GetActivity(ctx, i.ID)
		if err != nil {
			return err
		}
		if a != nil && (a.ActiveRequests > 0 || a.StartDeadlineAt != nil) {
			return fmt.Errorf("成员仍有模型请求或启动操作")
		}
		if i.Status == "stopped" && (a == nil || a.StopOperationID == "") {
			b, e := c.instances.bindingRepo.GetByInstanceID(ctx, i.ID)
			if e != nil {
				return e
			}
			if b == nil {
				continue
			}
			return fmt.Errorf("成员停止绑定仍未释放")
		}
		if a != nil && a.StopOperationID != "" {
			return fmt.Errorf("成员停止结果仍在核实，请待核实后继续原团队操作")
		}
		b, err := c.instances.bindingRepo.GetByInstanceID(ctx, i.ID)
		if err != nil {
			return err
		}
		if b == nil || b.Generation != m.Generation {
			return fmt.Errorf("成员运行绑定不可确认")
		}
		p, err := c.instances.runtimePodRepo.GetByID(ctx, b.RuntimePodID)
		if err != nil {
			return err
		}
		if p == nil || p.PodUID == nil || p.AgentEndpoint == nil {
			return fmt.Errorf("成员 Runtime 身份不可确认")
		}
		check, cancel := context.WithTimeout(ctx, 20*time.Second)
		proof, err := client.IdlePreflight(check, *p.AgentEndpoint, b.GatewayID, RuntimeIdleStopRequest{InstanceID: i.ID, Generation: i.RuntimeGeneration, PodUID: *p.PodUID})
		cancel()
		if err != nil {
			if legacyTeamManualStop(op.Reason, err) {
				continue
			}
			return fmt.Errorf("成员 %d 空闲预检未通过: %w", i.ID, err)
		}
		if proof == nil || !proof.ExecutionProofSupported || proof.ExecutionState != "idle" {
			return fmt.Errorf("成员 %d 仍在执行或执行状态未确认", i.ID)
		}
	}
	return nil
}

func (c *teamLifecycleCoordinator) execute(ctx context.Context, op *models.TeamLifecycleOperation) (int, error) {
	if op.DesiredState == "stopped" {
		if err := c.preflight(ctx, op); err != nil {
			if op.Attempts == 0 {
				if aborted, _ := c.store.AbortUnstartedStop(ctx, op); aborted {
					return 0, err
				}
			}
			return 0, err
		}
	}
	members := append([]models.TeamLifecycleMember(nil), op.Members...)
	// Workers recover before Leader. No task is sent during either operation.
	sort.SliceStable(members, func(i, j int) bool {
		if op.DesiredState == "running" {
			return members[i].Role != "leader" && members[j].Role == "leader"
		}
		return members[i].Role == "leader" && members[j].Role != "leader"
	})
	complete := 0
	for _, m := range members {
		if ctx.Err() != nil {
			return complete, ctx.Err()
		}
		release, err := acquireInstanceLifecycleOperation(ctx, c.instances.instanceRepo, m.InstanceID)
		if err != nil {
			return complete, err
		}
		err = c.auto.withInstanceMutation(ctx, m.InstanceID, func() error {
			i, e := c.instances.instanceRepo.GetByID(m.InstanceID)
			if e != nil {
				return e
			}
			if i == nil || i.WorkspacePath == nil || *i.WorkspacePath != m.Workspace {
				return fmt.Errorf("原成员实例或工作区已变化")
			}
			a, e := c.auto.store.GetActivity(ctx, i.ID)
			if e != nil {
				return e
			}
			if a != nil && a.StopOperationID != "" {
				return fmt.Errorf("成员停止尚未确认")
			}
			if op.DesiredState == "stopped" {
				if i.RuntimeGeneration != m.Generation {
					return fmt.Errorf("成员执行代数已变化")
				}
				if i.Status == "stopped" {
					b, e := c.instances.bindingRepo.GetByInstanceID(ctx, i.ID)
					if e != nil {
						return e
					}
					if b != nil {
						return fmt.Errorf("成员停止绑定仍未释放")
					}
					return nil
				}
				if op.Reason == "idle_timeout" {
					cfg, e := c.auto.store.Settings(ctx)
					if e != nil {
						return e
					}
					activity, e := c.auto.store.GetActivity(ctx, i.ID)
					if e != nil {
						return e
					}
					if activity == nil {
						return fmt.Errorf("成员活动记录缺失")
					}
					return c.auto.tryStopGuarded(repository.WithTeamStopAuthority(withTeamIdleStop(ctx, op.TeamID), op.TeamID, op.OperationID), *cfg, *activity)
				}
				return c.instances.stopInstance(i.ID)
			}
			b, e := c.instances.bindingRepo.GetByInstanceID(ctx, i.ID)
			if e != nil {
				return e
			}
			alreadyStarting := b != nil && b.Generation == i.RuntimeGeneration && (b.State == "creating" || b.State == "starting" || b.State == "running")
			if i.Status != "running" && !alreadyStarting {
				if e = c.instances.startInstance(i.ID); e != nil {
					return e
				}
			}
			return nil
		})
		release()
		if err != nil {
			return complete, err
		}
		if op.DesiredState == "running" {
			// Start() acceptance is not readiness. Preserve the same operation on retry.
			deadline := time.NewTimer(120 * time.Second)
			tick := time.NewTicker(time.Second)
			ready := false
			for !ready {
				b, e := c.instances.bindingRepo.GetByInstanceID(ctx, m.InstanceID)
				ready = e == nil && b != nil && b.State == "running"
				if ready {
					ms, readErr := c.teams.repo.ListMembersByTeamID(op.TeamID)
					if readErr != nil {
						return complete, readErr
					}
					team, readErr := c.teams.repo.GetTeamByID(op.TeamID)
					if readErr != nil || team == nil {
						return complete, fmt.Errorf("团队身份不可读取")
					}
					observed := c.teams.ObserveTeamMembers(ctx, []models.Team{*team}, ms)
					ready = false
					for _, member := range observed {
						if member.ID == m.MemberID && member.RuntimeObservation != nil && member.RuntimeObservation.State == "online" {
							ready = true
						}
					}
				}
				if ready {
					break
				}
				select {
				case <-ctx.Done():
					tick.Stop()
					deadline.Stop()
					return complete, ctx.Err()
				case <-deadline.C:
					tick.Stop()
					return complete, fmt.Errorf("成员 %d 启动尚未就绪，请继续原团队启动操作", m.InstanceID)
				case <-tick.C:
				}
			}
			tick.Stop()
			deadline.Stop()
		}
		complete++
		if err := c.store.Progress(ctx, op, complete); err != nil {
			return complete, err
		}
	}
	return complete, nil
}

type teamIdleStopContextKey struct{}

func withTeamIdleStop(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, teamIdleStopContextKey{}, id)
}

// Only an explicit manual operation can use the existing confirmed-stop path
// on a legacy Agent. Automatic idle stop must have positive execution proof.
func legacyTeamManualStop(reason string, err error) bool {
	return reason == "manual" && errors.Is(err, ErrRuntimeAgentNotFound)
}
