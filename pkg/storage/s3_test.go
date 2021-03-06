// Copyright 2020 PingCAP, Inc. Licensed under Apache-2.0.

package storage_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang/mock/gomock"
	. "github.com/pingcap/check"
	"github.com/pingcap/errors"
	"github.com/pingcap/kvproto/pkg/backup"

	"github.com/Orion7r/pr/pkg/mock"
	. "github.com/Orion7r/pr/pkg/storage"
)

type s3Suite struct {
	controller *gomock.Controller
	s3         *mock.MockS3API
	storage    *S3Storage
}

var _ = Suite(&s3Suite{})

// FIXME: Cannot use the real SetUpTest/TearDownTest to set up the mock
// otherwise the mock error will be ignored.

func (s *s3Suite) setUpTest(c gomock.TestReporter) {
	s.controller = gomock.NewController(c)
	s.s3 = mock.NewMockS3API(s.controller)
	s.storage = NewS3StorageForTest(
		s.s3,
		&backup.S3{
			Region:       "us-west-2",
			Bucket:       "bucket",
			Prefix:       "prefix/",
			Acl:          "acl",
			Sse:          "sse",
			StorageClass: "sc",
		},
	)
}

func (s *s3Suite) tearDownTest() {
	s.controller.Finish()
}

func (s *s3Suite) TestApply(c *C) {
	type testcase struct {
		name      string
		options   S3BackendOptions
		errMsg    string
		errReturn bool
	}
	testFn := func(test *testcase, c *C) {
		c.Log(test.name)
		_, err := ParseBackend("s3://bucket2/prefix/", &BackendOptions{S3: test.options})
		if test.errReturn {
			c.Assert(err, ErrorMatches, test.errMsg)
		} else {
			c.Assert(err, IsNil)
		}
	}
	tests := []testcase{
		{
			name: "access_key not found",
			options: S3BackendOptions{
				Region:          "us-west-2",
				SecretAccessKey: "cd",
			},
			errMsg:    "access_key not found.*",
			errReturn: true,
		},
		{
			name: "secret_access_key not found",
			options: S3BackendOptions{
				Region:    "us-west-2",
				AccessKey: "ab",
			},
			errMsg:    "secret_access_key not found.*",
			errReturn: true,
		},
		{
			name: "scheme not found",
			options: S3BackendOptions{
				Endpoint: "12345",
			},
			errMsg:    "scheme not found in endpoint.*",
			errReturn: true,
		},
		{
			name: "host not found",
			options: S3BackendOptions{
				Endpoint: "http:12345",
			},
			errMsg:    "host not found in endpoint.*",
			errReturn: true,
		},
		{
			name: "invalid endpoint",
			options: S3BackendOptions{
				Endpoint: "!http:12345",
			},
			errMsg:    "parse (.*)!http:12345(.*): first path segment in URL cannot contain colon.*",
			errReturn: true,
		},
	}
	for i := range tests {
		testFn(&tests[i], c)
	}
}

