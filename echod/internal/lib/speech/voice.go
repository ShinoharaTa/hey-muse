package speech

import "context"

// Voice is who says an answer: one of the endpoint's voices by name, the device's own (BuiltIn), or
// the owner's tts-gateway (ByGateway). It is what the voice setting holds.
type Voice string

// BuiltIn is the device's own voice (Local).
const BuiltIn Voice = "builtin"

// ByGateway is the owner's tts-gateway server (Gateway), in whichever of its voices is set there.
const ByGateway Voice = "gateway"

// Chosen is the voice a setting comes to, given whether the endpoint has a key and the gateway an
// address: one that is not there to ask cannot speak, so the device's own voice does. With nothing
// chosen, a gateway that is set up speaks. This is the one place that is decided.
func Chosen(setting, key string, gateway bool) Voice {
	switch v := Voice(setting); {
	case v == BuiltIn:
		return BuiltIn
	case v == ByGateway || (v == "" && gateway):
		if gateway {
			return ByGateway
		}
		return BuiltIn
	case key == "":
		return BuiltIn
	}
	return Voice(orDefault(setting, DefaultVoice))
}

// Choices are every voice a setting may hold, the device's own first.
func Choices() []Voice {
	all := []Voice{BuiltIn, ByGateway}
	for _, name := range Voices {
		all = append(all, Voice(name))
	}
	return all
}

// Offered are the voices a picker shows: only those that can speak, given whether the endpoint has
// a key and the gateway an address, so a voice that could be tapped and then not heard is not there.
func Offered(key, gateway bool) []Voice {
	all := []Voice{BuiltIn}
	if gateway {
		all = append(all, ByGateway)
	}
	if key {
		for _, name := range Voices {
			all = append(all, Voice(name))
		}
	}
	return all
}

// Local is whether the device itself speaks, with no endpoint asked.
func (v Voice) Local() bool { return v == BuiltIn }

// Label is the voice as a person reads it.
func (v Voice) Label() string {
	switch v {
	case BuiltIn:
		return "Built in"
	case ByGateway:
		return "tts-gateway"
	}
	return string(v)
}

// Speaker is both ways of speaking, as the settings have them.
type Speaker struct {
	Cloud   Client // its Voice is not read: Speak is told the voice
	Gateway Gateway
	Local   Local
}

// Speak says text in voice v and calls out with the samples at 16 kHz, whichever engine made them:
// the endpoint's come at Rate and are brought down here, the device's own and the gateway's are
// 16 kHz already.
func (s Speaker) Speak(ctx context.Context, v Voice, text string, out func(samples []int16) error) error {
	switch v {
	case BuiltIn:
		return s.Local.Speak(ctx, text, out)
	case ByGateway:
		return s.Gateway.Speak(ctx, text, out)
	}
	s.Cloud.Voice = string(v)
	var down Downsampler
	return s.Cloud.Speak(ctx, text, func(samples []int16) error { return out(down.Write(samples)) })
}
