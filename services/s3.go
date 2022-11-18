package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gabriel-vasile/mimetype"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/inflector"
	"github.com/pocketbase/pocketbase/tools/security"
	"gocloud.dev/blob"
	"gocloud.dev/blob/s3blob"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var extensionInvalidCharsRegex = regexp.MustCompile(`[^\w.*\-+=#]+`)

type S3Config struct {
	PublicUrl string `json:"publicUrl"`
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"accessKey"`
	Secret    string `json:"secret"`
}

func getS3Config(app *pocketbase.PocketBase) (*S3Config, error) {
	s3Config, err := GetInDBConfig(app, "s3")

	if err != nil || s3Config == "" {
		return nil, fmt.Errorf("s3 config not found")
	}

	var config S3Config
	err = json.Unmarshal([]byte(s3Config), &config)
	if err != nil {
		return nil, fmt.Errorf("invalid s3 config")
	}

	return &config, nil
}

func getSanitizeFilename(fileHeader *multipart.FileHeader, mt *mimetype.MIME) string {
	originalExt := filepath.Ext(fileHeader.Filename)
	sanitizedExt := extensionInvalidCharsRegex.ReplaceAllString(originalExt, "")
	if sanitizedExt == "" {
		sanitizedExt = mt.Extension()
	}
	originalName := strings.TrimSuffix(fileHeader.Filename, originalExt)
	sanitizedName := inflector.Snakecase(originalName)
	if length := len(sanitizedName); length < 3 {
		sanitizedName += "_" + security.RandomString(10)
	} else if length > 100 {
		sanitizedName = sanitizedName[:100]
	}

	return fmt.Sprintf(
		"%s_%s%s",
		sanitizedName,
		security.RandomString(10), // ensure that there is always a random part
		sanitizedExt,
	)
}

func getFileKey(fileHeader *multipart.FileHeader, mt *mimetype.MIME) string {
	dateStr := time.Now().Format("2006-01-02")
	filename := getSanitizeFilename(fileHeader, mt)
	return fmt.Sprintf("uploads/%s/%s", dateStr, filename)
}

func upload(file multipart.File, fileHeader *multipart.FileHeader, s3Config *S3Config) (string, error) {
	ctx := context.Background()

	cred := credentials.NewStaticCredentials(s3Config.AccessKey, s3Config.Secret, "")

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(s3Config.Region),
		Endpoint:    aws.String(s3Config.Endpoint),
		Credentials: cred,
	})
	if err != nil {
		return "", err
	}

	bucket, err := s3blob.OpenBucket(ctx, sess, s3Config.Bucket, nil)
	if err != nil {
		return "", err
	}

	mt, err := mimetype.DetectReader(file)
	if err != nil {
		return "", err
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return "", err
	}

	opts := &blob.WriterOptions{
		ContentType: mt.String(),
	}

	fileKey := getFileKey(fileHeader, mt)
	w, err := bucket.NewWriter(ctx, fileKey, opts)
	if err != nil {
		return "", err
	}

	if _, err := w.ReadFrom(file); err != nil {
		err := w.Close()
		return "", err
	}

	url := strings.TrimSuffix(s3Config.PublicUrl, "/") + "/" + fileKey
	return url, w.Close()
}

func registerS3Service(app *pocketbase.PocketBase) {
	// Register custom services here
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		_, err := e.Router.AddRoute(echo.Route{
			Method: http.MethodPost,
			Path:   "/api/upload",
			Handler: func(c echo.Context) error {
				file, fileHeader, err := c.Request().FormFile("file")
				defer func(file multipart.File) {
					err := file.Close()
					if err != nil {
						return
					}
				}(file)
				if err != nil {
					return apis.NewApiError(http.StatusBadRequest, err.Error(), nil)
				}

				s3Config, err := getS3Config(app)
				if err != nil {
					return apis.NewApiError(http.StatusInternalServerError, err.Error(), nil)
				}

				url, err := upload(file, fileHeader, s3Config)
				if err != nil {
					return apis.NewApiError(http.StatusInternalServerError, err.Error(), nil)
				}

				return c.JSON(http.StatusOK, map[string]any{
					"url": url,
				})
			},
			Middlewares: []echo.MiddlewareFunc{
				apis.RequireRecordAuth("users"),
			},
		})
		if err != nil {
			return err
		}

		return nil
	})

}
