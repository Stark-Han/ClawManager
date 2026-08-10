package handlers

import (
	"net/http"
	"strconv"

	"clawreef/internal/services"
	"clawreef/internal/utils"

	"github.com/gin-gonic/gin"
)

type SecurityHandler struct {
	service services.SecurityScanService
}

func NewSecurityHandler(service services.SecurityScanService) *SecurityHandler {
	return &SecurityHandler{service: service}
}

func (h *SecurityHandler) GetConfig(c *gin.Context) {
	item, err := h.service.GetConfig()
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Security scan config retrieved successfully", item)
}

func (h *SecurityHandler) SaveConfig(c *gin.Context) {
	var req services.SecurityScanConfigPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	userID, _ := c.Get("userID")
	item, err := h.service.SaveConfig(userID.(int), req)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Security scan config saved successfully", item)
}

func (h *SecurityHandler) StartScan(c *gin.Context) {
	var req services.StartSecurityScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	userID, _ := c.Get("userID")
	item, err := h.service.StartScan(userID.(int), req)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "Security scan started successfully", item)
}

func (h *SecurityHandler) ListJobs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := h.service.ListJobs(limit)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Security scan jobs retrieved successfully", items)
}

func (h *SecurityHandler) GetJob(c *gin.Context) {
	jobID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "invalid job ID")
		return
	}
	item, err := h.service.GetJob(jobID)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Security scan job retrieved successfully", item)
}

func (h *SecurityHandler) RescanSkill(c *gin.Context) {
	skillID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "invalid skill ID")
		return
	}
	var req struct {
		ScanMode string `json:"scan_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	userID, _ := c.Get("userID")
	item, err := h.service.RescanSkill(userID.(int), skillID, req.ScanMode)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "Skill rescan started successfully", item)
}
