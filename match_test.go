package main

import "testing"

func TestSplitTitleExt(t *testing.T) {
	cases := []struct {
		title    string
		wantStem string
		wantExt  string
	}{
		{"IMG123.jpg", "IMG123", ".jpg"},
		{"101.mp4", "101", ".mp4"},
		{"IMG_2721.HEIC", "IMG_2721", ".heic"},
		{"noext", "noext", ""},
	}
	for _, c := range cases {
		s, e := splitTitleExt(c.title)
		if s != c.wantStem || e != c.wantExt {
			t.Errorf("splitTitleExt(%q) = (%q,%q) want (%q,%q)", c.title, s, e, c.wantStem, c.wantExt)
		}
	}
}

func TestMatchExactAndPrefix(t *testing.T) {
	images := []ImageEntry{
		{Path: "/a/IMG123.jpg", Stem: "IMG123", Ext: ".jpg"},
		{Path: "/a/IMG456.png", Stem: "IMG456", Ext: ".png"},
		// Google 双向截断：图片名截断为 0000，title 截断为 000010C6...
		{Path: "/a/PREFIX-27318-0000.jpg", Stem: "PREFIX-27318-0000", Ext: ".jpg"},
	}
	jsons := []JSONEntry{
		{Path: "/a/IMG123.jpg.json", Meta: &PhotoMeta{Title: "IMG123.jpg"}},
		// title 含完整名（实际图片名被截断）→ 前缀匹配
		{Path: "/a/PREFIX-27318-000010C648561E40.json", Meta: &PhotoMeta{Title: "PREFIX-27318-000010C648561E40.jpg"}},
	}

	matches, orphans, unmatched := Match(images, jsons)
	if len(matches) != 2 {
		t.Fatalf("期望 2 匹配，得到 %d", len(matches))
	}
	if len(orphans) != 0 {
		t.Fatalf("期望 0 孤儿，得到 %d", len(orphans))
	}
	if len(unmatched) != 1 {
		t.Fatalf("期望 1 未匹配图片（IMG456.png），得到 %d", len(unmatched))
	}
}

func TestMatchOrphanJSON(t *testing.T) {
	images := []ImageEntry{
		{Path: "/a/IMG1.jpg", Stem: "IMG1", Ext: ".jpg"},
	}
	jsons := []JSONEntry{
		{Path: "/a/IMG1.jpg.json", Meta: &PhotoMeta{Title: "IMG1.jpg"}},
		{Path: "/a/ORPHAN.json", Meta: &PhotoMeta{Title: "NOEXIST.jpg"}},
	}
	matches, orphans, _ := Match(images, jsons)
	if len(matches) != 1 {
		t.Fatalf("期望 1 匹配，得到 %d", len(matches))
	}
	if len(orphans) != 1 {
		t.Fatalf("期望 1 孤儿，得到 %d", len(orphans))
	}
}

func TestMatchCaseInsensitive(t *testing.T) {
	images := []ImageEntry{
		{Path: "/a/IMG_4202.JPG", Stem: "IMG_4202", Ext: ".jpg"},
	}
	jsons := []JSONEntry{
		{Path: "/a/IMG_4202.json", Meta: &PhotoMeta{Title: "IMG_4202.HEIC"}},
	}
	// ext 不同 (.jpg vs .heic) → 不匹配，应为孤儿
	_, orphans, _ := Match(images, jsons)
	if len(orphans) != 1 {
		t.Fatalf("不同扩展名应不匹配，期望孤儿 1，得到 %d", len(orphans))
	}

	// 同扩展名不同大小写 → 匹配
	jsons2 := []JSONEntry{
		{Path: "/a/IMG_4202.json", Meta: &PhotoMeta{Title: "IMG_4202.JPG"}},
	}
	matches, _, _ := Match(images, jsons2)
	if len(matches) != 1 {
		t.Fatalf("大小写不敏感应匹配，得到 %d", len(matches))
	}
}
