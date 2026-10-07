// Package projects 项目/分组/回收站域。
// 契约来源：backend/api/routes/projects.py + schemas/project.py
package projects

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/importreceipt"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// TemplateCreator 为项目创建默认 main.tex 模板（由 latex 域注入；失败忽略，
// 对齐 Python 的 try/except pass 语义）。nil 表示跳过。
type TemplateCreator func(ctx context.Context, projectID, userID uuid.UUID) error

// Service 项目域逻辑。
type Service struct {
	DB       *gorm.DB
	OnCreate TemplateCreator
}

// NewService wires the projects domain.
func NewService(db *gorm.DB, tmpl TemplateCreator) *Service {
	return &Service{DB: db, OnCreate: tmpl}
}

// ── 响应 DTO（字段名/大小写逐字段对齐 Pydantic）──

type groupResponse struct {
	ID        uuid.UUID    `json:"id"`
	Name      string       `json:"name"`
	OwnerID   uuid.UUID    `json:"owner_id"`
	SortOrder int          `json:"sort_order"`
	CreatedAt dto.JSONTime `json:"created_at"`
}

type projectResponse struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	Description *string        `json:"description"`
	OwnerID     uuid.UUID      `json:"owner_id"`
	GroupID     *uuid.UUID     `json:"group_id"`
	Status      string         `json:"status"`
	Scenario    string         `json:"scenario"`
	IsDeleted   bool           `json:"is_deleted"`
	DeletedAt   *dto.JSONTime  `json:"deleted_at"`
	CreatedAt   dto.JSONTime   `json:"created_at"`
	UpdatedAt   dto.JSONTime   `json:"updated_at"`
	Group       *groupResponse `json:"group"`
}

type trashItemResponse struct {
	Type         string         `json:"type"`
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	DeletedAt    dto.JSONTime   `json:"deleted_at"`
	OriginalData map[string]any `json:"original_data"`
}

func toGroupResponse(g *store.ProjectGroup) *groupResponse {
	if g == nil {
		return nil
	}
	return &groupResponse{
		ID: g.ID.UUID, Name: g.Name, OwnerID: g.OwnerID.UUID,
		SortOrder: g.SortOrder, CreatedAt: dto.JSONTime{Time: g.CreatedAt},
	}
}

func toProjectResponse(p *store.Project, g *store.ProjectGroup) projectResponse {
	resp := projectResponse{
		ID: p.ID.UUID, Name: p.Name, Description: p.Description,
		OwnerID: p.OwnerID.UUID,
		GroupID: uuidPtr(p.GroupID), Status: p.Status,
		IsDeleted: p.DeletedAt.Valid,
		CreatedAt: dto.JSONTime{Time: p.CreatedAt},
		UpdatedAt: dto.JSONTime{Time: p.UpdatedAt},
		Group:     toGroupResponse(g),
		Scenario:  p.Scenario,
	}
	if p.DeletedAt.Valid {
		t := dto.JSONTime{Time: p.DeletedAt.Time}
		resp.DeletedAt = &t
	}
	return resp
}

func uuidPtr(u *store.UUID) *uuid.UUID {
	if u == nil {
		return nil
	}
	id := u.UUID
	return &id
}

// loadGroup 预载所属分组（selectinload 对应）。
func (s *Service) loadGroup(ctx context.Context, p *store.Project) *store.ProjectGroup {
	if p.GroupID == nil {
		return nil
	}
	var g store.ProjectGroup
	if err := s.DB.WithContext(ctx).Where("id = ?", p.GroupID).First(&g).Error; err != nil {
		return nil
	}
	return &g
}

func badReq(info string) *httpapi.APIError { return &httpapi.APIError{Code: 400, Info: info} }

// ── Handlers ──

