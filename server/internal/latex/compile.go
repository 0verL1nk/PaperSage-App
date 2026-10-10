package latex

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const maxBrowserPDFBytes = 100 * 1024 * 1024
const maxBrowserLogBytes = 2 * 1024 * 1024

func browserArtifactKey(projectID, buildID string) string {
	return "projects/" + projectID + "/latex/builds/" + buildID + "/output.pdf"
}

// 契约来源：backend/api/routes/latex.py + schemas/{latex,compile_job}.py

// ValidCompilers 对应 VALID_COMPILERS。
var ValidCompilers = map[string]bool{"pdflatex": true, "xelatex": true, "lualatex": true}

var rangeRe = regexp.MustCompile(`^bytes=(\d*)-(\d*)$`)

func writeErr(c *gin.Context, err error) {
	if se, ok := err.(*ServiceError); ok {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: se.Code, Info: se.Msg})
		return
	}
	httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
}

// MountCompile 注册编译相关路由（FastAPI prefix="/projects/{id}/latex"）。
func MountCompile(rg *gin.RouterGroup, authMw gin.HandlerFunc, svc *Service, queue *Queue) {
	g := rg.Group("/projects/:project_id/latex", authMw)

	verify := func(c *gin.Context) (uuid.UUID, string, bool) {
		pid, err := uuid.Parse(c.Param("project_id"))
		if err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return uuid.Nil, "", false
		}
		if err := svc.VerifyProjectAccess(c.Request.Context(), queue.DB, pid, auth.CurrentUser(c).ID.UUID); err != nil {
			writeErr(c, err)
			return uuid.Nil, "", false
		}
		return pid, hexNoDash(pid), true
	}

	// POST /compile remains as a clear migration error for older clients. New
	// compilation jobs must be created by the browser endpoint below; this route
	// must never enqueue work on the legacy CLSI queue.
	g.POST("/compile", func(c *gin.Context) {
		_, _, ok := verify(c)
		if !ok {
			return
		}
		httpapi.WriteError(c.Writer, &httpapi.APIError{
			Code: http.StatusGone, Info: "Server-side LaTeX compilation is disabled. Start compilation from the project editor to use the browser engine.",
		})
	})

	// Browser compilation persists a job without enqueueing the legacy CLSI queue.
	g.POST("/compile/browser", func(c *gin.Context) {
		pid, _, ok := verify(c)
		if !ok {
			return
		}
		var req struct {
			RootFile string `json:"root_file"`
			Compiler string `json:"compiler"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		if req.RootFile == "" {
			req.RootFile = "main.tex"
		}
		if req.Compiler == "" {
			req.Compiler = "pdflatex"
		}
		if req.Compiler != "pdflatex" && req.Compiler != "xelatex" {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "Browser compilation supports only pdflatex and xelatex"})
			return
		}
		job, err := queue.CreateBrowserJob(c.Request.Context(), pid, auth.CurrentUser(c).ID.UUID, req.RootFile, req.Compiler)
		if err != nil {
			writeErr(c, err)
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusAccepted, httpapi.Success(gin.H{
			"job_id": job.ID.UUID, "status": job.Status,
		}, ""))
	})

	// POST /compile/:job_id/browser-result → upload PDF to S3/R2 and finish once.
	g.POST("/compile/:job_id/browser-result", func(c *gin.Context) {
		pid, projectID, ok := verify(c)
		if !ok {
			return
		}
		jobID, err := uuid.Parse(c.Param("job_id"))
		if err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		job := queue.GetStatus(c.Request.Context(), jobID, auth.CurrentUser(c).ID.UUID)
		if job == nil || job.ProjectID.UUID != pid {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Compile job not found"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBrowserPDFBytes+maxBrowserLogBytes+1024*1024)
		if err := c.Request.ParseMultipartForm(16 * 1024 * 1024); err != nil {
			if c.Request.MultipartForm != nil {
				_ = c.Request.MultipartForm.RemoveAll()
			}
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "Browser result upload is invalid or too large"})
			return
		}
		defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
		pdfFile, _, err := c.Request.FormFile("pdf")
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "Browser result is missing its PDF"})
			return
		}
		defer func() { _ = pdfFile.Close() }()
		pdf, err := io.ReadAll(io.LimitReader(pdfFile, maxBrowserPDFBytes+1))
		if err != nil || !isStructurallyValidBrowserPDF(pdf) {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "Browser result must be a valid PDF under 100 MiB"})
			return
		}
		logs := c.Request.FormValue("logs")
		if len(logs) > maxBrowserLogBytes {
			logs = logs[:maxBrowserLogBytes]
		}
		sum := sha256.Sum256(pdf)
		buildID := "browser-" + strings.ReplaceAll(jobID.String(), "-", "") + "-" + hex.EncodeToString(sum[:16])
		objectKey := browserArtifactKey(projectID, buildID)
		if err := queue.Files.Storage.Put(c.Request.Context(), objectKey, pdf); err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 503, Info: "Unable to save the browser PDF to object storage; retry the upload"})
			return
		}
		if err := queue.CompleteBrowserJob(c.Request.Context(), jobID, pid, auth.CurrentUser(c).ID.UUID, buildID, logs); err != nil {
			// A lost database response can happen after the status update committed.
			// Re-read before cleanup so a retry cannot delete the PDF of a successful job.
			latest := queue.GetStatus(c.Request.Context(), jobID, auth.CurrentUser(c).ID.UUID)
			if latest != nil && latest.ProjectID.UUID == pid && latest.Status == string(store.JobSuccess) && latest.BuildID != nil && *latest.BuildID == buildID {
				_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
					"job_id": jobID, "status": string(store.JobSuccess), "build_id": buildID,
				}, ""))
				return
			}
			if latest != nil && latest.Status != string(store.JobRunning) {
				_ = queue.Files.Storage.Delete(c.Request.Context(), objectKey)
				httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 409, Info: "Compile job is already complete or cancelled"})
				return
			}
			// The job is still running, or status could not be read. Keep the
			// deterministic object key so an upload retry is safe after an
			// ambiguous database result.
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 503, Info: "PDF was uploaded but compile status could not be confirmed; retry the same result"})
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
			"job_id": jobID, "status": string(store.JobSuccess), "build_id": buildID,
		}, ""))
	})

	// POST /compile/:job_id/browser-failure → retain complete browser diagnostics.
	g.POST("/compile/:job_id/browser-failure", func(c *gin.Context) {
		pid, _, ok := verify(c)
		if !ok {
			return
		}
		jobID, err := uuid.Parse(c.Param("job_id"))
		if err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		var req struct {
			Error string `json:"error"`
			Logs  string `json:"logs"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		if req.Error == "" {
			req.Error = "Browser LaTeX compilation failed"
		}
		if err := queue.FailBrowserJob(c.Request.Context(), jobID, pid, auth.CurrentUser(c).ID.UUID, req.Error, req.Logs); err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 409, Info: "Compile job is already complete or cancelled"})
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"job_id": jobID, "status": string(store.JobFailed)}, ""))
	})

	// DELETE /compile?job_id= → 取消指定任务；否则停止项目编译
	g.DELETE("/compile", func(c *gin.Context) {
		pid, pidStr, ok := verify(c)
		if !ok {
			return
		}
		if jid := c.Query("job_id"); jid != "" {
			jobID, err := uuid.Parse(jid)
			if err != nil {
				httpapi.WriteError(c.Writer, httpapi.ErrValidation)
				return
			}
			userID := auth.CurrentUser(c).ID.UUID
			job := queue.GetStatus(c.Request.Context(), jobID, userID)
			if job == nil || job.ProjectID.UUID != pid {
				httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Compile job not found"})
				return
			}
			job, err = queue.Cancel(c.Request.Context(), jobID, userID)
			if err != nil {
				httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Unable to cancel compile job"})
				return
			}
			if job == nil || job.ProjectID.UUID != pid {
				httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Compile job not found"})
				return
			}
			_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{
				"cancelled": job.Status == string(store.JobCancelled), "status": job.Status, "job_id": jid,
			}, ""))
			return
		}
		_ = svc.Client.StopCompile(c.Request.Context(), pidStr)
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"stopped": true}, ""))
	})

	// GET /compile/history → {items, total}
	g.GET("/compile/history", func(c *gin.Context) {
		pid, _, ok := verify(c)
		if !ok {
			return
		}
		items, total := queue.GetHistory(c.Request.Context(), pid, auth.CurrentUser(c).ID.UUID)
		out := make([]gin.H, 0, len(items))
		for i := range items {
			j := &items[i]
			out = append(out, gin.H{
				"job_id": j.ID.UUID, "status": j.Status, "root_file": j.RootFile,
				"compiler": j.Compiler, "build_id": j.BuildID, "error": j.Error,
				"created_at": dto.JSONTime{Time: j.CreatedAt},
				"updated_at": dto.JSONTime{Time: j.UpdatedAt},
			})
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"items": out, "total": total}, ""))
	})

	// GET /compile/{job_id}/status → 轮询状态
	g.GET("/compile/:job_id/status", func(c *gin.Context) {
		_, _, ok := verify(c)
		if !ok {
			return
		}
		jobID, err := uuid.Parse(c.Param("job_id"))
		if err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		job := queue.GetStatus(c.Request.Context(), jobID, auth.CurrentUser(c).ID.UUID)
		if job == nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Compile job not found"})
			return
		}
		elapsed := job.UpdatedAt.Sub(job.CreatedAt).Seconds()
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{
			"job_id": job.ID.UUID, "status": job.Status, "build_id": job.BuildID,
			"error": job.Error, "logs": job.Logs, "root_file": job.RootFile,
			"compiler": job.Compiler, "elapsed_seconds": elapsed,
			"created_at": dto.JSONTime{Time: job.CreatedAt},
			"updated_at": dto.JSONTime{Time: job.UpdatedAt},
		}, ""))
	})

	// DELETE /cache
	g.DELETE("/cache", func(c *gin.Context) {
		_, pidStr, ok := verify(c)
		if !ok {
			return
		}
		_ = svc.Client.ClearCache(c.Request.Context(), pidStr)
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"cleared": true}, ""))
	})

	// GET /wordcount?file=&image=
	g.GET("/wordcount", func(c *gin.Context) {
		_, pidStr, ok := verify(c)
		if !ok {
			return
		}
		file := c.DefaultQuery("file", "main.tex")
		var image *string
		if v := c.Query("image"); v != "" {
			image = &v
		}
		texcount, err := svc.Client.Wordcount(c.Request.Context(), pidStr, file, image)
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 502, Info: fmt.Sprintf("无法获取编译输出: %v", err)})
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"texcount": texcount}, ""))
	})

	// POST /sync/code {file,line,column} → {pdf:[...]}
	g.POST("/sync/code", func(c *gin.Context) {
		_, pidStr, ok := verify(c)
		if !ok {
			return
		}
		var req struct {
			File   string `json:"file"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		pdf, err := svc.Client.SyncFromCode(c.Request.Context(), pidStr, req.File, req.Line, req.Column)
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 502, Info: fmt.Sprintf("无法获取编译输出: %v", err)})
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"pdf": pdf}, ""))
	})

	// POST /sync/pdf {page,h,v} → {code:[...]}
	g.POST("/sync/pdf", func(c *gin.Context) {
		_, pidStr, ok := verify(c)
		if !ok {
			return
		}
		var req struct {
			Page int     `json:"page"`
			H    float64 `json:"h"`
			V    float64 `json:"v"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		code, err := svc.Client.SyncFromPDF(c.Request.Context(), pidStr, req.Page, req.H, req.V)
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 502, Info: fmt.Sprintf("无法获取编译输出: %v", err)})
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"code": code}, ""))
	})

	// GET /output/{build_id}/output.zip → 代理 zip
	g.GET("/output/:build_id/output.zip", func(c *gin.Context) {
		_, pidStr, ok := verify(c)
		if !ok {
			return
		}
		buildID := c.Param("build_id")
		if strings.HasPrefix(buildID, "browser-") {
			pdf, err := queue.Files.Storage.Get(c.Request.Context(), browserArtifactKey(pidStr, buildID))
			if err != nil {
				writeErr(c, &ServiceError{Code: 404, Msg: "Browser compile artifact not found"})
				return
			}
			var body bytes.Buffer
			zw := zip.NewWriter(&body)
			file, err := zw.Create("output.pdf")
			if err == nil {
				_, err = file.Write(pdf)
			}
			if err == nil {
				err = zw.Close()
			} else {
				_ = zw.Close()
			}
			if err != nil {
				writeErr(c, err)
				return
			}
			c.Header("Content-Type", "application/zip")
			c.Status(http.StatusOK)
			_, _ = c.Writer.Write(body.Bytes())
			return
		}
		code, body, err := svc.Client.GetOutputZip(c.Request.Context(), pidStr, c.Param("build_id"))
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 502, Info: fmt.Sprintf("无法获取编译输出: %v", err)})
			return
		}
		c.Header("Content-Type", "application/zip")
		c.Status(code)
		_, _ = c.Writer.Write(body)
	})

	// GET|HEAD /output/{build_id}/pdf → Range 支持
	outputPDF := func(c *gin.Context) {
		_, pidStr, ok := verify(c)
		if !ok {
			return
		}
		buildID := c.Param("build_id")
		var pdf []byte
		var err error
		if strings.HasPrefix(buildID, "browser-") {
			pdf, err = queue.Files.Storage.Get(c.Request.Context(), browserArtifactKey(pidStr, buildID))
		} else {
			pdf, err = svc.GetOutputPDF(c.Request.Context(), pidStr, buildID)
		}
		if err != nil {
			writeErr(c, err)
			return
		}
		total := len(pdf)

		if c.Request.Method == http.MethodHead {
			c.Header("Content-Type", "application/pdf")
			c.Header("Content-Disposition", "inline")
			c.Header("Accept-Ranges", "bytes")
			c.Header("Content-Length", strconv.Itoa(total))
			c.Status(200)
			return
		}

		m := rangeRe.FindStringSubmatch(c.GetHeader("Range"))
		if m == nil {
			c.Header("Content-Type", "application/pdf")
			c.Header("Content-Disposition", "inline")
			c.Header("Accept-Ranges", "bytes")
			c.Header("Content-Length", strconv.Itoa(total))
			c.Status(200)
			_, _ = c.Writer.Write(pdf)
			return
		}
		start, end := 0, total-1
		if m[1] != "" {
			start, _ = strconv.Atoi(m[1])
		}
		if m[2] != "" {
			end, _ = strconv.Atoi(m[2])
		}
		if end > total-1 {
			end = total - 1
		}
		if start > end || start >= total {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 416, Info: "Requested range not satisfiable"})
			return
		}
		chunk := pdf[start : end+1]
		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
		c.Header("Accept-Ranges", "bytes")
		c.Header("Content-Length", strconv.Itoa(len(chunk)))
		c.Status(206)
		_, _ = c.Writer.Write(chunk)
	}
	g.GET("/output/:build_id/pdf", outputPDF)
	g.HEAD("/output/:build_id/pdf", outputPDF)
}
