package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"time"
)

// 本文件手写最小 EXIF APP1 段，把拍摄时间与 GPS 写入 JPEG。
// 策略：构造一个完整的最小 EXIF APP1，插入到 SOI(FFD8) 之后、
// 现有 APP0/APP1 之前，使其成为解码器的首选 EXIF 段。不修改原有字节。
// 字节序固定 little-endian。

// writeJPEGExif 把 t(拍摄时间) 与可选 GPS 写入 JPEG 字节，返回新切片。
// 策略：扫描原 JPEG 段，**剥离所有现有 APP1(Exif) 段**（避免多 APP1 冲突），
// 在 SOI 之后插入新的 APP1。其余段（APP0/JFIF、量化表、SOS 起的图像数据等）原样保留。
func writeJPEGExif(data []byte, t time.Time, geo *GeoData) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, errors.New("非 JPEG")
	}
	app1, err := buildExifAPP1(t, geo)
	if err != nil {
		return nil, err
	}
	stripped, err := stripExifAPP1(data)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(stripped)+len(app1))
	out = append(out, stripped[:2]...) // SOI
	out = append(out, app1...)         // 新 APP1
	out = append(out, stripped[2:]...) // 其余段 + 数据
	return out, nil
}

// stripExifAPP1 扫描 JPEG 段，移除所有 APP1(标识为 "Exif\0") 段；
// 遇到 SOS(FFDA) 则停止扫描（其后是压缩数据，不再是段结构）。
func stripExifAPP1(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, errors.New("非 JPEG")
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[0], data[1])
	i := 2
	for i+1 < len(data) {
		if data[i] != 0xFF {
			// 异常字节：从此处开始原样追加余下
			out = append(out, data[i:]...)
			break
		}
		marker := data[i+1]
		// 跳过 0xFF 填充字节
		if marker == 0xFF {
			out = append(out, data[i])
			i++
			continue
		}
		// SOS：图像数据开始，整体保留余下
		if marker == 0xDA {
			out = append(out, data[i:]...)
			break
		}
		// 无负载段（RSTn、SOI、EOI、TEM）
		if marker == 0x00 || marker == 0xD8 || marker == 0xD9 ||
			(marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			out = append(out, data[i], data[i+1])
			i += 2
			continue
		}
		// 含长度的段
		if i+3 >= len(data) {
			return nil, errors.New("JPEG 段截断")
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		if segLen < 2 || i+2+segLen > len(data) {
			return nil, errors.New("JPEG 段长度异常")
		}
		segEnd := i + 2 + segLen
		// 判断是否 APP1(Exif)
		isExif := false
		if marker == 0xE1 && segLen >= 8 {
			payload := data[i+4 : segEnd]
			if len(payload) >= 6 && string(payload[:4]) == "Exif" && payload[4] == 0 && payload[5] == 0 {
				isExif = true
			}
		}
		if !isExif {
			out = append(out, data[i:segEnd]...)
		}
		i = segEnd
	}
	return out, nil
}

// buildExifAPP1 构造 APP1 段（FFE1 + 长度 + "Exif\0" + TIFF 数据）。
func buildExifAPP1(t time.Time, geo *GeoData) ([]byte, error) {
	tiff, err := buildTIFF(t, geo)
	if err != nil {
		return nil, err
	}
	body := make([]byte, 0, 6+len(tiff))
	body = append(body, 'E', 'x', 'i', 'f', 0x00, 0x00)
	body = append(body, tiff...)
	if len(body) > 0xFFFF-2 {
		return nil, errors.New("EXIF 段过大")
	}
	out := make([]byte, 0, 4+len(body))
	out = append(out, 0xFF, 0xE1)
	out = append(out, byte((len(body)+2)>>8), byte(len(body)+2))
	out = append(out, body...)
	return out, nil
}

// gpsIFD 条目顺序（tag 升序）：0x0001 latRef, 0x0002 lat, 0x0003 lonRef, 0x0004 lon
const gpsIFDCount = 4

