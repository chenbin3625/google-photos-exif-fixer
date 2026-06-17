package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// 构造一个最小 JPEG（SOI + SOI 之后无段，仅放测试像素数据）用于 EXIF 注入测试。
func minimalJPEG() []byte {
	return []byte{0xFF, 0xD8, 0xFF, 0xD9} // SOI + EOI
}

func TestExifWriteAndDecode(t *testing.T) {
	tt := time.Date(2023, 1, 8, 14, 57, 51, 0, time.UTC)
	geo := &GeoData{Latitude: 39.9042, Longitude: 116.4074}

	out, err := writeJPEGExif(minimalJPEG(), tt, geo)
	if err != nil {
		t.Fatalf("writeJPEGExif: %v", err)
	}
	if !bytes.HasPrefix(out, []byte{0xFF, 0xD8}) {
		t.Fatal("缺少 SOI")
	}
	// APP1 marker 应紧跟 SOI
	if out[2] != 0xFF || out[3] != 0xE1 {
		t.Fatalf("APP1 marker 位置错误: % x", out[2:4])
	}

	// 写临时文件解码验证
	dir := t.TempDir()
	p := filepath.Join(dir, "t.jpg")
	if err := os.WriteFile(p, out, 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		t.Fatalf("goexif 解码失败: %v", err)
	}
	tag, err := x.Get(exif.DateTimeOriginal)
	if err != nil {
		t.Fatalf("无 DateTimeOriginal: %v", err)
	}
	s, _ := tag.StringVal()
	if s != "2023:01:08 14:57:51" {
		t.Fatalf("时间不匹配: %q", s)
	}

	// 验证 GPS
	latTag, err := x.Get("GPSLatitude")
	if err != nil {
		t.Fatalf("无 GPSLatitude: %v", err)
	}
	_ = latTag
	// goexif 提供 GPS() 便捷方法
	lat, lng, err := x.LatLong()
	if err != nil {
		t.Fatalf("LatLong 解析失败: %v", err)
	}
	if abs(lat-39.9042) > 0.01 || abs(lng-116.4074) > 0.01 {
		t.Fatalf("GPS 不匹配: lat=%f lng=%f", lat, lng)
	}
}

func TestExifNoGPS(t *testing.T) {
	tt := time.Date(2020, 6, 15, 9, 30, 0, 0, time.UTC)
	out, err := writeJPEGExif(minimalJPEG(), tt, &GeoData{})
	if err != nil {
		t.Fatalf("writeJPEGExif: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "t.jpg")
	os.WriteFile(p, out, 0644)
	f, _ := os.Open(p)
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	tag, err := x.Get(exif.DateTimeOriginal)
	if err != nil {
		t.Fatalf("无 DateTimeOriginal: %v", err)
	}
	s, _ := tag.StringVal()
	if s != "2020:06:15 09:30:00" {
		t.Fatalf("时间不匹配: %q", s)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
