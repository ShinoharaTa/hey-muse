package voice

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/HuskerMinion/techo5/echod/internal/lib/standalone"
)

func standaloneHomeReady(stt string) bool {
	host, _, err := net.SplitHostPort(stt)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return false
	}
	return standalone.Device.Enabled()
}

func speakHomeReply(ctx context.Context, text string, out func([]int16) error) (bool, error) {
	if !standalone.Device.Enabled() {
		return false, nil
	}
	name := fmt.Sprintf("%x.pcm", sha256.Sum256([]byte(text)))
	raw, err := os.ReadFile(filepath.Join(standalone.Directory, "voice", name))
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