// buildTIFF 构造 TIFF 数据块。
// 布局：头(8) + IFD0 + ExifIFD + [GPSIFD] + 数据区。
// IFD0: DateTime(0x0132), ExifIFDPointer(0x8769), [GPSIFDPointer(0x8825)]
// ExifIFD: DateTimeOriginal(0x9003), DateTimeDigitized(0x9004)
// GPSIFD: GPSLatitudeRef(0x0001), GPSLatitude(0x0002),
//         GPSLongitudeRef(0x0003), GPSLongitude(0x0004)
// 数据区: 3 个时间字符串(各 20 字节) + 可选 GPS 有理数(lat 24 + lon 24)
func buildTIFF(t time.Time, geo *GeoData) ([]byte, error) {
	dtStr := t.UTC().Format("2006:01:02 15:04:05") + "\x00" // 20 字节
	dtBytes := []byte(dtStr)

	hasGPS := geo != nil && geo.HasGPS()

	ifd0Count := 2
	if hasGPS {
		ifd0Count = 3
	}
	exifIFDCount := 2

	// 布局偏移
	const headerLen = 8
	ifd0Len := 2 + ifd0Count*12 + 4
	exifIFDLen := 2 + exifIFDCount*12 + 4
	gpsIFDLen := 0
	if hasGPS {
		gpsIFDLen = 2 + gpsIFDCount*12 + 4
	}

	ifd0Off := headerLen
	exifIFDOff := ifd0Off + ifd0Len
	gpsIFDOff := exifIFDOff + exifIFDLen
	dataOff := gpsIFDOff + gpsIFDLen

	dtOff1 := dataOff
	dtOff2 := dtOff1 + len(dtBytes)
	dtOff3 := dtOff2 + len(dtBytes)
	latRatOff := dtOff3 + len(dtBytes)
	lonRatOff := latRatOff + 24

	// GPS 预计算
	var latD, latM, latS, lonD, lonM, lonS uint32
	var latRef, lonRef byte
	if hasGPS {
		latD, latM, latS = decimalToDMS(geo.Latitude)
		lonD, lonM, lonS = decimalToDMS(geo.Longitude)
		latRef = 'N'
		if geo.Latitude < 0 {
			latRef = 'S'
		}
		lonRef = 'E'
		if geo.Longitude < 0 {
			lonRef = 'W'
		}
	}

	var buf bytes.Buffer

	// TIFF 头
	buf.Write([]byte{'I', 'I'})
	binary.Write(&buf, binary.LittleEndian, uint16(0x002A))
	binary.Write(&buf, binary.LittleEndian, uint32(ifd0Off))

	// IFD0
	binary.Write(&buf, binary.LittleEndian, uint16(ifd0Count))
	writeIFDEntry(&buf, 0x0132, 2, uint32(len(dtBytes)), uint32(dtOff1))
	writeIFDEntry(&buf, 0x8769, 4, 1, uint32(exifIFDOff))
	if hasGPS {
		writeIFDEntry(&buf, 0x8825, 4, 1, uint32(gpsIFDOff))
	}
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	// ExifIFD
	binary.Write(&buf, binary.LittleEndian, uint16(exifIFDCount))
	writeIFDEntry(&buf, 0x9003, 2, uint32(len(dtBytes)), uint32(dtOff2))
	writeIFDEntry(&buf, 0x9004, 2, uint32(len(dtBytes)), uint32(dtOff3))
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	// GPSIFD
	if hasGPS {
		binary.Write(&buf, binary.LittleEndian, uint16(gpsIFDCount))
		writeIFDEntryInlineASCII(&buf, 0x0001, []byte{latRef, 0x00})
		writeIFDEntry(&buf, 0x0002, 5, 3, uint32(latRatOff))
		writeIFDEntryInlineASCII(&buf, 0x0003, []byte{lonRef, 0x00})
		writeIFDEntry(&buf, 0x0004, 5, 3, uint32(lonRatOff))
		binary.Write(&buf, binary.LittleEndian, uint32(0))
	}

	// 数据区
	buf.Write(dtBytes) // dtOff1
	buf.Write(dtBytes) // dtOff2
	buf.Write(dtBytes) // dtOff3
	if hasGPS {
		writeRational(&buf, latD)
		writeRational(&buf, latM)
		writeRational(&buf, latS)
		writeRational(&buf, lonD)
		writeRational(&buf, lonM)
		writeRational(&buf, lonS)
	}

	// 校验总长度
	want := dataOff + 3*len(dtBytes)
	if hasGPS {
		want += 48
	}
	if buf.Len() != want {
		return nil, fmt.Errorf("EXIF 布局偏移不匹配: got=%d want=%d", buf.Len(), want)
	}
	return buf.Bytes(), nil
}

// writeIFDEntry 写 12 字节 IFD 条目（值偏移在外部数据区）。
func writeIFDEntry(buf *bytes.Buffer, tag uint16, typ uint16, count uint32, valueOrOffset uint32) {
	binary.Write(buf, binary.LittleEndian, tag)
	binary.Write(buf, binary.LittleEndian, typ)
	binary.Write(buf, binary.LittleEndian, count)
	binary.Write(buf, binary.LittleEndian, valueOrOffset)
}

// writeIFDEntryInlineASCII 写 ASCII 条目，值≤4 时内联存储。
func writeIFDEntryInlineASCII(buf *bytes.Buffer, tag uint16, val []byte) {
	binary.Write(buf, binary.LittleEndian, tag)
	binary.Write(buf, binary.LittleEndian, uint16(2)) // ASCII
	binary.Write(buf, binary.LittleEndian, uint32(len(val)))
	var inline [4]byte
	copy(inline[:], val)
	buf.Write(inline[:])
}

// writeRational 写 unsigned RATIONAL(8 字节: num + den)。
func writeRational(buf *bytes.Buffer, num uint32) {
	binary.Write(buf, binary.LittleEndian, num)
	binary.Write(buf, binary.LittleEndian, uint32(1))
}

// decimalToDMS 把十进制度转成 (度, 分, 秒) 整数（绝对值）。
func decimalToDMS(dec float64) (uint32, uint32, uint32) {
	abs := math.Abs(dec)
	d := math.Floor(abs)
	rem := (abs - d) * 60
	m := math.Floor(rem)
	s := (rem - m) * 60
	return uint32(d), uint32(m), uint32(math.Round(s))
}

// writeExifToJPEGFile 把 EXIF 写入磁盘上的 JPEG 文件（原地覆盖）。
func writeExifToJPEGFile(path string, t time.Time, geo *GeoData) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := writeJPEGExif(data, t, geo)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}
