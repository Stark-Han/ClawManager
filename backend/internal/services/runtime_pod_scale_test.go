package services

import (
	"clawreef/internal/models"
	"testing"
)

func TestDefaultRuntimeScaleCanonicalRolloutLineage(t *testing.T) {
	ptr := func(s string) *string { return &s }
	approved := func(id int64, sources string) models.RuntimeRollout {
		return models.RuntimeRollout{ID: id, RuntimeType: "openclaw", Status: "finished", SourceImagesJSON: ptr(sources), TargetImageRef: "image", PreflightID: ptr("approved"), PlanFingerprint: ptr("fingerprint")}
	}
	rollouts := []models.RuntimeRollout{
		approved(5, `{"ns/openclaw-runtime":"old","ns/runtime-openclaw-custom":"other"}`),
		approved(6, `{"ns/openclaw-runtime":"old","ns/openclaw-runtime-u5":"previous","ns/runtime-openclaw-custom":"custom"}`),
	}
	targets, err := runtimeDefaultScaleTargets("ns", "openclaw", "openclaw-runtime", rollouts)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].DeploymentName != "openclaw-runtime-u5-u6" || targets[1].DeploymentName != "openclaw-runtime-u6" || targets[0].UpgradeID != "6" || targets[0].SourceDeployment != "openclaw-runtime-u5" {
		t.Fatalf("wrong current default candidates: %+v", targets)
	}
	for _, r := range []models.RuntimeRollout{
		{ID: 7, RuntimeType: "openclaw", Status: "finished", TargetImageRef: "image"},
		approved(7, `{"ns/runtime-openclaw-custom":"custom"}`),
	} {
		if _, err := runtimeDefaultScaleTargets("ns", "openclaw", "openclaw-runtime", append(rollouts, r)); err == nil {
			t.Fatal("fell back to retained/default pool without latest canonical proof")
		}
	}
	defaults, err := runtimeDefaultScaleTargets("ns", "openclaw", "openclaw-runtime", nil)
	if err != nil || len(defaults) != 1 || defaults[0].DeploymentName != "openclaw-runtime" {
		t.Fatalf("initial default unavailable: %+v %v", defaults, err)
	}
}

func TestDefaultRuntimeScaleUsesConfiguredPoolRoot(t *testing.T) {
	scheduler := &RuntimeScheduler{}
	WithRuntimeSchedulerPoolPrefix("site-")(scheduler)
	source, preflight, fingerprint := `{"tenant/site-openclaw-runtime":"old"}`, "approved", "fingerprint"
	history := []models.RuntimeRollout{{ID: 9, RuntimeType: "openclaw", Status: "finished", SourceImagesJSON: &source, PreflightID: &preflight, PlanFingerprint: &fingerprint, TargetImageRef: "new"}}
	targets, err := runtimeDefaultScaleTargets("tenant", RuntimeTypeOpenClaw, scheduler.DefaultRuntimePoolName(RuntimeTypeOpenClaw), history)
	if err != nil || len(targets) != 1 || targets[0].DeploymentName != "site-openclaw-runtime-u9" {
		t.Fatalf("configured OpenClaw lineage was not found: %+v %v", targets, err)
	}
	targets, err = runtimeDefaultScaleTargets("tenant", RuntimeTypeOpenCode, scheduler.DefaultRuntimePoolName(RuntimeTypeOpenCode), nil)
	if err != nil || len(targets) != 1 || targets[0].DeploymentName != "site-opencode-runtime" {
		t.Fatalf("configured OpenCode default was not found: %+v %v", targets, err)
	}
}

func TestDefaultRuntimeScaleIgnoresRecoveryAndLabRollouts(t *testing.T) {
	base := "team-openclaw-runtime"
	preflight, fingerprint := "approved", "fingerprint"
	ordinarySource := `{"tenant/team-openclaw-runtime":"old"}`
	ordinary := models.RuntimeRollout{ID: 10, RuntimeType: RuntimeTypeOpenClaw, Status: "finished", SourceImagesJSON: &ordinarySource, PreflightID: &preflight, PlanFingerprint: &fingerprint, TargetImageRef: "new"}
	for _, scope := range []string{`{"recovery_mode":"retry"}`, `{"recovery_mode":"rebuild"}`, `{"upgrade_lab_run_id":42}`} {
		t.Run(scope, func(t *testing.T) {
			unrelatedSource := `{"tenant/other-runtime":"old"}`
			recovery := models.RuntimeRollout{ID: 11, RuntimeType: RuntimeTypeOpenClaw, Status: "finished", SourceImagesJSON: &unrelatedSource, PreflightJSON: &scope, PreflightID: &preflight, PlanFingerprint: &fingerprint, TargetImageRef: "recovery-image"}
			for _, test := range []struct {
				history []models.RuntimeRollout
				want    string
			}{
				{[]models.RuntimeRollout{recovery}, base},
				{[]models.RuntimeRollout{ordinary, recovery}, base + "-u10"},
			} {
				targets, err := runtimeDefaultScaleTargets("tenant", RuntimeTypeOpenClaw, base, test.history)
				if err != nil || len(targets) != 1 || targets[0].DeploymentName != test.want {
					t.Fatalf("scoped rollout changed the ordinary default: got %+v, %v; want %s", targets, err, test.want)
				}
			}
		})
	}
}
