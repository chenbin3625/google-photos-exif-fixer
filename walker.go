package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// 图片/视频扩展名白名单（小写，含点）。
var mediaExts = []string{
	".jpg", ".jpeg", ".png", ".heic", ".gif",
	".mp4", ".mov", ".m4v", ".avi",
	".livp", ".nef", ".dng", ".webp",
	".bmp", ".tiff", ".tif",
}

// mediaExt 返回（小写扩展名含点，是否有扩展名）。
func mediaExt(name string) (string, bool) {
	low := strings.ToLower(name)
	for _, e := range mediaExts {
		if strings.HasSuffix(low, e) {
			return e, true
		}
	}
	return "", false
}

// isJSONFile 判断是否 JSON sidecar。
func isJSONFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".json")
}

// sniffExt 对无扩展名文件嗅探真实类型，返回应补的扩展名（含点）。
func sniffExt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	mime := http.DetectContentType(buf[:n])
	switch {
	case strings.HasPrefix(mime, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mime, "image/png"):
		return ".png"
	case strings.HasPrefix(mime, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mime, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mime, "image/bmp"):
		return ".bmp"
	case strings.HasPrefix(mime, "image/tiff"):
		return ".tif"
	case strings.HasPrefix(mime, "video/"):
		return ".mov"
	case strings.HasPrefix(mime, "image/heic"), strings.HasPrefix(mime, "image/heif"):
		return ".heic"
	}
	return ""
}

// ImageEntry 是收集到的图片/视频。
type ImageEntry struct {
	Path    string // 输入绝对/相对路径
	Stem    string // 不含扩展名的文件名（原始大小写）
	Ext     string // 小写扩展名含点（无扩展名时由嗅探补）
	NoExt   bool   // 原始无扩展名
	Consumed bool   // 是否已被 JSON 匹配消费
}

// JSONEntry 是收集到的 JSON sidecar。
type JSONEntry struct {
	Path string
	Meta *PhotoMeta
}

// WalkResult 是遍历结果。
type WalkResult struct {
	Images []ImageEntry
	JSONs  []JSONEntry
}

// WalkDir 递归遍历输入目录，分类收集图片与 JSON。
func WalkDir(root string) (*WalkResult, error) {
	res := &WalkResult{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == ".DS_Store" {
			return nil
		}
		if isJSONFile(name) {
			meta, err := LoadMeta(path)
			if err != nil {
				// 解析失败的 JSON 仍记录路径，后续按孤儿处理
				res.JSONs = append(res.JSONs, JSONEntry{Path: path, Meta: nil})
				return nil
			}
			res.JSONs = append(res.JSONs, JSONEntry{Path: path, Meta: meta})
			return nil
		}
		ext, ok := mediaExt(name)
		noExt := false
		if !ok {
			// 无扩展名：嗅探
			if se := sniffExt(path); se != "" {
				ext = se
				noExt = true
			} else {
				// 既非已知媒体扩展也嗅探不出，跳过
				return nil
			}
		}
		stem := name
		if idx := strings.LastIndex(strings.ToLower(name), ext); idx >= 0 && !noExt {
			stem = name[:idx]
		}
		res.Images = append(res.Images, ImageEntry{
			Path:  path,
			Stem:  stem,
			Ext:   ext,
			NoExt: noExt,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
