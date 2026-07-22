package data

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var syncFile = func(file *os.File) error { return file.Sync() }

// ErrBackupAlreadyExists reports that a backup for the requested timestamp exists.
var ErrBackupAlreadyExists = errors.New("backup already exists")

// Backup atomically copies the data directory beside its source.
func Backup(now time.Time) (backupPath string, err error) {
	source := Dir()
	name := "data-backup-" + now.UTC().Format("20060102T150405Z")
	final := filepath.Join(filepath.Dir(source), name)
	temp := final + ".tmp"
	if _, err := os.Stat(final); err == nil {
		return "", fmt.Errorf("%w: %s", ErrBackupAlreadyExists, final)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	_ = os.RemoveAll(temp)
	defer func() {
		if err != nil {
			_ = os.RemoveAll(temp)
		}
	}()

	sourceInfo, statErr := os.Stat(source)
	if os.IsNotExist(statErr) {
		if err = os.MkdirAll(temp, 0700); err != nil {
			return "", err
		}
		if err = commitBackup(temp, final); err != nil {
			return "", err
		}
		return final, nil
	}
	if statErr != nil {
		return "", statErr
	}
	if err = os.MkdirAll(temp, sourceInfo.Mode().Perm()); err != nil {
		return "", err
	}
	if err = os.Chmod(temp, sourceInfo.Mode().Perm()); err != nil {
		return "", err
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(source, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		destination := filepath.Join(temp, rel)
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		switch {
		case entry.IsDir():
			if err := os.Mkdir(destination, info.Mode().Perm()); err != nil {
				return err
			}
			return os.Chmod(destination, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, destination)
		case info.Mode().IsRegular():
			src, err := os.Open(path)
			if err != nil {
				return err
			}
			dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
			if err != nil {
				_ = src.Close()
				return err
			}
			_, copyErr := io.Copy(dst, src)
			syncErr := error(nil)
			if copyErr == nil {
				syncErr = syncFile(dst)
			}
			srcErr := src.Close()
			dstErr := dst.Close()
			if copyErr != nil {
				return copyErr
			}
			if srcErr != nil {
				return srcErr
			}
			if syncErr != nil {
				return syncErr
			}
			if dstErr != nil {
				return dstErr
			}
			return os.Chmod(destination, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported data file type: %s", path)
		}
	})
	if err != nil {
		return "", err
	}
	if err = commitBackup(temp, final); err != nil {
		return "", err
	}
	return final, nil
}

func commitBackup(temp, final string) error {
	if err := syncDirectories(temp); err != nil {
		return err
	}
	if err := os.Rename(temp, final); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(final))
}

func syncDirectories(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		return syncDirectory(path)
	})
}

// Windows does not support fsync on directory handles. Unix filesystems do;
// skipping only this unsupported operation avoids claiming directory durability.
func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := syncFile(dir)
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
