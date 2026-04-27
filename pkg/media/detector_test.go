package media

import (
	"testing"
)

func TestDetector_JPEG(t *testing.T) {
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	result := DetectMediaType(jpegHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatJPEG {
		t.Errorf("expected FormatJPEG, got %s", result.Format)
	}
	if result.MimeType != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %s", result.MimeType)
	}
}

func TestDetector_JPEG_ID3(t *testing.T) {
	id3Header := []byte("ID3\x04\x00\x00\x00\x00\x00\x00")
	result := DetectMediaType(id3Header, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatMP3 {
		t.Errorf("expected FormatMP3, got %s", result.Format)
	}
}

func TestDetector_PNG(t *testing.T) {
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	result := DetectMediaType(pngHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatPNG {
		t.Errorf("expected FormatPNG, got %s", result.Format)
	}
	if result.MimeType != "image/png" {
		t.Errorf("expected image/png, got %s", result.MimeType)
	}
}

func TestDetector_GIF(t *testing.T) {
	gifHeader := []byte("GIF89a\x00\x00\x00\x00\x00\x00")
	result := DetectMediaType(gifHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatGIF {
		t.Errorf("expected FormatGIF, got %s", result.Format)
	}
}

func TestDetector_GIF87a(t *testing.T) {
	gifHeader := []byte("GIF87a\x00\x00\x00\x00\x00\x00")
	result := DetectMediaType(gifHeader, "", "")

	if result.Format != FormatGIF {
		t.Errorf("expected FormatGIF, got %s", result.Format)
	}
}

func TestDetector_WebP(t *testing.T) {
	webpHeader := make([]byte, 20)
	copy(webpHeader[:4], "RIFF")
	webpHeader[8] = 'W'
	webpHeader[9] = 'E'
	webpHeader[10] = 'B'
	webpHeader[11] = 'P'
	result := DetectMediaType(webpHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatWebP {
		t.Errorf("expected FormatWebP, got %s", result.Format)
	}
}

func TestDetector_BMP(t *testing.T) {
	bmpHeader := []byte{0x42, 0x4D, 0x00, 0x00, 0x00, 0x00}
	result := DetectMediaType(bmpHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatBMP {
		t.Errorf("expected FormatBMP, got %s", result.Format)
	}
}

func TestDetector_TIFF_MM(t *testing.T) {
	tiffHeader := []byte("MM\x00\x2A\x00\x00\x00\x00")
	result := DetectMediaType(tiffHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatTIFF {
		t.Errorf("expected FormatTIFF, got %s", result.Format)
	}
}

func TestDetector_TIFF_II(t *testing.T) {
	tiffHeader := []byte("II\x2A\x00\x00\x00\x00\x00")
	result := DetectMediaType(tiffHeader, "", "")

	if result.Format != FormatTIFF {
		t.Errorf("expected FormatTIFF, got %s", result.Format)
	}
}

func TestDetector_ICO(t *testing.T) {
	icoHeader := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}
	result := DetectMediaType(icoHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatICO {
		t.Errorf("expected FormatICO, got %s", result.Format)
	}
}

func TestDetector_AVIF(t *testing.T) {
	avifHeader := make([]byte, 20)
	avifHeader[4] = 'f'
	avifHeader[5] = 't'
	avifHeader[6] = 'y'
	avifHeader[7] = 'p'
	copy(avifHeader[8:12], "avif")
	result := DetectMediaType(avifHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatAVIF {
		t.Errorf("expected FormatAVIF, got %s", result.Format)
	}
}

func TestDetector_HEIC(t *testing.T) {
	heicHeader := make([]byte, 20)
	heicHeader[4] = 'f'
	heicHeader[5] = 't'
	heicHeader[6] = 'y'
	heicHeader[7] = 'p'
	copy(heicHeader[8:12], "heic")
	result := DetectMediaType(heicHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatHEIC {
		t.Errorf("expected FormatHEIC, got %s", result.Format)
	}
}

func TestDetector_SVG(t *testing.T) {
	svgData := []byte("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"100\" height=\"100\"></svg>")
	result := DetectMediaType(svgData, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatSVG {
		t.Errorf("expected FormatSVG, got %s", result.Format)
	}
}

func TestDetector_MP3_Framesync(t *testing.T) {
	mp3Header := []byte{0xFF, 0xFB, 0x90, 0x00, 0x00, 0x00}
	result := DetectMediaType(mp3Header, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatMP3 {
		t.Errorf("expected FormatMP3, got %s", result.Format)
	}
}

func TestDetector_WAV(t *testing.T) {
	wavHeader := make([]byte, 20)
	copy(wavHeader[:4], "RIFF")
	wavHeader[8] = 'W'
	wavHeader[9] = 'A'
	wavHeader[10] = 'V'
	wavHeader[11] = 'E'
	result := DetectMediaType(wavHeader, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatWAV {
		t.Errorf("expected FormatWAV, got %s", result.Format)
	}
}

func TestDetector_OGG(t *testing.T) {
	oggHeader := []byte("OggS\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00OpusHead")
	result := DetectMediaType(oggHeader, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatOGG {
		t.Errorf("expected FormatOGG, got %s", result.Format)
	}
}

func TestDetector_FLAC(t *testing.T) {
	flacHeader := []byte("\x7fFLAC\x01\x00\x00\x00")
	result := DetectMediaType(flacHeader, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatFLAC {
		t.Errorf("expected FormatFLAC, got %s", result.Format)
	}
}

func TestDetector_M4A(t *testing.T) {
	m4aHeader := make([]byte, 20)
	m4aHeader[4] = 'f'
	m4aHeader[5] = 't'
	m4aHeader[6] = 'y'
	m4aHeader[7] = 'p'
	copy(m4aHeader[8:12], "M4A ")
	result := DetectMediaType(m4aHeader, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatM4A {
		t.Errorf("expected FormatM4A, got %s", result.Format)
	}
}

func TestDetector_AMR(t *testing.T) {
	amrHeader := []byte("#!AMR\n")
	result := DetectMediaType(amrHeader, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatAMR {
		t.Errorf("expected FormatAMR, got %s", result.Format)
	}
}

func TestDetector_MIDI(t *testing.T) {
	midiHeader := []byte("MThd\x00\x00\x00\x06")
	result := DetectMediaType(midiHeader, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatMIDI {
		t.Errorf("expected FormatMIDI, got %s", result.Format)
	}
}

func TestDetector_MP4(t *testing.T) {
	mp4Header := make([]byte, 20)
	mp4Header[4] = 'f'
	mp4Header[5] = 't'
	mp4Header[6] = 'y'
	mp4Header[7] = 'p'
	copy(mp4Header[8:12], "isom")
	result := DetectMediaType(mp4Header, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != FormatMP4 {
		t.Errorf("expected FormatMP4, got %s", result.Format)
	}
}

func TestDetector_WebM(t *testing.T) {
	webmHeader := []byte{0x1A, 0x45, 0xDF, 0xA3, 'w', 'e', 'b', 'm'}
	result := DetectMediaType(webmHeader, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != FormatWebM {
		t.Errorf("expected FormatWebM, got %s", result.Format)
	}
}

func TestDetector_MKV(t *testing.T) {
	mkvHeader := []byte{0x1A, 0x45, 0xDF, 0xA3, 'm', 'a', 't', 'r'}
	result := DetectMediaType(mkvHeader, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != FormatMKV {
		t.Errorf("expected FormatMKV, got %s", result.Format)
	}
}

func TestDetector_AVI(t *testing.T) {
	aviHeader := make([]byte, 20)
	copy(aviHeader[:4], "RIFF")
	aviHeader[8] = 'A'
	aviHeader[9] = 'V'
	aviHeader[10] = 'I'
	aviHeader[11] = ' '
	result := DetectMediaType(aviHeader, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != FormatAVI {
		t.Errorf("expected FormatAVI, got %s", result.Format)
	}
}

func TestDetector_MOV(t *testing.T) {
	movHeader := make([]byte, 20)
	movHeader[4] = 'w'
	movHeader[5] = 'i'
	movHeader[6] = 'd'
	movHeader[7] = 'e'
	result := DetectMediaType(movHeader, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != FormatMOV {
		t.Errorf("expected FormatMOV, got %s", result.Format)
	}
}

func TestDetector_FLV(t *testing.T) {
	flvHeader := []byte("FLV\x01\x00\x00\x00\x00")
	result := DetectMediaType(flvHeader, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != FormatFLV {
		t.Errorf("expected FormatFLV, got %s", result.Format)
	}
}

func TestDetector_PDF(t *testing.T) {
	pdfHeader := []byte("%PDF-1.4\n%test")
	result := DetectMediaType(pdfHeader, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatPDF {
		t.Errorf("expected FormatPDF, got %s", result.Format)
	}
	if result.MimeType != "application/pdf" {
		t.Errorf("expected application/pdf, got %s", result.MimeType)
	}
}

func TestDetector_ZIP(t *testing.T) {
	zipHeader := []byte("PK\x03\x04\x14\x00\x00\x00")
	result := DetectMediaType(zipHeader, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatZIP {
		t.Errorf("expected FormatZIP, got %s", result.Format)
	}
}

func TestDetector_DOCX(t *testing.T) {
	docxHeader := make([]byte, 500)
	copy(docxHeader[:2], "PK")
	docxHeader[2] = 0x03
	docxHeader[3] = 0x04
	copy(docxHeader[30:35], "word/")
	result := DetectMediaType(docxHeader, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatDOCX {
		t.Errorf("expected FormatDOCX, got %s", result.Format)
	}
}

func TestDetector_XLSX(t *testing.T) {
	xlsxHeader := make([]byte, 500)
	copy(xlsxHeader[:2], "PK")
	xlsxHeader[2] = 0x03
	xlsxHeader[3] = 0x04
	copy(xlsxHeader[30:33], "xl/")
	result := DetectMediaType(xlsxHeader, "", "")

	if result.Format != FormatXLSX {
		t.Errorf("expected FormatXLSX, got %s", result.Format)
	}
}

func TestDetector_PPTX(t *testing.T) {
	pptxHeader := make([]byte, 500)
	copy(pptxHeader[:2], "PK")
	pptxHeader[2] = 0x03
	pptxHeader[3] = 0x04
	copy(pptxHeader[30:34], "ppt/")
	result := DetectMediaType(pptxHeader, "", "")

	if result.Format != FormatPPTX {
		t.Errorf("expected FormatPPTX, got %s", result.Format)
	}
}

func TestDetector_RTF(t *testing.T) {
	rtfHeader := []byte("{\\rtf1\\ansi\\deff0")
	result := DetectMediaType(rtfHeader, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatRTF {
		t.Errorf("expected FormatRTF, got %s", result.Format)
	}
}

func TestDetector_PlainText(t *testing.T) {
	textData := []byte("Hello, this is a plain text file.\nIt has multiple lines.\nNo commas on every line here.")
	result := DetectMediaType(textData, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatTXT {
		t.Errorf("expected FormatTXT, got %s", result.Format)
	}
}

func TestDetector_CSV(t *testing.T) {
	csvData := []byte("name,age,city\nAlice,30,NYC\nBob,25,LA\n")
	result := DetectMediaType(csvData, "", "")

	if result.Format != FormatCSV {
		t.Errorf("expected FormatCSV, got %s", result.Format)
	}
}

func TestDetector_OldDOC(t *testing.T) {
	docHeader := []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1\x00\x00\x00\x00")
	result := DetectMediaType(docHeader, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatDOC {
		t.Errorf("expected FormatDOC, got %s", result.Format)
	}
}

func TestDetector_EmptyData(t *testing.T) {
	result := DetectMediaType([]byte{}, "", "")

	if result.Format != FormatUnknown {
		t.Errorf("expected FormatUnknown for empty data, got %s", result.Format)
	}
}

func TestDetector_ShortData(t *testing.T) {
	result := DetectMediaType([]byte{0xFF}, "", "")

	if result.Format != FormatUnknown {
		t.Errorf("expected FormatUnknown for 1-byte data, got %s", result.Format)
	}
}

func TestDetector_MIMEFallback(t *testing.T) {
	result := DetectMediaType([]byte("unknown binary data here"), "", "image/png")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage from MIME fallback, got %s", result.Type)
	}
	if result.Format != FormatPNG {
		t.Errorf("expected FormatPNG from MIME fallback, got %s", result.Format)
	}
}

func TestDetector_ExtensionFallback(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "photo.jpg", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage from extension fallback, got %s", result.Type)
	}
	if result.Format != FormatJPEG {
		t.Errorf("expected FormatJPEG from extension fallback, got %s", result.Format)
	}
}

func TestDetector_ExtensionFallbackUnknown(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "file.xyz", "")

	if result.Format != FormatUnknown {
		t.Errorf("expected FormatUnknown for unknown extension, got %s", result.Format)
	}
}

func TestDetector_BytesOverrideMIME(t *testing.T) {
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	result := DetectMediaType(pngHeader, "wrong.jpg", "image/jpeg")

	if result.Format != FormatPNG {
		t.Errorf("expected bytes to override wrong MIME, got %s", result.Format)
	}
}

func TestDetector_MIMEOverrideExtension(t *testing.T) {
	result := DetectMediaType([]byte("some data"), "file.bin", "video/mp4")

	if result.Format != FormatMP4 {
		t.Errorf("expected MIME to override extension, got %s", result.Format)
	}
	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
}

func TestDetector_DetectFromBytes(t *testing.T) {
	d := NewDetector()
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	result := d.DetectFromBytes(jpegHeader)

	if result.Format != FormatJPEG {
		t.Errorf("expected FormatJPEG, got %s", result.Format)
	}
}

func TestDetector_DetectFromBytesEmpty(t *testing.T) {
	d := NewDetector()
	result := d.DetectFromBytes(nil)

	if result.Format != FormatUnknown {
		t.Errorf("expected FormatUnknown, got %s", result.Format)
	}
}

func TestDetector_MIMEOverrides(t *testing.T) {
	d := NewDetector()
	d.RegisterMIMEOverride("image/x-custom", FormatWebP)

	result := d.Detect([]byte("some data"), "", "image/x-custom")

	if result.Format != FormatWebP {
		t.Errorf("expected FormatWebP from override, got %s", result.Format)
	}
}

func TestDetector_FormatToType(t *testing.T) {
	tests := []struct {
		format Format
		want   Type
	}{
		{FormatJPEG, TypeImage},
		{FormatPNG, TypeImage},
		{FormatGIF, TypeImage},
		{FormatWebP, TypeImage},
		{FormatBMP, TypeImage},
		{FormatSVG, TypeImage},
		{FormatTIFF, TypeImage},
		{FormatICO, TypeImage},
		{FormatAVIF, TypeImage},
		{FormatHEIC, TypeImage},
		{FormatMP3, TypeAudio},
		{FormatWAV, TypeAudio},
		{FormatOGG, TypeAudio},
		{FormatFLAC, TypeAudio},
		{FormatAAC, TypeAudio},
		{FormatM4A, TypeAudio},
		{FormatWMA, TypeAudio},
		{FormatAMR, TypeAudio},
		{FormatMIDI, TypeAudio},
		{FormatMP4, TypeVideo},
		{FormatWebM, TypeVideo},
		{FormatAVI, TypeVideo},
		{FormatMKV, TypeVideo},
		{FormatMOV, TypeVideo},
		{FormatWMV, TypeVideo},
		{FormatFLV, TypeVideo},
		{FormatMPEG, TypeVideo},
		{Format3GP, TypeVideo},
		{FormatPDF, TypeDoc},
		{FormatDOC, TypeDoc},
		{FormatDOCX, TypeDoc},
		{FormatXLS, TypeDoc},
		{FormatXLSX, TypeDoc},
		{FormatPPT, TypeDoc},
		{FormatPPTX, TypeDoc},
		{FormatTXT, TypeDoc},
		{FormatRTF, TypeDoc},
		{FormatCSV, TypeDoc},
		{FormatZIP, TypeDoc},
		{FormatEPUB, TypeDoc},
		{FormatUnknown, TypeDoc},
	}

	for _, tt := range tests {
		got := formatToType(tt.format)
		if got != tt.want {
			t.Errorf("formatToType(%s) = %s, want %s", tt.format, got, tt.want)
		}
	}
}

func TestDetector_FormatToMIME(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatJPEG, "image/jpeg"},
		{FormatPNG, "image/png"},
		{FormatGIF, "image/gif"},
		{FormatWebP, "image/webp"},
		{FormatMP3, "audio/mpeg"},
		{FormatWAV, "audio/wav"},
		{FormatMP4, "video/mp4"},
		{FormatWebM, "video/webm"},
		{FormatPDF, "application/pdf"},
		{FormatTXT, "text/plain"},
		{FormatZIP, "application/zip"},
		{FormatUnknown, "application/octet-stream"},
	}

	for _, tt := range tests {
		got := formatToMIME(tt.format)
		if got != tt.want {
			t.Errorf("formatToMIME(%s) = %s, want %s", tt.format, got, tt.want)
		}
	}
}

func TestDetector_IsUTF8Text(t *testing.T) {
	if !IsUTF8Text([]byte("Hello, world!")) {
		t.Error("expected plain ASCII to be UTF-8 text")
	}
	if !IsUTF8Text([]byte("Hello\nWorld\t!")) {
		t.Error("expected text with newlines/tabs to be UTF-8 text")
	}
	if IsUTF8Text([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}) {
		t.Error("expected binary data not to be UTF-8 text")
	}
}

func TestDetector_3GP(t *testing.T) {
	header := make([]byte, 20)
	header[4] = 'f'
	header[5] = 't'
	header[6] = 'y'
	header[7] = 'p'
	copy(header[8:12], "3gp4")
	result := DetectMediaType(header, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != Format3GP {
		t.Errorf("expected Format3GP, got %s", result.Format)
	}
}

func TestDetector_EPUB(t *testing.T) {
	epubHeader := make([]byte, 500)
	copy(epubHeader[:2], "PK")
	epubHeader[2] = 0x03
	epubHeader[3] = 0x04
	copy(epubHeader[30:60], "mimetypeapplication/epub+zip")
	result := DetectMediaType(epubHeader, "", "")

	if result.Format != FormatEPUB {
		t.Errorf("expected FormatEPUB, got %s", result.Format)
	}
}

func TestDetector_MPEG(t *testing.T) {
	mpegHeader := []byte{0x00, 0x00, 0x01, 0xB3}
	result := DetectMediaType(mpegHeader, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != FormatMPEG {
		t.Errorf("expected FormatMPEG, got %s", result.Format)
	}
}

func TestDetector_AAC(t *testing.T) {
	aacHeader := []byte{0xFF, 0xF1, 0x00, 0x00}
	result := DetectMediaType(aacHeader, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatAAC {
		t.Errorf("expected FormatAAC, got %s", result.Format)
	}
}

func TestDetector_DetectAllInputs(t *testing.T) {
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	result := DetectMediaType(pngHeader, "test.png", "image/png")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatPNG {
		t.Errorf("expected FormatPNG, got %s", result.Format)
	}
	if result.MimeType != "image/png" {
		t.Errorf("expected image/png, got %s", result.MimeType)
	}
}

func TestDetector_MIMETypeFallback_Text(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "", "text/csv")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatCSV {
		t.Errorf("expected FormatCSV, got %s", result.Format)
	}
}

func TestDetector_UnknownMIMEType(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "", "application/x-unknown")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatUnknown {
		t.Errorf("expected FormatUnknown, got %s", result.Format)
	}
}

func TestDetector_JSON(t *testing.T) {
	jsonData := []byte(`{"name":"test","value":123}`)
	result := DetectMediaType(jsonData, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatJSON {
		t.Errorf("expected FormatJSON, got %s", result.Format)
	}
	if result.MimeType != "application/json" {
		t.Errorf("expected application/json, got %s", result.MimeType)
	}
}

func TestDetector_JSONArray(t *testing.T) {
	jsonData := []byte(`[1,2,3]`)
	result := DetectMediaType(jsonData, "", "")

	if result.Format != FormatJSON {
		t.Errorf("expected FormatJSON, got %s", result.Format)
	}
}

func TestDetector_XML(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0"?><root><item>test</item></root>`)
	result := DetectMediaType(xmlData, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatXML {
		t.Errorf("expected FormatXML, got %s", result.Format)
	}
}

func TestDetector_HTML(t *testing.T) {
	htmlData := []byte(`<!DOCTYPE html><html><head><title>Test</title></head><body></body></html>`)
	result := DetectMediaType(htmlData, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatHTML {
		t.Errorf("expected FormatHTML, got %s", result.Format)
	}
}

func TestDetector_HTMLLower(t *testing.T) {
	htmlData := []byte(`<html><body>Hello</body></html>`)
	result := DetectMediaType(htmlData, "", "")

	if result.Format != FormatHTML {
		t.Errorf("expected FormatHTML, got %s", result.Format)
	}
}

func TestDetector_YAML(t *testing.T) {
	yamlData := []byte("name: test\nvalue: 123\nitems:\n  one\n  two\n")
	result := DetectMediaType(yamlData, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatYAML {
		t.Errorf("expected FormatYAML, got %s", result.Format)
	}
}

func TestDetector_Markdown(t *testing.T) {
	mdData := []byte("# Title\n\nThis is a paragraph.\n\n- Item one\n- Item two\n")
	result := DetectMediaType(mdData, "", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatMD {
		t.Errorf("expected FormatMD, got %s", result.Format)
	}
}

func TestDetector_MarkdownLink(t *testing.T) {
	mdData := []byte("# Hello\n\n[Click here](https://example.com)\n")
	result := DetectMediaType(mdData, "", "")

	if result.Format != FormatMD {
		t.Errorf("expected FormatMD, got %s", result.Format)
	}
}

func TestDetector_MarkdownCodeBlock(t *testing.T) {
	mdData := []byte("# Code\n\n```go\nfmt.Println(\"hello\")\n```\n")
	result := DetectMediaType(mdData, "", "")

	if result.Format != FormatMD {
		t.Errorf("expected FormatMD, got %s", result.Format)
	}
}

func TestDetector_JPEG2000(t *testing.T) {
	header := make([]byte, 20)
	header[4] = 'f'
	header[5] = 't'
	header[6] = 'y'
	header[7] = 'p'
	copy(header[8:12], "jp2 ")
	result := DetectMediaType(header, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatJPEG2000 {
		t.Errorf("expected FormatJPEG2000, got %s", result.Format)
	}
}

func TestDetector_PSD(t *testing.T) {
	psdHeader := []byte("8BPS\x00\x01\x00\x00")
	result := DetectMediaType(psdHeader, "", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatPSD {
		t.Errorf("expected FormatPSD, got %s", result.Format)
	}
}

func TestDetector_WMA(t *testing.T) {
	wmaHeader := make([]byte, 20)
	copy(wmaHeader[:16], []byte{0x30, 0x26, 0xB2, 0x75, 0x8E, 0x66, 0xCF, 0x11, 0xA6, 0xD9, 0x00, 0xAA, 0x00, 0x62, 0xCE, 0x6C})
	wmaHeader[17] = 0x02
	result := DetectMediaType(wmaHeader, "", "")

	if result.Type != TypeAudio {
		t.Errorf("expected TypeAudio, got %s", result.Type)
	}
	if result.Format != FormatWMA {
		t.Errorf("expected FormatWMA, got %s", result.Format)
	}
}

func TestDetector_WMV(t *testing.T) {
	wmvHeader := make([]byte, 20)
	copy(wmvHeader[:16], []byte{0x30, 0x26, 0xB2, 0x75, 0x8E, 0x66, 0xCF, 0x11, 0xA6, 0xD9, 0x00, 0xAA, 0x00, 0x62, 0xCE, 0x6C})
	wmvHeader[17] = 0x03
	result := DetectMediaType(wmvHeader, "", "")

	if result.Type != TypeVideo {
		t.Errorf("expected TypeVideo, got %s", result.Type)
	}
	if result.Format != FormatWMV {
		t.Errorf("expected FormatWMV, got %s", result.Format)
	}
}

func TestDetector_ExtensionJSON(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "data.json", "")

	if result.Type != TypeDoc {
		t.Errorf("expected TypeDoc, got %s", result.Type)
	}
	if result.Format != FormatJSON {
		t.Errorf("expected FormatJSON, got %s", result.Format)
	}
}

func TestDetector_ExtensionXML(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "data.xml", "")

	if result.Format != FormatXML {
		t.Errorf("expected FormatXML, got %s", result.Format)
	}
}

func TestDetector_ExtensionHTML(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "page.html", "")

	if result.Format != FormatHTML {
		t.Errorf("expected FormatHTML, got %s", result.Format)
	}
}

func TestDetector_ExtensionYAML(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "config.yaml", "")

	if result.Format != FormatYAML {
		t.Errorf("expected FormatYAML, got %s", result.Format)
	}
}

func TestDetector_ExtensionMarkdown(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "README.md", "")

	if result.Format != FormatMD {
		t.Errorf("expected FormatMD, got %s", result.Format)
	}
}

func TestDetector_ExtensionJPEG2000(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "image.jp2", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatJPEG2000 {
		t.Errorf("expected FormatJPEG2000, got %s", result.Format)
	}
}

func TestDetector_ExtensionPSD(t *testing.T) {
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	result := DetectMediaType(binaryData, "design.psd", "")

	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
	if result.Format != FormatPSD {
		t.Errorf("expected FormatPSD, got %s", result.Format)
	}
}

func TestDetector_ExtractAudioMetadataWAV(t *testing.T) {
	wavHeader := make([]byte, 40)
	copy(wavHeader[:4], "RIFF")
	wavHeader[8] = 'W'
	wavHeader[9] = 'A'
	wavHeader[10] = 'V'
	wavHeader[11] = 'E'
	wavHeader[22] = 0x02
	wavHeader[23] = 0x00
	wavHeader[24] = 0x44
	wavHeader[25] = 0xAC
	wavHeader[26] = 0x00
	wavHeader[27] = 0x00
	wavHeader[32] = 0x10
	wavHeader[33] = 0x00

	meta := ExtractAudioMetadata(wavHeader)

	if meta["channels"] != uint16(2) {
		t.Errorf("expected 2 channels, got %v", meta["channels"])
	}
	if meta["sample_rate"] != uint32(44100) {
		t.Errorf("expected 44100 sample rate, got %v", meta["sample_rate"])
	}
	if meta["bits_per_sample"] != uint16(16) {
		t.Errorf("expected 16 bits per sample, got %v", meta["bits_per_sample"])
	}
}

func TestDetector_ExtractAudioMetadataMP3ID3(t *testing.T) {
	mp3Data := []byte("ID3\x04\x00\x00\x00\x00\x00\x00test")
	meta := ExtractAudioMetadata(mp3Data)

	if meta["has_id3"] != true {
		t.Errorf("expected has_id3 to be true")
	}
	if meta["id3_version"] != 4 {
		t.Errorf("expected id3_version 4, got %v", meta["id3_version"])
	}
}

func TestDetector_ExtractAudioMetadataOpus(t *testing.T) {
	opusData := make([]byte, 40)
	copy(opusData[:4], "OggS")
	copy(opusData[28:36], "OpusHead")
	meta := ExtractAudioMetadata(opusData)

	if meta["codec"] != "opus" {
		t.Errorf("expected codec opus, got %v", meta["codec"])
	}
}

func TestDetector_ExtractVideoMetadataMatroska(t *testing.T) {
	mkvData := []byte{0x1A, 0x45, 0xDF, 0xA3, 'm', 'a', 't', 'r'}
	meta := ExtractVideoMetadata(mkvData)

	if meta["container"] != "matroska" {
		t.Errorf("expected container matroska, got %v", meta["container"])
	}
}

func TestDetector_ExtractVideoMetadataWebM(t *testing.T) {
	webmData := []byte{0x1A, 0x45, 0xDF, 0xA3, 'w', 'e', 'b', 'm'}
	meta := ExtractVideoMetadata(webmData)

	if meta["container"] != "webm" {
		t.Errorf("expected container webm, got %v", meta["container"])
	}
}

func TestDetector_ExtractVideoMetadataFLV(t *testing.T) {
	flvData := []byte("FLV\x01\x05\x00\x00\x00\x09\x00\x00\x00\x00")
	meta := ExtractVideoMetadata(flvData)

	if meta["container"] != "flv" {
		t.Errorf("expected container flv, got %v", meta["container"])
	}
	if meta["has_audio"] != true {
		t.Errorf("expected has_audio to be true")
	}
	if meta["has_video"] != true {
		t.Errorf("expected has_video to be true")
	}
}
