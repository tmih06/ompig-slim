package shellconfig

import "testing"

func TestAutomaticBashSelectionIsNative(t *testing.T) {
	t.Setenv("SystemRoot", `D:\Windows`)
	for _, tc := range []struct {
		name, output, want string
	}{
		{"WSL only", "D:\\Windows\\System32\\bash.exe\r\n", ""},
		{"native after WSL", "D:\\Windows\\Sysnative\\bash.exe\r\nC:\\msys64\\usr\\bin\\bash.exe\r\n", `C:\msys64\usr\bin\bash.exe`},
		{"case insensitive WSL", "d:/WINDOWS/system32/BASH.EXE\n", ""},
		{"missing native", "C:\\missing\\bash.exe\n", ""},
		{"native with spaces", "C:\\Program Files\\Git\\bin\\bash.exe\r\n", `C:\Program Files\Git\bin\bash.exe`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := FirstNativeBashMatch(tc.output, func(path string) bool { return path != `C:\missing\bash.exe` })
			if got != tc.want {
				t.Fatalf("selected shell = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAutomaticBashSelectionWithoutSystemRoot(t *testing.T) {
	t.Setenv("SystemRoot", "")
	if got := FirstNativeBashMatch("C:\\Windows\\System32\\bash.exe\n", func(string) bool { return true }); got != "" {
		t.Fatalf("selected WSL launcher %q", got)
	}
}
