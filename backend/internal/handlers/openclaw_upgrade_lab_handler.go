package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"clawreef/internal/services"
	"clawreef/internal/utils"

	"github.com/gin-gonic/gin"
)

type OpenClawUpgradeLabHandler struct {
	service *services.OpenClawUpgradeLabService
}

type createUpgradeLabRequest struct {
	InstanceCount int `json:"instance_count"`
}

type startUpgradeLabRequest struct {
	TargetImageRef string `json:"target_image_ref" binding:"required"`
	BatchSize      int    `json:"batch_size"`
}

func NewOpenClawUpgradeLabHandler(service *services.OpenClawUpgradeLabService) *OpenClawUpgradeLabHandler {
	return &OpenClawUpgradeLabHandler{service: service}
}

func (h *OpenClawUpgradeLabHandler) Latest(c *gin.Context) {
	view, err := h.service.Latest(c.Request.Context())
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw upgrade lab loaded", view)
}

func (h *OpenClawUpgradeLabHandler) Get(c *gin.Context) {
	id, ok := upgradeLabRunID(c)
	if !ok {
		return
	}
	view, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	if view == nil {
		utils.Error(c, http.StatusNotFound, "upgrade lab run not found")
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw upgrade lab loaded", view)
}

func (h *OpenClawUpgradeLabHandler) CreateBaseline(c *gin.Context) {
	var req createUpgradeLabRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	actor := currentUserIDPtr(c)
	if actor == nil {
		utils.Error(c, http.StatusUnauthorized, "administrator identity is unavailable")
		return
	}
	view, err := h.service.CreateBaseline(c.Request.Context(), *actor, req.InstanceCount)
	if err != nil {
		utils.Error(c, http.StatusConflict, err.Error())
		return
	}
	utils.Success(c, http.StatusCreated, "OpenClaw 7.1 upgrade lab created", view)
}

func (h *OpenClawUpgradeLabHandler) StartUpgrade(c *gin.Context) {
	id, ok := upgradeLabRunID(c)
	if !ok {
		return
	}
	var req startUpgradeLabRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	actor := currentUserIDPtr(c)
	if actor == nil {
		utils.Error(c, http.StatusUnauthorized, "administrator identity is unavailable")
		return
	}
	view, err := h.service.StartUpgrade(c.Request.Context(), id, *actor, strings.TrimSpace(req.TargetImageRef), req.BatchSize)
	if err != nil {
		utils.Error(c, http.StatusConflict, err.Error())
		return
	}
	utils.Success(c, http.StatusAccepted, "OpenClaw upgrade lab rollout started", view)
}

func (h *OpenClawUpgradeLabHandler) CaptureBaseline(c *gin.Context) {
	id, ok := upgradeLabRunID(c)
	if !ok {
		return
	}
	actor := currentUserIDPtr(c)
	if actor == nil {
		utils.Error(c, http.StatusUnauthorized, "administrator identity is unavailable")
		return
	}
	view, err := h.service.CaptureBaseline(c.Request.Context(), id, *actor)
	if err != nil {
		utils.Error(c, http.StatusConflict, err.Error())
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw 7.1 baseline evidence captured", view)
}

func (h *OpenClawUpgradeLabHandler) Reset(c *gin.Context) {
	id, ok := upgradeLabRunID(c)
	if !ok {
		return
	}
	var req createUpgradeLabRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}
	actor := currentUserIDPtr(c)
	if actor == nil {
		utils.Error(c, http.StatusUnauthorized, "administrator identity is unavailable")
		return
	}
	view, err := h.service.Reset(c.Request.Context(), id, *actor, req.InstanceCount)
	if err != nil {
		utils.Error(c, http.StatusConflict, err.Error())
		return
	}
	utils.Success(c, http.StatusCreated, "OpenClaw upgrade lab reset to 7.1", view)
}

func (h *OpenClawUpgradeLabHandler) Cleanup(c *gin.Context) {
	id, ok := upgradeLabRunID(c)
	if !ok {
		return
	}
	actor := currentUserIDPtr(c)
	if actor == nil {
		utils.Error(c, http.StatusUnauthorized, "administrator identity is unavailable")
		return
	}
	if err := h.service.Cleanup(c.Request.Context(), id, *actor); err != nil {
		utils.Error(c, http.StatusConflict, err.Error())
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw upgrade lab cleaned", nil)
}

func upgradeLabRunID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "invalid upgrade lab run id")
		return 0, false
	}
	return id, true
}
