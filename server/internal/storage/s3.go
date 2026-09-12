// Package storage S3 兼容对象存储客户端（R2 / MinIO / AWS）。
// 契约来源：backend/services/storage/s3_client.py
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
)

// Storage 对象存储抽象（documents 与 latex 域共用的最小接口）。
type Storage interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	List(ctx context.Context, prefix string) ([]string, error)
	Exists(ctx context.Context, key string) bool
}

// S3 是 aws-sdk-go-v2 实现（sigv4、自定义 endpoint、path-style）。
type S3 struct {
	client *s3.Client
	bucket string
}

// NewS3 按配置构建客户端。region "auto"（R2）按 boto3 对齐降级为 us-east-1。
func NewS3(ctx context.Context, cfg *config.Settings) (*S3, error) {
	region := cfg.S3Region
	if region == "auto" || region == "" {
		region = "us-east-1"
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKey, cfg.S3SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
		}
		o.UsePathStyle = true // MinIO / R2 兼容
	})
	return &S3{client: client, bucket: cfg.S3Bucket}, nil
}

// Put 上传对象。
func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(data),
	})
	return err
}

// Get 下载对象。
func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()
	return io.ReadAll(out.Body)
}

// Delete 删除对象。
func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	return err
}

// PresignGet 生成临时读 URL（默认 300s，对齐 generate_presigned_url）。
func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	presign := s3.NewPresignClient(s.client)
	out, err := presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

// List 分页列出前缀下全部 key（字典序，对齐 paginator 行为）。
func (s *S3) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	var token *string
	for {
		out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(s.bucket), Prefix: aws.String(prefix), ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range out.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
		if out.NextContinuationToken == nil {
			break
		}
		token = out.NextContinuationToken
	}
	sort.Strings(keys)
	return keys, nil
}

// Exists 检查对象存在（HEAD）。
func (s *S3) Exists(ctx context.Context, key string) bool {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	return err == nil
}

// BuildKey 组装 LaTeX 文件 key：projects/{project}/latex/{path}。
func BuildKey(projectID, filePath string) string {
	return path.Join("projects", projectID, "latex", filePath)
}

// ErrNotFound 表示对象不存在（GetObject NoSuchKey）。
var ErrNotFound = errors.New("object not found")