func (s *s3Suite) TestApplyUpdate(c *C) {
	type testcase struct {
		name    string
		options S3BackendOptions
		setEnv  bool
		s3      *backup.S3
	}
	testFn := func(test *testcase, c *C) {
		c.Log(test.name)
		if test.setEnv {
			os.Setenv("AWS_ACCESS_KEY_ID", "ab")
			os.Setenv("AWS_SECRET_ACCESS_KEY", "cd")
		}
		u, err := ParseBackend("s3://bucket/prefix/", &BackendOptions{S3: test.options})
		s3 := u.GetS3()
		c.Assert(err, IsNil)
		c.Assert(s3, DeepEquals, test.s3)
	}

	tests := []testcase{
		{
			name: "no region and no endpoint",
			options: S3BackendOptions{
				Region:   "",
				Endpoint: "",
			},
			s3: &backup.S3{
				Region: "us-east-1",
				Bucket: "bucket",
				Prefix: "prefix",
			},
		},
		{
			name: "no endpoint",
			options: S3BackendOptions{
				Region: "us-west-2",
			},
			s3: &backup.S3{
				Region: "us-west-2",
				Bucket: "bucket",
				Prefix: "prefix",
			},
		},
		{
			name: "https endpoint",
			options: S3BackendOptions{
				Endpoint: "https://s3.us-west-2",
			},
			s3: &backup.S3{
				Region:   "us-east-1",
				Endpoint: "https://s3.us-west-2",
				Bucket:   "bucket",
				Prefix:   "prefix",
			},
		},
		{
			name: "http endpoint",
			options: S3BackendOptions{
				Endpoint: "http://s3.us-west-2",
			},
			s3: &backup.S3{
				Region:   "us-east-1",
				Endpoint: "http://s3.us-west-2",
				Bucket:   "bucket",
				Prefix:   "prefix",
			},
		},
		{
			name: "ceph provider",
			options: S3BackendOptions{
				Region:         "us-west-2",
				ForcePathStyle: true,
				Provider:       "ceph",
			},
			s3: &backup.S3{
				Region:         "us-west-2",
				ForcePathStyle: true,
				Bucket:         "bucket",
				Prefix:         "prefix",
			},
		},
		{
			name: "ali provider",
			options: S3BackendOptions{
				Region:         "us-west-2",
				ForcePathStyle: true,
				Provider:       "alibaba",
			},
			s3: &backup.S3{
				Region:         "us-west-2",
				ForcePathStyle: false,
				Bucket:         "bucket",
				Prefix:         "prefix",
			},
		},
		{
			name: "netease provider",
			options: S3BackendOptions{
				Region:         "us-west-2",
				ForcePathStyle: true,
				Provider:       "netease",
			},
			s3: &backup.S3{
				Region:         "us-west-2",
				ForcePathStyle: false,
				Bucket:         "bucket",
				Prefix:         "prefix",
			},
		},
		{
			name: "useAccelerateEndpoint",
			options: S3BackendOptions{
				Region:                "us-west-2",
				ForcePathStyle:        true,
				UseAccelerateEndpoint: true,
			},
			s3: &backup.S3{
				Region:         "us-west-2",
				ForcePathStyle: false,
				Bucket:         "bucket",
				Prefix:         "prefix",
			},
		},
		{
			name: "keys",
			options: S3BackendOptions{
				Region:          "us-west-2",
				AccessKey:       "ab",
				SecretAccessKey: "cd",
			},
			s3: &backup.S3{
				Region:          "us-west-2",
				AccessKey:       "ab",
				SecretAccessKey: "cd",
				Bucket:          "bucket",
				Prefix:          "prefix",
			},
			setEnv: true,
		},
	}
	for i := range tests {
		testFn(&tests[i], c)
	}
}

