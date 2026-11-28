package review

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 进程内编排：并发跑单元、等终态回调、确定性聚合。单实例部署；
// 状态真相在 DB，进程重启由 ReconcileOrphaned 收敛。

const (
	defaultUnitConcurrency = 3
	defaultRunTimeout      = 20 * time.Minute
	// defaultUnitTimeout 是单个单元的独立时限：一个卡住的单元只能拖垮自己，
	// 不能吃光整次检查的预算（此前只有 Run 级总超时，一个单元挂住会让其余单元
	// 在总超时处一起被清掉，整单零产物）。
	defaultUnitTimeout = 5 * time.Minute
	unitCancelGrace    = 10 * time.Second
)

type runExecutor struct {
	svc   *Service
	runID store.UUID

	mu        sync.Mutex
	cancelled bool
	current   map[string]store.UUID
	results   map[string]*unitResult
	cancelCtx context.CancelFunc
}

func (e *runExecutor) isCancelled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cancelled
}

func (e *runExecutor) cancel() {
	e.mu.Lock()
	e.cancelled = true
	ids := make([]store.UUID, 0, len(e.current))
	for _, id := range e.current {
		ids = append(ids, id)
	}
	cancel := e.cancelCtx
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	for _, id := range ids {
		e.svc.Agent.CancelRun(id)
	}
}

func (e *runExecutor) setRunning(unitID string, runID store.UUID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.current[unitID] = runID
}

func (e *runExecutor) clearRunning(unitID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.current, unitID)
}

func (e *runExecutor) setResult(unitID string, result *unitResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.results[unitID] = result
}

func (e *runExecutor) result(unitID string) *unitResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.results[unitID]
}

func (s *Service) execute(runID store.UUID) {
	var run store.ReviewRun
	if err := s.DB.Where("id = ?", runID).First(&run).Error; err != nil {
		return
	}
	if run.Status != store.ReviewRunQueued && run.Status != store.ReviewRunRunning {
		return
	}
	timeout := defaultRunTimeout
	if s.Cfg != nil && s.Cfg.ReviewRunTimeout > 0 {
		timeout = s.Cfg.ReviewRunTimeout
	}
	unitTimeout := defaultUnitTimeout
	if s.Cfg != nil && s.Cfg.ReviewUnitTimeout > 0 {
		unitTimeout = s.Cfg.ReviewUnitTimeout
	}
	if unitTimeout > timeout {
		unitTimeout = timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	exec := &runExecutor{
		svc: s, runID: runID, current: map[string]store.UUID{},
		results: map[string]*unitResult{}, cancelCtx: cancel,
	}
	s.executors.Store(runID, exec)
	defer s.executors.Delete(runID)

	now := time.Now().UTC()
	_ = s.DB.Model(&store.ReviewRun{}).Where("id = ? AND status = ?", runID, store.ReviewRunQueued).
		Updates(map[string]any{"status": store.ReviewRunRunning, "started_at": now}).Error

	pc, err := s.loadProjectContext(ctx, run.UserID, run.ProjectID)
	if err != nil {
		s.failRun(context.Background(), runID, "读取项目材料失败："+err.Error())
		return
	}
	model, err := agent.ResolveModelWithEnv(ctx, s.DB, run.UserID, s.Cfg)
	if err != nil {
		s.failRun(context.Background(), runID, "模型配置不可用，请重新配置后再运行检查")
		return
	}

	var units []store.ReviewUnit
	if err := s.DB.Where("review_run_id = ?", runID).Order("seq").Find(&units).Error; err != nil {
		s.failRun(context.Background(), runID, "读取检查单元失败")
		return
	}
	concurrency := defaultUnitConcurrency
	if s.Cfg != nil && s.Cfg.ReviewUnitConcurrency > 0 {
		concurrency = s.Cfg.ReviewUnitConcurrency
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range units {
		unit := units[i]
		if unit.Status != store.ReviewUnitPending {
			continue
		}
		wg.Add(1)
		go func(u store.ReviewUnit) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				s.markUnit(u.ID, store.ReviewUnitCancelled, "检查已取消")
				return
			}
			s.runUnit(ctx, exec, &run, &u, pc, model, unitTimeout)
		}(unit)
	}
	wg.Wait()
	s.finishRun(context.Background(), exec, &run, pc)
}

