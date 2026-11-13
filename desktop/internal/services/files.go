package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxDropFileBytes   = 40 << 20
	maxImportFileBytes = 40 << 20
	maxImportTotal     = 200 << 20
	maxImportFiles     = 500
)

// DroppedFile is one file handed to the web layer, base64 encoded.
type DroppedFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Data string `json:"data"`
}

var importableExtensions = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".txt": true, ".md": true,
	".tex": true, ".bib": true, ".cls": true, ".sty": true, ".bst": true,
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".svg": true,
	".csv": true, ".xlsx": true, ".pptx": true, ".zip": true,
}

// ReadFiles returns the contents of native file-drop paths. Entries that are
// missing, not regular files, or too large are skipped instead of failing the
// whole drop.
func (s *DesktopService) ReadFiles(paths []string) ([]DroppedFile, error) {
	files := make([]DroppedFile, 0, len(paths))
	var total int64
	for _, raw := range paths {
		abs, err := filepath.Abs(raw)
		if err != nil {
			continue
		}
		info, err := os.Lstat(abs)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxDropFileBytes {
			continue
		}
		if total+info.Size() > maxImportTotal {
			return nil, errors.New("拖入的文件总大小超过限制")
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		total += int64(len(data))
		files = append(files, DroppedFile{
			Name: filepath.Base(abs),
			Path: abs,
			Data: base64.StdEncoding.EncodeToString(data),
		})
	}
	return files, nil
}

// ReadFolder walks a user-picked folder and returns importable files with
// paths relative to that folder. Hidden directories and unsupported types are
// skipped.
func (s *DesktopService) ReadFolder(root string) ([]DroppedFile, error) {
	abs, err := validateAbsolutePath(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil, errors.New("选择的路径不是文件夹")
	}

	files := make([]DroppedFile, 0, 32)
	var total int64
	walkErr := filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != abs && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !importableExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxImportFileBytes {
			return nil
		}
		if len(files) >= maxImportFiles {
			return errors.New("文件夹中的文件数量超过限制")
		}
		if total+info.Size() > maxImportTotal {
			return errors.New("文件夹中的文件总大小超过限制")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		total += int64(len(data))
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		files = append(files, DroppedFile{
			Name: filepath.ToSlash(rel),
			Path: path,
			Data: base64.StdEncoding.EncodeToString(data),
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return files, nil
}

// SaveFileBytes asks for a destination and writes the decoded payload.
func (s *DesktopService) SaveFileBytes(defaultName, dataBase64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return "", fmt.Errorf("invalid file data: %w", err)
	}
	if len(data) == 0 || len(data) > maxImportTotal {
		return "", errors.New("文件大小超出限制")
	}
	name := strings.TrimSpace(filepath.Base(defaultName))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "papersage-export.zip"
	}
	path, err := s.app.Dialog.SaveFile().SetFilename(name).PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write export file: %w", err)
	}
	return path, nil
}
