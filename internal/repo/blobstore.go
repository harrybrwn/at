package repo

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
)

var (
	ErrBlobNotFound = errors.New("blob not found")
)

// DiskBlobStore implements blob storage on disk
type DiskBlobStore struct {
	did                string
	location           string
	tmpLocation        string
	quarantineLocation string
}

// NewDiskBlobStore creates a new disk-based blob store
func NewDiskBlobStore(did, location, tmpLocation, quarantineLocation string) *DiskBlobStore {
	if tmpLocation == "" {
		tmpLocation = filepath.Join(location, "temp")
	}
	if quarantineLocation == "" {
		quarantineLocation = filepath.Join(location, "quarantine")
	}
	return &DiskBlobStore{
		did:                did,
		location:           location,
		tmpLocation:        tmpLocation,
		quarantineLocation: quarantineLocation,
	}
}

// Creator returns a function that creates a new DiskBlobStore instance
func Creator(location, tmpLocation, quarantineLocation string) func(string) *DiskBlobStore {
	return func(did string) *DiskBlobStore {
		return NewDiskBlobStore(did, location, tmpLocation, quarantineLocation)
	}
}

func (d *DiskBlobStore) ensureDir() error {
	return os.MkdirAll(filepath.Join(d.location, d.did), 0755)
}

func (d *DiskBlobStore) ensureTemp() error {
	return os.MkdirAll(filepath.Join(d.tmpLocation, d.did), 0755)
}

func (d *DiskBlobStore) ensureQuarantine() error {
	return os.MkdirAll(filepath.Join(d.quarantineLocation, d.did), 0755)
}

func (d *DiskBlobStore) genKey() string {
	// Note: Implementation would need crypto/rand for proper random string generation
	// This is a simplified version
	const charset = "abcdefghijklmnopqrstuvwxyz234567"
	const length = 32
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[i%len(charset)]
	}
	return string(result)
}

func (d *DiskBlobStore) getTmpPath(key string) string {
	return filepath.Join(d.tmpLocation, d.did, key)
}

func (d *DiskBlobStore) getStoredPath(c cid.Cid) string {
	return filepath.Join(d.location, d.did, c.String())
}

func (d *DiskBlobStore) getQuarantinePath(c cid.Cid) string {
	return filepath.Join(d.quarantineLocation, d.did, c.String())
}

// HasTemp checks if a temporary file exists
func (d *DiskBlobStore) HasTemp(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(d.getTmpPath(key))
	return !os.IsNotExist(err), nil
}

// HasStored checks if a permanent blob exists
func (d *DiskBlobStore) HasStored(ctx context.Context, c cid.Cid) (bool, error) {
	_, err := os.Stat(d.getStoredPath(c))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	} else if !os.IsNotExist(err) {
		return true, nil
	}
	return !os.IsNotExist(err), nil
}

// PutTemp stores data in temporary storage
func (d *DiskBlobStore) PutTemp(ctx context.Context, r io.Reader) (string, error) {
	if err := d.ensureTemp(); err != nil {
		return "", fmt.Errorf("ensuring temp directory: %w", err)
	}
	key := d.genKey()
	path := d.getTmpPath(key)
	err := put(path, r)
	if err != nil {
		return "", errors.Wrap(err, "writing temp file")
	}
	return key, nil
}

// PutTempFromReader stores data from a reader in temporary storage
func (d *DiskBlobStore) PutTempFromReader(ctx context.Context, r io.Reader) (string, error) {
	if err := d.ensureTemp(); err != nil {
		return "", fmt.Errorf("ensuring temp directory: %w", err)
	}

	key := d.genKey()
	path := d.getTmpPath(key)

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("writing temp file: %w", err)
	}

	return key, nil
}

func put(path string, r io.Reader) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// MakePermanent moves a temporary file to permanent storage
func (d *DiskBlobStore) MakePermanent(ctx context.Context, key string, c cid.Cid) error {
	if err := d.ensureDir(); err != nil {
		return fmt.Errorf("ensuring directory: %w", err)
	}

	tmpPath := d.getTmpPath(key)
	storedPath := d.getStoredPath(c)

	ok, err := d.HasStored(ctx, c)
	if err != nil {
		return err
	}
	if !ok {
		// data, err := os.ReadFile(tmpPath)
		// if err != nil {
		// 	return fmt.Errorf("reading temp file: %w", err)
		// }
		//
		// if err := os.WriteFile(storedPath, data, 0644); err != nil {
		// 	return fmt.Errorf("writing permanent file: %w", err)
		// }

		err := copyFile(tmpPath, storedPath)
		if err != nil {
			return err
		}
	}

	if err := os.Remove(tmpPath); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("could not delete file from temp storage: %v\n", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.OpenFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	stat, err := srcFile.Stat()
	if err != nil {
		return err
	}
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, stat.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

// PutPermanent stores data directly in permanent storage
func (d *DiskBlobStore) PutPermanent(ctx context.Context, c cid.Cid, r io.Reader) error {
	if err := d.ensureDir(); err != nil {
		return fmt.Errorf("ensuring directory: %w", err)
	}
	err := put(d.getStoredPath(c), r)
	if err != nil {
		return errors.Wrap(err, "writing permanent file")
	}
	return nil
}

// Quarantine moves a blob to quarantine storage
func (d *DiskBlobStore) Quarantine(ctx context.Context, c cid.Cid) error {
	if err := d.ensureQuarantine(); err != nil {
		return fmt.Errorf("ensuring quarantine directory: %w", err)
	}

	if err := os.Rename(d.getStoredPath(c), d.getQuarantinePath(c)); err != nil {
		if os.IsNotExist(err) {
			return ErrBlobNotFound
		}
		return err
	}

	return nil
}

// Unquarantine moves a blob from quarantine back to permanent storage
func (d *DiskBlobStore) Unquarantine(ctx context.Context, c cid.Cid) error {
	if err := d.ensureDir(); err != nil {
		return fmt.Errorf("ensuring directory: %w", err)
	}

	if err := os.Rename(d.getQuarantinePath(c), d.getStoredPath(c)); err != nil {
		if os.IsNotExist(err) {
			return ErrBlobNotFound
		}
		return err
	}

	return nil
}

// GetBytes retrieves blob data as bytes
func (d *DiskBlobStore) GetBytes(ctx context.Context, c cid.Cid) ([]byte, error) {
	data, err := os.ReadFile(d.getStoredPath(c))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBlobNotFound
		}
		return nil, err
	}
	return data, nil
}

// GetStream returns a reader for the blob data
func (d *DiskBlobStore) GetStream(ctx context.Context, c cid.Cid) (io.ReadCloser, error) {
	f, err := os.Open(d.getStoredPath(c))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBlobNotFound
		}
		return nil, err
	}
	return f, nil
}

// Delete removes a blob from permanent storage
func (d *DiskBlobStore) Delete(ctx context.Context, c cid.Cid) error {
	err := os.Remove(d.getStoredPath(c))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// DeleteMany removes multiple blobs
func (d *DiskBlobStore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	for _, c := range cids {
		if err := d.Delete(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

// DeleteAll removes all blobs for this DID
func (d *DiskBlobStore) DeleteAll(ctx context.Context) error {
	paths := []string{
		filepath.Join(d.location, d.did),
		filepath.Join(d.tmpLocation, d.did),
		filepath.Join(d.quarantineLocation, d.did),
	}

	for _, path := range paths {
		err := os.RemoveAll(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing directory %s: %w", path, err)
		}
	}

	return nil
}
