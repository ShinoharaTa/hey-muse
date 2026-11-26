package speech

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// testWAV is samples as the gateway sends them: RIFF, a fmt chunk, a chunk to skip, then data.
func testWAV(rate uint32, samples []int16) []byte {
	var b []byte
	data := 2 * len(samples)
	b = append(b, "RIFF"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(4+24+12+8+data))
	b = append(b, "WAVEfmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint32(b, rate)
	b = binary.LittleEndian.AppendUint32(b, rate*2)
	b = binary.LittleEndian.AppendUint16(b, 2)
	b = binary.LittleEndian.AppendUint16(b, 16)
	b = append(b, "LIST"...)
	b = binary.LittleEndian.AppendUint32(b, 3) // odd: padded to 4
	b = append(b, 'a', 'b', 'c', 0)
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(data))
	for _, s := range samples {
		b = binary.LittleEndian.AppendUint16(b, uint16(s))
	}
	return b
}

// fakeGateway answers each sentence with as many samples as it has characters, all of one value
// counting up, and keeps what it was asked.
type fakeGateway struct {
	mu     sync.Mutex
	asked  []map[string]any
	auth   []string
	status int
	rate   uint32
}

func (f *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/synthesize" || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "wrong request", http.StatusBadRequest)
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.asked = append(f.asked, body)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	n := len(f.asked)
	f.mu.Unlock()
	if f.status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		w.Write([]byte(`{"error":{"code":"model_not_installed","message":"voice kikka is not installed"}}`))
		return
	}
	rate := f.rate
	if rate == 0 {
		rate = GatewayRate
	}
	samples := make([]int16, len([]rune(body["text"].(string))))
	for i := range samples {
		samples[i] = int16(n)
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Write(testWAV(rate, samples))
}

func TestGatewaySpeaksASentenceAtATime(t *testing.T) {
	f := &fakeGateway{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	g := Gateway{Base: srv.URL + "/", Token: "tok", Voice: "kikka"}
	var got []int16
	err := g.Speak(context.Background(), "おはよう。今日は晴れだよ！\n傘はいらないかな？", func(s []int16) error {
		got = append(got, s...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []int16{}
	for i, s := range []string{"おはよう。", "今日は晴れだよ！", "傘はいらないかな？"} {
		for range []rune(s) {
			want = append(want, int16(i+1))
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("samples %v, want %v", got, want)
	}
	for i, body := range f.asked {
		if body["format"] != "wav" || body["sampleRate"] != float64(GatewayRate) || body["voice"] != "kikka" || f.auth[i] != "Bearer tok" {
			t.Errorf("request %d: %v auth %q", i, body, f.auth[i])
		}
		pause := float64(gatewayPause)
		if i == len(f.asked)-1 {
			pause = 0
		}
		if body["trailingSilenceMs"] != pause {
			t.Errorf("request %d: trailing silence %v, want %v", i, body["trailingSilenceMs"], pause)
		}
	}
}

func TestGatewaySendsNoVoiceOrTokenUnset(t *testing.T) {
	f := &fakeGateway{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	if err := (Gateway{Base: srv.URL}).Speak(context.Background(), "はい", func([]int16) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, has := f.asked[0]["voice"]; has || f.auth[0] != "" {
		t.Errorf("asked %v with auth %q", f.asked[0], f.auth[0])
	}
}

func TestGatewayReportsItsRefusal(t *testing.T) {
	f := &fakeGateway{status: http.StatusUnprocessableEntity}
	srv := httptest.NewServer(f)
	defer srv.Close()
	err := (Gateway{Base: srv.URL}).Speak(context.Background(), "はい。", func([]int16) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "voice kikka is not installed") || !strings.Contains(err.Error(), "model_not_installed") {
		t.Fatalf("err %v", err)
	}
}

func TestGatewayRefusesAnotherRate(t *testing.T) {
	f := &fakeGateway{rate: 44100}
	srv := httptest.NewServer(f)
	defer srv.Close()
	err := (Gateway{Base: srv.URL}).Speak(context.Background(), "はい。", func([]int16) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "44100") {
		t.Fatalf("err %v", err)
	}
}

func TestGatewayStopsWhenOutFails(t *testing.T) {
	f := &fakeGateway{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	stop := errors.New("stop")
	err := (Gateway{Base: srv.URL}).Speak(context.Background(), "一つ目。二つ目。", func([]int16) error { return stop })
	if !errors.Is(err, stop) || len(f.asked) != 1 {
		t.Fatalf("err %v after %d requests", err, len(f.asked))
	}
}

func TestGatewayNeedsAnAddress(t *testing.T) {
	if err := (Gateway{}).Speak(context.Background(), "はい", func([]int16) error { return nil }); err == nil {
		t.Fatal("spoke with no address")
	}
}

func TestSentences(t *testing.T) {
	for in, want := range map[string][]string{
		"おはよう。今日は晴れ！":             {"おはよう。", "今日は晴れ！"},
		"「いいよ。」って言った。":            {"「いいよ。」", "って言った。"},
		"一行目\n\n二行目":              {"一行目", "二行目"},
		"  ":                      nil,
		"Really? Yes!":            {"Really?", "Yes!"},
		"終わりに句点がない":               {"終わりに句点がない"},
		strings.Repeat("あ", 1500): {strings.Repeat("あ", gatewayMaxPart), strings.Repeat("あ", 500)},
	} {
		if got := sentences(in); !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("sentences(%q) = %q, want %q", in, got, want)
		}
	}
}

// The gateway speaks through Speaker at 16 kHz as it came, with no downsampling.
func TestSpeakerUsesTheGateway(t *testing.T) {
	f := &fakeGateway{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	var n int
	err := Speaker{Gateway: Gateway{Base: srv.URL}}.Speak(context.Background(), ByGateway, "あいうえお", func(s []int16) error {
		n += len(s)
		return nil
	})
	if err != nil || n != 5 {
		t.Fatalf("err %v, %d samples", err, n)
	}
}
