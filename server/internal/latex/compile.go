package latex

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
)

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

	// POST /compile → 202 {job_id, status}
	g.POST("/compile", func(c *gin.Context) {
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
		if req.Compiler == "" {
			req.Compiler = "pdflatex"
		}
		if !ValidCompilers[req.Compiler] {
			httpapi.WriteError(c.Writer, &httpapi.APIError{
				Code: 400, Info: "Invalid compiler. Must be one of: lualatex, pdflatex, xelatex",
			})
			return
		}
		job, err := queue.Submit(c.Request.Context(), pid, auth.CurrentUser(c).ID.UUID, req.RootFile, req.Compiler)
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusAccepted, httpapi.Success(gin.H{
			"job_id": job.ID.UUID, "status": job.Status,
		}, ""))
	})

	// DELETE /compile?job_id= → 取消指定任务；否则停止项目编译
	g.DELETE("/compile", func(c *gin.Context) {
		_, pidStr, ok := verify(c)
		if !ok {
			return
		}
		if jid := c.Query("job_id"); jid != "" {
			jobID, err := uuid.Parse(jid)
			if err != nil {
				httpapi.WriteError(c.Writer, httpapi.ErrValidation)
				return
			}
			job := queue.Cancel(c.Request.Context(), jobID, auth.CurrentUser(c).ID.UUID)
			if job == nil {
				httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Compile job not found"})
				return
			}
			_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"cancelled": true, "job_id": jid}, ""))
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
		pdf, err := svc.GetOutputPDF(c.Request.Context(), pidStr, c.Param("build_id"))
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
