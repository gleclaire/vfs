package os

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/c2fo/vfs/v3"
	"github.com/c2fo/vfs/v3/utils"
)

//File implements vfs.File interface for os fs.
type File struct {
	file     *os.File
	name     string
	location vfs.Location
}

// newFile initializer returns a pointer to File.
func newFile(name string) (*File, error) {
	fileName := filepath.Base(name)
	fullPath, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}

	fullPath = filepath.Dir(fullPath)

	fullPath = utils.AddTrailingSlash(fullPath)

	location := Location{fileSystem: &FileSystem{}, name: fullPath}
	return &File{name: fileName, location: &location}, nil
}

// Delete unlinks the file returning any error or nil.
func (f *File) Delete() error {
	err := os.Remove(f.Path())
	if err == nil {
		f.file = nil
	}
	return err
}

// LastModified returns the timestamp of the file's mtime or error, if any.
func (f *File) LastModified() (*time.Time, error) {
	stats, err := os.Stat(f.Path())
	if err != nil {
		return nil, err
	}

	statsTime := stats.ModTime()
	return &statsTime, err
}

// Name returns the full name of the File relative to Location.Name().
func (f *File) Name() string {
	return f.name
}

// Path returns the the path of the File relative to Location.Name().
func (f *File) Path() string {
	return filepath.Join(f.location.Path(), f.name)
}

// Size returns the size (in bytes) of the File or any error.
func (f *File) Size() (uint64, error) {
	stats, err := os.Stat(f.Path())
	if err != nil {
		return 0, err
	}

	return uint64(stats.Size()), err
}

// Close implements the io.Closer interface, closing the underlying *os.File. its an error, if any.
func (f *File) Close() error {
	if f.file == nil {
		// Do nothing on files that were never referenced
		return nil
	}

	err := f.file.Close()
	if err == nil {
		f.file = nil
	}
	return err
}

// Read implements the io.Reader interface.  It returns the bytes read and an error, if any.
func (f *File) Read(p []byte) (int, error) {
	if exists, err := f.Exists(); err != nil {
		return 0, err
	} else if !exists {
		return 0, fmt.Errorf("failed to read. File does not exist at %s", f)
	}

	file, err := f.openFile()
	if err != nil {
		return 0, err
	}

	return file.Read(p)
}

//Seek implements the io.Seeker interface.  It accepts an offset and "whench" where 0 means relative to the origin of
// the file, 1 means relative to the current offset, and 2 means relative to the end.  It returns the new offset and
// an error, if any.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	file, err := f.openFile()
	if err != nil {
		return 0, err
	}

	return file.Seek(offset, whence)
}

// Exists true if the file exists on the filesystem, otherwise false, and an error, if any.
func (f *File) Exists() (bool, error) {
	_, err := os.Stat(f.Path())
	if err != nil {
		//file does not exist
		if os.IsNotExist(err) {
			return false, nil
		}
		//some other error
		return false, err
	}
	//file exists
	return true, nil
}

//Write implements the io.Writer interface.  It accepts a slice of bytes and returns the number of bytes written and an error, if any.
func (f *File) Write(p []byte) (n int, err error) {
	file, err := f.openFile()
	if err != nil {
		return 0, err
	}
	return file.Write(p)
}

// Location returns the underlying os.Location.
func (f *File) Location() vfs.Location {
	return f.location
}

// MoveToFile move a file. It accepts a target vfs.File and returns an error, if any.
//TODO we might consider using os.Rename() for efficiency when target.Location().FileSystem().Scheme equals f.Location().FileSystem().Scheme()
func (f *File) MoveToFile(target vfs.File) error {
	_, err := f.copyWithName(target.Name(), target.Location())
	if err != nil {
		return err
	}

	err = f.Delete()
	return err
}

// MoveToLocation moves a file to a new Location. It accepts a target vfs.Location and returns a vfs.File and an error, if any.
//TODO we might consider using os.Rename() for efficiency when location.FileSystem().Scheme() equals f.Location().FileSystem().Scheme()
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	_, err := f.copyWithName(f.name, location)
	if err != nil {
		return f, err
	}

	delErr := f.Delete()
	if delErr != nil {
		return f, delErr
	}
	f.location = location
	return f, nil
}

// CopyToFile copies the file to a new File.  It accepts a vfs.File and returns an error, if any.
func (f *File) CopyToFile(target vfs.File) error {
	_, err := f.copyWithName(target.Name(), target.Location())
	return err
}

// CopyToLocation copies existing File to new Location with the same name.  It accepts a vfs.Location and returns a vfs.File and error, if any.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	return f.copyWithName(f.name, location)
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	return utils.GetFileURI(f)
}

// String implement fmt.Stringer, returning the file's URI as the default string.
func (f *File) String() string {
	return f.URI()
}

func (f *File) copyWithName(name string, location vfs.Location) (vfs.File, error) {
	newFile, err := location.FileSystem().NewFile(location.Volume(), path.Join(location.Path(), name))
	if err != nil {
		return nil, err
	}

	if err := utils.TouchCopy(newFile, f); err != nil {
		return nil, err
	}
	fCloseErr := f.Close()
	if fCloseErr != nil {
		return nil, fCloseErr
	}

	newFileCloseErr := newFile.Close()
	if newFileCloseErr != nil {
		return nil, newFileCloseErr
	}
	return newFile, nil
}

func (f *File) openFile() (*os.File, error) {
	if f.file != nil {
		return f.file, nil
	}

	// Ensure the path exists before opening the file, NoOp if dir already exists.
	var fileMode os.FileMode = 0666
	if err := os.MkdirAll(f.location.Path(), os.ModeDir|0777); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(f.Path(), os.O_RDWR|os.O_CREATE, fileMode)
	f.file = file
	return file, err
}
