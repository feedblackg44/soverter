// Command icongen builds every derived icon from assets/icon.png (1024x1024 source art):
// assets/soverter.ico (multi-size, PNG-compressed entries), the MSIX logos in shellext/ and assets/soverter.rc.
// Run through assets/icons.ps1, which also compiles the .rc into cmd/soverter/rsrc_windows_amd64.syso.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/draw"
)

func resize(src image.Image, n int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, n, n))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		log.Fatal(err)
	}
	return b.Bytes()
}

func write(path string, b []byte) {
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatal(err)
	}
}

func main() {
	f, err := os.Open("assets/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}

	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	var hdr bytes.Buffer
	var data bytes.Buffer
	binary.Write(&hdr, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	off := 6 + 16*len(sizes)
	for _, n := range sizes {
		p := encode(resize(src, n))
		w := byte(n)
		if n >= 256 {
			w = 0
		}
		hdr.Write([]byte{w, w, 0, 0})
		binary.Write(&hdr, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&hdr, binary.LittleEndian, [2]uint32{uint32(len(p)), uint32(off + data.Len())})
		data.Write(p)
	}
	write("assets/soverter.ico", append(hdr.Bytes(), data.Bytes()...))

	// MSIX logos (referenced by AppxManifest.xml)
	write("shellext/logo.png", encode(resize(src, 150)))
	write("shellext/logo44.png", encode(resize(src, 44)))
	write("shellext/logo50.png", encode(resize(src, 50)))

	// 1 = first icon group: Gio loads it for the window, and "exe,0" is the menu icon.
	write("assets/soverter.rc", []byte("1 ICON \"soverter.ico\"\r\n"))
}