func (s *s3Suite) TestS3Storage(c *C) {
	type testcase struct {
		name           string
		s3             *backup.S3
		errReturn      bool
		hackCheck      bool
		sendCredential bool
	}
	testFn := func(test *testcase, c *C) {
		c.Log(test.name)
		ctx := aws.BackgroundContext()
		s3 := &backup.StorageBackend{
			Backend: &backup.StorageBackend_S3{
				S3: test.s3,
			},
		}
		_, err := New(ctx, s3, &ExternalStorageOptions{
			SendCredentials: test.sendCredential,
			SkipCheckPath:   test.hackCheck,
		})
		if test.errReturn {
			c.Assert(err, NotNil)
			return
		}
		c.Assert(err, IsNil)
		if test.sendCredential {
			c.Assert(len(test.s3.AccessKey), Greater, 0)
		} else {
			c.Assert(len(test.s3.AccessKey), Equals, 0)
		}
	}
	tests := []testcase{
		{
			name: "no region and endpoint",
			s3: &backup.S3{
				Region:   "",
				Endpoint: "",
				Bucket:   "bucket",
				Prefix:   "prefix",
			},
			errReturn:      true,
			sendCredential: true,
		},
		{
			name: "no region",
			s3: &backup.S3{
				Region:   "",
				Endpoint: "http://10.1.2.3",
				Bucket:   "bucket",
				Prefix:   "prefix",
			},
			errReturn:      true,
			sendCredential: true,
		},
		{
			name: "no endpoint",
			s3: &backup.S3{
				Region:   "us-west-2",
				Endpoint: "",
				Bucket:   "bucket",
				Prefix:   "prefix",
			},
			errReturn:      true,
			sendCredential: true,
		},
		{
			name: "no region",
			s3: &backup.S3{
				Region:   "",
				Endpoint: "http://10.1.2.3",
				Bucket:   "bucket",
				Prefix:   "prefix",
			},
			errReturn:      false,
			hackCheck:      true,
			sendCredential: true,
		},
		{
			name: "normal region",
			s3: &backup.S3{
				Region:   "us-west-2",
				Endpoint: "",
				Bucket:   "bucket",
				Prefix:   "prefix",
			},
			errReturn:      false,
			hackCheck:      true,
			sendCredential: true,
		},
		{
			name: "keys configured explicitly",
			s3: &backup.S3{
				Region:          "us-west-2",
				AccessKey:       "ab",
				SecretAccessKey: "cd",
				Bucket:          "bucket",
				Prefix:          "prefix",
			},
			errReturn:      false,
			hackCheck:      true,
			sendCredential: true,
		},
		{
			name: "no access key",
			s3: &backup.S3{
				Region:          "us-west-2",
				SecretAccessKey: "cd",
				Bucket:          "bucket",
				Prefix:          "prefix",
			},
			errReturn:      false,
			hackCheck:      true,
			sendCredential: true,
		},
		{
			name: "no secret access key",
			s3: &backup.S3{
				Region:    "us-west-2",
				AccessKey: "ab",
				Bucket:    "bucket",
				Prefix:    "prefix",
			},
			errReturn:      false,
			hackCheck:      true,
			sendCredential: true,
		},
		{
			name: "no secret access key",
			s3: &backup.S3{
				Region:    "us-west-2",
				AccessKey: "ab",
				Bucket:    "bucket",
				Prefix:    "prefix",
			},
			errReturn:      false,
			hackCheck:      true,
			sendCredential: false,
		},
	}
	for i := range tests {
		testFn(&tests[i], c)
	}
}

func (s *s3Suite) TestS3URI(c *C) {
	backend, err := ParseBackend("s3://bucket/prefix/", nil)
	c.Assert(err, IsNil)
	storage, err := New(context.Background(), backend, &ExternalStorageOptions{SkipCheckPath: true})
	c.Assert(err, IsNil)
	c.Assert(storage.URI(), Equals, "s3://bucket/prefix/")
}

func (s *s3Suite) TestS3Range(c *C) {
	contentRange := "bytes 0-9/443"
	ri, err := ParseRangeInfo(&contentRange)
	c.Assert(err, IsNil)
	c.Assert(ri, Equals, RangeInfo{Start: 0, End: 9, Size: 443})

	_, err = ParseRangeInfo(nil)
	c.Assert(err, ErrorMatches, "ContentRange is empty.*")

	badRange := "bytes "
	_, err = ParseRangeInfo(&badRange)
	c.Assert(err, ErrorMatches, "invalid content range: 'bytes '.*")
}

