package main

import (
	"path/filepath"
	"strings"
)

// splitTitleExt 把 title 拆成 (stem, 小写ext含点)。
func splitTitleExt(title string) (string, string) {
	ext, ok := mediaExt(title)
	if ok {
		idx := strings.LastIndex(strings.ToLower(title), ext)
		return title[:idx], ext
	}
	return title, ""
}

// MatchResult 是一次匹配的产出。
type MatchResult struct {
	Image *ImageEntry
	Meta  *PhotoMeta
}

// indexImages 建索引：按 (小写stem, ext) 与 小写stem。
type imgIndex struct {
	byStemExt map[string][]*ImageEntry
	byStem    map[string][]*ImageEntry
}

func newIndex(images []*ImageEntry) *imgIndex {
	idx := &imgIndex{
		byStemExt: map[string][]*ImageEntry{},
		byStem:    map[string][]*ImageEntry{},
	}
	for i := range images {
		im := images[i]
		key := strings.ToLower(im.Stem) + "\x00" + im.Ext
		idx.byStemExt[key] = append(idx.byStemExt[key], im)
		idx.byStem[strings.ToLower(im.Stem)] = append(idx.byStem[strings.ToLower(im.Stem)], im)
	}
	return idx
}

func (idx *imgIndex) exact(stem, ext string) []*ImageEntry {
	return idx.byStemExt[strings.ToLower(stem)+"\x00"+ext]
}

// prefixFind 在同 ext 下找 stem 互为前缀的候选（用于 Google 截断名）。
// 返回最长匹配的 stem 对应候选。
func (idx *imgIndex) prefixFind(stem, ext string) []*ImageEntry {
	low := strings.ToLower(stem)
	var best []*ImageEntry
	bestLen := 0
	for key, lst := range idx.byStemExt {
		// key 形如 "stem\x00ext"
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 || parts[1] != ext {
			continue
		}
		s := parts[0]
		if len(s) < 5 || len(low) < 5 {
			continue
		}
		if strings.HasPrefix(low, s) || strings.HasPrefix(s, low) {
			if len(s) > bestLen {
				bestLen = len(s)
				best = lst
			}
		}
	}
	return best
}

// Match 把 JSON 与图片配对，返回：
//   - matches: 成功配对
//   - orphanJSONs: 无对应图片的 JSON（Meta 可能为 nil 表示解析失败）
//   - unmatchedImages: 未被任何 JSON 消费的图片
func Match(images []ImageEntry, jsons []JSONEntry) (matches []MatchResult, orphanJSONs []JSONEntry, unmatchedImages []ImageEntry) {
	// 指针化以便标记 Consumed
	imgPtrs := make([]*ImageEntry, len(images))
	for i := range images {
		imgPtrs[i] = &images[i]
	}
	idx := newIndex(imgPtrs)

	pick := func(lst []*ImageEntry) *ImageEntry {
		// 优先未消费、且与 JSON 同目录
		var first *ImageEntry
		for _, im := range lst {
			if im.Consumed {
				continue
			}
			if first == nil {
				first = im
			}
		}
		if first == nil {
			// 全消费则返回 nil（不重复消费）
			return nil
		}
		return first
	}

	for _, j := range jsons {
		if j.Meta == nil {
			orphanJSONs = append(orphanJSONs, j)
			continue
		}
		stem, ext := splitTitleExt(j.Meta.Title)
		var im *ImageEntry
		if lst := idx.exact(stem, ext); len(lst) > 0 {
			im = pick(lst)
		}
		if im == nil && ext != "" {
			if lst := idx.prefixFind(stem, ext); len(lst) > 0 {
				im = pick(lst)
			}
		}
		if im != nil {
			im.Consumed = true
			matches = append(matches, MatchResult{Image: im, Meta: j.Meta})
		} else {
			orphanJSONs = append(orphanJSONs, j)
		}
	}

	// Live Photo 配对：未消费的无扩展名/视频文件，若同 stem 有已消费图片则复用其 Meta
	for i := range images {
		im := &images[i]
		if im.Consumed {
			continue
		}
		if lp := findLivePhotoPartner(idx, im, matches); lp != nil {
			im.Consumed = true
			matches = append(matches, MatchResult{Image: im, Meta: lp.Meta})
		}
	}

	for i := range images {
		if !images[i].Consumed {
			unmatchedImages = append(unmatchedImages, images[i])
		}
	}
	return
}

// findLivePhotoPartner 找同 stem（大小写不敏感）已匹配的图片作为 Live Photo 静图来源。
func findLivePhotoPartner(idx *imgIndex, im *ImageEntry, matches []MatchResult) *MatchResult {
	low := strings.ToLower(im.Stem)
	for _, m := range matches {
		if strings.ToLower(m.Image.Stem) == low && m.Image.Path != im.Path {
			return &m
		}
	}
	return nil
}

// relPath 计算相对输入根的路径，用于孤儿 JSON 输出布局。
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return rel
}
