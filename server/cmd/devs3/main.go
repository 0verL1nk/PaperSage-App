// devs3 — 本地开发用 S3 兼容服务（gofakes3 + afero 文件系统后端）。
//
// 供没有 Docker/MinIO 的环境（如 Windows 本机）跑通上传链路：
//
//	go run ./cmd/devs3                       # 默认 :9000，数据落 ./devs3-data
//	S3_ADDR=:9001 S3_DIR=./tmp go run ./cmd/devs3
//
// 后端配置（与 e2e/CI 的 MinIO 一致）：
//
//	S3_ENDPOINT=http://localhost:9000
//	S3_ACCESS_KEY=dev-access S3_SECRET_KEY=dev-secret
//	S3_BUCKET=papersage S3_REGION=us-east-1
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3afero"
	"github.com/spf13/afero"
)

func main() {
	addr := flag.String("addr", envOr("S3_ADDR", ":9000"), "listen address")
	dir := flag.String("dir", envOr("S3_DIR", "./devs3-data"), "data directory")
	bucket := flag.String("bucket", envOr("S3_BUCKET", "papersage"), "pre-created bucket")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatalf("devs3: create data dir: %v", err)
	}
	fs := afero.NewOsFs()
	// 对象与元数据分目录；s3afero 要求对根有完全控制权，必须 BasePathFs
	backend, err := s3afero.SingleBucket(*bucket,
		afero.NewBasePathFs(fs, filepath.Join(*dir, "objects")),
		afero.NewBasePathFs(fs, filepath.Join(*dir, "meta")))
	if err != nil {
		log.Fatalf("devs3: backend: %v", err)
	}
	srv := gofakes3.New(backend)
	log.Printf("devs3: listening on http://localhost%s (bucket=%s, dir=%s, creds=dev-access/dev-secret)",
		*addr, *bucket, *dir)
	log.Fatal(http.ListenAndServe(*addr, srv.Server()))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
