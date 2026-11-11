package services

import (
	"clawreef/internal/models"
	"clawreef/internal/services/k8s"
	"context"
)

func (s *RuntimeScheduler) ColdTeamNFSWorkspace(ctx context.Context, source, target models.RuntimePod, workspace string) (bool, error) {
	client, ok := s.agentClient.(*slotRuntimeAgentClient)
	if !ok || client.kube == nil || source.Namespace != target.Namespace {
		return false, nil
	}
	return k8s.VerifyColdNFSWorkspace(ctx, client.kube, source.Namespace, source.PodName, stringValue(source.PodUID), target.PodName, stringValue(target.PodUID), workspace)
}
