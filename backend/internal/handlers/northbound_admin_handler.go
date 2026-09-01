package handlers

import (
	"errors"
	"net/http"

	"clawreef/internal/models"
	"clawreef/internal/services"
	"clawreef/internal/utils"
	"github.com/gin-gonic/gin"
)

type NorthboundAdminHandler struct {
	service *services.NorthboundAdminService
}

func NewNorthboundAdminHandler(service *services.NorthboundAdminService) *NorthboundAdminHandler {
	return &NorthboundAdminHandler{service: service}
}

func adminActorID(c *gin.Context) int { value, _ := c.Get("userID"); id, _ := value.(int); return id }

func (h *NorthboundAdminHandler) Overview(c *gin.Context) {
	value, err := h.service.Overview(c.Request.Context())
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Northbound settings retrieved", value)
}

func (h *NorthboundAdminHandler) SaveSettings(c *gin.Context) {
	var req models.NorthboundAdminSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	value, err := h.service.SaveSettings(c.Request.Context(), adminActorID(c), &req)
	if err != nil {
		if errors.Is(err, services.ErrNorthboundSettingsConflict) {
			utils.Error(c, http.StatusConflict, err.Error())
			return
		}
		utils.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.Success(c, http.StatusOK, "Northbound settings saved", value)
}

func (h *NorthboundAdminHandler) SaveCaller(c *gin.Context) {
	var req models.NorthboundCallerPolicy
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	value, err := h.service.SaveCallerPolicy(c.Request.Context(), adminActorID(c), &req)
	if err != nil {
		utils.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.Success(c, http.StatusOK, "Northbound caller policy saved", value)
}

func (h *NorthboundAdminHandler) SetExternalNodePort(c *gin.Context) {
	var req struct {
		NodePort int `json:"node_port" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	value, err := h.service.SetExternalNodePort(c.Request.Context(), adminActorID(c), req.NodePort)
	if err != nil {
		utils.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.Success(c, http.StatusOK, "Northbound external NodePort updated", value)
}

func (h *NorthboundAdminHandler) DownloadCA(c *gin.Context) {
	value, err := h.service.PublicCA(c.Request.Context())
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=clawmanager-northbound-ca.crt")
	c.Data(http.StatusOK, "application/x-x509-ca-cert", value)
}

func (h *NorthboundAdminHandler) DownloadPreparedCA(c *gin.Context) {
	value, err := h.service.PreparedCA(c.Request.Context())
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=clawmanager-northbound-managed-ca.crt")
	c.Data(http.StatusOK, "application/x-x509-ca-cert", value)
}
func (h *NorthboundAdminHandler) PrepareCertificate(c *gin.Context) {
	var req services.CertificatePrepareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	value, err := h.service.PrepareManagedCertificate(c.Request.Context(), adminActorID(c), req)
	if err != nil {
		utils.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.Success(c, http.StatusOK, "Managed northbound certificate prepared", value)
}
func (h *NorthboundAdminHandler) ActivateCertificate(c *gin.Context) {
	var req struct {
		Confirmation string `json:"confirmation"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	if req.Confirmation != "ACTIVATE" {
		utils.Error(c, http.StatusBadRequest, "confirmation must be ACTIVATE")
		return
	}
	value, err := h.service.ActivateManagedCertificate(c.Request.Context(), adminActorID(c))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.Success(c, http.StatusOK, "Managed northbound certificate activated", value)
}
