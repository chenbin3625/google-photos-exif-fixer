package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	flagIn      = flag.String("in", "/Users/chenbin/Downloads/Takeout/Google 相册", "输入目录")
	flagOut     = flag.String("out", "/Users/chenbin/Downloads/Takeout_merged", "输出目录")
	flagDryRun  = flag.Bool("dry-run", true, "试运行（不写文件）；设为 false 真写")
	flagWorkers = flag.Int("workers", 4, "并发数")
	flagLog     = flag.String("log", "", "日志文件路径（默认 <out>/_merge.log）")
	flagNoEXIF  = flag.Bool("no-exif", false, "跳过 JPEG EXIF 写入（仍设 mtime）")
)

type stats struct {
	matched       int64
	orphanJSON    int64
	unmatchedImg  int64
	exifOK        int64
	exifSkip      int64
	exifFail      int64
	copyErr       int64
}

func main() {
	flag.Parse()

	in := *flagIn
	out := *flagOut
	logPath := *flagLog
	if logPath == "" {
		logPath = filepath.Join(out, "_merge.log")
	}

	// 准备日志
	var logF *os.File
	var logMu sync.Mutex
	if !*flagDryRun {
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "创建日志目录失败: %v\n", err)
			os.Exit(1)
		}
		f, err := os.Create(logPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建日志失败: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		logF = f
	}
	logf := func(format string, a ...any) {
		msg := fmt.Sprintf(format, a...)
		logMu.Lock()
		if logF != nil {
			fmt.Fprintln(logF, msg)
		}
		logMu.Unlock()
	}

	fmt.Printf("输入: %s\n输出: %s\n试运行: %v\n并发: %d\n\n", in, out, *flagDryRun, *flagWorkers)

	// Pass 1: 遍历
	fmt.Println("扫描输入目录...")
	wr, err := WalkDir(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "遍历失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("图片/视频: %d\nJSON: %d\n\n", len(wr.Images), len(wr.JSONs))

	// Pass 2: 匹配
	fmt.Println("匹配 JSON 与图片...")
	matches, orphans, unmatchedImgs := Match(wr.Images, wr.JSONs)
	fmt.Printf("配对成功: %d\n孤儿 JSON: %d\n无 JSON 图片: %d\n\n", len(matches), len(orphans), len(unmatchedImgs))

	var st stats

	// 处理孤儿 JSON：复制到 _unmatched/
	unmatchedDir := filepath.Join(out, "_unmatched")
	for _, j := range orphans {
		rel := relPath(in, j.Path)
		dst := filepath.Join(unmatchedDir, rel)
		title := "<解析失败>"
		if j.Meta != nil {
			title = j.Meta.Title
		}
		logf("[ORPHAN-JSON] %s (title=%s) -> %s", j.Path, title, dst)
		st.orphanJSON++
		if !*flagDryRun {
			if err := copyFile(j.Path, uniquePath(unmatchedDir, rel)); err != nil {
				logf("[ORPHAN-JSON-COPY-ERR] %s: %v", j.Path, err)
			}
		}
	}

	// 构建输出任务
	tasks := make([]*Task, 0, len(matches)+len(unmatchedImgs))
	for _, m := range matches {
		jpeg := isJPEG(m.Image.Ext)
		t, src := ResolveTakenTime(m.Meta, m.Image.Path, jpeg)
		sub := t.Format("2006/01")
		tasks = append(tasks, &Task{
			Src: m.Image.Path, TakenTime: t, Source: src, Meta: m.Meta,
			WriteExif: jpeg && !*flagNoEXIF, SubDir: sub, OutName: outputName(m.Image),
		})
	}
	// 无 JSON 图片兜底
	for i := range unmatchedImgs {
		im := &unmatchedImgs[i]
		jpeg := isJPEG(im.Ext)
		t, src := ResolveTakenTime(nil, im.Path, jpeg)
		sub := t.Format("2006/01")
		tasks = append(tasks, &Task{
			Src: im.Path, TakenTime: t, Source: src, Meta: nil,
			WriteExif: false, // 无 JSON 不写 EXIF（避免写入兜底来源的不可靠时间）
			SubDir: sub, OutName: outputName(im),
		})
	}

	// 执行（并发）
	taskCh := make(chan *Task)
	resCh := make(chan TaskResult, 64)
	var wg sync.WaitGroup
	nw := *flagWorkers
	if nw < 1 {
		nw = 1
	}
	for i := 0; i < nw; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range taskCh {
				resCh <- execTask(t, out, logf)
			}
		}()
	}
	go func() {
		for _, t := range tasks {
			taskCh <- t
		}
		close(taskCh)
		wg.Wait()
		close(resCh)
	}()

	count := 0
	for r := range resCh {
		count++
		if r.Err != nil {
			atomic.AddInt64(&st.copyErr, 1)
			logf("[COPY-ERR] %s -> %s: %v", r.Task.Src, r.Dst, r.Err)
		} else {
			logf("[OK] %s -> %s (taken=%s via %s, exif=%s)", r.Task.Src, r.Dst,
				r.Task.TakenTime.Format("2006-01-02 15:04:05"), r.Task.Source, r.ExifStatus)
		}
		switch {
		case strings.HasPrefix(r.ExifStatus, "ok"):
			atomic.AddInt64(&st.exifOK, 1)
		case strings.HasPrefix(r.ExifStatus, "skip"):
			atomic.AddInt64(&st.exifSkip, 1)
		case strings.HasPrefix(r.ExifStatus, "failed"):
			atomic.AddInt64(&st.exifFail, 1)
		}
		if count%500 == 0 {
			fmt.Printf("已处理 %d/%d...\n", count, len(tasks))
		}
	}
	st.matched = int64(len(tasks)) - int64(len(unmatchedImgs))
	st.unmatchedImg = int64(len(unmatchedImgs))

	// 汇总
	fmt.Println("\n===== 汇总 =====")
	fmt.Printf("输出任务总数:   %d\n", len(tasks))
	fmt.Printf("配对成功:       %d\n", st.matched)
	fmt.Printf("无 JSON 图片:   %d\n", st.unmatchedImg)
	fmt.Printf("孤儿 JSON:      %d\n", st.orphanJSON)
	fmt.Printf("EXIF 写入成功:  %d\n", st.exifOK)
	fmt.Printf("EXIF 跳过(非JPEG/无JSON): %d\n", st.exifSkip)
	fmt.Printf("EXIF 写入失败:  %d\n", st.exifFail)
	fmt.Printf("复制错误:       %d\n", st.copyErr)
	if !*flagDryRun {
		fmt.Printf("\n日志: %s\n", logPath)
	}
	if *flagDryRun {
		fmt.Println("\n(试运行模式：未写入任何文件。加 -dry-run=false 真正执行。)")
	}
}