func (s *Service) list(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID

	q := s.DB.WithContext(ctx).Model(&store.Project{}).Where("owner_id = ?", store.UUID{UUID: uid})
	if gid := c.Query("group_id"); gid != "" {
		g, err := uuid.Parse(gid)
		if err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		q = q.Where("group_id = ?", store.UUID{UUID: g})
	}
	if c.Query("include_deleted") == "true" {
		// 契约保留：include_deleted=true 时含已删行（回收站语义）
		q = q.Unscoped()
	}
	var ps []store.Project
	_ = q.Order("updated_at DESC").Find(&ps).Error

	out := make([]projectResponse, 0, len(ps))
	for i := range ps {
		out = append(out, toProjectResponse(&ps[i], s.loadGroup(ctx, &ps[i])))
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(out, ""))
}

type createRequest struct {
	Name        *string    `json:"name"`
	Description *string    `json:"description"`
	GroupID     *uuid.UUID `json:"group_id"`
	Scenario    string     `json:"scenario"`
}

func (s *Service) create(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID

	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || *req.Name == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	if req.Scenario == "" {
		req.Scenario = "undergraduate_thesis"
	}
	if req.Scenario != "undergraduate_thesis" && req.Scenario != "intro_paper" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	importKey, hasImportKey, keyErr := importreceipt.Parse(c.Request.Header)
	if keyErr != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	if !hasImportKey {
		var existing store.Project
		err := s.DB.WithContext(ctx).
			Where("owner_id = ? AND name = ?", store.UUID{UUID: uid}, *req.Name).
			First(&existing).Error
		if err == nil {
			httpapi.WriteError(c.Writer, badReq("Project with this name already exists"))
			return
		}
	}
	if req.GroupID != nil {
		var g store.ProjectGroup
		if err := s.DB.WithContext(ctx).
			Where("id = ? AND owner_id = ?", store.UUID{UUID: *req.GroupID}, store.UUID{UUID: uid}).
			First(&g).Error; err != nil {
			httpapi.WriteError(c.Writer, badReq("Group not found or does not belong to user"))
			return
		}
	}

	var groupID *store.UUID
	if req.GroupID != nil {
		groupID = &store.UUID{UUID: *req.GroupID}
	}
	p := store.Project{Name: *req.Name, Description: req.Description, OwnerID: store.UUID{UUID: uid}, GroupID: groupID, Status: "active", Scenario: req.Scenario}
	if hasImportKey {
		fingerprint, err := importreceipt.Fingerprint(struct {
			Name        string
			Description *string
			GroupID     *uuid.UUID
			Scenario    string
		}{Name: p.Name, Description: p.Description, GroupID: req.GroupID, Scenario: p.Scenario})
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
			return
		}
		p.ID = store.MustNewUUID()
		var replay bool
		err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			resourceID, wasReplay, err := importreceipt.Claim(tx, p.OwnerID, importKey, "project", fingerprint, p.ID)
			if err != nil {
				return err
			}
			replay = wasReplay
			if replay {
				p.ID = resourceID
				return tx.Where("id = ? AND owner_id = ?", p.ID, p.OwnerID).First(&p).Error
			}
			var duplicate int64
			if err := tx.Model(&store.Project{}).Where("owner_id = ? AND name = ?", p.OwnerID, p.Name).Count(&duplicate).Error; err != nil {
				return err
			}
			if duplicate > 0 {
				return errProjectNameExists
			}
			return tx.Create(&p).Error
		})
		if errors.Is(err, importreceipt.ErrConflict) || errors.Is(err, gorm.ErrRecordNotFound) {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 409, Info: "Import key conflicts with existing content or resource"})
			return
		}
		if errors.Is(err, errProjectNameExists) {
			httpapi.WriteError(c.Writer, badReq("Project with this name already exists"))
			return
		}
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
			return
		}
	} else if err := s.DB.WithContext(ctx).Create(&p).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}

	if s.OnCreate != nil && !hasImportKey {
		_ = s.OnCreate(ctx, p.ID.UUID, uid) // 失败忽略（非关键）
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toProjectResponse(&p, s.loadGroup(ctx, &p)), ""))
}

