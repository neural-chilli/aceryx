package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/neural-chilli/aceryx/internal/storage"
)

type Config struct {
	Bucket          string
	Region          string
	Prefix          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UseIAMRole      bool
	UseSSL          bool
	SSE             string
	SSEKMSKeyID     string
}

type Store struct {
	bucket string
	prefix string
	client *s3.Client
	pre    *s3.PresignClient
	cfg    Config
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}
	if strings.TrimSpace(cfg.Region) == "" {
		cfg.Region = "us-east-1"
	}
	loadOpts := make([]func(*awsconfig.LoadOptions) error, 0, 3)
	loadOpts = append(loadOpts, awsconfig.WithRegion(cfg.Region))
	if strings.TrimSpace(cfg.AccessKeyID) != "" || strings.TrimSpace(cfg.SecretAccessKey) != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	clientOpts := make([]func(*s3.Options), 0, 2)
	if strings.TrimSpace(cfg.Endpoint) != "" {
		endpoint := strings.TrimSpace(cfg.Endpoint)
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.UsePathStyle = true
			o.BaseEndpoint = &endpoint
		})
	}
	client := s3.NewFromConfig(awsCfg, clientOpts...)
	return &Store{
		bucket: cfg.Bucket,
		prefix: strings.Trim(strings.TrimSpace(cfg.Prefix), "/"),
		client: client,
		pre:    s3.NewPresignClient(client),
		cfg:    cfg,
	}, nil
}

func (s *Store) Put(ctx context.Context, key string, data io.Reader, metadata storage.ObjectMetadata) error {
	meta := map[string]string{}
	for k, v := range metadata.Custom {
		meta[k] = v
	}
	if metadata.Checksum != "" {
		meta["checksum"] = metadata.Checksum
	}
	in := &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         strPtr(s.objectKey(key)),
		Body:        data,
		Metadata:    meta,
		ContentType: strPtr(metadata.ContentType),
	}
	if metadata.ContentLength > 0 {
		in.ContentLength = &metadata.ContentLength
	}
	switch strings.ToLower(strings.TrimSpace(s.cfg.SSE)) {
	case "sse-kms", "aws:kms":
		in.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		if strings.TrimSpace(s.cfg.SSEKMSKeyID) != "" {
			in.SSEKMSKeyId = strPtr(s.cfg.SSEKMSKeyID)
		}
	default:
		in.ServerSideEncryption = types.ServerSideEncryptionAes256
	}
	_, err := s.client.PutObject(ctx, in)
	if err != nil {
		return mapS3Error(err, s.bucket)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, storage.ObjectMetadata, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: strPtr(s.objectKey(key))})
	if err != nil {
		return nil, storage.ObjectMetadata{}, mapS3Error(err, s.bucket)
	}
	meta := storage.ObjectMetadata{
		ContentType:   value(out.ContentType),
		ContentLength: valueInt64(out.ContentLength),
		Custom:        out.Metadata,
	}
	if out.Metadata != nil {
		meta.Checksum = out.Metadata["checksum"]
	}
	return out.Body, meta, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: strPtr(s.objectKey(key))})
	if err != nil {
		return mapS3Error(err, s.bucket)
	}
	return nil
}

func (s *Store) List(ctx context.Context, prefix string, opts storage.ListOpts) ([]storage.ObjectInfo, error) {
	max := int32(opts.MaxResults)
	if max <= 0 {
		max = 1000
	}
	input := &s3.ListObjectsV2Input{
		Bucket:            &s.bucket,
		Prefix:            strPtr(s.objectKey(prefix)),
		MaxKeys:           &max,
		ContinuationToken: nil,
	}
	if strings.TrimSpace(opts.Delimiter) != "" {
		input.Delimiter = strPtr(opts.Delimiter)
	}
	if strings.TrimSpace(opts.Cursor) != "" {
		input.ContinuationToken = strPtr(opts.Cursor)
	}
	out, err := s.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, mapS3Error(err, s.bucket)
	}
	res := make([]storage.ObjectInfo, 0, len(out.Contents))
	for _, item := range out.Contents {
		res = append(res, storage.ObjectInfo{
			Key:          trimPrefix(value(item.Key), s.prefix),
			Size:         valueInt64(item.Size),
			LastModified: valueTime(item.LastModified),
		})
	}
	return res, nil
}

func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: strPtr(s.objectKey(key))})
	if err == nil {
		return true, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(strings.ToLower(err.Error()), "status code: 404") {
		return false, nil
	}
	return false, mapS3Error(err, s.bucket)
}

func (s *Store) PresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	out, err := s.pre.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: strPtr(s.objectKey(key))}, func(options *s3.PresignOptions) {
		options.Expires = expiry
	})
	if err != nil {
		return "", mapS3Error(err, s.bucket)
	}
	return out.URL, nil
}

func (s *Store) objectKey(key string) string {
	return storage.JoinKey(s.prefix, key)
}

func mapS3Error(err error, bucket string) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "nosuchbucket") || strings.Contains(msg, "not found") {
		return fmt.Errorf("bucket %q not found", bucket)
	}
	if strings.Contains(msg, "accessdenied") || strings.Contains(msg, "forbidden") {
		return fmt.Errorf("permission denied: write to bucket %q", bucket)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("s3 operation timed out: %w", err)
	}
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return fmt.Errorf("s3 network error: %w", err)
	}
	return fmt.Errorf("s3 operation failed: %w", err)
}

func trimPrefix(key, prefix string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return key
	}
	key = strings.TrimPrefix(key, prefix)
	key = strings.TrimPrefix(key, "/")
	return key
}

func value[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

func valueInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func valueTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return v.UTC()
}

func strPtr(s string) *string { return &s }
