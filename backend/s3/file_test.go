package s3

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v3"
	"github.com/c2fo/vfs/v3/mocks"
)

type fileTestSuite struct {
	suite.Suite
}

var (
	s3apiMock *mocks.S3API
	fs        FileSystem
	testFile  vfs.File
)

func (ts *fileTestSuite) SetupTest() {
	var err error
	s3apiMock = &mocks.S3API{}
	fs = FileSystem{client: s3apiMock}
	testFile, err = fs.NewFile("bucket", "some/path/to/file.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func (ts *fileTestSuite) TestRead() {
	contents := "hello world!"
	s3apiMock.On("GetObject", mock.AnythingOfType("*s3.GetObjectInput")).Return(&s3.GetObjectOutput{
		Body: nopCloser{bytes.NewBufferString(contents)},
	}, nil)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)

	file, err := fs.NewFile("bucket", "/some/path/file.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file")
	}

	var localFile = bytes.NewBuffer([]byte{})

	_, copyErr := io.Copy(localFile, file)
	assert.NoError(ts.T(), copyErr, "no error expected")
	closeErr := file.Close()
	assert.NoError(ts.T(), closeErr, "no error expected")

	s3apiMock.AssertExpectations(ts.T())
	ts.Equal(localFile.String(), contents, "Copying an s3 file to a buffer should fill buffer with file's contents")
}

// TODO: Write on Close() (actual s3 calls wait until file is closed to be made.)
func (ts *fileTestSuite) TestWrite() {
	file, err := fs.NewFile("bucket", "hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file")
	}

	contents := []byte("Hello world!")
	count, err := file.Write(contents)

	ts.Equal(len(contents), count, "Returned count of bytes written should match number of bytes passed to Write.")
	ts.Nil(err, "Error should be nil when calling Write")
}