var errProjectNameExists = errors.New("project name already exists")

func (s *Service) listGroups(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	var gs []store.ProjectGroup
	_ = s.DB.WithContext(ctx).Where("owner_id = ?", store.UUID{UUID: uid}).
		Order("sort_order, name").Find(&gs).Error
	out := make([]groupResponse, 0, len(gs))
	for i := range gs {
		out = append(out, *toGroupResponse(&gs[i]))
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(out, ""))
}

func (s *Service) createGroup(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID

	var req struct {
		Name *string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || *req.Name == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	var gs []store.ProjectGroup
	_ = s.DB.WithContext(ctx).Where("owner_id = ?", store.UUID{UUID: uid}).Find(&gs).Error
	maxOrder := -1
	for _, g := range gs {
		if g.SortOrder > maxOrder {
			maxOrder = g.SortOrder
		}
	}
	g := store.ProjectGroup{Name: *req.Name, OwnerID: store.UUID{UUID: uid}, SortOrder: maxOrder + 1}
	if err := s.DB.WithContext(ctx).Create(&g).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toGroupResponse(&g), ""))
}

func (s *Service) updateGroup(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	gid, err := uuid.Parse(c.Param("group_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	var req struct {
		Name      *string `json:"name"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	var g store.ProjectGroup
	err = s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: gid}, store.UUID{UUID: uid}).First(&g).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Group not found"})
		return
	}
	if req.Name != nil {
		g.Name = *req.Name
	}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}
	_ = s.DB.WithContext(ctx).Save(&g).Error
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toGroupResponse(&g), ""))
}

func (s *Service) deleteGroup(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	gid, err := uuid.Parse(c.Param("group_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	var g store.ProjectGroup
	err = s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: gid}, store.UUID{UUID: uid}).First(&g).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Group not found"})
		return
	}
	// 组内项目移到未分组
	_ = s.DB.WithContext(ctx).Model(&store.Project{}).
		Where("group_id = ? AND owner_id = ?", store.UUID{UUID: gid}, store.UUID{UUID: uid}).
		Update("group_id", nil).Error
	_ = s.DB.WithContext(ctx).Delete(&g).Error

	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"id": gid.String(), "deleted": true}, ""))
}

func (s *Service) listTrash(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	var ps []store.Project
	_ = s.DB.WithContext(ctx).
		Unscoped().Where("owner_id = ? AND deleted_at IS NOT NULL", store.UUID{UUID: uid}).Find(&ps).Error

	out := make([]trashItemResponse, 0, len(ps))
	for i := range ps {
		p := &ps[i]
		deleted := dto.JSONTime{Time: p.DeletedAt.Time}
		out = append(out, trashItemResponse{
			Type: "project", ID: p.ID.UUID, Name: p.Name, DeletedAt: deleted,
			OriginalData: gin.H{"description": p.Description, "status": p.Status},
		})
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(out, ""))
}

func (s *Service) restoreProject(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	res := s.DB.WithContext(ctx).Model(&store.Project{}).
		Unscoped().Where("id = ? AND owner_id = ? AND deleted_at IS NOT NULL", store.UUID{UUID: pid}, store.UUID{UUID: uid}).
		Update("deleted_at", nil)
	if res.Error != nil || res.RowsAffected == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Project not found in trash"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"id": pid.String(), "restored": true}, ""))
}

func (s *Service) purgeProject(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	res := s.DB.WithContext(ctx).
		Unscoped().Where("id = ? AND owner_id = ? AND deleted_at IS NOT NULL", store.UUID{UUID: pid}, store.UUID{UUID: uid}).
		Delete(&store.Project{})
	if res.Error != nil || res.RowsAffected == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Project not found in trash"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"id": pid.String(), "permanently_deleted": true}, ""))
}

func (s *Service) getProject(c *gin.Context) {
	p, ok := s.loadOwnedProject(c, false)
	if !ok {
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toProjectResponse(p, s.loadGroup(c.Request.Context(), p)), ""))
}

type updateRequest struct {
	Name        *string    `json:"name"`
	Description *string    `json:"description"`
	GroupID     *uuid.UUID `json:"group_id"`
	Status      *string    `json:"status"`
}

func (s *Service) updateProject(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	p, ok := s.loadOwnedProject(c, false)
	if !ok {
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	if req.Name != nil && *req.Name != p.Name {
		var existing store.Project
		err := s.DB.WithContext(ctx).
			Where("owner_id = ? AND name = ? AND id != ?",
				store.UUID{UUID: uid}, *req.Name, p.ID, false).First(&existing).Error
		if err == nil {
			httpapi.WriteError(c.Writer, badReq("Project with this name already exists"))
			return
		}
	}
	if req.GroupID != nil {
		// Python: `if project_data.group_id:` — 全零 UUID 亦为 truthy，同样走存在性检查
		var g store.ProjectGroup
		err := s.DB.WithContext(ctx).
			Where("id = ? AND owner_id = ?", store.UUID{UUID: *req.GroupID}, store.UUID{UUID: uid}).
			First(&g).Error
		if err != nil {
			httpapi.WriteError(c.Writer, badReq("Group not found or does not belong to user"))
			return
		}
		p.GroupID = &store.UUID{UUID: *req.GroupID}
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = req.Description
	}
	if req.Status != nil {
		p.Status = *req.Status
	}
	_ = s.DB.WithContext(ctx).Save(p).Error
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toProjectResponse(p, s.loadGroup(ctx, p)), ""))
}

func (s *Service) deleteProject(c *gin.Context) {
	ctx := c.Request.Context()
	p, ok := s.loadOwnedProject(c, true)
	if !ok {
		return
	}
	_ = s.DB.WithContext(ctx).Delete(&store.Project{}, "id = ?", p.ID).Error
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"id": p.ID.String(), "deleted": true}, ""))
}

// loadOwnedProject 按 id+owner 查项目；requireActive=true 时仅限未软删
// （对齐 Python delete 的 `not Project.is_deleted` 条件）。
func (s *Service) loadOwnedProject(c *gin.Context, requireActive bool) (*store.Project, bool) {
	uid := auth.CurrentUser(c).ID.UUID
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return nil, false
	}
	// requireActive=true：仅未删（gorm.DeletedAt 自动过滤）
	// requireActive=false：含已删行（详情/更新可在回收站态继续操作）
	q := s.DB.WithContext(c.Request.Context()).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: pid}, store.UUID{UUID: uid})
	if !requireActive {
		q = q.Unscoped()
	}
	var p store.Project
	if err := q.First(&p).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Project not found"})
		return nil, false
	}
	return &p, true
}

// Mount 注册项目域路由（对应 FastAPI prefix="/projects"）。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	g := rg.Group("/projects", authMw)
	// 组级路由挂 "/"（FastAPI prefix+"/" 形态）；无尾斜杠请求由 gin
	// RedirectTrailingSlash 307 处理，与 FastAPI redirect_slashes 行为同构
	g.GET("/", s.list)
	g.POST("/", s.create)
	g.GET("/groups", s.listGroups)
	g.POST("/groups", s.createGroup)
	g.PUT("/groups/:group_id", s.updateGroup)
	g.DELETE("/groups/:group_id", s.deleteGroup)
	g.GET("/trash", s.listTrash)
	g.POST("/trash/project/:project_id/restore", s.restoreProject)
	g.DELETE("/trash/project/:project_id", s.purgeProject)
	g.GET("/:project_id", s.getProject)
	g.PUT("/:project_id", s.updateProject)
	g.DELETE("/:project_id", s.deleteProject)
}
