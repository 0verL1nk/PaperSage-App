package latex

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 契约来源：backend/services/latex/compile_queue.py

// Queue 异步编译队列（信号量并发控制 + DB 持久化 + 启动恢复）。
type Queue struct {
	DB      *gorm.DB
	Files   *FilesService
	Compile func(ctx context.Context, projectID string, files []ProjectFile, rootFile, compiler, syncType string, syncState *string) (*CompileResult, error)
	Clsi    ClsiClient
	Cfg     *config.Settings

	sem    chan struct{}
	mu     sync.Mutex
	cancel map[uuid.UUID]context.CancelFunc
	wg     sync.WaitGroup
}

// NewQueue wires the compile queue.
func NewQueue(db *gorm.DB, files *FilesService, svc *Service, cfg *config.Settings) *Queue {
	q := &Queue{
		DB: db, Files: files, Clsi: svc.Client, Cfg: cfg,
		sem:    make(chan struct{}, cfg.MaxConcurrentCompiles),
		cancel: make(map[uuid.UUID]context.CancelFunc),
	}
	q.Compile = svc.CompileProject
	return q
}

// Submit 建档 pending 并调度后台执行。
func (q *Queue) Submit(ctx context.Context, projectID, userID uuid.UUID, rootFile, compiler string) (*store.CompileJob, error) {
	job := store.CompileJob{
		ProjectID: store.UUID{UUID: projectID},
		UserID:    store.UUID{UUID: userID},
		Status:    string(store.JobPending),
		RootFile:  rootFile,
		Compiler:  compiler,
	}
	if err := q.DB.WithContext(ctx).Create(&job).Error; err != nil {
		return nil, err
	}
	q.schedule(job.ID.UUID)
	return &job, nil
}

