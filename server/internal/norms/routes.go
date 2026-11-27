package norms

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func pathUUID(c *gin.Context, param, resource string) (store.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound(resource))
		return store.UUID{}, false
	}
	return store.UUID{UUID: id}, true
}

func (s *Service) listPacks(c *gin.Context) {
	items, err := s.ListPacks(c.Request.Context(), auth.CurrentUser(c).ID)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(items, ""))
}

func (s *Service) createPack(c *gin.Context) {
	var req packRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.CreatePack(c.Request.Context(), auth.CurrentUser(c).ID, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(item, ""))
}

func (s *Service) updatePack(c *gin.Context) {
	packID, ok := pathUUID(c, "pack_id", "Norm pack")
	if !ok {
		return
	}
	var req packPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.UpdatePack(c.Request.Context(), auth.CurrentUser(c).ID, packID, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(item, ""))
}

func (s *Service) deletePack(c *gin.Context) {
	packID, ok := pathUUID(c, "pack_id", "Norm pack")
	if !ok {
		return
	}
	if err := s.DeletePack(c.Request.Context(), auth.CurrentUser(c).ID, packID); err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"deleted": true}, ""))
}

func queryInt(c *gin.Context, key string) int {
	raw := c.Query(key)
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func (s *Service) listEntries(c *gin.Context) {
	f := entryFilter{
		Query:    c.Query("q"),
		Category: c.Query("category"),
		Scenario: c.Query("scenario"),
		Tag:      c.Query("tag"),
		PackID:   c.Query("packId"),
		Binding:  c.Query("binding"),
		State:    c.Query("state"),
		Page:     queryInt(c, "page"),
		PageSize: queryInt(c, "pageSize"),
	}
	page, err := s.SearchEntries(c.Request.Context(), auth.CurrentUser(c).ID, f)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(page, ""))
}

func (s *Service) createEntry(c *gin.Context) {
	var req entryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.CreateEntry(c.Request.Context(), auth.CurrentUser(c).ID, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(item, ""))
}

func (s *Service) getEntry(c *gin.Context) {
	entryID, ok := pathUUID(c, "entry_id", "Norm entry")
	if !ok {
		return
	}
	item, err := s.GetEntry(c.Request.Context(), auth.CurrentUser(c).ID, entryID)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(item, ""))
}

func (s *Service) updateEntry(c *gin.Context) {
	entryID, ok := pathUUID(c, "entry_id", "Norm entry")
	if !ok {
		return
	}
	var req entryUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.UpdateEntry(c.Request.Context(), auth.CurrentUser(c).ID, entryID, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(item, ""))
}

func (s *Service) deleteEntry(c *gin.Context) {
	entryID, ok := pathUUID(c, "entry_id", "Norm entry")
	if !ok {
		return
	}
	if err := s.DeleteEntry(c.Request.Context(), auth.CurrentUser(c).ID, entryID); err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"deleted": true}, ""))
}

func (s *Service) listProjectNorms(c *gin.Context) {
	projectID, ok := pathUUID(c, "project_id", "Project")
	if !ok {
		return
	}
	items, err := s.ListProjectNorms(c.Request.Context(), auth.CurrentUser(c).ID, projectID)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(items, ""))
}

func (s *Service) adopt(c *gin.Context) {
	projectID, ok := pathUUID(c, "project_id", "Project")
	if !ok {
		return
	}
	var req adoptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	items, err := s.Adopt(c.Request.Context(), auth.CurrentUser(c).ID, projectID, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(items, ""))
}

func (s *Service) updateAdoption(c *gin.Context) {
	projectID, ok := pathUUID(c, "project_id", "Project")
	if !ok {
		return
	}
	entryID, ok := pathUUID(c, "entry_id", "Norm entry")
	if !ok {
		return
	}
	var req adoptionPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.UpdateAdoption(c.Request.Context(), auth.CurrentUser(c).ID, projectID, entryID, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(item, ""))
}

func (s *Service) unadopt(c *gin.Context) {
	projectID, ok := pathUUID(c, "project_id", "Project")
	if !ok {
		return
	}
	entryID, ok := pathUUID(c, "entry_id", "Norm entry")
	if !ok {
		return
	}
	if err := s.Unadopt(c.Request.Context(), auth.CurrentUser(c).ID, projectID, entryID); err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"deleted": true}, ""))
}

// Mount 挂载规范库与项目采用路由。全部要求登录态。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	n := rg.Group("/norms", authMw)
	n.GET("/packs", s.listPacks)
	n.POST("/packs", s.createPack)
	n.PATCH("/packs/:pack_id", s.updatePack)
	n.DELETE("/packs/:pack_id", s.deletePack)
	n.GET("/entries", s.listEntries)
	n.POST("/entries", s.createEntry)
	n.GET("/entries/:entry_id", s.getEntry)
	n.PUT("/entries/:entry_id", s.updateEntry)
	n.DELETE("/entries/:entry_id", s.deleteEntry)

	p := rg.Group("/projects", authMw)
	p.GET("/:project_id/norms", s.listProjectNorms)
	p.POST("/:project_id/norms/adopt", s.adopt)
	p.PATCH("/:project_id/norms/:entry_id", s.updateAdoption)
	p.DELETE("/:project_id/norms/:entry_id", s.unadopt)
}