// TestWriteNoError ensures the Write API issues a PutObject request and wait
// until the object is available in the S3 bucket.
func (s *s3Suite) TestWriteNoError(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	putCall := s.s3.EXPECT().
		PutObjectWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
			c.Assert(aws.StringValue(input.Bucket), Equals, "bucket")
			c.Assert(aws.StringValue(input.Key), Equals, "prefix/file")
			c.Assert(aws.StringValue(input.ACL), Equals, "acl")
			c.Assert(aws.StringValue(input.ServerSideEncryption), Equals, "sse")
			c.Assert(aws.StringValue(input.StorageClass), Equals, "sc")
			body, err := ioutil.ReadAll(input.Body)
			c.Assert(err, IsNil)
			c.Assert(body, DeepEquals, []byte("test"))
			return &s3.PutObjectOutput{}, nil
		})
	s.s3.EXPECT().
		WaitUntilObjectExistsWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.HeadObjectInput) error {
			c.Assert(aws.StringValue(input.Bucket), Equals, "bucket")
			c.Assert(aws.StringValue(input.Key), Equals, "prefix/file")
			return nil
		}).
		After(putCall)

	err := s.storage.Write(ctx, "file", []byte("test"))
	c.Assert(err, IsNil)
}

// TestReadNoError ensures the Read API issues a GetObject request and correctly
// read the entire body.
func (s *s3Suite) TestReadNoError(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	s.s3.EXPECT().
		GetObjectWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			c.Assert(aws.StringValue(input.Bucket), Equals, "bucket")
			c.Assert(aws.StringValue(input.Key), Equals, "prefix/file")
			return &s3.GetObjectOutput{
				Body: ioutil.NopCloser(bytes.NewReader([]byte("test"))),
			}, nil
		})

	content, err := s.storage.Read(ctx, "file")
	c.Assert(err, IsNil)
	c.Assert(content, DeepEquals, []byte("test"))
}

// TestFileExistsNoError ensures the FileExists API issues a HeadObject request
// and reports a file exists.
func (s *s3Suite) TestFileExistsNoError(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	s.s3.EXPECT().
		HeadObjectWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
			c.Assert(aws.StringValue(input.Bucket), Equals, "bucket")
			c.Assert(aws.StringValue(input.Key), Equals, "prefix/file")
			return &s3.HeadObjectOutput{}, nil
		})

	exists, err := s.storage.FileExists(ctx, "file")
	c.Assert(err, IsNil)
	c.Assert(exists, IsTrue)
}

// TestFileExistsNoSuckKey ensures FileExists API reports file missing if S3's
// HeadObject request replied NoSuchKey.
func (s *s3Suite) TestFileExistsMissing(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	s.s3.EXPECT().
		HeadObjectWithContext(ctx, gomock.Any()).
		Return(nil, awserr.New(s3.ErrCodeNoSuchKey, "no such key", nil))

	exists, err := s.storage.FileExists(ctx, "file-missing")
	c.Assert(err, IsNil)
	c.Assert(exists, IsFalse)
}

// TestWriteError checks that a PutObject error is propagated.
func (s *s3Suite) TestWriteError(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	expectedErr := awserr.New(s3.ErrCodeNoSuchBucket, "no such bucket", nil)

	s.s3.EXPECT().
		PutObjectWithContext(ctx, gomock.Any()).
		Return(nil, expectedErr)

	err := s.storage.Write(ctx, "file2", []byte("test"))
	c.Assert(err, ErrorMatches, `\Q`+expectedErr.Error()+`\E`)
}

// TestWriteError checks that a GetObject error is propagated.
func (s *s3Suite) TestReadError(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	expectedErr := awserr.New(s3.ErrCodeNoSuchKey, "no such key", nil)

	s.s3.EXPECT().
		GetObjectWithContext(ctx, gomock.Any()).
		Return(nil, expectedErr)

	_, err := s.storage.Read(ctx, "file-missing")
	c.Assert(err, ErrorMatches, `\Q`+expectedErr.Error()+`\E`)
}

// TestFileExistsError checks that a HeadObject error is propagated.
func (s *s3Suite) TestFileExistsError(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	expectedErr := errors.New("just some unrelated error")

	s.s3.EXPECT().
		HeadObjectWithContext(ctx, gomock.Any()).
		Return(nil, expectedErr)

	_, err := s.storage.FileExists(ctx, "file3")
	c.Assert(err, ErrorMatches, `\Q`+expectedErr.Error()+`\E`)
}

