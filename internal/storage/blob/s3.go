package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// S3Store implements interfaces.FileStore using any S3-compatible object storage.
type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
	logger *common.Logger
}

// Compile-time check
var _ interfaces.FileStore = (*S3Store)(nil)

// NewS3Store creates an S3Store backed by the provided BlobConfig.
func NewS3Store(cfg common.BlobConfig, logger *common.Logger) (*S3Store, error) {
	ctx := context.Background()

	var opts []func(*config.LoadOptions) error

	if cfg.Region != "" && cfg.Region != "auto" {
		opts = append(opts, config.WithRegion(cfg.Region))
	} else {
		opts = append(opts, config.WithRegion("us-east-1"))
	}

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &S3Store{
		client: client,
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
		logger: logger,
	}, nil
}

// objectKey builds the S3 key: {prefix}/{category}/{key}
func (s *S3Store) objectKey(category, key string) string {
	if s.prefix != "" {
		return s.prefix + "/" + category + "/" + key
	}
	return category + "/" + key
}

func (s *S3Store) SaveFile(ctx context.Context, category, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.objectKey(category, key)),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("failed to save blob %s/%s: %w", category, key, err)
	}
	return nil
}

func (s *S3Store) GetFile(ctx context.Context, category, key string) ([]byte, string, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(category, key)),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get blob %s/%s: %w", category, key, err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read blob body %s/%s: %w", category, key, err)
	}

	contentType := "application/octet-stream"
	if result.ContentType != nil && *result.ContentType != "" {
		contentType = *result.ContentType
	}

	return data, contentType, nil
}

func (s *S3Store) DeleteFile(ctx context.Context, category, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(category, key)),
	})
	if err != nil {
		return fmt.Errorf("failed to delete blob %s/%s: %w", category, key, err)
	}
	return nil
}

func (s *S3Store) HasFile(ctx context.Context, category, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(category, key)),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		var notFound *types.NotFound
		if errors.As(err, &nsk) || errors.As(err, &notFound) {
			return false, nil
		}
		// HeadObject may also return a generic 404 — check HTTP status
		var respErr interface{ HTTPStatusCode() int }
		if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
			return false, nil
		}
		return false, fmt.Errorf("failed to check blob %s/%s: %w", category, key, err)
	}
	return true, nil
}
