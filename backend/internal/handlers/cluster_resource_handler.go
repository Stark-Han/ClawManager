package handlers

import (
	"net/http"

	"clawreef/internal/services"
	"clawreef/internal/utils"

	"github.com/gin-gonic/gin"
)

type ClusterResourceHandler struct {
	clusterResourceService services.ClusterResourceService
}

func NewClusterResourceHandler(clusterResourceService services.ClusterResourceService) *ClusterResourceHandler {
	return &ClusterResourceHandler{clusterResourceService: clusterResourceService}
}

func (h *ClusterResourceHandler) GetOverview(c *gin.Context) {
	overview, err := h.clusterResourceService.GetOverview(c.Request.Context())
	if err != nil {
		utils.HandleError(c, err)
		return
	}

	utils.Success(c, http.StatusOK, "Cluster resource overview retrieved successfully", overview)
}