func (ts *fileTestSuite) TestSeek() {
	contents := "hello world!"
	file, err := fs.NewFile("bucket", "hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file")
	}

	s3apiMock.On("GetObject", mock.AnythingOfType("*s3.GetObjectInput")).Return(&s3.GetObjectOutput{
		Body: nopCloser{bytes.NewBufferString(contents)},
	}, nil)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)

	_, seekErr := file.Seek(6, 0)
	assert.NoError(ts.T(), seekErr, "no error expected")

	var localFile = bytes.NewBuffer([]byte{})

	_, copyErr := io.Copy(localFile, file)
	assert.NoError(ts.T(), copyErr, "no error expected")

	ts.Equal("world!", localFile.String(), "Seeking should download the file and move the cursor as expected")

	localFile = bytes.NewBuffer([]byte{})
	_, seekErr2 := file.Seek(0, 0)
	assert.NoError(ts.T(), seekErr2, "no error expected")

	_, copyErr2 := io.Copy(localFile, file)
	assert.NoError(ts.T(), copyErr2, "no error expected")
	ts.Equal(contents, localFile.String(), "Subsequent calls to seek work on temp file as expected")

	closeErr := file.Close()
	assert.NoError(ts.T(), closeErr, "no error expected")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestGetLocation() {
	file, err := fs.NewFile("bucket", "path/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	location := file.Location()
	ts.Equal("s3", location.FileSystem().Scheme(), "Should initialize location with FS underlying file.")
	ts.Equal("/path/", location.Path(), "Should initialize path with the location of the file.")
	ts.Equal("bucket", location.Volume(), "Should initialize bucket with the bucket containing the file.")
}

func (ts *fileTestSuite) TestExists() {
	file, err := fs.NewFile("bucket", "/path/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)

	exists, err := file.Exists()
	ts.True(exists, "Should return true for exists based on this setup")
	ts.Nil(err, "Shouldn't return an error when exists is true")
}

func (ts *fileTestSuite) TestNotExists() {
	file, err := fs.NewFile("bucket", "/path/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, awserr.New(s3.ErrCodeNoSuchKey, "key doesn't exist", nil))

	exists, err := file.Exists()
	ts.False(exists, "Should return false for exists based on setup")
	ts.Nil(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestCopyToFile() {
	targetFile := &File{
		fileSystem: &FileSystem{
			client: s3apiMock,
		},
		bucket: "TestBucket",
		key:    "testKey.txt",
	}

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(&s3.CopyObjectOutput{}, nil)

	err := testFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestEmptyCopyToFile() {
	targetFile := &mocks.File{}
	targetFile.On("Write", mock.Anything).Return(0, nil)
	targetFile.On("Close").Return(nil)

	expectedSize := int64(0)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{ContentLength: &expectedSize}, nil, nil)

	err := testFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")
	s3apiMock.AssertExpectations(ts.T())

	// Assert that file was still written to and closed when the reader size is 0 bytes.
	targetFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile() {
	targetFile := &File{
		fileSystem: &FileSystem{
			client: s3apiMock,
		},
		bucket: "TestBucket",
		key:    "testKey.txt",
	}

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(&s3.CopyObjectOutput{}, nil)
	s3apiMock.On("DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)

	err := testFile.MoveToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile_CopyError() {
	targetFile := &File{
		fileSystem: &FileSystem{
			client: s3apiMock,
		},
		bucket: "TestBucket",
		key:    "testKey.txt",
	}

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(nil, errors.New("some copy error"))

	err := testFile.MoveToFile(targetFile)
	ts.NotNil(err, "Error shouldn't be returned from successful call to CopyToFile")
	s3apiMock.AssertNotCalled(ts.T(), "DeleteObject", mock.Anything)
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestCopyToLocation() {
	expectedText := "hello world!"
	otherFs := new(mocks.FileSystem)
	otherFs.On("Scheme", mock.Anything).Return("")
	otherFile := new(mocks.File)
	location := new(mocks.Location)
	location.On("FileSystem", mock.Anything).Return(otherFs)
	location.On("Volume").Return("bucket")

	s3apiMock.On("GetObject", mock.AnythingOfType("*s3.GetObjectInput")).Return(&s3.GetObjectOutput{
		Body: nopCloser{bytes.NewBufferString(expectedText)},
	}, nil)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)
	file, err := fs.NewFile("bucket", "/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}

	defer func() {
		closeErr := file.Close()
		assert.NoError(ts.T(), closeErr, "no error expected")
	}()

	otherFs.On("Scheme").Return("")
	otherFs.On("NewFile", mock.Anything, mock.Anything).Return(otherFile, nil)
	otherFile.On("Write", mock.Anything).Return(len(expectedText), nil)
	otherFile.On("Close", mock.Anything).Return(nil)
	location.On("Path", mock.Anything).Return("/someother/path")
	_, err = file.CopyToLocation(location)
	if err != nil {
		ts.Fail("Shouldn't return error for this call to CopyToLocation")
	}

	location.AssertExpectations(ts.T())
	otherFs.AssertExpectations(ts.T())
	otherFs.AssertCalled(ts.T(), "NewFile", "bucket", "/someother/path/hello.txt")
	otherFile.AssertExpectations(ts.T())
	otherFile.AssertCalled(ts.T(), "Write", []byte(expectedText))
}

func (ts *fileTestSuite) TestCopyToLocationWithinS3() {
	otherFs := new(mocks.FileSystem)
	otherFs.On("Scheme", mock.Anything).Return(Scheme)
	otherFile := new(mocks.File)
	location := new(mocks.Location)
	location.On("FileSystem", mock.Anything).Return(otherFs)
	location.On("Path", mock.Anything).Return("new/file/path").Twice()
	location.On("Volume", mock.Anything).Return("newBucket").Twice()

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(&s3.CopyObjectOutput{}, nil)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)

	file, err := fs.NewFile("bucket", "/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}

	otherFs.On("Scheme").Return(Scheme)
	otherFs.On("NewFile", "newBucket", "new/file/path/hello.txt").Return(otherFile, nil)

	_, err = file.CopyToLocation(location)
	assert.NoError(ts.T(), err, "no error expected")

	closeErr := file.Close()
	assert.NoError(ts.T(), closeErr, "no error expected")

	s3apiMock.AssertExpectations(ts.T())
	otherFs.AssertExpectations(ts.T())
	location.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToLocation() {
	// Copy portion tested through CopyToLocation, just need to test whether or not Delete happens
	// in addition to CopyToLocation
	otherFs := new(mocks.FileSystem)
	otherFs.On("Scheme", mock.Anything).Return(Scheme)
	otherFile := new(mocks.File)
	location := new(mocks.Location)
	location.On("FileSystem", mock.Anything).Return(otherFs)
	location.On("Path", mock.Anything).Return("new/file/path").Twice()
	location.On("Volume", mock.Anything).Return("newBucket").Twice()

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(&s3.CopyObjectOutput{}, nil)
	s3apiMock.On("DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)

	file, err := fs.NewFile("bucket", "/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}

	defer func() {
		closeErr := file.Close()
		assert.NoError(ts.T(), closeErr, "no error expected")
	}()

	otherFs.On("Scheme").Return(Scheme)
	otherFs.On("NewFile", "newBucket", "new/file/path/hello.txt").Return(otherFile, nil)

	_, err = file.MoveToLocation(location)
	assert.NoError(ts.T(), err, "no error expected")

	s3apiMock.AssertExpectations(ts.T())
	otherFs.AssertExpectations(ts.T())
	location.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToLocationFail() {
	// If CopyToLocation fails we need to ensure DeleteObject isn't called.
	otherFs := new(mocks.FileSystem)
	otherFs.On("Scheme", mock.Anything).Return(Scheme).Once()
	location := new(mocks.Location)
	location.On("FileSystem", mock.Anything).Return(otherFs).Once()
	location.On("Path", mock.Anything).Return("new/file/path").Once()
	location.On("Volume", mock.Anything).Return("newBucket").Once()

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(nil, errors.New("didn't copy, oh noes"))
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)

	file, err := fs.NewFile("bucket", "/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}

	_, merr := file.MoveToLocation(location)
	assert.Error(ts.T(), merr, "MoveToLocation error not expected")

	closeErr := file.Close()
	assert.NoError(ts.T(), closeErr, "no close error expected")

	s3apiMock.AssertExpectations(ts.T())
	s3apiMock.AssertNotCalled(ts.T(), "DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput"))
	otherFs.AssertExpectations(ts.T())
	location.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestDelete() {
	s3apiMock.On("DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)
	err := testFile.Delete()
	ts.Nil(err, "Successful delete should not return an error.")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestLastModified() {
	now := time.Now()
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{
		LastModified: &now,
	}, nil)
	modTime, err := testFile.LastModified()
	ts.Nil(err, "Error should be nil when correctly returning time of object.")
	ts.Equal(&now, modTime, "Returned time matches expected LastModified time.")
}

func (ts *fileTestSuite) TestLastModifiedFail() {
	//setup error on HEAD
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(nil,
		errors.New("boom"))
	m, e := testFile.LastModified()
	ts.Error(e, "got error as exepcted")
	ts.Nil(m, "nil ModTime returned")
}

func (ts *fileTestSuite) TestName() {
	ts.Equal("file.txt", testFile.Name(), "Name should return just the name of the file.")
}

func (ts *fileTestSuite) TestSize() {
	contentLength := int64(100)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{
		ContentLength: &contentLength,
	}, nil)

	size, err := testFile.Size()
	ts.Nil(err, "Error should be nil when requesting size for file that exists.")
	ts.Equal(uint64(100), size, "Size should return the ContentLength value from s3 HEAD request.")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestPath() {
	ts.Equal("/some/path/to/file.txt", testFile.Path(), "Should return file.key (with leading slash)")
}

func (ts *fileTestSuite) TestURI() {
	s3apiMock = &mocks.S3API{}
	fs = FileSystem{client: s3apiMock}
	file, _ := fs.NewFile("mybucket", "/some/file/test.txt")
	expected := "s3://mybucket/some/file/test.txt"
	ts.Equal(expected, file.URI(), "%s does not match %s", file.URI(), expected)
}

func (ts *fileTestSuite) TestStringer() {
	fs = FileSystem{client: &mocks.S3API{}}
	file, _ := fs.NewFile("mybucket", "/some/file/test.txt")
	ts.Equal("s3://mybucket/some/file/test.txt", file.String())
}

func (ts *fileTestSuite) TestUploadInput() {
	fs = FileSystem{client: &mocks.S3API{}}
	file, _ := fs.NewFile("mybucket", "/some/file/test.txt")
	ts.Equal("AES256", *uploadInput(file.(*File)).ServerSideEncryption, "sse was set")
	ts.Equal("some/file/test.txt", *uploadInput(file.(*File)).Key, "key was set")
	ts.Equal("mybucket", *uploadInput(file.(*File)).Bucket, "bucket was set")
}

func (ts *fileTestSuite) TestNewFile() {
	// fs is nil
	_, err := newFile(nil, "", "")
	ts.Errorf(err, "non-nil s3.fileSystem pointer is required")

	fs := &FileSystem{}
	// bucket is ""
	_, err = newFile(fs, "", "asdf")
	ts.Errorf(err, "non-empty strings for bucket and key are required")
	// key is ""
	_, err = newFile(fs, "asdf", "")
	ts.Errorf(err, "non-empty strings for bucket and key are required")

	//
	file, err := newFile(fs, "mybucket", "/path/to/key")
	ts.NoError(err, "newFile should succeed")
	ts.IsType(&File{}, file, "newFile returned a File struct")
	ts.Equal("mybucket", file.bucket)
	ts.Equal("path/to/key", file.key)

}

func TestFile(t *testing.T) {
	suite.Run(t, new(fileTestSuite))
}
