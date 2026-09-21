package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
)

// maxBinary caps how much of an archive entry is read. A real strata binary
// is ~5 MB; anything near this size is not one.
const maxBinary = 200 << 20

// ExtractBinary returns the contents of the regular file named exactly name
// at the root of a release archive ("tar.gz" or "zip"). The archive is read
// in memory and nothing is written to disk from archive paths, and nested or
// "../" entries are never taken - so a hostile archive can't place a file
// anywhere.
func ExtractBinary(archive []byte, format, name string) ([]byte, error) {
	switch format {
	case "tar.gz":
		return fromTarGz(archive, name)
	case "zip":
		return fromZip(archive, name)
	}
	return nil, fmt.Errorf("unknown archive format %q", format)
}

func fromTarGz(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("reading archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading archive: %w", err)
		}
		if h.Typeflag == tar.TypeReg && path.Clean(h.Name) == name {
			return readCapped(tr)
		}
	}
	return nil, fmt.Errorf("archive has no %s at its root", name)
}

func fromZip(archive []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("reading archive: %w", err)
	}
	for _, f := range zr.File {
		if f.Mode().IsRegular() && path.Clean(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("reading archive: %w", err)
			}
			defer rc.Close()
			return readCapped(rc)
		}
	}
	return nil, fmt.Errorf("archive has no %s at its root", name)
}

func readCapped(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxBinary+1))
	if err != nil {
		return nil, fmt.Errorf("reading archive: %w", err)
	}
	if len(b) > maxBinary {
		return nil, fmt.Errorf("archive entry is over %d MB - not a strata binary", maxBinary>>20)
	}
	return b, nil
}
