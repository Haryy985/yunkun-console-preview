package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// OfflineFiles runs in an independent copy of the installed executable. Go's
// Windows filesystem implementation supports long paths without changing policy.
func OfflineFiles(jobPath, operation string) error {
	var job offlineJob
	if err := readJSON(jobPath, &job); err != nil {
		return err
	}
	root, err := filepath.Abs(job.Root)
	if err != nil {
		return err
	}
	updates := filepath.Join(root, ".yunkun-local", "updates")
	work, err := filepath.Abs(job.Work)
	if err != nil {
		return err
	}
	if !withinPath(work, updates) || !samePath(jobPath, filepath.Join(work, "job.json")) {
		return errors.New("invalid offline update workspace")
	}
	if resolved, err := filepath.EvalSymlinks(root); err != nil || !samePath(resolved, root) {
		return errors.New("linked installation roots are unsupported")
	}
	if resolved, err := filepath.EvalSymlinks(work); err != nil || !samePath(resolved, work) {
		return errors.New("linked update workspaces are unsupported")
	}
	backup := filepath.Join(work, "backup")
	candidate := filepath.Join(work, "candidate")
	exclude := func(file string) bool {
		return samePath(file, updates) || samePath(file, filepath.Join(root, ".yunkun-local", "update-state.json")) ||
			samePath(file, filepath.Join(root, ".yunkun-local", "update-state.json.new"))
	}
	switch operation {
	case "check":
		var size int64
		err := walkRegular(root, exclude, func(file, rel string, info fs.FileInfo) error {
			size += info.Size()
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Println(size)
		return walkRegular(candidate, nil, func(file, rel string, info fs.FileInfo) error {
			_, err := offlineEntry("package/" + filepath.ToSlash(rel))
			return err
		})
	case "backup":
		if err := os.Mkdir(backup, 0o700); err != nil {
			return err
		}
		return copyTreeVerified(root, backup, exclude)
	case "install":
		return walkRegular(candidate, nil, func(file, rel string, info fs.FileInfo) error {
			if _, err := offlineEntry("package/" + filepath.ToSlash(rel)); err != nil {
				return err
			}
			dst := filepath.Join(root, rel)
			if strings.HasPrefix(filepath.ToSlash(rel), "skills/") {
				if _, err := os.Lstat(dst); err == nil {
					dst = filepath.Join(root, ".yunkun-local", "upgrade-bundled-skills", strings.TrimPrefix(filepath.ToSlash(rel), "skills/"))
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			return copyVerifiedFile(file, dst, info.Mode())
		})
	case "restore":
		// Keep the failed program and migrated database for diagnosis. Do not run
		// an old binary against the new schema or erase failed customer data.
		if _, err := os.Stat(filepath.Join(work, "backup-verified")); err != nil {
			return errors.New("refusing recovery without a verified snapshot")
		}
		failed := filepath.Join(work, "failed-installation")
		if err := os.Mkdir(failed, 0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), ".yunkun-local") {
				continue
			}
			if err := os.Rename(filepath.Join(root, entry.Name()), filepath.Join(failed, entry.Name())); err != nil {
				return err
			}
		}
		private := filepath.Join(root, ".yunkun-local")
		failedPrivate := filepath.Join(failed, ".yunkun-local")
		if err := os.Mkdir(failedPrivate, 0o700); err != nil {
			return err
		}
		entries, err = os.ReadDir(private)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			src := filepath.Join(private, entry.Name())
			if exclude(src) {
				continue
			}
			if err := os.Rename(src, filepath.Join(failedPrivate, entry.Name())); err != nil {
				return err
			}
		}
		return copyTreeVerified(backup, root, nil)
	default:
		return errors.New("unsupported offline file operation")
	}
}

func withinPath(child, root string) bool {
	rel, err := filepath.Rel(root, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func walkRegular(root string, exclude func(string) bool, visit func(string, string, fs.FileInfo) error) error {
	return filepath.Walk(root, func(file string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("linked paths are unsupported: %s", file)
		}
		if exclude != nil && exclude(file) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special files are unsupported: %s", file)
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		return visit(file, rel, info)
	})
}

func copyTreeVerified(src, dst string, exclude func(string) bool) error {
	// Keep empty directories as well; PostgreSQL requires several on startup.
	return filepath.Walk(src, func(file string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("linked paths are unsupported: %s", file)
		}
		if exclude != nil && exclude(file) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, file)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return errors.New("unsupported snapshot file")
		}
		return copyVerifiedFile(file, target, info.Mode())
	})
}

func copyVerifiedFile(src, dst string, mode fs.FileMode) error {
	if info, err := os.Lstat(dst); err == nil && !info.Mode().IsRegular() {
		return errors.New("copy destination is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dst), ".update-copy-")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	if err := out.Chmod(mode.Perm()); err != nil {
		out.Close()
		return err
	}
	sourceHash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, sourceHash), in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	a := hex.EncodeToString(sourceHash.Sum(nil))
	b, err := fileSHA256(out.Name())
	if err != nil || a != b {
		return fmt.Errorf("file verification failed: %s", dst)
	}
	return os.Rename(out.Name(), dst)
}