func (s *Service) runUnit(ctx context.Context, exec *runExecutor, run *store.ReviewRun,
	unit *store.ReviewUnit, pc *projectContext, model *agent.ResolvedModel, unitTimeout time.Duration) {

	if exec.isCancelled() || ctx.Err() != nil {
		s.markUnit(unit.ID, store.ReviewUnitCancelled, "检查已取消")
		return
	}
	started := time.Now().UTC()
	// 只有仍处于 pending 的单元才允许启动：若重启收敛或其他写入方已给出终态，不得复活。
	res := s.DB.Model(&store.ReviewUnit{}).Where("id = ? AND status = ?", unit.ID, store.ReviewUnitPending).
		Updates(map[string]any{"status": store.ReviewUnitRunning, "started_at": started})
	if res.Error != nil {
		slog.Warn("review unit start write failed", "unit_id", unit.ID.String(), "err", res.Error)
		return
	}
	if res.RowsAffected == 0 {
		return
	}

	var files, sections, dims []string
	_ = json.Unmarshal(unit.FilePathsJSON, &files)
	_ = json.Unmarshal(unit.SectionIDsJSON, &sections)
	_ = json.Unmarshal(unit.DimensionsJSON, &dims)
	planned := &plannedUnit{
		Kind: unit.Kind, Label: unit.Label, FilePaths: files, SectionIDs: sections,
		Dimensions: dims, Truncated: contextTruncated(unit.ContextJSON),
	}
	done := make(chan struct{})
	var once sync.Once
	settleStatus, settleFailure := "", ""
	// 报告一旦通过服务端校验，单元目的就达成了：随即停掉该单元的 Agent 循环，
	// 不再让模型继续读文件/寒暄而把迭代预算耗尽。
	var stopMu sync.Mutex
	var stopRun func()
	stopAfterSubmit := func() {
		stopMu.Lock()
		fn := stopRun
		stopMu.Unlock()
		if fn != nil {
			fn()
		}
	}
	spec := &agent.ReviewUnitSpec{
		UnitID:      unit.ID.String(),
		Instruction: buildUnitInstruction(*planned, pc),
		ContextJSON: unit.ContextJSON,
		Files:       files,
		Dimensions:  dims,
		Norms:       pc.norms,
		Reads:       agent.NewReviewUnitReads(),
		Submit: func(submitCtx context.Context, payload []byte, sources []agent.ReviewSource) error {
			result, verr := s.validateUnitReport(submitCtx, pc, planned, payload, sources)
			if verr != nil {
				return verr
			}
			exec.setResult(unit.ID.String(), result)
			stopAfterSubmit()
			return nil
		},
	}
	agentRunID, err := s.Agent.StartReviewUnit(agent.ReviewUnitRequest{
		ConversationID: run.ConversationID, ProjectID: run.ProjectID, UserID: run.UserID,
		Model: model, Content: "请按系统指令检查本单元，并调用 submit_review_unit_report 提交报告。",
		Spec: spec,
		OnSettled: func(status, failure string) {
			settleStatus, settleFailure = status, failure
			once.Do(func() { close(done) })
		},
	})
	if err != nil {
		s.markUnit(unit.ID, store.ReviewUnitFailed, "启动检查单元失败："+err.Error())
		return
	}
	exec.setRunning(unit.ID.String(), agentRunID)
	stopMu.Lock()
	stopRun = func() { s.Agent.CancelRun(agentRunID) }
	stopMu.Unlock()
	if err := s.DB.Model(&store.ReviewUnit{}).Where("id = ?", unit.ID).Update("agent_run_id", agentRunID).Error; err != nil {
		slog.Warn("review unit agent_run_id write failed", "unit_id", unit.ID.String(), "err", err)
	}

	// 每个单元有自己的时限：到点只终止本单元，其余单元继续。
	unitCtx, stopUnit := context.WithTimeout(ctx, unitTimeout)
	defer stopUnit()
	var deadlineHit bool
	select {
	case <-done:
	case <-unitCtx.Done():
		s.Agent.CancelRun(agentRunID)
		select {
		case <-done:
		case <-time.After(unitCancelGrace):
			settleStatus = agent.RunCancelled
		}
		// Run 级超时/用户取消沿用「检查已取消」；只有单元自身到点才单独报超时。
		deadlineHit = ctx.Err() == nil && settleStatus != agent.RunCompleted
	}
	exec.clearRunning(unit.ID.String())

	// 判定标准是「服务端是否收到并通过校验的报告」，不是 Agent 循环是否优雅收尾：
	// 模型提交后继续读文件会撞上迭代上限，此前会把已校验的报告一起丢掉。
	if result := exec.result(unit.ID.String()); result != nil && !exec.isCancelled() {
		findingsJSON, _ := json.Marshal(result.Findings)
		dimsJSON, _ := json.Marshal(result.Dimensions)
		sourcesJSON, _ := json.Marshal(result.SourcesRead)
		now := time.Now().UTC()
		if err := s.DB.Model(&store.ReviewUnit{}).Where("id = ?", unit.ID).Updates(map[string]any{
			"status": store.ReviewUnitSucceeded, "findings_json": findingsJSON,
			"dimension_results_json": dimsJSON, "sources_read_json": sourcesJSON,
			"error_text": "", "completed_at": now,
		}).Error; err != nil {
			slog.Warn("review unit result write failed", "unit_id", unit.ID.String(), "err", err)
		}
		return
	}
	if settleStatus == agent.RunCompleted {
		s.markUnit(unit.ID, store.ReviewUnitFailed, "模型未提交通过服务端校验的报告")
		return
	}
	if deadlineHit {
		s.markUnit(unit.ID, store.ReviewUnitFailed,
			fmt.Sprintf("本单元超过 %s未完成，已终止；其余单元不受影响", humanDuration(unitTimeout)))
		return
	}
	status := store.ReviewUnitFailed
	reason := settleFailure
	if settleStatus == agent.RunCancelled {
		status = store.ReviewUnitCancelled
		reason = "检查已取消"
	}
	if reason == "" {
		reason = "检查未完成，请重试"
	}
	s.markUnit(unit.ID, status, reason)
}