// CreateBrowserJob creates a persisted job without scheduling the CLSI queue.
// The browser owns compilation and submits one terminal result afterward.
func (q *Queue) CreateBrowserJob(ctx context.Context, projectID, userID uuid.UUID, rootFile, compiler string) (*store.CompileJob, error) {
	job := store.CompileJob{
		ProjectID: store.UUID{UUID: projectID},
		UserID:    store.UUID{UUID: userID},
		Status:    string(store.JobRunning),
		RootFile:  rootFile,
		Compiler:  compiler,
	}
	if err := q.DB.WithContext(ctx).Create(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// CompleteBrowserJob records a browser-produced artifact after its object has
// been stored. Repeating the same completion for a successful job is safe.
func (q *Queue) CompleteBrowserJob(ctx context.Context, jobID, projectID, userID uuid.UUID, buildID, logs string) error {
	result := q.DB.WithContext(ctx).Model(&store.CompileJob{}).
		Where("id = ? AND project_id = ? AND user_id = ? AND status = ?",
			store.UUID{UUID: jobID}, store.UUID{UUID: projectID}, store.UUID{UUID: userID}, string(store.JobRunning)).
		Updates(map[string]any{
			"status": string(store.JobSuccess), "build_id": buildID,
			"error": nil, "logs": logs,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var job store.CompileJob
	if err := q.DB.WithContext(ctx).
		Where("id = ? AND project_id = ? AND user_id = ?",
			store.UUID{UUID: jobID}, store.UUID{UUID: projectID}, store.UUID{UUID: userID}).
		First(&job).Error; err != nil {
		return err
	}
	if job.Status == string(store.JobSuccess) && job.BuildID != nil && *job.BuildID == buildID {
		return nil
	}
	return fmt.Errorf("compile job is no longer running")
}

// FailBrowserJob stores full browser diagnostics and makes a failed browser
// compilation visible in existing history/status APIs.
func (q *Queue) FailBrowserJob(ctx context.Context, jobID, projectID, userID uuid.UUID, message, logs string) error {
	if len(message) > maxLogSize {
		message = message[:maxLogSize]
	}
	if len(logs) > maxBrowserLogBytes {
		logs = logs[:maxBrowserLogBytes]
	}
	result := q.DB.WithContext(ctx).Model(&store.CompileJob{}).
		Where("id = ? AND project_id = ? AND user_id = ? AND status = ?",
			store.UUID{UUID: jobID}, store.UUID{UUID: projectID}, store.UUID{UUID: userID}, string(store.JobRunning)).
		Updates(map[string]any{"status": string(store.JobFailed), "error": message, "logs": logs})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var job store.CompileJob
	if err := q.DB.WithContext(ctx).
		Where("id = ? AND project_id = ? AND user_id = ?",
			store.UUID{UUID: jobID}, store.UUID{UUID: projectID}, store.UUID{UUID: userID}).
		First(&job).Error; err != nil {
		return err
	}
	if job.Status == string(store.JobFailed) && job.Error != nil && *job.Error == message {
		return nil
	}
	return fmt.Errorf("compile job is no longer running")
}

func (q *Queue) schedule(jobID uuid.UUID) {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		q.Execute(jobID)
	}()
}

// Execute 执行单个编译任务（信号量 + 超时 + 冲突重试 + 失败清理）。
// 导出以便启动恢复与测试直接驱动。
func (q *Queue) Execute(jobID uuid.UUID) {
	ctx, cancel := context.WithCancel(context.Background())
	q.mu.Lock()
	q.cancel[jobID] = cancel
	q.mu.Unlock()
	defer func() {
		cancel()
		q.mu.Lock()
		delete(q.cancel, jobID)
		q.mu.Unlock()
	}()

	select {
	case q.sem <- struct{}{}:
		defer func() { <-q.sem }()
	case <-ctx.Done():
		q.markStatus(jobID, string(store.JobCancelled), nil)
		return
	}

	var job store.CompileJob
	if err := q.DB.WithContext(ctx).Where("id = ?", store.UUID{UUID: jobID}).First(&job).Error; err != nil {
		return
	}
	if job.Status != string(store.JobPending) {
		return
	}
	started := q.DB.WithContext(ctx).Model(&store.CompileJob{}).
		Where("id = ? AND status = ?", job.ID, string(store.JobPending)).
		Update("status", string(store.JobRunning))
	if started.Error != nil || started.RowsAffected != 1 {
		return
	}
	job.Status = string(store.JobRunning)

	q.runCompile(ctx, &job)
}

func (q *Queue) runCompile(ctx context.Context, job *store.CompileJob) {
	projectIDStr := hexNoDash(job.ProjectID.UUID)

	defer func() {
		_ = q.DB.WithContext(context.Background()).Model(&store.CompileJob{}).
			Where("id = ? AND status = ?", job.ID, string(store.JobRunning)).
			Updates(map[string]any{
				"status": job.Status, "build_id": job.BuildID, "error": job.Error,
				"logs": job.Logs, "sync_state": job.SyncState,
			}).Error
	}()

	files := q.loadProjectFiles(context.Background(), job.ProjectID.UUID)

	syncType, syncStateVal, currentHashes := ComputeSyncParams(files, job.SyncState, q.Cfg.CompileIncrementalSyncEnabled)
	filesToSend := files
	if syncType == "incremental" {
		filesToSend = FilterFilesForIncremental(files, job.SyncState)
	}

	timeout := time.Duration(q.Cfg.LatexCompileTimeout) * time.Second
	tctx, tcancel := context.WithTimeout(ctx, timeout)
	defer tcancel()

	result, err := q.Compile(tctx, projectIDStr, filesToSend, job.RootFile, job.Compiler, syncType, syncStateVal)
	if err != nil {
		job.Status = string(store.JobFailed)
		e := err.Error()
		job.Error = &e
		q.cleanupClsi(projectIDStr)
		return
	}
	if result.Status == StatusConflict {
		slog.Info("conflict, retrying full sync", "job", job.ID.UUID)
		fctx, fcancel := context.WithTimeout(ctx, timeout)
		defer fcancel()
		result, err = q.Compile(fctx, projectIDStr, files, job.RootFile, job.Compiler, "full", nil)
		if err != nil {
			job.Status = string(store.JobFailed)
			e := err.Error()
			job.Error = &e
			q.cleanupClsi(projectIDStr)
			return
		}
	}

	job.BuildID = result.BuildID
	job.Logs = result.Logs
	job.Error = result.Error
	if result.Status == StatusSuccess {
		job.Status = string(store.JobSuccess)
		if b, err := json.Marshal(currentHashes); err == nil {
			job.SyncState = datatypes.JSON(b)
		}
	} else {
		job.Status = string(store.JobFailed)
	}
}

func (q *Queue) markStatus(jobID uuid.UUID, status string, errPtr *string) {
	updates := map[string]any{"status": status}
	if errPtr != nil {
		updates["error"] = *errPtr
	}
	_ = q.DB.WithContext(context.Background()).Model(&store.CompileJob{}).
		Where("id = ?", store.UUID{UUID: jobID}).Updates(updates).Error
}

// loadProjectFiles 文本并行载入 + 二进制预签名 URL。
func (q *Queue) loadProjectFiles(ctx context.Context, projectID uuid.UUID) []ProjectFile {
	pairs := q.Files.LoadTextContents(ctx, projectID, q.Cfg.S3ParallelFetchConcurrency)
	files := make([]ProjectFile, 0, len(pairs))
	for _, p := range pairs {
		c := p[1]
		files = append(files, ProjectFile{Path: p[0], Content: &c})
	}
	ttl := time.Duration(q.Cfg.S3PresignedURLTTLSeconds) * time.Second
	for _, bk := range q.Files.GetBinaryFileKeys(ctx, projectID) {
		url, err := q.Files.Storage.PresignGet(ctx, bk[1], ttl)
		if err != nil {
			continue
		}
		files = append(files, ProjectFile{Path: bk[0], URL: strPtr(url), IsBinary: true})
	}
	return files
}

func (q *Queue) cleanupClsi(projectIDStr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = q.Clsi.StopCompile(ctx, projectIDStr)
	_ = q.Clsi.ClearCache(ctx, projectIDStr)
}

// ComputeSyncParams 判定增量/全量并生成当前内容哈希快照（纯函数，对齐 _compute_sync_params）。
func ComputeSyncParams(files []ProjectFile, previous datatypes.JSON, incrementalEnabled bool) (string, *string, map[string]string) {
	current := map[string]string{}
	for _, f := range files {
		if f.Content != nil {
			sum := md5.Sum([]byte(*f.Content))
			current[f.Path] = hex.EncodeToString(sum[:])
		}
	}
	if !incrementalEnabled || len(previous) == 0 {
		return "full", nil, current
	}
	var prev map[string]string
	if err := json.Unmarshal(previous, &prev); err != nil || prev == nil {
		return "full", nil, current
	}
	if !sameKeySet(current, prev) {
		return "full", nil, current
	}
	if mapsEqual(current, prev) {
		return "full", nil, current
	}
	parts := make([]string, 0, len(prev))
	for _, k := range sortedKeys(prev) {
		parts = append(parts, fmt.Sprintf("%s:%s", k, prev[k]))
	}
	sum := md5.Sum([]byte(joinColon(parts)))
	v := hex.EncodeToString(sum[:])
	return "incremental", &v, current
}

// FilterFilesForIncremental 增量时仅发送变更文本 + 全部二进制（纯函数）。
func FilterFilesForIncremental(files []ProjectFile, previous datatypes.JSON) []ProjectFile {
	if len(previous) == 0 {
		return files
	}
	var prev map[string]string
	if err := json.Unmarshal(previous, &prev); err != nil {
		return files
	}
	out := []ProjectFile{}
	for _, f := range files {
		if f.IsBinary {
			out = append(out, f)
			continue
		}
		if f.Content != nil {
			sum := md5.Sum([]byte(*f.Content))
			if prev[f.Path] != hex.EncodeToString(sum[:]) {
				out = append(out, f)
			}
		}
	}
	return out
}

// GetStatus 按 id+owner 查任务。
func (q *Queue) GetStatus(ctx context.Context, jobID, userID uuid.UUID) *store.CompileJob {
	var job store.CompileJob
	err := q.DB.WithContext(ctx).
		Where("id = ? AND user_id = ?", store.UUID{UUID: jobID}, store.UUID{UUID: userID}).
		First(&job).Error
	if err != nil {
		return nil
	}
	return &job
}

// Cancel 取消任务（终态直接返回；活跃任务取消上下文）。
func (q *Queue) Cancel(ctx context.Context, jobID, userID uuid.UUID) (*store.CompileJob, error) {
	var job store.CompileJob
	err := q.DB.WithContext(ctx).
		Where("id = ? AND user_id = ?", store.UUID{UUID: jobID}, store.UUID{UUID: userID}).
		First(&job).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if jobStatus(job.Status).IsTerminal() {
		return &job, nil
	}
	result := q.DB.WithContext(ctx).Model(&store.CompileJob{}).
		Where("id = ? AND user_id = ? AND status IN ?", store.UUID{UUID: jobID}, store.UUID{UUID: userID},
			[]string{string(store.JobPending), string(store.JobRunning)}).
		Update("status", string(store.JobCancelled))
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		var current store.CompileJob
		if err := q.DB.WithContext(ctx).
			Where("id = ? AND user_id = ?", store.UUID{UUID: jobID}, store.UUID{UUID: userID}).
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, err
		}
		return &current, nil
	}
	job.Status = string(store.JobCancelled)
	q.mu.Lock()
	cancel := q.cancel[jobID]
	q.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return &job, nil
}

// GetHistory 按创建时间倒序取最近 N 条 + 总数。
func (q *Queue) GetHistory(ctx context.Context, projectID, userID uuid.UUID) ([]store.CompileJob, int64) {
	base := q.DB.WithContext(ctx).Model(&store.CompileJob{}).
		Where("project_id = ? AND user_id = ?", store.UUID{UUID: projectID}, store.UUID{UUID: userID})
	var total int64
	_ = base.Count(&total).Error
	var items []store.CompileJob
	_ = base.Order("created_at DESC").Limit(q.Cfg.CompileHistoryLimit).Find(&items).Error
	return items, total
}

// RecoverOnStartup 恢复 pending、标记孤儿 running 为 failed。
func (q *Queue) RecoverOnStartup(ctx context.Context) {
	var pending []store.CompileJob
	_ = q.DB.WithContext(ctx).Where("status = ?", string(store.JobPending)).Find(&pending).Error
	for _, job := range pending {
		q.schedule(job.ID.UUID)
		slog.Info("recovered pending compile job", "job", job.ID.UUID)
	}
	_ = q.DB.WithContext(ctx).Model(&store.CompileJob{}).
		Where("status = ?", string(store.JobRunning)).
		Updates(map[string]any{"status": string(store.JobFailed), "error": "Process restarted during compilation"}).Error
}

// CleanupOldJobs 将超过 max age 的终态任务标记 cancelled。
func (q *Queue) CleanupOldJobs(ctx context.Context) int64 {
	cutoff := time.Now().UTC().Add(-time.Duration(q.Cfg.CompileJobMaxAgeHours) * time.Hour)
	res := q.DB.WithContext(ctx).Model(&store.CompileJob{}).
		Where("status IN ? AND updated_at < ?",
			[]string{string(store.JobSuccess), string(store.JobFailed), string(store.JobTimeout), string(store.JobCancelled)},
			cutoff).
		Update("status", string(store.JobCancelled))
	return res.RowsAffected
}

// Wait 等待全部活跃任务结束（测试与优雅停机用）。
func (q *Queue) Wait() { q.wg.Wait() }

func hexNoDash(u uuid.UUID) string { return hex.EncodeToString(u[:]) }

func sameKeySet(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func mapsEqual(a, b map[string]string) bool {
	if !sameKeySet(a, b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func joinColon(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ":"
		}
		out += p
	}
	return out
}
