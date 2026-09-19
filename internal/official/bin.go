package official

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// installBase is Unity's CDN directory holding install.sh and install.ps1.
const installBase = "https://public-cdn.cloud.unity3d.com/hub/prod/cli/"

// Bin resolves the official CLI binary: UTK_UNITY_BIN overrides, else PATH.
func Bin() (string, error) {
	if p := os.Getenv("UTK_UNITY_BIN"); p != "" {
		return p, nil
	}
	p, err := exec.LookPath("unity")
	if err != nil {
		// The install command, not a doc link: the link is what leaves people
		// stuck, and this is the one thing they need to type next. The two
		// installers are not interchangeable — install.sh aborts on MINGW/MSYS
		// and tells you to use install.ps1, so printing the bash one-liner on
		// Windows just sends the user around another loop.
		install := "curl -fsSL " + installBase + "install.sh | UNITY_CLI_CHANNEL=beta bash"
		if runtime.GOOS == "windows" {
			install = `powershell -c "$env:UNITY_CLI_CHANNEL='beta'; irm ` + installBase + `install.ps1 | iex"`
		}
		return "", errors.New("official Unity CLI not found on PATH\n" +
			"  install it:  " + install + "\n" +
			"  then reopen the terminal, or point utk at an existing binary with UTK_UNITY_BIN")
	}
	return p, nil
}
