package storage

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/models"
)

type Storage struct {
	client *s3.Client
	bucket string
}

func NewFromConfig(cfg config.Config) (*Storage, error) {
	awsConfig, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(cfg.S3Endpoint)
		options.UsePathStyle = true
	})

	return &Storage{client: client, bucket: cfg.S3Bucket}, nil
}

func (s *Storage) UploadSource(
	ctx context.Context,
	content []byte,
	userID string,
	documentID string,
	filename string,
	contentType string,
) (models.SourceRef, error) {

	key := joinKey(userID, documentID, "source", filename)
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
		S3Bucket:       s.bucket,
		SourceFilename: filename,
	}, nil
}

func (s *Storage) DeleteDocumentFiles(ctx context.Context, document models.Document) error {
	if document.S3Bucket == nil {
		return errors.New("document bucket is nil")
	}

	bucket := *document.S3Bucket
	prefix := models.S3DirectoryPath(document.UserID, document.ID) + "/"

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

