package s3

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/bacalhau-project/bacalhau/pkg/models"
	"github.com/bacalhau-project/bacalhau/pkg/publisher"
	s3helper "github.com/bacalhau-project/bacalhau/pkg/s3"
	"github.com/rs/zerolog/log"
)

type PublisherParams struct {
	LocalDir       string
	ClientProvider *s3helper.ClientProvider
}

// Compile-time check that publisher implements the correct interface:
var _ publisher.Publisher = (*Publisher)(nil)

type Publisher struct {
	localDir       string
	clientProvider *s3helper.ClientProvider
}

func NewPublisher(params PublisherParams) *Publisher {
	return &Publisher{
		localDir:       params.LocalDir,
		clientProvider: params.ClientProvider,
	}
}

// IsInstalled returns true if the S3 client is installed.
func (publisher *Publisher) IsInstalled(_ context.Context) (bool, error) {
	return publisher.clientProvider.IsInstalled(), nil
}

// ValidateJob validates the job spec and returns an error if the job is invalid.
func (publisher *Publisher) ValidateJob(_ context.Context, j models.Job) error {
	_, err := s3helper.DecodePublisherSpec(j.Task().Publisher)
	return err
}

func (publisher *Publisher) PublishResult(
	ctx context.Context,
	executionID string,
	j models.Job,
	resultPath string,
) (models.SpecConfig, error) {
	spec, err := s3helper.DecodePublisherSpec(j.Task().Publisher)
	if err != nil {
		return models.SpecConfig{}, err
	}

	if spec.Compress {
		return publisher.publishArchive(ctx, spec, executionID, j, resultPath)
	}
	return publisher.publishDirectory(ctx, spec, executionID, j, resultPath)
}

func (publisher *Publisher) publishArchive(
	ctx context.Context,
	spec s3helper.PublisherSpec,
	executionID string,
	j models.Job,
	resultPath string,
) (models.SpecConfig, error) {
	client := publisher.clientProvider.GetClient(spec.Endpoint, spec.Region)
	key := ParsePublishedKey(spec.Key, executionID, j, true)

	// Create a new GZIP writer that writes to the file.
	targetFile, err := os.CreateTemp(publisher.localDir, "bacalhau-archive-*.tar.gz")
	if err != nil {
		return models.SpecConfig{}, err
	}
	defer targetFile.Close()
	defer os.Remove(targetFile.Name())

	err = archiveDirectory(resultPath, targetFile)
	if err != nil {
		return models.SpecConfig{}, err
	}

	// reset the archived file to read and upload it
	_, err = targetFile.Seek(0, io.SeekStart)
	if err != nil {
		return models.SpecConfig{}, err
	}

	// Upload the GZIP archive to S3.
	res, err := client.Uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:            aws.String(spec.Bucket),
		Key:               aws.String(key),
		Body:              targetFile,
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	})
	if err != nil {
		return models.SpecConfig{}, err
	}
	log.Debug().Msgf("Uploaded s3://%s/%s", spec.Bucket, aws.ToString(res.Key))

	return models.SpecConfig{
		Type: models.StorageSourceS3,
		Params: s3helper.SourceSpec{
			Bucket:         spec.Bucket,
			Key:            key,
			Endpoint:       spec.Endpoint,
			Region:         spec.Region,
			ChecksumSHA256: aws.ToString(res.ChecksumSHA256),
			VersionID:      aws.ToString(res.VersionID),
		}.ToMap(),
	}, nil
}

func (publisher *Publisher) publishDirectory(
	ctx context.Context,
	spec s3helper.PublisherSpec,
	executionID string,
	j models.Job,
	resultPath string,
) (models.SpecConfig, error) {
	client := publisher.clientProvider.GetClient(spec.Endpoint, spec.Region)
	key := ParsePublishedKey(spec.Key, executionID, j, false)

	// Walk the directory tree and upload each file to S3.
	err := filepath.Walk(resultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil // skip directories
		}
		// Read the file contents.
		data, err := os.Open(path)
		if err != nil {
			return err
		}
		defer data.Close()

		relativePath, err := filepath.Rel(resultPath, path)
		if err != nil {
			return err
		}
		// Upload the file to S3.
		res, err := client.Uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket:            aws.String(spec.Bucket),
			Key:               aws.String(key + filepath.ToSlash(relativePath)),
			Body:              data,
			ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
		})
		if err != nil {
			return err
		}
		log.Debug().Msgf("Uploaded s3://%s/%s", spec.Bucket, aws.ToString(res.Key))
		return nil
	})

	if err != nil {
		return models.SpecConfig{}, err
	}

	return models.SpecConfig{
		Type: models.StorageSourceS3,
		Params: s3helper.SourceSpec{
			Bucket:   spec.Bucket,
			Key:      key,
			Region:   spec.Region,
			Endpoint: spec.Endpoint,
		}.ToMap(),
	}, nil
}
