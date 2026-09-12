// fakes3 — 本地开发/E2E 用进程内 S3 服务器（gofakes3 内存后端）。
// docker 不可用的环境用它替代 MinIO：
//
//	go run ./cmd/fakes3            # 监听 :9000，创建 papersage 桶
//	S3_ENDPOINT=http://localhost:9000 S3_ACCESS_KEY=test S3_SECRET_KEY=test \
//	  go run ./cmd/papersage
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func main() {
	addr := flag.String("addr", ":9000", "listen address")
	bucket := flag.String("bucket", "papersage", "bucket to pre-create")
	flag.Parse()

	backend := s3mem.New()
	if err := backend.CreateBucket(*bucket); err != nil {
		log.Fatalf("fakes3: create bucket: %v", err)
	}
	srv := gofakes3.New(backend)
	log.Printf("fakes3: listening on %s (bucket=%s, creds test/test)", *addr, *bucket)
	log.Fatal(http.ListenAndServe(*addr, srv.Server()))
}
