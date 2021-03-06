// Package all imports all VFS implementations.
package all

import (
	_ "github.com/c2fo/vfs/v3/backend/gs" // register gs backend
	_ "github.com/c2fo/vfs/v3/backend/os" // register os backend
	_ "github.com/c2fo/vfs/v3/backend/s3" // register s3 backend
)
