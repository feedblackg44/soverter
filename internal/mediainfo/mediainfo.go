// Package mediainfo reads the properties of a media file with ffprobe: container, duration, size, bitrate and
// every video/audio stream. Parsing is pure (ffprobe JSON in, structs out) so it is covered by tests without ffprobe.
package mediainfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Info is everything the properties window shows about one file.
type Info struct {
	Path      string
	Container string        // human readable container name, e.g. "QuickTime / MOV"
	Size      int64         // bytes
	Duration  time.Duration // 0 = unknown
	BitRate   float64       // overall, bit/s; 0 = unknown
	Video     []Video
	Audio     []Audio
	Subtitles int
}

// Video is one video stream.
type Video struct {
	Codec      string // short codec name, e.g. "h264"
	CodecLong  string
	Profile    string
	Width      int
	Height     int
	FPS        float64
	BitRate    float64 // bit/s; 0 = unknown
	PixFmt     string
	BitDepth   int
	Aspect     string // display aspect ratio, e.g. "16:9"
	Rotation   int    // degrees, 0 if none
	Interlaced bool
	HDR        string // "HDR10", "HLG" or ""
	ColorSpace string
}

// Audio is one audio stream.
type Audio struct {
	Codec      string
	Profile    string
	Channels   int
	Layout     string
	SampleRate int
	BitRate    float64 // bit/s; 0 = unknown
	Language   string
}

type probeJSON struct {
	Format struct {
		FormatName     string `json:"format_name"`
		FormatLongName string `json:"format_long_name"`
		Duration       string `json:"duration"`
		Size           string `json:"size"`
		BitRate        string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecType          string            `json:"codec_type"`
		CodecName          string            `json:"codec_name"`
		CodecLongName      string            `json:"codec_long_name"`
		Profile            string            `json:"profile"`
		Width              int               `json:"width"`
		Height             int               `json:"height"`
		PixFmt             string            `json:"pix_fmt"`
		BitsPerRawSample   string            `json:"bits_per_raw_sample"`
		AvgFrameRate       string            `json:"avg_frame_rate"`
		RFrameRate         string            `json:"r_frame_rate"`
		BitRate            string            `json:"bit_rate"`
		DisplayAspectRatio string            `json:"display_aspect_ratio"`
		FieldOrder         string            `json:"field_order"`
		ColorSpace         string            `json:"color_space"`
		ColorTransfer      string            `json:"color_transfer"`
		SampleRate         string            `json:"sample_rate"`
		Channels           int               `json:"channels"`
		ChannelLayout      string            `json:"channel_layout"`
		Tags               map[string]string `json:"tags"`
		Disposition        struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
		SideData []struct {
			Rotation float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
}

// Parse parses the output of `ffprobe -v error -show_format -show_streams -of json <file>`.
func Parse(data []byte) (Info, error) {
	var pj probeJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return Info{}, fmt.Errorf("ffprobe: %w", err)
	}
	var in Info
	in.Container = containerName(pj.Format.FormatLongName, pj.Format.FormatName)
	in.Size, _ = strconv.ParseInt(pj.Format.Size, 10, 64)
	in.BitRate, _ = strconv.ParseFloat(pj.Format.BitRate, 64)
	if d, err := strconv.ParseFloat(pj.Format.Duration, 64); err == nil && d > 0 {
		in.Duration = time.Duration(d * float64(time.Second))
	}
	for _, s := range pj.Streams {
		bps, _ := strconv.ParseFloat(s.BitRate, 64)
		switch s.CodecType {
		case "video":
			if s.Disposition.AttachedPic == 1 {
				continue // cover art, not a video stream
			}
			v := Video{
				Codec: s.CodecName, CodecLong: s.CodecLongName, Profile: s.Profile,
				Width: s.Width, Height: s.Height, BitRate: bps, PixFmt: s.PixFmt,
				FPS: frameRate(s.AvgFrameRate, s.RFrameRate), ColorSpace: s.ColorSpace,
				Interlaced: s.FieldOrder != "" && s.FieldOrder != "progressive" && s.FieldOrder != "unknown",
				HDR:        hdr(s.ColorTransfer),
				Aspect:     aspect(s.DisplayAspectRatio, s.Width, s.Height),
				BitDepth:   bitDepth(s.BitsPerRawSample, s.PixFmt),
			}
			for _, sd := range s.SideData {
				if sd.Rotation != 0 {
					v.Rotation = ((int(sd.Rotation) % 360) + 360) % 360
				}
			}
			in.Video = append(in.Video, v)
		case "audio":
			sr, _ := strconv.Atoi(s.SampleRate)
			in.Audio = append(in.Audio, Audio{
				Codec: s.CodecName, Profile: s.Profile, Channels: s.Channels, Layout: s.ChannelLayout,
				SampleRate: sr, BitRate: bps, Language: language(s.Tags["language"]),
			})
		case "subtitle":
			in.Subtitles++
		}
	}
	return in, nil
}

// Probe runs ffprobe on file. The size falls back to the file system when ffprobe does not report it.
func Probe(ctx context.Context, ffprobe, file string) (Info, error) {
	if ffprobe == "" {
		return Info{}, errors.New("ffprobe not found")
	}
	cmd := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_format", "-show_streams", "-of", "json", file)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		detail := err.Error()
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			detail = strings.TrimSpace(string(ee.Stderr))
		}
		return Info{}, errors.New(detail)
	}
	in, err := Parse(out)
	if err != nil {
		return Info{}, err
	}
	in.Path = file
	if in.Size == 0 {
		if st, err := os.Stat(file); err == nil {
			in.Size = st.Size()
		}
	}
	if in.BitRate == 0 && in.Duration > 0 && in.Size > 0 {
		in.BitRate = float64(in.Size) * 8 / in.Duration.Seconds()
	}
	return in, nil
}

// containerName prefers the long name; the short one is a list of aliases ("mov,mp4,m4a,3gp,3g2,mj2") and only a fallback.
func containerName(long, short string) string {
	if long != "" {
		return long
	}
	return short
}

// language drops the "und" (undetermined) placeholder.
func language(l string) string {
	if l == "und" {
		return ""
	}
	return l
}

func frameRate(avg, r string) float64 {
	for _, s := range []string{avg, r} {
		if f := parseRatio(s); f > 0 {
			return f
		}
	}
	return 0
}

// parseRatio parses "30000/1001" (or a plain number); 0 for "0/0" and garbage.
func parseRatio(s string) float64 {
	n, d, ok := strings.Cut(s, "/")
	a, err := strconv.ParseFloat(n, 64)
	if err != nil {
		return 0
	}
	if !ok {
		return a
	}
	b, err := strconv.ParseFloat(d, 64)
	if err != nil || b == 0 {
		return 0
	}
	return a / b
}

// aspect returns the display aspect ratio as reported, or one computed from the frame size.
func aspect(dar string, w, h int) string {
	if dar != "" && dar != "0:1" && dar != "N/A" {
		return dar
	}
	if w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	return fmt.Sprintf("%d:%d", w/g, h/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func hdr(transfer string) string {
	switch transfer {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	}
	return ""
}

func bitDepth(raw, pixFmt string) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	switch {
	case strings.Contains(pixFmt, "12"):
		return 12
	case strings.Contains(pixFmt, "10"):
		return 10
	case pixFmt != "":
		return 8
	}
	return 0
}
