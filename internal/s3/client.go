package s3

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/haumanto/s3-usage-monitoring/internal/db"
)

type Client struct {
	client *s3.Client
	bucket string
}

func NewClient(account *db.S3Account) (*Client, error) {
	var cfg aws.Config
	var err error

	if account.Endpoint != "" {
		cfg, err = config.LoadDefaultConfig(context.Background(),
			config.WithRegion(account.Region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(account.AccessKey, account.SecretKey, "")),
			config.WithBaseEndpoint(account.Endpoint),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(context.Background(),
			config.WithRegion(account.Region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(account.AccessKey, account.SecretKey, "")),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if account.Endpoint != "" {
			o.UsePathStyle = true
		}
	})

	return &Client{client: client, bucket: account.Bucket}, nil
}

// CalculateUsage computes total bytes used in the configured bucket(s).
// If no bucket is specified, it sums usage across all accessible buckets.
func (c *Client) CalculateUsage(ctx context.Context) (int64, error) {
	if c.bucket != "" {
		return c.calculateBucketUsage(ctx, c.bucket)
	}

	result, err := c.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return 0, fmt.Errorf("list buckets: %w", err)
	}

	var total int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for _, b := range result.Buckets {
		if b.Name == nil {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			usage, err := c.calculateBucketUsage(ctx, name)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			mu.Lock()
			total += usage
			mu.Unlock()
		}(aws.ToString(b.Name))
	}
	wg.Wait()

	if firstErr != nil {
		return total, firstErr
	}
	return total, nil
}

func (c *Client) calculateBucketUsage(ctx context.Context, bucket string) (int64, error) {
	var total int64
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			ContinuationToken: continuationToken,
		}

		result, err := c.client.ListObjectsV2(ctx, input)
		if err != nil {
			var noSuchBucket *types.NoSuchBucket
			if errors.As(err, &noSuchBucket) {
				return 0, fmt.Errorf("bucket %s does not exist", bucket)
			}
			return 0, fmt.Errorf("list objects in %s: %w", bucket, err)
		}

		for _, obj := range result.Contents {
			if obj.Size != nil {
				total += *obj.Size
			}
		}

		if !aws.ToBool(result.IsTruncated) {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	return total, nil
}