// humanDuration 把单元时限写成用户能读懂的长度（5m0s → 5 分钟）。
func humanDuration(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%.0f 分钟", d.Minutes())
	}
	return fmt.Sprintf("%.0f 秒", d.Seconds())
}

func (s *Service) markUnit(unitID store.UUID, status, reason string) {
	now := time.Now().UTC()
	if err := s.DB.Model(&store.ReviewUnit{}).
		Where("id = ? AND status IN ?", unitID, []string{store.ReviewUnitPending, store.ReviewUnitRunning}).
		Updates(map[string]any{"status": status, "error_text": truncateText(reason, 500), "completed_at": now}).Error; err != nil {
		// 状态写不进去会让单元卡在 running；必须留痕，否则只能看到「未完成」而查不到原因。
		slog.Warn("review unit state write failed", "unit_id", unitID.String(), "status", status, "err", err)
	}
}

func contextTruncated(raw []byte) bool {
	var ctx struct {
		Chapters []struct {
			Truncated bool `json:"truncated"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(raw, &ctx); err != nil {
		return false
	}
	for _, c := range ctx.Chapters {
		if c.Truncated {
			return true
		}
	}
	return false
}

func (s *Service) finishRun(ctx context.Context, exec *runExecutor, run *store.ReviewRun, pc *projectContext) {
	var latest store.ReviewRun
	if err := s.DB.First(&latest, "id = ?", run.ID).Error; err != nil {
		return
	}
	// 终态只写一次：重启收敛、用户取消或其他写入方已收敛的任务不得被本进程覆盖。
	if latest.Status != store.ReviewRunQueued && latest.Status != store.ReviewRunRunning {
		return
	}
	if exec.isCancelled() {
		now := time.Now().UTC()
		if err := s.DB.Model(&store.ReviewUnit{}).
			Where("review_run_id = ? AND status IN ?", run.ID, []string{store.ReviewUnitPending, store.ReviewUnitRunning}).
			Updates(map[string]any{"status": store.ReviewUnitCancelled, "error_text": "检查已取消", "completed_at": now}).Error; err != nil {
			slog.Warn("review units cancel write failed", "run_id", run.ID.String(), "err", err)
		}
		return
	}
	var units []store.ReviewUnit
	if err := s.DB.Where("review_run_id = ?", run.ID).Order("seq").Find(&units).Error; err != nil {
		s.failRun(ctx, run.ID, "读取检查结果失败")
		return
	}
	success, failed := 0, 0
	for i := range units {
		u := &units[i]
		switch u.Status {
		case store.ReviewUnitSucceeded, store.ReviewUnitReused:
			success++
		case store.ReviewUnitFailed, store.ReviewUnitCancelled:
			failed++
		default:
			// 未终态单元一律按失败收敛，绝不把未覆盖展示成通过；原因要能指向可执行动作。
			s.markUnit(u.ID, store.ReviewUnitFailed, "本单元未在时限内完成，未产出结论")
			u.Status = store.ReviewUnitFailed
			u.ErrorText = "本单元未在时限内完成，未产出结论"
			failed++
		}
	}
	if success == 0 {
		reason := fmt.Sprintf("所有检查单元都失败了（%d 个）：%s", failed, summarizeUnitFailures(units))
		s.failRun(ctx, run.ID, reason)
		return
	}
	report, _ := buildReport(run, units, pc)
	payload, err := json.Marshal(report)
	if err != nil {
		s.failRun(ctx, run.ID, "检查报告序列化失败")
		return
	}
	out, err := s.Research.Create(ctx, run.UserID, run.ProjectID, research.CreateInput{
		Kind: store.ResearchArtifactChapterReview, Payload: payload, Origin: "agent", RunID: &run.ID,
	})
	if err != nil {
		slog.Warn("review report persist failed", "run_id", run.ID.String(), "err", err)
		s.failRun(ctx, run.ID, "检查报告保存失败："+err.Error())
		return
	}
	status := store.ReviewRunSucceeded
	errorText := ""
	if failed > 0 {
		status = store.ReviewRunPartial
		errorText = fmt.Sprintf("%d 个检查单元失败，未覆盖的范围已在报告中标注", failed)
	}
	artifactID := store.UUID{UUID: out.ID}
	now := time.Now().UTC()
	res := s.DB.Model(&store.ReviewRun{}).
		Where("id = ? AND status IN ?", run.ID, []string{store.ReviewRunQueued, store.ReviewRunRunning}).
		Updates(map[string]any{
			"status": status, "artifact_id": artifactID, "artifact_revision": out.CurrentRevision,
			"error_text": errorText, "completed_at": now,
		})
	if res.Error != nil {
		slog.Warn("review run finish write failed", "run_id", run.ID.String(), "err", res.Error)
	}
}

// summarizeUnitFailures 把单元失败原因去重后汇总，避免只用「第一个失败」代表全部。
func summarizeUnitFailures(units []store.ReviewUnit) string {
	reasons := make([]string, 0, 3)
	seen := map[string]bool{}
	for i := range units {
		u := &units[i]
		if u.Status != store.ReviewUnitFailed && u.Status != store.ReviewUnitCancelled {
			continue
		}
		reason := strings.TrimSpace(u.ErrorText)
		if reason == "" {
			reason = "未返回失败原因"
		}
		if seen[reason] {
			continue
		}
		seen[reason] = true
		reasons = append(reasons, reason)
		if len(reasons) == 3 {
			break
		}
	}
	if len(reasons) == 0 {
		return "请稍后重试"
	}
	return strings.Join(reasons, "；")
}

// buildReport 纯确定性聚合：维度状态取最坏、findings 合并加前缀、scope 记录单元来源。
func buildReport(run *store.ReviewRun, units []store.ReviewUnit, pc *projectContext) (research.ChapterReview, ScopeSnapshot) {
	var scope ScopeSnapshot
	_ = json.Unmarshal(run.ScopeJSON, &scope)

	// 输入漂移提示：运行期间文件被改过，定位可能已过期。
	if len(scope.FileHashes) > 0 {
		for path, oldHash := range scope.FileHashes {
			content, ok := pc.fileByPath[path]
			if !ok {
				scope.Warnings = append(scope.Warnings, fmt.Sprintf("运行期间文件 %s 被删除，相关定位可能已过期", path))
				continue
			}
			if newHash := hashContent(content); newHash != oldHash {
				scope.Warnings = append(scope.Warnings, "运行期间有文件被修改，部分定位可能已过期")
				break
			}
		}
	}

	rank := map[string]int{"checked": 0, "not_checked": 1, "findings": 2, "insufficient": 3, "failed": 4}
	type agg struct {
		status string
		notes  []string
	}
	aggs := map[string]*agg{}
	for _, key := range scope.Dimensions {
		aggs[key] = &agg{}
	}
	for i := range units {
		u := &units[i]
		var results []research.ReviewDimension
		_ = json.Unmarshal(u.DimensionResultsJSON, &results)
		for _, r := range results {
			a := aggs[r.Key]
			if a == nil {
				a = &agg{}
				aggs[r.Key] = a
			}
			status := r.Status
			if u.Status == store.ReviewUnitFailed || u.Status == store.ReviewUnitCancelled {
				status = "failed"
			}
			if a.status == "" || rank[status] > rank[a.status] {
				a.status = status
			}
			if r.Note != "" {
				a.notes = append(a.notes, u.Label+"："+r.Note)
			}
		}
		if u.Status == store.ReviewUnitFailed {
			if key := unitDimension(u); key != "" {
				a := aggs[key]
				if a == nil {
					a = &agg{}
					aggs[key] = a
				}
				a.status = "failed"
				a.notes = append(a.notes, u.Label+"："+firstNonEmpty(u.ErrorText, "检查失败"))
			}
		}
	}
	dimensions := make([]research.ReviewDimension, 0, len(scope.Dimensions))
	for _, key := range scope.Dimensions {
		a := aggs[key]
		if a == nil || a.status == "" {
			dimensions = append(dimensions, research.ReviewDimension{Key: key, Status: "not_checked", Note: "该维度没有完成的检查单元"})
			continue
		}
		dimensions = append(dimensions, research.ReviewDimension{Key: key, Status: a.status, Note: joinNotes(a.notes)})
	}

	var findings []research.ReviewFinding
	seen := map[string]bool{}
	var sources []research.ReviewSource
	seenSources := map[string]bool{}
	for i := range units {
		u := &units[i]
		if u.Status != store.ReviewUnitSucceeded && u.Status != store.ReviewUnitReused {
			continue
		}
		var fs []research.ReviewFinding
		_ = json.Unmarshal(u.FindingsJSON, &fs)
		for _, f := range fs {
			if f.Fingerprint == "" {
				f.Fingerprint = findingFingerprint(f)
			}
			if seen[f.Fingerprint] {
				continue
			}
			seen[f.Fingerprint] = true
			f.ID = fmt.Sprintf("u%d-%s", u.Seq, f.ID)
			findings = append(findings, f)
		}
		var srcs []research.ReviewSource
		_ = json.Unmarshal(u.SourcesReadJSON, &srcs)
		for _, src := range srcs {
			key := src.Kind + "\x00" + src.Name
			if seenSources[key] {
				continue
			}
			seenSources[key] = true
			sources = append(sources, src)
		}
	}

	scope.Units = make([]ScopeUnit, 0, len(units))
	for i := range units {
		u := &units[i]
		source := "fresh"
		if u.Status == store.ReviewUnitReused {
			source = "reused"
		}
		scope.Units = append(scope.Units, ScopeUnit{
			ID: u.ID.String(), Label: u.Label, Kind: u.Kind, Status: u.Status,
			Source: source, Fingerprint: u.InputFingerprint,
		})
	}

	input := research.ChapterReviewInput{
		PlanStatus: "missing", NormsAvailable: pc.normsOK, NormsNote: pc.normsNote, Norms: pc.norms,
	}
	if pc.plan != nil && pc.plan.Plan != nil {
		artifactID := pc.plan.ArtifactID
		revision := pc.plan.Revision
		input.PlanArtifactID = &artifactID
		input.PlanRevision = &revision
		input.PlanStatus = pc.plan.Status
		for _, sec := range pc.plan.Plan.Sections {
			input.PlanSections = append(input.PlanSections, research.ReviewPlanSection{ID: sec.ID, Title: sec.Title})
		}
	}
	report := research.ChapterReview{
		Input: input,
		Scope: &research.ChapterReviewScope{
			Dimensions: scope.Dimensions, Files: scope.Files, SectionIDs: scope.SectionIDs,
			Incremental: scope.Incremental, Warnings: scope.Warnings, Units: toResearchUnits(scope.Units),
		},
		Dimensions: dimensions, Findings: findings, SourcesRead: sources,
	}
	return report, scope
}

func toResearchUnits(units []ScopeUnit) []research.ReviewScopeUnit {
	out := make([]research.ReviewScopeUnit, 0, len(units))
	for _, u := range units {
		out = append(out, research.ReviewScopeUnit{
			ID: u.ID, Label: u.Label, Kind: u.Kind, Status: u.Status, Source: u.Source, Fingerprint: u.Fingerprint,
		})
	}
	return out
}

func unitDimension(u *store.ReviewUnit) string {
	var dims []string
	_ = json.Unmarshal(u.DimensionsJSON, &dims)
	if len(dims) == 0 {
		return ""
	}
	return dims[0]
}

func joinNotes(notes []string) string {
	out := ""
	for _, note := range notes {
		if out != "" {
			out += "；"
		}
		out += note
	}
	return truncateText(out, 1000)
}

func hashContent(content string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}
