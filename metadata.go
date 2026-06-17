package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// PhotoMeta 是 Google 相册 JSON sidecar 中我们关心的字段。
type PhotoMeta struct {
	Title          string      `json:"title"`
	Description    string      `json:"description"`
	PhotoTakenTime *TimeField  `json:"photoTakenTime"`
	CreationTime   *TimeField  `json:"creationTime"`
	GeoData        *GeoData    `json:"geoData"`
	GeoDataExif    *GeoData    `json:"geoDataExif"`
}

// TimeField 包含 Unix 时间戳与格式化字符串。
type TimeField struct {
	Timestamp string `json:"timestamp"`
	Formatted string `json:"formatted"`
}

// GeoData 是 GPS 信息。
type GeoData struct {
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Altitude      float64 `json:"altitude"`
	LatitudeSpan  float64 `json:"latitudeSpan"`
	LongitudeSpan float64 `json:"longitudeSpan"`
}

// HasGPS 返回是否有有效（非零）GPS。
func (g *GeoData) HasGPS() bool {
	if g == nil {
		return false
	}
	return math.Abs(g.Latitude) > 1e-6 || math.Abs(g.Longitude) > 1e-6
}

// LoadMeta 从路径读取并解析 JSON。
func LoadMeta(path string) (*PhotoMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m PhotoMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", filepath.Base(path), err)
	}
	return &m, nil
}

// TakenTime 返回 photoTakenTime；若缺失返回错误。
func (m *PhotoMeta) TakenTime() (time.Time, error) {
	if m == nil || m.PhotoTakenTime == nil || m.PhotoTakenTime.Timestamp == "" {
		return time.Time{}, errors.New("无 photoTakenTime")
	}
	return parseUnix(m.PhotoTakenTime.Timestamp)
}

func parseUnix(s string) (time.Time, error) {
	ts, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts, 0).UTC(), nil
}

// Geo 返回优先的 GPS（geoDataExif 优先，其次 geoData）。
func (m *PhotoMeta) Geo() *GeoData {
	if m == nil {
		return nil
	}
	if m.GeoDataExif != nil && m.GeoDataExif.HasGPS() {
		return m.GeoDataExif
	}
	if m.GeoData.HasGPS() {
		return m.GeoData
	}
	return nil
}

var yearRE = regexp.MustCompile(`(\d{4})`)
var exifDateRE = regexp.MustCompile(`(\d{4}):(\d{2}):(\d{2})\s+(\d{2}):(\d{2}):(\d{2})`)

// ResolveTakenTime 解析拍摄时间，按兜底链：
//  1. JSON photoTakenTime（meta 非空时）
//  2. JPEG EXIF DateTimeOriginal
//  3. 文件夹名年份推断
//  4. 文件 mtime
// 返回时间和来源描述。
func ResolveTakenTime(meta *PhotoMeta, imgPath string, isJPEG bool) (time.Time, string) {
	if meta != nil {
		if t, err := meta.TakenTime(); err == nil {
			return t, "json:photoTakenTime"
		}
	}
	if isJPEG {
		if t, ok := readJPEGExifTime(imgPath); ok {
			return t, "exif:DateTimeOriginal"
		}
	}
	if t, ok := inferFromFolder(imgPath); ok {
		return t, "folder:year"
	}
	if fi, err := os.Stat(imgPath); err == nil {
		return fi.ModTime().UTC(), "file:mtime"
	}
	return time.Unix(0, 0).UTC(), "fallback:epoch"
}

// inferFromFolder 从所在目录名中提取年份，推断为该年 1 月 1 日。
func inferFromFolder(path string) (time.Time, bool) {
	dir := filepath.Base(filepath.Dir(path))
	m := yearRE.FindStringSubmatch(dir)
	if m == nil {
		return time.Time{}, false
	}
	y, err := strconv.Atoi(m[1])
	if err != nil || y < 1970 || y > 2100 {
		return time.Time{}, false
	}
	return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), true
}

// readJPEGExifTime 用 goexif 读取 DateTimeOriginal。
func readJPEGExifTime(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		return time.Time{}, false
	}
	tag, err := x.Get(exif.DateTimeOriginal)
	if err != nil {
		return time.Time{}, false
	}
	s, err := tag.StringVal()
	if err != nil {
		return time.Time{}, false
	}
	m := exifDateRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	h, _ := strconv.Atoi(m[4])
	mi, _ := strconv.Atoi(m[5])
	se, _ := strconv.Atoi(m[6])
	t := time.Date(y, time.Month(mo), d, h, mi, se, 0, time.UTC)
	if t.Year() < 1970 || t.Year() > 2100 {
		return time.Time{}, false
	}
	return t, true
}
