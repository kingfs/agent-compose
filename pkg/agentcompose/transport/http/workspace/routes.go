package workspace

import (
	"agent-compose/pkg/agentcompose/configsvc"
	workspacepkg "agent-compose/pkg/agentcompose/workspace"
	appconfig "agent-compose/pkg/config"
	"context"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type FileEntry struct {
	Path      string `json:"path"`
	Dir       bool   `json:"dir"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updated_at"`
}

type FilesResponse struct {
	WorkspaceID string      `json:"workspace_id"`
	Files       []FileEntry `json:"files"`
}

type FileWorkspaceLoader interface {
	LoadFileWorkspaceConfig(ctx context.Context, workspaceID string) (configsvc.WorkspaceConfig, workspacepkg.FileWorkspaceContent, error)
	WorkspaceUploadLimitBytes() int64
}

func RegisterRoutes(app *echo.Echo, service FileWorkspaceLoader) {
	base := "/api/agent-compose/workspaces"
	app.GET(base+"/:workspaceID/files", func(c echo.Context) error {
		workspace, content, err := service.LoadFileWorkspaceConfig(c.Request().Context(), c.Param("workspaceID"))
		if err != nil {
			return ToHTTPError(err)
		}
		defer func() { _ = content.Root.Close() }()
		files, err := ListFiles(content.Root)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, FilesResponse{WorkspaceID: workspace.ID, Files: files})
	})
	app.POST(base+"/:workspaceID/upload", func(c echo.Context) error {
		limit := service.WorkspaceUploadLimitBytes()
		if limit <= 0 {
			limit = appconfig.DefaultWorkspaceUploadLimitBytes
		}
		if c.Request().ContentLength > limit {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "workspace upload exceeds configured limit")
		}
		c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, limit)
		_, content, err := service.LoadFileWorkspaceConfig(c.Request().Context(), c.Param("workspaceID"))
		if err != nil {
			return ToHTTPError(err)
		}
		defer func() { _ = content.Root.Close() }()
		fileHeader, err := c.FormFile("file")
		if err != nil {
			if strings.Contains(err.Error(), "http: request body too large") {
				return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "workspace upload exceeds configured limit")
			}
			return echo.NewHTTPError(http.StatusBadRequest, "missing form file \"file\"")
		}
		uploadType := strings.ToLower(strings.TrimSpace(c.FormValue("upload_type")))
		targetPath := strings.TrimSpace(c.FormValue("path"))
		switch uploadType {
		case "", "file":
			if err := StoreUploadedFile(fileHeader, content.Root, targetPath); err != nil {
				return ToUploadHTTPError(err)
			}
		case "archive":
			if err := ExtractUploadedArchive(fileHeader, content.Root); err != nil {
				return ToUploadHTTPError(err)
			}
		default:
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unsupported upload_type %q", uploadType))
		}
		files, err := ListFiles(content.Root)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, FilesResponse{WorkspaceID: c.Param("workspaceID"), Files: files})
	})
	app.GET(base+"/:workspaceID/download", func(c echo.Context) error {
		_, content, err := service.LoadFileWorkspaceConfig(c.Request().Context(), c.Param("workspaceID"))
		if err != nil {
			return ToHTTPError(err)
		}
		defer func() { _ = content.Root.Close() }()
		relPath, err := workspacepkg.CleanRelativePath(c.QueryParam("path"), false)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		relPath = filepath.ToSlash(relPath)
		info, err := content.Root.Lstat(relPath)
		if err != nil {
			if os.IsNotExist(err) {
				return echo.NewHTTPError(http.StatusNotFound, err.Error())
			}
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "download path must not be a symlink")
		}
		if info.IsDir() {
			return echo.NewHTTPError(http.StatusBadRequest, "download path must be a file")
		}
		file, err := content.Root.Open(relPath)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		defer func() { _ = file.Close() }()
		c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", filepath.Base(relPath)))
		c.Response().Header().Set(echo.HeaderContentType, "application/octet-stream")
		return c.Stream(http.StatusOK, "application/octet-stream", file)
	})
}

func ToUploadHTTPError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "http: request body too large") {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "workspace upload exceeds configured limit")
	}
	return echo.NewHTTPError(http.StatusBadRequest, err.Error())
}

func ToHTTPError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "not found"):
		return echo.NewHTTPError(http.StatusNotFound, message)
	case strings.Contains(message, "not a file workspace"), strings.Contains(message, "invalid"), strings.Contains(message, "missing"):
		return echo.NewHTTPError(http.StatusBadRequest, message)
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, message)
	}
}

func ListFiles(contentRoot *os.Root) ([]FileEntry, error) {
	items := make([]FileEntry, 0)
	err := fs.WalkDir(contentRoot.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		relPath := filepath.ToSlash(path)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace file %s is a symlink", relPath)
		}
		info, err := contentRoot.Lstat(relPath)
		if err != nil {
			return err
		}
		items = append(items, FileEntry{
			Path:      filepath.ToSlash(relPath),
			Dir:       entry.IsDir(),
			Size:      info.Size(),
			UpdatedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list workspace files: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
	return items, nil
}

func StoreUploadedFile(fileHeader *multipart.FileHeader, contentRoot *os.Root, targetPath string) error {
	if targetPath == "" {
		targetPath = fileHeader.Filename
	}
	cleanTarget, err := workspacepkg.CleanRelativePath(targetPath, false)
	if err != nil {
		return err
	}
	src, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("open uploaded file: %w", err)
	}
	defer func() { _ = src.Close() }()
	cleanTarget = filepath.ToSlash(cleanTarget)
	if err := workspacepkg.EnsureRootParentDir(contentRoot, cleanTarget); err != nil {
		return fmt.Errorf("create upload target parent: %w", err)
	}
	if err := contentRoot.RemoveAll(cleanTarget); err != nil {
		return fmt.Errorf("remove upload target file: %w", err)
	}
	dst, err := contentRoot.OpenFile(cleanTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create upload target file: %w", err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("write upload target file: %w", err)
	}
	return nil
}

func ExtractUploadedArchive(fileHeader *multipart.FileHeader, contentRoot *os.Root) error {
	src, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("open uploaded archive: %w", err)
	}
	defer func() { _ = src.Close() }()
	return workspacepkg.ExtractTarArchive(src, contentRoot)
}
