package disk

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/atdiar/errors"
	"github.com/atdiar/xhttp/handlers/upload"
)

func Upload(ctx context.Context, u upload.Object) (n int64, rollbackFn func() error, err error) {
	var uploadname string
	var uploadpath string

	if u.Filename == "" {
		u.Filename = u.ID
	}

	if u.ChunksTotal > 2 {
		// todo set fieldname for the chunk and uploadpath for

		uploadname = u.Filename + "." + strconv.FormatInt(u.ChunkOffset, 10)
		uploadpath = filepath.Dir(filepath.Join("tmp/", u.EvalPath()))
	}

	err = os.MkdirAll(uploadpath, os.ModePerm)
	if err != nil {
		return 0, func() error { return nil }, err
	}

	file, err := os.Create(filepath.Join(uploadpath, uploadname))
	if err != nil {
		return 0, func() error { return nil }, err
	}

	n, err = io.Copy(file, u.Binary)
	if err != nil {
		return n, func() error { return os.Remove(filepath.Join(uploadpath, uploadname)) }, errors.New(file.Sync().Error()).Wraps(err)
	}

	return n, func() error { return os.Remove(filepath.Join(uploadpath, uploadname)) }, file.Sync()
}

func UuloadComplete(ctx context.Context, uploadid string) error {
	// merge chunks 
}
