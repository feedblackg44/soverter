package mediainfo

import (
	"testing"
	"time"
)

const sample = `{
 "streams": [
  {"codec_type":"video","codec_name":"hevc","codec_long_name":"H.265","profile":"Main 10","width":3840,"height":2160,
   "pix_fmt":"yuv420p10le","avg_frame_rate":"60000/1001","r_frame_rate":"60/1","bit_rate":"25000000",
   "display_aspect_ratio":"16:9","field_order":"progressive","color_transfer":"smpte2084","color_space":"bt2020nc",
   "side_data_list":[{"rotation":-90}]},
  {"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}},
  {"codec_type":"audio","codec_name":"aac","profile":"LC","channels":6,"channel_layout":"5.1","sample_rate":"48000","bit_rate":"384000","tags":{"language":"eng"}},
  {"codec_type":"audio","codec_name":"opus","channels":2,"sample_rate":"48000"},
  {"codec_type":"subtitle","codec_name":"subrip"}
 ],
 "format": {"format_name":"matroska,webm","format_long_name":"Matroska / WebM","duration":"125.5","size":"400000000","bit_rate":"25500000"}
}`

func TestParse(t *testing.T) {
	in, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if in.Container != "Matroska / WebM" || in.Size != 400000000 || in.Duration != 125500*time.Millisecond || in.BitRate != 25500000 {
		t.Fatalf("format: %+v", in)
	}
	if len(in.Video) != 1 {
		t.Fatalf("cover art must be skipped: %d video streams", len(in.Video))
	}
	v := in.Video[0]
	if v.Codec != "hevc" || v.Width != 3840 || v.Height != 2160 || v.Profile != "Main 10" || v.BitRate != 25000000 {
		t.Fatalf("video: %+v", v)
	}
	if v.FPS < 59.93 || v.FPS > 59.95 {
		t.Fatalf("fps = %v", v.FPS)
	}
	if v.BitDepth != 10 || v.HDR != "HDR10" || v.Aspect != "16:9" || v.Interlaced {
		t.Fatalf("video details: %+v", v)
	}
	if v.Rotation != 270 {
		t.Fatalf("rotation -90 must read as 270, got %d", v.Rotation)
	}
	if len(in.Audio) != 2 || in.Audio[0].Channels != 6 || in.Audio[0].Language != "eng" || in.Audio[0].SampleRate != 48000 || in.Audio[1].BitRate != 0 {
		t.Fatalf("audio: %+v", in.Audio)
	}
	if in.Subtitles != 1 {
		t.Fatalf("subtitles = %d", in.Subtitles)
	}
}

func TestFrameRateAndAspectFallbacks(t *testing.T) {
	if f := frameRate("0/0", "25/1"); f != 25 {
		t.Fatalf("fallback to r_frame_rate: %v", f)
	}
	if f := frameRate("0/0", "0/0"); f != 0 {
		t.Fatalf("unknown fps must be 0: %v", f)
	}
	if a := aspect("", 1920, 1080); a != "16:9" {
		t.Fatalf("computed aspect: %q", a)
	}
	if a := aspect("0:1", 640, 480); a != "4:3" {
		t.Fatalf("0:1 must be recomputed: %q", a)
	}
}

func TestParseGarbage(t *testing.T) {
	if _, err := Parse([]byte("not json")); err == nil {
		t.Fatal("garbage must fail")
	}
	in, err := Parse([]byte(`{}`))
	if err != nil || len(in.Video) != 0 || in.Duration != 0 {
		t.Fatalf("empty: %+v %v", in, err)
	}
}
