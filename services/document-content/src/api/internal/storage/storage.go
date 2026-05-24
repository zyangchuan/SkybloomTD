package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"skybloom/document-content-api/internal/models"
)

type Storage struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewFromEnv() (*Storage, error) {
	bucket := strings.TrimSpace(os.Getenv("AWS_S3_BUCKET"))
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("AWS_S3_ENDPOINT_URL")), "/")
	accessKey := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))
	region := envOrDefault("AWS_REGION", envOrDefault("AWS_DEFAULT_REGION", "us-east-1"))
	prefix := strings.Trim(strings.TrimSpace(os.Getenv("AWS_S3_PREFIX")), "/")

	if bucket == "" {
		return nil, nil
	}
	if endpoint == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS_S3_ENDPOINT_URL, AWS_ACCESS_KEY_ID, and AWS_SECRET_ACCESS_KEY are required when AWS_S3_BUCKET is set")
	}

	if strings.HasPrefix(bucket, "s3://") {
		parsed, err := url.Parse(bucket)
		if err != nil {
			return nil, err
		}
		bucket = parsed.Host
		uriPrefix := strings.Trim(parsed.Path, "/")
		prefix = joinKey(uriPrefix, prefix)
	}

	awsConfig, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		awsconfig.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(
				func(service, region string, options ...any) (aws.Endpoint, error) {
					return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
				},
			),
		),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = true
	})
	return &Storage{client: client, bucket: bucket, prefix: prefix}, nil
}

func (s *Storage) UploadSource(
	ctx context.Context,
	content []byte,
	userID string,
	documentID string,
	filename string,
	contentType string,
) (models.SourceRef, error) {
	key := joinKey(s.prefix, "users", userID, "documents", documentID, "source", filename)
	input := &s3.PutObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
		Body:   bytes.NewReader(content),
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = &contentType
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return models.SourceRef{}, err
	}
	return models.SourceRef{
		Type:        "s3",
		Bucket:      s.bucket,
		Key:         key,
		Filename:    filename,
		ContentType: contentType,
	}, nil
}

func joinKey(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, "/")
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