// execTask 执行单个复制+元数据任务。
func execTask(t *Task, outRoot string, logf func(string, ...any)) TaskResult {
	dir := filepath.Join(outRoot, t.SubDir)
	dst := uniquePath(dir, t.OutName)

	if *flagDryRun {
		return TaskResult{Task: t, Dst: dst, ExifStatus: "skip"}
	}

	// 复制
	if err := copyFile(t.Src, dst); err != nil {
		return TaskResult{Task: t, Dst: dst, Err: err}
	}

	// JPEG EXIF 写入（best-effort）：总是用 JSON 的 photoTakenTime 覆盖/写入，
	// 因为 Google 里改过的时间只体现在 JSON，原图 EXIF 可能是旧的。
	exifStatus := "skip"
	if t.WriteExif && t.Meta != nil {
		if err := writeExifToJPEGFile(dst, t.TakenTime, t.Meta.Geo()); err != nil {
			exifStatus = "failed:" + err.Error()
			// 写入失败：回滚（重新复制无 EXIF 版本）
			_ = copyFile(t.Src, dst)
		} else {
			// 验证：重读 EXIF DateTimeOriginal 可读
			if _, ok := verifyExifTime(dst); ok {
				exifStatus = "ok"
			} else {
				exifStatus = "failed:verify"
				_ = copyFile(t.Src, dst)
			}
		}
	}

	// 设 mtime（所有格式）：在 EXIF 写入之后，避免被 WriteFile 重置
	if err := setMtime(dst, t.TakenTime); err != nil {
		logf("[MTIME-ERR] %s: %v", dst, err)
	}
	return TaskResult{Task: t, Dst: dst, ExifStatus: exifStatus}
}

// verifyExifTime 写入后验证可读。
func verifyExifTime(path string) (time.Time, bool) {
	t, ok := readJPEGExifTime(path)
	return t, ok
}

// 保留 bufio 引用（日志缓冲备用）
var _ = bufio.NewWriter
