package video

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSupportedVideoFile(t *testing.T) {
	cases := map[string]bool{
		"movie.mp4":  true,
		"MOVIE.MP4":  true,
		"clip.Mkv":   true,
		"a.webm":     true,
		"b.mov":      true,
		"c.avi":      true,
		"d.m4v":      true,
		"e.txt":      false,
		"f.exe":      false,
		"g.lnk":      false,
		"noext":      false,
		"fake.mp4.b": false,
		// AppleDouble metadata companions (macOS writes "._name" next to every
		// file) are not real media even when the extension looks like a video.
		"._MIDV-186.mp4": false,
		"._movie.mkv":    false,
		"foo._bar.mp4":   true, // "._" only matches the path basename start
	}
	for name, want := range cases {
		if got := IsSupportedVideoFile(name); got != want {
			t.Errorf("IsSupportedVideoFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestNormalizeAndValidate(t *testing.T) {
	dir := t.TempDir()

	regular := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(regular, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	unsupported := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(unsupported, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("regular video file", func(t *testing.T) {
		normalized, info, err := NormalizeAndValidate(regular)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if normalized != regular {
			t.Errorf("normalized = %q, want %q", normalized, regular)
		}
		if info.Size() != 4 {
			t.Errorf("size = %d, want 4", info.Size())
		}
	})

	t.Run("case-insensitive extension", func(t *testing.T) {
		upper := filepath.Join(dir, "UPPER.MP4")
		if err := os.WriteFile(upper, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := NormalizeAndValidate(upper); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		if _, _, err := NormalizeAndValidate(filepath.Join(dir, "gone.mp4")); err != ErrNotExist {
			t.Errorf("err = %v, want ErrNotExist", err)
		}
	})

	t.Run("unsupported extension", func(t *testing.T) {
		if _, _, err := NormalizeAndValidate(unsupported); err != ErrUnsupportedType {
			t.Errorf("err = %v, want ErrUnsupportedType", err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		if _, _, err := NormalizeAndValidate(dir); err != ErrFolderNotSupported {
			t.Errorf("err = %v, want ErrFolderNotSupported", err)
		}
		if _, _, err := NormalizeAndValidate(dir + string(os.PathSeparator)); err != ErrFolderNotSupported {
			t.Errorf("trailing separator: err = %v, want ErrFolderNotSupported", err)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		if _, _, err := NormalizeAndValidate("   "); err != ErrEmptyPath {
			t.Errorf("err = %v, want ErrEmptyPath", err)
		}
	})
}

func TestUserMessage(t *testing.T) {
	if got := UserMessage(ErrAlreadyAdded); got != "已在收藏中" {
		t.Errorf("UserMessage(ErrAlreadyAdded) = %q", got)
	}
	if got := UserMessage(ErrFolderNotSupported); got == "" {
		t.Error("UserMessage returned empty string")
	}
}
