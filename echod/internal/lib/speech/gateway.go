package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Gateway is a tts-gateway server (github.com/ShinoharaTa/tts-gateway), which makes Japanese speech
// with ZEN-TTS on a machine of the owner's. It is asked for 16 kHz WAV, so what comes back is the
// device's own rate and needs no converting here.
type Gateway struct {
	Base  string // like http://192.168.1.20:8710
	Token string // TTS_TOKEN, for a server that has one; empty sends none
	Voice string // one of the server's voices by id; empty is its default
	HTTP  *http.Client
}

// GatewayRate is the rate the gateway is asked for: the device's voice rate.
const GatewayRate = 16000

// GatewayCredit is what ZEN-TTS's licence asks be shown wherever its speech is used.
const GatewayCredit = "音声合成：ZEN-TTS（https://zenryokukikai.com）"

// gatewayPause is the quiet after each sentence but the last, as the gateway's own sentencePauseMs.
const gatewayPause = 250

// gatewayMaxPart is the most characters asked for at once; the gateway takes 2,000.
const gatewayMaxPart = 1000

// Speak says text through the gateway and calls out with the samples at 16 kHz. The answer is asked
// for a sentence at a time, so the first is playing while the next is made: the gateway answers only
// once all it was asked for is made.
func (g Gateway) Speak(ctx context.Context, text string, out func(samples []int16) error) error {
	base := strings.TrimRight(g.Base, "/")
	if base == "" {
		return errors.New("speech: no tts-gateway address")
	}
	parts := sentences(text)
	for i, part := range parts {
		pause := gatewayPause
		if i == len(parts)-1 {
			pause = 0
		}
		if err := g.speakPart(ctx, base, part, pause, out); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
	}
	return nil
}

func (g Gateway) speakPart(ctx context.Context, base, text string, pause int, out func([]int16) error) error {
	body := map[string]any{
		"text":              text,
		"format":            "wav",
		"sampleRate":        GatewayRate,
		"trailingSilenceMs": pause,
	}
	if g.Voice != "" {
		body["voice"] = g.Voice
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/synthesize", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	hc := g.HTTP
	if hc == nil {
		hc = client
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("speech: tts-gateway: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return gatewayRefusal(resp)
	}
	data, err := wavData(resp.Body)
	if err != nil {
		return err
	}
	return stream(data, out)
}

// gatewayRefusal is the gateway's error as one line: its message when it sent its JSON form.
func gatewayRefusal(resp *http.Response) error {
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxError))
	var e struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if json.Unmarshal(msg, &e) == nil && e.Error.Message != "" {
		return fmt.Errorf("speech: tts-gateway: %s: %s (%s)", resp.Status, e.Error.Message, e.Error.Code)
	}
	return fmt.Errorf("speech: tts-gateway: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
}

// wavData reads a WAV header up to its samples and returns the rest as the samples: 16-bit PCM,
// mono, at GatewayRate, or an error saying what it is instead.
func wavData(r io.Reader) (io.Reader, error) {
	var head [12]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, fmt.Errorf("speech: tts-gateway: not a WAV: %w", err)
	}
	if string(head[0:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return nil, errors.New("speech: tts-gateway: not a WAV")
	}
	sawFormat := false
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return nil, fmt.Errorf("speech: tts-gateway: a WAV with no samples: %w", err)
		}
		id, size := string(ch[0:4]), binary.LittleEndian.Uint32(ch[4:8])
		switch id {
		case "fmt ":
			if size < 16 || size > 1024 {
				return nil, errors.New("speech: tts-gateway: a WAV format chunk of a strange size")
			}
			f := make([]byte, size+size&1)
			if _, err := io.ReadFull(r, f); err != nil {
				return nil, fmt.Errorf("speech: tts-gateway: %w", err)
			}
			kind, chans := binary.LittleEndian.Uint16(f[0:2]), binary.LittleEndian.Uint16(f[2:4])
			rate, bits := binary.LittleEndian.Uint32(f[4:8]), binary.LittleEndian.Uint16(f[14:16])
			if (kind != 1 && kind != 0xfffe) || chans != 1 || rate != GatewayRate || bits != 16 {
				return nil, fmt.Errorf("speech: tts-gateway: a WAV of %d Hz, %d channels, %d bits, not 16 kHz 16-bit mono", rate, chans, bits)
			}
			sawFormat = true
		case "data":
			if !sawFormat {
				return nil, errors.New("speech: tts-gateway: a WAV with its samples before its format")
			}
			// A streamed WAV may give its size as 0 or all ones; either way the samples run to the end.
			if size == 0 || size == 0xffffffff {
				return r, nil
			}
			return io.LimitReader(r, int64(size)), nil
		default:
			if _, err := io.CopyN(io.Discard, r, int64(size+size&1)); err != nil {
				return nil, fmt.Errorf("speech: tts-gateway: %w", err)
			}
		}
	}
}

// sentences cuts an answer where a sentence ends (。！？!? or a line break), keeping the mark with
// its sentence, and drops the blank ones. A piece longer than gatewayMaxPart is cut there too.
func sentences(text string) []string {
	var parts []string
	var cur strings.Builder
	end := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			parts = append(parts, s)
		}
		cur.Reset()
	}
	for _, r := range text {
		if r == '\n' {
			end()
			continue
		}
		cur.WriteRune(r)
		if strings.ContainsRune("。！？!?", r) || utf8.RuneCountInString(cur.String()) >= gatewayMaxPart {
			end()
		}
	}
	end()
	// A closing bracket or quote after the mark belongs to the sentence before it.
	for i := 1; i < len(parts); i++ {
		if r, _ := utf8.DecodeRuneInString(parts[i]); strings.ContainsRune("」』）)\"”", r) {
			parts[i-1] += string(r)
			parts[i] = strings.TrimSpace(parts[i][utf8.RuneLen(r):])
		}
	}
	return slicesCompact(parts)
}

func slicesCompact(parts []string) []string {
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
