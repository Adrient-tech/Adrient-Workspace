package drive

import "testing"

func TestCategory(t *testing.T) {
	cases := map[string]string{
		"image/png": "Images", "video/mp4": "Video", "audio/mpeg": "Audio", "application/pdf": "PDF",
		"text/plain": "Text", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "Spreadsheets",
		"application/zip": "Other",
	}
	for in, want := range cases {
		if got := Category(in); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
}
