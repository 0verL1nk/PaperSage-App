package review

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 检查任务的创建、查询与取消。执行由 orchestrator 在进程内推进，状态真相在 DB。

// ErrRunNotFound 检查任务不存在或不属于当前用户。
var ErrRunNotFound = errors.New("review run not found")

// ActiveRunError 表示同一项目已有进行中的检查。
type ActiveRunError struct{ RunID string }

func (e *ActiveRunError) Error() string { return "已有正在进行的检查" }

// UnitDTO 是检查单元的执行状态（不含 findings；报告里才有结论）。
type UnitDTO struct {
	ID          string        `json:"id"`
	Seq         int           `json:"seq"`
	Kind        string        `json:"kind"`
	Label       string        `json:"label"`
	FilePaths   []string      `json:"filePaths"`
	SectionIDs  []string      `json:"sectionIds,omitempty"`
	Dimensions  []string      `json:"dimensions"`
	Status      string        `json:"status"`
	Error       string        `json:"error,omitempty"`
	AgentRunID  *string       `json:"agentRunId,omitempty"`
	Reused      bool          `json:"reused"`
	Findings    int           `json:"findings"`
	StartedAt   *dto.JSONTime `json:"startedAt,omitempty"`
	CompletedAt *dto.JSONTime `json:"completedAt,omitempty"`
}

// RunProgress 是页面进度条的数据来源（真实单元状态，不是定时动画）。
type RunProgress struct {
	Total    int      `json:"total"`
	Finished int      `json:"finished"`
	Reused   int      `json:"reused"`
	Failed   int      `json:"failed"`
	Current  []string `json:"current"`
}

// RunDTO 是一次检查的完整快照。
type RunDTO struct {
	ID               string        `json:"id"`
	Status           string        `json:"status"`
	Error            string        `json:"error,omitempty"`
	Scope            ScopeSnapshot `json:"scope"`
	Incremental      bool          `json:"incremental"`
	ModelName        string        `json:"modelName,omitempty"`
	ArtifactID       *string       `json:"artifactId,omitempty"`
	ArtifactRevision *int          `json:"artifactRevision,omitempty"`
	Progress         RunProgress   `json:"progress"`
	Units            []UnitDTO     `json:"units"`
	StartedAt        *dto.JSONTime `json:"startedAt,omitempty"`
	CompletedAt      *dto.JSONTime `json:"completedAt,omitempty"`
	CreatedAt        dto.JSONTime  `json:"createdAt"`
	ElapsedMs        int64         `json:"elapsedMs"`
}

