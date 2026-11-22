// Package sizecalc computes the video bitrate needed to hit a target file size.
// It is pure: ffprobe output in, numbers out.
package sizecalc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"vidpreset/internal/i18n"

	"vidpreset/internal/preset"
)

const (
	// ContainerMargin is the share of the target size reserved for container overhead and rate-control error.
	ContainerMargin = 0.05
	// MinVideoKbps is the lowest video bitrate considered meaningful.
	MinVideoKbps = 50
	// FallbackAudioKbps is assumed when the source has audio but ffprobe reports no bitrate for it.
	FallbackAudioKbps = 128
)

// ErrNoDuration is returned when ffprobe could not determine the duration.
var ErrNoDuration = i18n.Err("не удалось определить длительность файла (ffprobe), режим «по размеру» для него недоступен")

// Media is what we need to know about the source.
type Media struct {
	Duration      time.Duration
	HasAudio      bool
	AudioKbps     float64 // real bitrate of the first audio stream; 0 if unknown
	AudioEstimate bool    // true if AudioKbps was derived or assumed rather than reported
}

type probeJSON struct {
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		BitRate   string `json:"bit_rate"`
	} `json:"streams"`
}

// ParseProbe parses the output of
//
//	ffprobe -v error -show_entries format=duration,bit_rate:stream=codec_type,bit_rate -of json <file>
func ParseProbe(data []byte) (Media, error) {
	var pj probeJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return Media{}, fmt.Errorf("ffprobe: %w", err)
	}
	var m Media
	if d, err := strconv.ParseFloat(pj.Format.Duration, 64); err == nil && d > 0 {
		m.Duration = time.Duration(d * float64(time.Second))
	}
	var videoBps, audioBps float64
	var seenAudio, seenVideo bool
	for _, s := range pj.Streams {
		bps, _ := strconv.ParseFloat(s.BitRate, 64)
		switch s.CodecType {
		case "audio":
			if !seenAudio {
				seenAudio, audioBps = true, bps
			}
		case "video":
			if !seenVideo {
				seenVideo, videoBps = true, bps
			}
		}
	}
	m.HasAudio = seenAudio
	switch {
	case !seenAudio:
	case audioBps > 0:
		m.AudioKbps = audioBps / 1000
	default:
		// e.g. Matroska without per-stream bitrates: derive from the container total, else assume.
		total, _ := strconv.ParseFloat(pj.Format.BitRate, 64)
		if total > 0 && videoBps > 0 && total > videoBps {
			m.AudioKbps = (total - videoBps) / 1000
		} else {
			m.AudioKbps = FallbackAudioKbps
		}
		m.AudioEstimate = true
	}
	return m, nil
}

// ParseBitrate parses "192k", "1.5M", "192000" into kbit/s.
func ParseBitrate(s string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	mult := 1.0 / 1000 // plain number = bit/s
	switch {
	case strings.HasSuffix(s, "k"):
		mult, s = 1, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult, s = 1000, strings.TrimSuffix(s, "m")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, fmt.Errorf(i18n.T("неверный битрейт %q"), s)
	}
	return f * mult, nil
}

// OutputAudioKbps is the audio bitrate that ends up in the output file.
func OutputAudioKbps(m Media, a preset.Audio) float64 {
	if !m.HasAudio || a.Mode == preset.AudioRemove {
		return 0
	}
	switch a.Mode {
	case preset.AudioAAC, preset.AudioOpus:
		def := "192k"
		if a.Mode == preset.AudioOpus {
			def = "128k"
		}
		if a.Bitrate != "" {
			def = a.Bitrate
		}
		k, err := ParseBitrate(def)
		if err != nil {
			return FallbackAudioKbps
		}
		return k
	}
	return m.AudioKbps // copy
}

// VideoKbps returns the video bitrate (kbit/s) so that video + audio fit into targetMB:
//
//	(targetMB * 8192 * (1 - margin)) / duration - audioKbps
func VideoKbps(targetMB float64, m Media, a preset.Audio) (int, error) {
	if m.Duration <= 0 {
		return 0, ErrNoDuration
	}
	if targetMB <= 0 {
		return 0, errors.New(i18n.T("не задан целевой размер"))
	}
	sec := m.Duration.Seconds()
	audio := OutputAudioKbps(m, a)
	kbps := targetMB*8192*(1-ContainerMargin)/sec - audio
	if kbps < MinVideoKbps {
		needMB := (MinVideoKbps + audio) * sec / (8192 * (1 - ContainerMargin))
		return 0, fmt.Errorf(i18n.T("целевой размер %.4g МБ слишком мал для видео длительностью %s: нужно не меньше ~%.1f МБ"),
			targetMB, m.Duration.Round(time.Second), needMB)
	}
	return int(kbps), nil
}
