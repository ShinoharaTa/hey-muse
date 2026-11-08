package voice

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

const homeDirectory = "/data/misc/techo5/standalone-home"

func standaloneHomeReady(stt string) bool {
	host, _, err := net.SplitHostPort(stt)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return false
	}
	_, err = os.Stat(homeDirectory + "/enabled")
	return err == nil
}

func speakHomeReply(ctx context.Context, text string, out func([]int16) error) (bool, error) {
	if _, err := os.Stat(homeDirectory + "/enabled"); err != nil {
		return false, nil
	}
	name := fmt.Sprintf("%x.pcm", sha256.Sum256([]byte(text)))
	raw, err := os.ReadFile(filepath.Join(homeDirectory, "voice", name))
	if err != nil || len(raw) == 0 || len(raw)%2 != 0 {
		return false, nil
	}
	for len(raw) > 0 {
		if err := ctx.Err(); err != nil {
			return true, err
		}
		n := min(2*museChunk, len(raw))
		if err := out(samplesOf(raw[:n])); err != nil {
			return true, err
		}
		raw = raw[n:]
	}
	return true, nil
}
