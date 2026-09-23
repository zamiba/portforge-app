package models

import (
	"encoding/json"
	"testing"
)

// Notices ride on a SoftwareVersion, beside that release's notes. This pins the
// wire shape the catalog writes and the frontend reads.
func TestNoticesShape(t *testing.T) {
	const in = `{"_itemType":"VideoGameFanPort","versions":[
		{"_itemType":"SoftwareVersion","title":"Nautilus - Alfa 3.0.0","content":"- notes",
		 "notices":[{"type":"warning","affectedPlatforms":["Windows"],"message":"Broken on **Windows**."}]},
		{"_itemType":"SoftwareVersion","title":"Mary Celeste - Alfa 2.0.0"}]}`

	var v VideoGameVersion
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(v.Versions) != 2 {
		t.Fatalf("want 2 versions, got %d", len(v.Versions))
	}
	if got := len(v.Versions[0].Notices); got != 1 {
		t.Fatalf("want 1 notice on the first version, got %d", got)
	}
	n := v.Versions[0].Notices[0]
	if n.Type != NoticeWarning || n.Message != "Broken on **Windows**." {
		t.Errorf("notice decoded as %+v", n)
	}
	if len(v.Versions[1].Notices) != 0 {
		t.Errorf("a version without notices should have none, got %+v", v.Versions[1].Notices)
	}

	// A version carrying no notices must not emit the key, so adding this field
	// leaves every existing item's metadata byte-identical when rewritten.
	out, err := json.Marshal(v.Versions[1])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != `{"_itemType":"SoftwareVersion","title":"Mary Celeste - Alfa 2.0.0"}` {
		t.Errorf("empty notices should be omitted, got %s", out)
	}
}

// An unknown type renders as a warning rather than disappearing: the catalog is
// synced independently of app releases, so an older app will meet newer notices.
func TestNoticeLevelDegradesToWarning(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{NoticeInfo, NoticeInfo},
		{"INFO", NoticeInfo},
		{NoticeWarning, NoticeWarning},
		{"critical", NoticeWarning},
		{"", NoticeWarning},
	} {
		if got := (Notice{Type: tc.in}).Level(); got != tc.want {
			t.Errorf("Level(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNoticeAppliesTo(t *testing.T) {
	for _, tc := range []struct {
		name      string
		platforms []string
		target    string
		want      bool
	}{
		{"no platforms means every platform", nil, "Linux", true},
		{"exact match", []string{"Windows"}, "Windows", true},
		{"other platform", []string{"Windows"}, "Linux", false},
		{"one of several", []string{"Windows", "Mac"}, "Mac", true},
		{"bare OS covers an arch build", []string{"Windows"}, "Windows-amd64", true},
		{"arch notice covers a bare OS build", []string{"Mac-arm64"}, "Mac", true},
		{"a different arch of the same OS", []string{"Mac-arm64"}, "Mac-x64", false},
		{"a token this version does not know", []string{"Steam Deck"}, "Linux", false},
		{"case is not significant", []string{"windows"}, "Windows", true},
	} {
		n := Notice{Type: NoticeWarning, AffectedPlatforms: tc.platforms}
		if got := n.AppliesTo(tc.target); got != tc.want {
			t.Errorf("%s: AppliesTo(%q) with %v = %v, want %v",
				tc.name, tc.target, tc.platforms, got, tc.want)
		}
	}
}