// TestOpenAsBufio checks that we can open a file for reading via bufio.
func (s *s3Suite) TestOpenAsBufio(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	s.s3.EXPECT().
		GetObjectWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			c.Assert(aws.StringValue(input.Range), Equals, "bytes=0-")
			return &s3.GetObjectOutput{
				Body:         ioutil.NopCloser(bytes.NewReader([]byte("plain text\ncontent"))),
				ContentRange: aws.String("bytes 0-17/18"),
			}, nil
		})

	reader, err := s.storage.Open(ctx, "plain-text-file")
	c.Assert(err, IsNil)
	defer c.Assert(reader.Close(), IsNil)
	bufReader := bufio.NewReaderSize(reader, 5)
	content, err := bufReader.ReadString('\n')
	c.Assert(err, IsNil)
	c.Assert(content, Equals, "plain text\n")
	content, err = bufReader.ReadString('\n')
	c.Assert(err, ErrorMatches, "EOF")
	c.Assert(content, Equals, "content")
}

// alphabetReader is used in TestOpenReadSlowly. This Reader produces a single
// upper case letter one Read() at a time.
type alphabetReader struct{ character byte }

func (r *alphabetReader) Read(buf []byte) (int, error) {
	if r.character > 'Z' {
		return 0, io.EOF
	}
	if len(buf) == 0 {
		return 0, nil
	}
	buf[0] = r.character
	r.character++
	return 1, nil
}

func (r *alphabetReader) Close() error {
	return nil
}

// TestOpenReadSlowly checks that we can open a file for reading, even if the
// reader emits content one byte at a time.
func (s *s3Suite) TestOpenReadSlowly(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	s.s3.EXPECT().
		GetObjectWithContext(ctx, gomock.Any()).
		Return(&s3.GetObjectOutput{
			Body:         &alphabetReader{character: 'A'},
			ContentRange: aws.String("bytes 0-25/26"),
		}, nil)

	reader, err := s.storage.Open(ctx, "alphabets")
	c.Assert(err, IsNil)
	res, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)
	c.Assert(res, DeepEquals, []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
}

// TestOpenSeek checks that Seek is implemented correctly.
func (s *s3Suite) TestOpenSeek(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	someRandomBytes := make([]byte, 1000000)
	rand.Read(someRandomBytes)
	// ^ we just want some random bytes for testing, we don't care about its security.

	// The first call should serve the first 64 KiB.
	firstCall := s.s3.EXPECT().
		GetObjectWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			c.Assert(aws.StringValue(input.Range), Equals, "bytes=0-")
			return &s3.GetObjectOutput{
				Body:         ioutil.NopCloser(bytes.NewReader(someRandomBytes)),
				ContentRange: aws.String("bytes 0-999999/1000000"),
			}, nil
		})

	secondCall := s.s3.EXPECT().
		GetObjectWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			c.Assert(aws.StringValue(input.Range), Equals, "bytes=998000-")
			return &s3.GetObjectOutput{
				Body:         ioutil.NopCloser(bytes.NewReader(someRandomBytes[998000:])),
				ContentRange: aws.String("bytes 998000-999999/1000000"),
			}, nil
		}).
		After(firstCall)

	s.s3.EXPECT().
		GetObjectWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			c.Assert(aws.StringValue(input.Range), Equals, "bytes=990100-")
			return &s3.GetObjectOutput{
				Body:         ioutil.NopCloser(bytes.NewReader(someRandomBytes[990100:])),
				ContentRange: aws.String("bytes 990100-999999/1000000"),
			}, nil
		}).
		After(secondCall)

	reader, err := s.storage.Open(ctx, "random")
	c.Assert(err, IsNil)
	defer reader.Close()

	// first do some simple read...
	slice := make([]byte, 100)
	n, err := io.ReadFull(reader, slice)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 100)
	c.Assert(slice, DeepEquals, someRandomBytes[:100])

	// a short seek will not result in a different GetObject request.
	offset, err := reader.Seek(2000, io.SeekStart)
	c.Assert(err, IsNil)
	c.Assert(offset, Equals, int64(2000))
	n, err = io.ReadFull(reader, slice)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 100)
	c.Assert(slice, DeepEquals, someRandomBytes[2000:2100])

	// a long seek will perform a new GetObject request
	offset, err = reader.Seek(-2000, io.SeekEnd)
	c.Assert(err, IsNil)
	c.Assert(offset, Equals, int64(998000))
	n, err = io.ReadFull(reader, slice)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 100)
	c.Assert(slice, DeepEquals, someRandomBytes[998000:998100])

	// jumping backward should be fine, but would perform a new GetObject request.
	offset, err = reader.Seek(-8000, io.SeekCurrent)
	c.Assert(err, IsNil)
	c.Assert(offset, Equals, int64(990100))
	n, err = io.ReadFull(reader, slice)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 100)
	c.Assert(slice, DeepEquals, someRandomBytes[990100:990200])
}

