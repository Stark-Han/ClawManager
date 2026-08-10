package handlers

import (
	"net/http"
	"strconv"

	"clawreef/internal/services"
	"clawreef/internal/utils"

	"github.com/gin-gonic/gin"
)

// OpenClawConfigHandler handles OpenClaw config center APIs.
type OpenClawConfigHandler struct {
	service services.OpenClawConfigService
}

// NewOpenClawConfigHandler creates a new OpenClaw config handler.
func NewOpenClawConfigHandler(service services.OpenClawConfigService) *OpenClawConfigHandler {
	return &OpenClawConfigHandler{service: service}
}

func (h *OpenClawConfigHandler) ListResources(c *gin.Context) {
	userID, _ := c.Get("userID")
	resourceType := c.Query("resource_type")

	items, err := h.service.ListResources(userID.(int), resourceType)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config resources retrieved successfully", items)
}

func (h *OpenClawConfigHandler) GetResource(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid resource ID")
		return
	}

	item, err := h.service.GetResource(userID.(int), id)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config resource retrieved successfully", item)
}

func (h *OpenClawConfigHandler) CreateResource(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req services.UpsertOpenClawConfigResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	item, err := h.service.CreateResource(userID.(int), req)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "OpenClaw config resource created successfully", item)
}

func (h *OpenClawConfigHandler) UpdateResource(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid resource ID")
		return
	}

	var req services.UpsertOpenClawConfigResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	item, err := h.service.UpdateResource(userID.(int), id, req)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config resource updated successfully", item)
}

func (h *OpenClawConfigHandler) DeleteResource(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid resource ID")
		return
	}

	if err := h.service.DeleteResource(userID.(int), id); err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config resource deleted successfully", nil)
}

func (h *OpenClawConfigHandler) CloneResource(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid resource ID")
		return
	}

	item, err := h.service.CloneResource(userID.(int), id)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "OpenClaw config resource cloned successfully", item)
}

func (h *OpenClawConfigHandler) ValidateResource(c *gin.Context) {
	var req services.UpsertOpenClawConfigResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	if err := h.service.ValidateResource(req); err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config resource validated successfully", gin.H{"valid": true})
}

func (h *OpenClawConfigHandler) ListBundles(c *gin.Context) {
	userID, _ := c.Get("userID")
	items, err := h.service.ListBundles(userID.(int))
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config bundles retrieved successfully", items)
}

func (h *OpenClawConfigHandler) GetBundle(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid bundle ID")
		return
	}

	item, err := h.service.GetBundle(userID.(int), id)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config bundle retrieved successfully", item)
}

func (h *OpenClawConfigHandler) CreateBundle(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req services.UpsertOpenClawConfigBundleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	item, err := h.service.CreateBundle(userID.(int), req)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "OpenClaw config bundle created successfully", item)
}

func (h *OpenClawConfigHandler) UpdateBundle(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid bundle ID")
		return
	}

	var req services.UpsertOpenClawConfigBundleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	item, err := h.service.UpdateBundle(userID.(int), id, req)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config bundle updated successfully", item)
}

func (h *OpenClawConfigHandler) DeleteBundle(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid bundle ID")
		return
	}

	if err := h.service.DeleteBundle(userID.(int), id); err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config bundle deleted successfully", nil)
}

func (h *OpenClawConfigHandler) CloneBundle(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid bundle ID")
		return
	}

	item, err := h.service.CloneBundle(userID.(int), id)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "OpenClaw config bundle cloned successfully", item)
}

func (h *OpenClawConfigHandler) CompilePreview(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req services.OpenClawConfigPlan
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	result, err := h.service.CompilePreview(userID.(int), req)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw config preview compiled successfully", result)
}

func (h *OpenClawConfigHandler) ListSnapshots(c *gin.Context) {
	userID, _ := c.Get("userID")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	items, err := h.service.ListSnapshots(userID.(int), limit)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw injection snapshots retrieved successfully", items)
}

func (h *OpenClawConfigHandler) GetSnapshot(c *gin.Context) {
	userID, _ := c.Get("userID")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid snapshot ID")
		return
	}

	item, err := h.service.GetSnapshot(userID.(int), id)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "OpenClaw injection snapshot retrieved successfully", item)
}