// Create 校验范围、锁定输入并启动检查；重复 clientRequestId 返回同一任务。
func (s *Service) Create(ctx context.Context, userID, projectID store.UUID, req CreateRequest) (*RunDTO, error) {
	if s.Agent == nil || s.Research == nil {
		return nil, errors.New("检查服务未装配")
	}
	if req.ClientRequestID == "" || len(req.ClientRequestID) > 64 {
		return nil, httpapi.BadRequest("缺少有效的请求标识，请刷新页面后重试")
	}
	// 幂等：同一请求重试直接返回已有任务。
	var existing store.ReviewRun
	err := s.DB.WithContext(ctx).Where("project_id = ? AND client_request_id = ?", projectID, req.ClientRequestID).
		First(&existing).Error
	if err == nil {
		return s.Get(ctx, userID, projectID, existing.ID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// 同一项目同时只允许一个进行中的检查（单实例部署，应用层互斥足够）。
	var active store.ReviewRun
	err = s.DB.WithContext(ctx).Where("project_id = ? AND status IN ?", projectID, []string{store.ReviewRunQueued, store.ReviewRunRunning}).
		Order("created_at DESC").First(&active).Error
	if err == nil {
		return nil, &ActiveRunError{RunID: active.ID.String()}
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if s.limiter != nil && !s.limiter.AllowWindow("review:"+userID.String(), 6, time.Hour) {
		return nil, &httpapi.APIError{Code: 429, Info: "检查次数过于频繁，请稍后再试"}
	}

	model, err := agent.ResolveModelWithEnv(ctx, s.DB, userID, s.Cfg)
	if err != nil {
		if errors.Is(err, agent.ErrNoModel) || errors.Is(err, agent.ErrInvalidModel) {
			return nil, httpapi.BadRequest("尚未配置可用的 AI 模型，请先在设置里配置模型后再运行检查")
		}
		return nil, err
	}
	pc, err := s.loadProjectContext(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	planned, scope, err := s.Plan(ctx, pc, model, req)
	if err != nil {
		return nil, err
	}
	if req.Scope.Incremental {
		for i := range planned {
			planned[i].Reused = s.findReusableUnit(ctx, projectID, planned[i].Fingerprint)
		}
	}
	scopeJSON, err := json.Marshal(scope)
	if err != nil {
		return nil, err
	}
	conv := store.AgentConversation{
		ID: store.MustNewUUID(), ProjectID: projectID, UserID: userID,
		Title: "检查任务", Kind: "review",
	}
	run := store.ReviewRun{
		ID: store.MustNewUUID(), ProjectID: projectID, UserID: userID,
		ConversationID: conv.ID, ClientRequestID: req.ClientRequestID,
		Status: store.ReviewRunQueued, ScopeJSON: datatypes.JSON(scopeJSON),
		Incremental:   req.Scope.Incremental,
		ModelProvider: model.Name, ModelName: model.Model,
		PromptVersion: agent.ReviewUnitPromptVersion,
	}
	now := time.Now().UTC()
	units := make([]store.ReviewUnit, 0, len(planned))
	for i := range planned {
		u := planned[i]
		dimsJSON, _ := json.Marshal(u.Dimensions)
		filesJSON, _ := json.Marshal(u.FilePaths)
		sectionsJSON, _ := json.Marshal(u.SectionIDs)
		ru := store.ReviewUnit{
			ID: store.MustNewUUID(), ReviewRunID: run.ID, Seq: i + 1,
			Kind: u.Kind, Label: u.Label,
			FilePathsJSON: datatypes.JSON(filesJSON), SectionIDsJSON: datatypes.JSON(sectionsJSON),
			DimensionsJSON: datatypes.JSON(dimsJSON), ContextJSON: datatypes.JSON(u.ContextJSON),
			InputFingerprint: u.Fingerprint, Status: store.ReviewUnitPending,
			FindingsJSON: datatypes.JSON("[]"), DimensionResultsJSON: datatypes.JSON("[]"),
			SourcesReadJSON: datatypes.JSON("[]"),
		}
		if u.Reused != nil {
			ru.Status = store.ReviewUnitReused
			ru.ReusedFromUnitID = &u.Reused.ID
			ru.FindingsJSON = u.Reused.FindingsJSON
			ru.DimensionResultsJSON = u.Reused.DimensionResultsJSON
			ru.CompletedAt = &now
		}
		units = append(units, ru)
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conv).Error; err != nil {
			return err
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if len(units) > 0 {
			if err := tx.Create(&units).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	go s.execute(run.ID)
	return s.Get(ctx, userID, projectID, run.ID)
}

func (s *Service) findReusableUnit(ctx context.Context, projectID store.UUID, fingerprint string) *store.ReviewUnit {
	var unit store.ReviewUnit
	err := s.DB.WithContext(ctx).Model(&store.ReviewUnit{}).
		Select("review_units.*").
		Joins("JOIN review_runs ON review_runs.id = review_units.review_run_id").
		Where("review_runs.project_id = ? AND review_units.input_fingerprint = ? AND review_units.status = ?",
			projectID, fingerprint, store.ReviewUnitSucceeded).
		Order("review_units.completed_at DESC").First(&unit).Error
	if err != nil {
		return nil
	}
	return &unit
}

// Get 读取任务快照（含真实单元进度）。
func (s *Service) Get(ctx context.Context, userID, projectID, runID store.UUID) (*RunDTO, error) {
	var run store.ReviewRun
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND project_id = ? AND user_id = ?", runID, projectID, userID).
		First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	var units []store.ReviewUnit
	if err := s.DB.WithContext(ctx).Where("review_run_id = ?", run.ID).Order("seq").Find(&units).Error; err != nil {
		return nil, err
	}
	return buildRunDTO(run, units), nil
}

// List 返回项目最近的检查任务（刷新后恢复进行中任务）。
func (s *Service) List(ctx context.Context, userID, projectID store.UUID, limit int) ([]*RunDTO, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	var runs []store.ReviewRun
	if err := s.DB.WithContext(ctx).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Order("created_at DESC").Limit(limit).Find(&runs).Error; err != nil {
		return nil, err
	}
	out := make([]*RunDTO, 0, len(runs))
	for _, run := range runs {
		var units []store.ReviewUnit
		if err := s.DB.WithContext(ctx).Where("review_run_id = ?", run.ID).Order("seq").Find(&units).Error; err != nil {
			return nil, err
		}
		out = append(out, buildRunDTO(run, units))
	}
	return out, nil
}

// Cancel 幂等取消：未启动单元直接终止，运行中单元取消对应 Run；取消不落报告。
func (s *Service) Cancel(ctx context.Context, userID, projectID, runID store.UUID) (*RunDTO, error) {
	var run store.ReviewRun
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND project_id = ? AND user_id = ?", runID, projectID, userID).
		First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}
	switch run.Status {
	case store.ReviewRunSucceeded, store.ReviewRunPartial, store.ReviewRunFailed, store.ReviewRunCancelled:
		return s.Get(ctx, userID, projectID, runID)
	}
	now := time.Now().UTC()
	if err := s.DB.WithContext(ctx).Model(&store.ReviewRun{}).Where("id = ?", run.ID).
		Updates(map[string]any{
			"status": store.ReviewRunCancelled, "cancel_requested_at": now, "completed_at": now,
		}).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Model(&store.ReviewUnit{}).
		Where("review_run_id = ? AND status = ?", run.ID, store.ReviewUnitPending).
		Updates(map[string]any{"status": store.ReviewUnitCancelled, "completed_at": now, "error_text": "检查已取消"}).Error; err != nil {
		return nil, err
	}
	if exec, ok := s.executors.Load(run.ID); ok {
		if e, ok := exec.(*runExecutor); ok {
			e.cancel()
		}
	}
	return s.Get(ctx, userID, projectID, runID)
}

// ReconcileOrphaned 在启动时收敛上一进程遗留的 queued/running 任务。
func (s *Service) ReconcileOrphaned(ctx context.Context) {
	now := time.Now().UTC()
	reason := "服务重启，检查已中断，请重新运行"
	if err := s.DB.WithContext(ctx).Model(&store.ReviewUnit{}).
		Where("status IN ?", []string{store.ReviewUnitPending, store.ReviewUnitRunning}).
		Updates(map[string]any{"status": store.ReviewUnitFailed, "error_text": reason, "completed_at": now}).Error; err != nil {
		return
	}
	_ = s.DB.WithContext(ctx).Model(&store.ReviewRun{}).
		Where("status IN ?", []string{store.ReviewRunQueued, store.ReviewRunRunning}).
		Updates(map[string]any{"status": store.ReviewRunFailed, "error_text": reason, "completed_at": now}).Error
}

func buildRunDTO(run store.ReviewRun, units []store.ReviewUnit) *RunDTO {
	var scope ScopeSnapshot
	_ = json.Unmarshal(run.ScopeJSON, &scope)
	out := &RunDTO{
		ID: run.ID.String(), Status: run.Status, Error: run.ErrorText,
		Scope: scope, Incremental: run.Incremental, ModelName: run.ModelName,
		CreatedAt: dto.JSONTime{Time: run.CreatedAt},
		StartedAt: jsonTimePtr(run.StartedAt), CompletedAt: jsonTimePtr(run.CompletedAt),
		Units: make([]UnitDTO, 0, len(units)),
	}
	if run.ArtifactID != nil {
		id := run.ArtifactID.String()
		out.ArtifactID = &id
	}
	out.ArtifactRevision = run.ArtifactRevision
	if run.StartedAt != nil {
		end := time.Now().UTC()
		if run.CompletedAt != nil {
			end = *run.CompletedAt
		}
		out.ElapsedMs = end.Sub(*run.StartedAt).Milliseconds()
	}
	progress := RunProgress{Total: len(units), Current: []string{}}
	for _, unit := range units {
		var files, sections, dims []string
		_ = json.Unmarshal(unit.FilePathsJSON, &files)
		_ = json.Unmarshal(unit.SectionIDsJSON, &sections)
		_ = json.Unmarshal(unit.DimensionsJSON, &dims)
		var findings []json.RawMessage
		_ = json.Unmarshal(unit.FindingsJSON, &findings)
		ud := UnitDTO{
			ID: unit.ID.String(), Seq: unit.Seq, Kind: unit.Kind, Label: unit.Label,
			FilePaths: files, SectionIDs: sections, Dimensions: dims,
			Status: unit.Status, Error: unit.ErrorText, Findings: len(findings),
			Reused:    unit.Status == store.ReviewUnitReused,
			StartedAt: jsonTimePtr(unit.StartedAt), CompletedAt: jsonTimePtr(unit.CompletedAt),
		}
		if unit.AgentRunID != nil {
			id := unit.AgentRunID.String()
			ud.AgentRunID = &id
		}
		switch unit.Status {
		case store.ReviewUnitSucceeded, store.ReviewUnitFailed, store.ReviewUnitReused, store.ReviewUnitCancelled:
			progress.Finished++
		case store.ReviewUnitRunning:
			progress.Current = append(progress.Current, unit.Label)
		}
		if unit.Status == store.ReviewUnitReused {
			progress.Reused++
		}
		if unit.Status == store.ReviewUnitFailed {
			progress.Failed++
		}
		out.Units = append(out.Units, ud)
	}
	out.Progress = progress
	return out
}

func jsonTimePtr(t *time.Time) *dto.JSONTime {
	if t == nil {
		return nil
	}
	return &dto.JSONTime{Time: *t}
}

// failRun 把任务置为失败并写入可读原因；已有终态（取消/重启收敛）的任务不再覆盖。
func (s *Service) failRun(ctx context.Context, runID store.UUID, reason string) {
	now := time.Now().UTC()
	res := s.DB.WithContext(ctx).Model(&store.ReviewRun{}).
		Where("id = ? AND status IN ?", runID, []string{store.ReviewRunQueued, store.ReviewRunRunning}).
		Updates(map[string]any{"status": store.ReviewRunFailed, "error_text": truncateText(reason, 500), "completed_at": now})
	if res.Error != nil {
		slog.Warn("review run fail write failed", "run_id", runID.String(), "err", res.Error)
	}
}

func truncateText(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
