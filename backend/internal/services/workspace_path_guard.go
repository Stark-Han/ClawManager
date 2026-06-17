package services

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrWorkspacePathInvalid  = errors.New("workspace path is invalid")
	ErrWorkspacePathEscape   = errors.New("workspace path escapes workspace")
	ErrWorkspacePathNotFound = errors.New("workspace entry not found")
)

type ResolvedWorkspacePath struct {
	Root         string
	Path         string
	RealPath     string
	RelativePath string
}

func ResolveWorkspacePath(workspaceRoot, relativePath string, allowMissingLeaf bool) (*ResolvedWorkspacePath, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return nil, fmt.Errorf("%w: workspace root is required", ErrWorkspacePathInvalid)
	}
	if strings.Contains(root, "\x00") {
		return nil, fmt.Errorf("%w: workspace root contains null byte", ErrWorkspacePathInvalid)
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWorkspacePathInvalid, err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: workspace root", ErrWorkspacePathNotFound)
		}
		return nil, fmt.Errorf("%w: %v", ErrWorkspacePathInvalid, err)
	}
	rootReal = filepath.Clean(rootReal)
	rootInfo, err := os.Stat(rootReal)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: workspace root", ErrWorkspacePathNotFound)
		}
		return nil, fmt.Errorf("%w: %v", ErrWorkspacePathInvalid, err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("%w: workspace root is not a directory", ErrWorkspacePathInvalid)
	}

	relative, err := cleanWorkspaceRelativePath(relativePath)
	if err != nil {
		return nil, err
	}

	targetPath := rootReal
	if relative != "" {
		targetPath = filepath.Join(rootReal, filepath.FromSlash(relative))
	}
	targetPath = filepath.Clean(targetPath)
	if !isWorkspaceSubpath(rootReal, targetPath) {
		return nil, fmt.Errorf("%w: %s", ErrWorkspacePathEscape, relativePath)
	}

	realPath, err := resolveWorkspaceRealPath(targetPath, allowMissingLeaf)
	if err != nil {
		return nil, err
	}
	realPath = filepath.Clean(realPath)
	if !isWorkspaceSubpath(rootReal, realPath) {
		return nil, fmt.Errorf("%w: %s", ErrWorkspacePathEscape, relativePath)
	}

	return &ResolvedWorkspacePath{
		Root:         rootReal,
		Path:         targetPath,
		RealPath:     realPath,
		RelativePath: relative,
	}, nil
}

func cleanWorkspaceRelativePath(relativePath string) (string, error) {
	raw := strings.TrimSpace(relativePath)
	if raw == "" || raw == "." {
		return "", nil
	}
	if strings.Contains(raw, "\x00") {
		return "", fmt.Errorf("%w: path contains null byte", ErrWorkspacePathInvalid)
	}
	if filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", fmt.Errorf("%w: absolute paths are not allowed", ErrWorkspacePathEscape)
	}

	normalized := strings.ReplaceAll(raw, "\\", "/")
	if path.IsAbs(normalized) || isWindowsDrivePath(normalized) {
		return "", fmt.Errorf("%w: absolute paths are not allowed", ErrWorkspacePathEscape)
	}

	parts := strings.Split(normalized, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", fmt.Errorf("%w: traversal is not allowed", ErrWorkspacePathEscape)
		default:
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return "", nil
	}
	return path.Clean(strings.Join(cleaned, "/")), nil
}

func isWindowsDrivePath(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	first := value[0]
	return (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')
}

func resolveWorkspaceRealPath(targetPath string, allowMissingLeaf bool) (string, error) {
	realPath, err := filepath.EvalSymlinks(targetPath)
	if err == nil {
		return realPath, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("%w: %v", ErrWorkspacePathInvalid, err)
	}
	if !allowMissingLeaf {
		return "", fmt.Errorf("%w: %s", ErrWorkspacePathNotFound, targetPath)
	}

	existingPath := targetPath
	missingParts := []string{}
	for {
		info, statErr := os.Lstat(existingPath)
		if statErr == nil {
			if len(missingParts) > 0 {
				info, statErr = os.Stat(existingPath)
				if statErr != nil {
					if os.IsNotExist(statErr) {
						return "", fmt.Errorf("%w: %s", ErrWorkspacePathNotFound, existingPath)
					}
					return "", fmt.Errorf("%w: %v", ErrWorkspacePathInvalid, statErr)
				}
				if !info.IsDir() {
					return "", fmt.Errorf("%w: parent is not a directory", ErrWorkspacePathInvalid)
				}
			}
			parentReal, evalErr := filepath.EvalSymlinks(existingPath)
			if evalErr != nil {
				if os.IsNotExist(evalErr) {
					return "", fmt.Errorf("%w: %s", ErrWorkspacePathNotFound, existingPath)
				}
				return "", fmt.Errorf("%w: %v", ErrWorkspacePathInvalid, evalErr)
			}
			parts := append([]string{parentReal}, missingParts...)
			return filepath.Join(parts...), nil
		}
		if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("%w: %v", ErrWorkspacePathInvalid, statErr)
		}

		parent := filepath.Dir(existingPath)
		if parent == existingPath {
			return "", fmt.Errorf("%w: %s", ErrWorkspacePathNotFound, targetPath)
		}
		missingParts = append([]string{filepath.Base(existingPath)}, missingParts...)
		existingPath = parent
	}
}

func isWorkspaceSubpath(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if sameWorkspacePath(root, target) {
		return true
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func sameWorkspacePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
