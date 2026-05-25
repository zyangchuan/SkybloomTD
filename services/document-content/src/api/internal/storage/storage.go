package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

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

func (s *Storage) DeleteDocumentAssets(ctx context.Context, document models.Document) error {
	targets := uniqueDeleteTargets(document)
	for _, target := range targets {
		if target.Prefix != "" {
			if err := s.deletePrefix(ctx, target.Bucket, target.Prefix); err != nil {
				return err
			}
			continue
		}
		if target.Key != "" {
			if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(target.Bucket),
				Key:    aws.String(target.Key),
			}); err != nil {
				return err
			}
		}
	}

	if document.SourcePath != nil && strings.TrimSpace(*document.SourcePath) != "" {
		if err := os.RemoveAll(filepath.Dir(*document.SourcePath)); err != nil {
			return err
		}
	}
	return nil
}

type deleteTarget struct {
	Bucket string
	Prefix string
	Key    string
}

func (s *Storage) deletePrefix(ctx context.Context, bucket string, prefix string) error {
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}

		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}
			objects = append(objects, types.ObjectIdentifier{Key: object.Key})
		}
		if len(objects) == 0 {
			continue
		}
		if _, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true),
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func uniqueDeleteTargets(document models.Document) []deleteTarget {
	targets := make([]deleteTarget, 0, 2)
	seen := map[deleteTarget]bool{}
	add := func(bucket *string, key *string) {
		if bucket == nil || key == nil {
			return
		}
		cleanBucket := strings.TrimSpace(*bucket)
		cleanKey := strings.Trim(strings.TrimSpace(*key), "/")
		if cleanBucket == "" || cleanKey == "" {
			return
		}

		target := deleteTarget{
			Bucket: cleanBucket,
			Prefix: documentS3Prefix(cleanKey, document.ID.String()),
		}
		if target.Prefix == "" {
			target.Key = cleanKey
		}
		if !seen[target] {
			targets = append(targets, target)
			seen[target] = true
		}
	}

	add(document.SourceBucket, document.SourceKey)
	add(document.S3Bucket, document.S3Key)
	return targets
}

func documentS3Prefix(key string, documentID string) string {
	marker := "/documents/" + documentID + "/"
	index := strings.Index(key, marker)
	if index == -1 {
		if strings.HasPrefix(key, strings.TrimPrefix(marker, "/")) {
			return strings.TrimPrefix(marker, "/")
		}
		return ""
	}
	return key[:index+len(marker)]
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