// TestWalkDir checks WalkDir retrieves all directory content under a prefix.
func (s *s3Suite) TestWalkDir(c *C) {
	s.setUpTest(c)
	defer s.tearDownTest()
	ctx := aws.BackgroundContext()

	contents := []*s3.Object{
		{
			Key:  aws.String("prefix/sp/.gitignore"),
			Size: aws.Int64(437),
		},
		{
			Key:  aws.String("prefix/sp/01.jpg"),
			Size: aws.Int64(27499),
		},
		{
			Key:  aws.String("prefix/sp/1-f.png"),
			Size: aws.Int64(32507),
		},
		{
			Key:  aws.String("prefix/sp/10-f.png"),
			Size: aws.Int64(549735),
		},
		{
			Key:  aws.String("prefix/sp/10-t.jpg"),
			Size: aws.Int64(44151),
		},
	}

	// first call serve item #0, #1; second call #2, #3; third call #4.
	firstCall := s.s3.EXPECT().
		ListObjectsWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
			c.Assert(aws.StringValue(input.Bucket), Equals, "bucket")
			c.Assert(aws.StringValue(input.Prefix), Equals, "prefix/sp/")
			c.Assert(aws.StringValue(input.Marker), Equals, "")
			c.Assert(aws.Int64Value(input.MaxKeys), Equals, int64(2))
			c.Assert(aws.StringValue(input.Delimiter), Equals, "")
			return &s3.ListObjectsOutput{
				IsTruncated: aws.Bool(true),
				Contents:    contents[:2],
			}, nil
		})
	secondCall := s.s3.EXPECT().
		ListObjectsWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
			c.Assert(aws.StringValue(input.Marker), Equals, aws.StringValue(contents[1].Key))
			c.Assert(aws.Int64Value(input.MaxKeys), Equals, int64(2))
			return &s3.ListObjectsOutput{
				IsTruncated: aws.Bool(true),
				Contents:    contents[2:4],
			}, nil
		}).
		After(firstCall)
	s.s3.EXPECT().
		ListObjectsWithContext(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
			c.Assert(aws.StringValue(input.Marker), Equals, aws.StringValue(contents[3].Key))
			c.Assert(aws.Int64Value(input.MaxKeys), Equals, int64(2))
			return &s3.ListObjectsOutput{
				IsTruncated: aws.Bool(false),
				Contents:    contents[4:],
			}, nil
		}).
		After(secondCall)

	// Ensure we receive the items in order.
	i := 0
	err := s.storage.WalkDir(
		ctx,
		&WalkOption{SubDir: "sp", ListCount: 2},
		func(path string, size int64) error {
			comment := Commentf("index = %d", i)
			c.Assert("prefix/"+path, Equals, *contents[i].Key, comment)
			c.Assert(size, Equals, *contents[i].Size, comment)
			i++
			return nil
		},
	)
	c.Assert(err, IsNil)
	c.Assert(i, Equals, len(contents))
}
