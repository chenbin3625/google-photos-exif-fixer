package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// isJPEG 判断扩展名是否 JPEG。
func isJPEG(ext string) bool {
	return ext == ".jpg" || ext == ".jpeg"
}

// outputName 计算输出文件名；无扩展名文件用嗅探后的扩展名。
func outputName(im *ImageEntry) string {
	base := filepath.Base(im.Path)
	if im.NoExt {
		// 用 stem + 嗅探到的 ext
		return im.Stem + im.Ext
	}
	return base
}

// uniquePath 在 dir 下返回不存在的目标路径，遇冲突追加 _<n>。
func uniquePath(dir, name string) string {
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; ; n++ {
		c := filepath.Join(dir, fmt.Sprintf("%s_%d%s", stem, n, ext))
		if _, err := os.Stat(c); os.IsNotExist(err) {
			return c
		}
	}
}

// copyFile 复制 src 到 dst（覆盖式写临时文件再 rename，保证原子）。
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // 若 rename 成功则已不存在
	if _, err := io.CopyBuffer(tmp, in, make([]byte, 1<<20)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}

// setMtime 设置文件修改时间与访问时间。
func setMtime(path string, t time.Time) error {
	return os.Chtimes(path, t, t)
}

// Task 是一个待执行的输出任务。
type Task struct {
	Src        string   // 输入图片路径
	TakenTime  time.Time
	Source     string   // 时间来源描述
	Meta       *PhotoMeta
	WriteExif  bool     // 是否写 JPEG EXIF
	SubDir     string   // 输出子目录 YYYY/MM
	OutName    string   // 输出文件名
}

// TaskResult 是任务执行结果。
type TaskResult struct {
	Task       *Task
	Dst        string
	ExifStatus string // "skipped"/"ok"/"failed:<err>"
	Err        error
}
