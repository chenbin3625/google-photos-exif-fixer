# google-photos-exif-fixer

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE) [![Release](https://img.shields.io/github/v/release/chenbin3625/google-photos-exif-fixer)](https://github.com/chenbin3625/google-photos-exif-fixer/releases)

> Write the photo metadata (capture time, GPS) exported from Google Photos Takeout back into EXIF, and organize directories by year and month.

## Background

After exporting Google Photos through [Google Takeout](https://takeout.google.com/), you get:

- Photo/video files, but the **EXIF metadata may be missing** (capture time, GPS coordinates, etc.)
- File names may be **truncated** (Google's 46-character limit)
- Each file comes with a `.json` sidecar that holds the complete metadata

These JSON files are the real source of the metadata, but most photo management software will not read them.

**google-photos-exif-fixer** solves this problem: it automatically writes the metadata from the JSON sidecars back into the photos' EXIF, and outputs a clean photo library organized by `YYYY/MM`.

## Features

- **JSON-photo pairing**: exact matching + prefix matching (to cope with Google's truncated file names) + automatic Live Photo association
- **EXIF writing**: hand-written binary EXIF APP1 segment, writing the capture time (`DateTimeOriginal`) and GPS coordinates
- **Time fallback chain**: JSON `photoTakenTime` → EXIF `DateTimeOriginal` → folder year inference → file mtime
- **Directory organization**: output split into `YYYY/MM/` directories, automatically handling file name conflicts
- **Concurrent processing**: configurable number of workers, fast processing of large numbers of photos
- **Dry-run mode**: trial run by default, execute for real once you have confirmed everything is correct
- **Detailed logging**: the processing result of every file is recorded in the log

## Installation

### Download from GitHub Releases

Go to the [Releases](https://github.com/chenbin3625/google-photos-exif-fixer/releases) page to download the binary for your platform.

### Build from source

```bash
go install github.com/chenbin3625/google-photos-exif-fixer@latest
```

Or clone and build locally:

```bash
git clone https://github.com/chenbin3625/google-photos-exif-fixer.git
cd google-photos-exif-fixer
go build -o google-photos-exif-fixer .
```

## Usage

### Basic usage

```bash
# 1. Trial run (default), preview what will happen
./google-photos-exif-fixer \
  -in "/path/to/Takeout/Google 相册" \
  -out "/path/to/output"

# 2. Once you have confirmed everything is correct, execute for real
./google-photos-exif-fixer \
  -in "/path/to/Takeout/Google 相册" \
  -out "/path/to/output" \
  -dry-run=false
```

### CLI flags

| Flag | Default | Description |
|---|---|---|
| `-in` | `./Takeout/Google 相册` | Input directory (Google Takeout export path) |
| `-out` | `./Takeout_merged` | Output directory |
| `-dry-run` | `true` | Trial run mode, does not write files; set to `false` to execute for real |
| `-workers` | `4` | Number of concurrent workers |
| `-log` | `<out>/_merge.log` | Log file path |
| `-no-exif` | `false` | Skip JPEG EXIF writing (the file mtime is still set) |

### Output structure

```
output/
├── 2019/
│   ├── 01/
│   │   ├── IMG_1234.jpg          ← EXIF fixed
│   │   └── IMG_1235.heic
│   └── 06/
│       └── vacation.mp4
├── 2023/
│   └── 12/
│       └── photo.jpg
└── _unmatched/                   ← orphan JSON with no matching image
    └── ...
```

## How it works

```
Input directory
  │
  ├─ Walk all files, classify as ImageEntry / JSONEntry
  │
  ├─ Match: JSON sidecar ↔ image
  │    ├─ Exact match (stem + ext)
  │    ├─ Prefix match (to cope with Google truncation)
  │    └─ Live Photo association
  │
  ├─ Parse the capture time (multi-level fallback)
  │    ├─ JSON photoTakenTime
  │    ├─ EXIF DateTimeOriginal
  │    ├─ Folder year inference
  │    └─ File mtime
  │
  └─ Concurrent output
       ├─ Copy files to YYYY/MM/
       ├─ Write EXIF (JPEG, when JSON is present)
       └─ Set file mtime
```

### EXIF writing details

For JPEG files, the tool will:

1. **Strip** the existing APP1(Exif) segment (to avoid conflicts between multiple EXIF segments)
2. **Construct** a minimal EXIF APP1 segment (hand-written binary TIFF structure):
   - IFD0: `DateTime`, `ExifIFDPointer`, `GPSIFDPointer`
   - ExifIFD: `DateTimeOriginal`, `DateTimeDigitized`
   - GPSIFD: latitude/longitude (degrees-minutes-seconds RATIONAL format)
3. **Insert** it after the SOI, making it the EXIF that decoders prefer
4. **Verify**: re-read the EXIF to confirm `DateTimeOriginal` is readable, and roll back if that fails

## Supported formats

**Images**: JPG/JPEG, PNG, HEIC, GIF, WebP, BMP, TIFF/TIF, NEF, DNG

**Videos**: MP4, MOV, M4V, AVI, LIVP

> EXIF writing only takes effect for JPEG. For other formats only the file mtime is set.

## License

MIT

---

# 中文

> 把 Google Photos Takeout 导出的照片元数据（拍摄时间、GPS）写回 EXIF，并按年月整理目录。

## 背景

通过 [Google Takeout](https://takeout.google.com/) 导出 Google 相册后，你会得到：

- 照片/视频文件，但 **EXIF 元数据可能丢失**（拍摄时间、GPS 坐标等）
- 文件名可能被 **截断**（Google 的 46 字符限制）
- 每个文件附带一个 `.json` sidecar，里面保存着完整的元数据

这些 JSON 文件才是元数据的真实来源，但大多数照片管理软件不会读取它们。

**google-photos-exif-fixer** 解决这个问题：自动将 JSON sidecar 中的元数据写回照片的 EXIF，并输出一份干净的、按 `YYYY/MM` 整理的照片库。

## 功能

- **JSON-照片配对**：精确匹配 + 前缀匹配（应对 Google 截断文件名）+ Live Photo 自动关联
- **EXIF 写入**：手写二进制 EXIF APP1 段，写入拍摄时间（`DateTimeOriginal`）和 GPS 坐标
- **时间回退链**：JSON `photoTakenTime` → EXIF `DateTimeOriginal` → 文件夹年份推断 → 文件 mtime
- **目录整理**：输出按 `YYYY/MM/` 分目录，自动处理文件名冲突
- **并发处理**：可配置 worker 数量，快速处理大量照片
- **Dry-run 模式**：默认试运行，确认无误后再真正执行
- **详细日志**：每个文件的处理结果都记录在日志中

## 安装

### 从 GitHub Releases 下载

前往 [Releases](https://github.com/chenbin3625/google-photos-exif-fixer/releases) 页面下载对应平台的二进制文件。

### 从源码编译

```bash
go install github.com/chenbin3625/google-photos-exif-fixer@latest
```

或克隆后本地编译：

```bash
git clone https://github.com/chenbin3625/google-photos-exif-fixer.git
cd google-photos-exif-fixer
go build -o google-photos-exif-fixer .
```

## 使用

### 基本用法

```bash
# 1. 试运行（默认），预览会做什么
./google-photos-exif-fixer \
  -in "/path/to/Takeout/Google 相册" \
  -out "/path/to/output"

# 2. 确认无误后，真正执行
./google-photos-exif-fixer \
  -in "/path/to/Takeout/Google 相册" \
  -out "/path/to/output" \
  -dry-run=false
```

### CLI 参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-in` | `./Takeout/Google 相册` | 输入目录（Google Takeout 导出路径） |
| `-out` | `./Takeout_merged` | 输出目录 |
| `-dry-run` | `true` | 试运行模式，不写入文件；设为 `false` 真正执行 |
| `-workers` | `4` | 并发 worker 数量 |
| `-log` | `<out>/_merge.log` | 日志文件路径 |
| `-no-exif` | `false` | 跳过 JPEG EXIF 写入（仍会设置文件 mtime） |

### 输出结构

```
output/
├── 2019/
│   ├── 01/
│   │   ├── IMG_1234.jpg          ← EXIF 已修复
│   │   └── IMG_1235.heic
│   └── 06/
│       └── vacation.mp4
├── 2023/
│   └── 12/
│       └── photo.jpg
└── _unmatched/                   ← 无对应图片的孤儿 JSON
    └── ...
```

## 工作原理

```
输入目录
  │
  ├─ 遍历所有文件，分类为 ImageEntry / JSONEntry
  │
  ├─ 匹配：JSON sidecar ↔ 图片
  │    ├─ 精确匹配（stem + ext）
  │    ├─ 前缀匹配（应对 Google 截断）
  │    └─ Live Photo 关联
  │
  ├─ 解析拍摄时间（多级回退）
  │    ├─ JSON photoTakenTime
  │    ├─ EXIF DateTimeOriginal
  │    ├─ 文件夹年份推断
  │    └─ 文件 mtime
  │
  └─ 并发输出
       ├─ 复制文件到 YYYY/MM/
       ├─ 写入 EXIF（JPEG，有 JSON 时）
       └─ 设置文件 mtime
```

### EXIF 写入细节

对于 JPEG 文件，工具会：

1. **剥离** 现有 APP1(Exif) 段（避免多 EXIF 段冲突）
2. **构造** 最小 EXIF APP1 段（手写二进制 TIFF 结构）：
   - IFD0: `DateTime`, `ExifIFDPointer`, `GPSIFDPointer`
   - ExifIFD: `DateTimeOriginal`, `DateTimeDigitized`
   - GPSIFD: 纬度/经度（度分秒 RATIONAL 格式）
3. **插入** 到 SOI 之后，使其成为解码器的首选 EXIF
4. **验证**：重读 EXIF 确认 `DateTimeOriginal` 可读，失败则回滚

## 支持的格式

**图片**：JPG/JPEG、PNG、HEIC、GIF、WebP、BMP、TIFF/TIF、NEF、DNG

**视频**：MP4、MOV、M4V、AVI、LIVP

> EXIF 写入仅对 JPEG 生效。其他格式只设置文件 mtime。

## 许可证

MIT
