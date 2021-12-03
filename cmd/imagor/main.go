package main

import (
	"flag"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/cshum/imagor"
	"github.com/cshum/imagor/loader/httploader"
	"github.com/cshum/imagor/processor/vipsprocessor"
	"github.com/cshum/imagor/server"
	"github.com/cshum/imagor/store/filestore"
	"github.com/cshum/imagor/store/s3store"
	"github.com/peterbourgon/ff/v3"
	"go.uber.org/zap"
	"os"
	"time"
)

func main() {
	var (
		fs       = flag.NewFlagSet("imagor", flag.ExitOnError)
		logger   *zap.Logger
		err      error
		loaders  []imagor.Loader
		storages []imagor.Storage
	)

	var (
		debug = fs.Bool("debug", false, "debug mode")
		port  = fs.Int("port", 9000, "sever port")

		imagorSecret = fs.String("imagor-secret", "",
			"Hash secret for signing imagor url")
		imagorUnsafe = fs.Bool("imagor-unsafe", false,
			"Enable unsafe imagor url that does not require hash signing")
		imagorRequestTimeout = fs.Duration("imagor-request-timeout",
			time.Second*30, "Timeout for performing imagor request")
		imagorSaveTimeout = fs.Duration("imagor-save-timeout",
			time.Minute, "Timeout for saving requesting image for storage")

		serverAddress = fs.String("server-address", "",
			"Server address")
		serverPathPrefix = fs.String("server-path-prefix", "",
			"Server path prefix")
		serverCORS = fs.Bool("server-cors", false,
			"Enable CORS")

		vipsDisableBlur = fs.Bool("vips-disable-blur", false,
			"Disable blur operations for vips processor")
		vipsDisableFilters = fs.String("vips-disable-filters", "",
			"Disable filters by csv e.g. blur,watermark,rgb")

		httpLoaderForwardHeaders = fs.String(
			"http-loader-forward-headers", "",
			"Forward request header to http loader request by csv e.g. User-Agent,Accept")
		httpLoaderForwardUserAgent = fs.Bool(
			"http-loader-forward-user-agent", false,
			"Enable forward require user agent to http loader request")
		httpLoaderForwardAllHeaders = fs.Bool(
			"http-loader-forward-all-headers", false,
			"Enable clone request header to http loader request")
		httpLoaderAllowedSources = fs.String(
			"http-loader-allowed-sources", "",
			"Allowed hosts whitelist to load images from if set. Accept csv wth glob pattern e.g. *.google.com,*.github.com")
		httpLoaderMaxAllowedSize = fs.Int(
			"http-loader-max-allowed-size", 0,
			"Maximum allowed size in bytes for loading images if set")
		httpLoaderInsecureSkipVerifyTransport = fs.Bool(
			"http-loader-insecure-skip-verify-transport", false,
			"Use HTTP transport with InsecureSkipVerify true")

		awsRegion = fs.String("aws-region", "",
			"AWS Region. Required if using S3 loader or storage")
		awsAccessKeyId = fs.String("aws-access-key-id", "",
			"AWS Access Key ID. Required if using S3 loader or storage")
		awsSecretAccessKey = fs.String("aws-secret-access-key", "",
			"AWS Secret Access Key. Required if using S3 loader or storage")

		s3LoaderBucket = fs.String("s3-loader-bucket", "",
			"S3 Bucket for S3 loader. Will activate S3 loader only if this value present")
		s3LoaderBaseDir = fs.String("s3-loader-base-dir", "/",
			"Base directory for S3 loader")
		s3LoaderPathPrefix = fs.String("s3-loader-path-prefix", "/",
			"Base path prefix for S3 loader")

		s3StorageBucket = fs.String("s3-storage-bucket", "",
			"S3 Bucket for S3 storage. Will activate S3 storage only if this value present")
		s3StorageBaseDir = fs.String("s3-storage-base-dir", "",
			"Base directory for S3 storage")
		s3StoragePathPrefix = fs.String("s3-storage-path-prefix", "",
			"Base path prefix for S3 storage")

		fileLoaderBaseDir = fs.String("file-loader-base-dir", "",
			"Base directory for file loader. Will activate file loader only if this value present")
		fileLoaderPathPrefix = fs.String("file-loader-path-prefix", "",
			"Base path prefix for file loader")

		fileStorageBaseDir = fs.String("file-storage-base-dir", "",
			"Base directory for file storage. Will activate file storage only if this value present")
		fileStoragePathPrefix = fs.String("file-storage-path-prefix", "",
			"Base path prefix for file storage")
	)

	if err = ff.Parse(fs, os.Args[1:], ff.WithEnvVarNoPrefix()); err != nil {
		panic(err)
	}

	if *debug {
		if logger, err = zap.NewDevelopment(); err != nil {
			panic(err)
		}
	} else {
		if logger, err = zap.NewProduction(); err != nil {
			panic(err)
		}
	}

	if *awsRegion != "" && *awsAccessKeyId != "" && *awsSecretAccessKey != "" {
		sess, err := session.NewSession(&aws.Config{
			Region: awsRegion,
			Credentials: credentials.NewStaticCredentials(
				*awsAccessKeyId, *awsSecretAccessKey, ""),
		})
		if err != nil {
			panic(err)
		}
		if *s3LoaderBucket != "" {
			loaders = append(loaders,
				s3store.New(sess, *s3LoaderBucket,
					s3store.WithPathPrefix(*s3LoaderPathPrefix),
					s3store.WithBaseDir(*s3LoaderBaseDir),
				),
			)
		}
		if *s3StorageBucket != "" {
			loaders = append(loaders,
				s3store.New(sess, *s3StorageBucket,
					s3store.WithPathPrefix(*s3StoragePathPrefix),
					s3store.WithBaseDir(*s3StorageBaseDir),
				),
			)
		}
	}

	if *fileLoaderBaseDir != "" {
		loaders = append(loaders,
			filestore.New(
				*fileLoaderBaseDir,
				filestore.WithPathPrefix(*fileLoaderPathPrefix),
			),
		)
	}

	if *fileStorageBaseDir != "" {
		storages = append(storages,
			filestore.New(
				*fileLoaderBaseDir,
				filestore.WithPathPrefix(*fileStoragePathPrefix),
			),
		)
	}

	loaders = append(loaders,
		httploader.New(
			httploader.WithForwardUserAgent(*httpLoaderForwardUserAgent),
			httploader.WithForwardAllHeaders(*httpLoaderForwardAllHeaders),
			httploader.WithForwardHeaders(*httpLoaderForwardHeaders),
			httploader.WithAllowedSources(*httpLoaderAllowedSources),
			httploader.WithMaxAllowedSize(*httpLoaderMaxAllowedSize),
			httploader.WithInsecureSkipVerifyTransport(*httpLoaderInsecureSkipVerifyTransport),
		),
	)

	server.New(
		imagor.New(
			imagor.WithLoaders(loaders...),
			imagor.WithStorages(storages...),
			imagor.WithProcessors(
				vipsprocessor.New(
					vipsprocessor.WithDisableBlur(*vipsDisableBlur),
					vipsprocessor.WithDisableFilters(*vipsDisableFilters),
					vipsprocessor.WithLogger(logger),
					vipsprocessor.WithDebug(*debug),
				),
			),
			imagor.WithSecret(*imagorSecret),
			imagor.WithRequestTimeout(*imagorRequestTimeout),
			imagor.WithSaveTimeout(*imagorSaveTimeout),
			imagor.WithUnsafe(*imagorUnsafe),
			imagor.WithLogger(logger),
			imagor.WithDebug(*debug),
		),
		server.WithAddress(*serverAddress),
		server.WithPort(*port),
		server.WithPathPrefix(*serverPathPrefix),
		server.WithCORS(*serverCORS),
		server.WithLogger(logger),
		server.WithDebug(*debug),
	).Run()
}
