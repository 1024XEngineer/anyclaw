package media

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"mime"
	"strings"
)

type Format string

const (
	FormatUnknown Format = "unknown"

	FormatJPEG     Format = "jpeg"
	FormatPNG      Format = "png"
	FormatGIF      Format = "gif"
	FormatWebP     Format = "webp"
	FormatBMP      Format = "bmp"
	FormatSVG      Format = "svg"
	FormatTIFF     Format = "tiff"
	FormatICO      Format = "ico"
	FormatAVIF     Format = "avif"
	FormatHEIC     Format = "heic"
	FormatJPEG2000 Format = "jpeg2000"
	FormatPSD      Format = "psd"

	FormatMP3  Format = "mp3"
	FormatWAV  Format = "wav"
	FormatOGG  Format = "ogg"
	FormatFLAC Format = "flac"
	FormatAAC  Format = "aac"
	FormatM4A  Format = "m4a"
	FormatWMA  Format = "wma"
	FormatAMR  Format = "amr"
	FormatMIDI Format = "midi"

	FormatMP4  Format = "mp4"
	FormatWebM Format = "webm"
	FormatAVI  Format = "avi"
	FormatMKV  Format = "mkv"
	FormatMOV  Format = "mov"
	FormatWMV  Format = "wmv"
	FormatFLV  Format = "flv"
	FormatMPEG Format = "mpeg"
	Format3GP  Format = "3gp"

	FormatPDF  Format = "pdf"
	FormatDOC  Format = "doc"
	FormatDOCX Format = "docx"
	FormatXLS  Format = "xls"
	FormatXLSX Format = "xlsx"
	FormatPPT  Format = "ppt"
	FormatPPTX Format = "pptx"
	FormatTXT  Format = "txt"
	FormatRTF  Format = "rtf"
	FormatCSV  Format = "csv"
	FormatZIP  Format = "zip"
	FormatEPUB Format = "epub"
	FormatJSON Format = "json"
	FormatXML  Format = "xml"
	FormatHTML Format = "html"
	FormatYAML Format = "yaml"
	FormatMD   Format = "markdown"
)

var mimeToFormat = map[string]Format{
	"image/jpeg":                FormatJPEG,
	"image/jpg":                 FormatJPEG,
	"image/png":                 FormatPNG,
	"image/gif":                 FormatGIF,
	"image/webp":                FormatWebP,
	"image/bmp":                 FormatBMP,
	"image/x-bmp":               FormatBMP,
	"image/svg+xml":             FormatSVG,
	"image/tiff":                FormatTIFF,
	"image/x-tiff":              FormatTIFF,
	"image/x-icon":              FormatICO,
	"image/vnd.microsoft.icon":  FormatICO,
	"image/avif":                FormatAVIF,
	"image/heic":                FormatHEIC,
	"image/heif":                FormatHEIC,
	"image/jp2":                 FormatJPEG2000,
	"image/x-photoshop":         FormatPSD,
	"image/vnd.adobe.photoshop": FormatPSD,

	"audio/mpeg":     FormatMP3,
	"audio/mp3":      FormatMP3,
	"audio/x-mp3":    FormatMP3,
	"audio/wav":      FormatWAV,
	"audio/x-wav":    FormatWAV,
	"audio/ogg":      FormatOGG,
	"audio/flac":     FormatFLAC,
	"audio/x-flac":   FormatFLAC,
	"audio/aac":      FormatAAC,
	"audio/mp4":      FormatM4A,
	"audio/x-m4a":    FormatM4A,
	"audio/x-ms-wma": FormatWMA,
	"audio/amr":      FormatAMR,
	"audio/midi":     FormatMIDI,
	"audio/x-midi":   FormatMIDI,

	"video/mp4":        FormatMP4,
	"video/webm":       FormatWebM,
	"video/x-msvideo":  FormatAVI,
	"video/x-matroska": FormatMKV,
	"video/quicktime":  FormatMOV,
	"video/x-ms-wmv":   FormatWMV,
	"video/x-flv":      FormatFLV,
	"video/mpeg":       FormatMPEG,
	"video/3gpp":       Format3GP,

	"application/pdf":    FormatPDF,
	"application/msword": FormatDOC,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": FormatDOCX,
	"application/vnd.ms-excel": FormatXLS,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         FormatXLSX,
	"application/vnd.ms-powerpoint":                                             FormatPPT,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": FormatPPTX,
	"text/plain":            FormatTXT,
	"text/rtf":              FormatRTF,
	"text/csv":              FormatCSV,
	"application/zip":       FormatZIP,
	"application/epub+zip":  FormatEPUB,
	"application/json":      FormatJSON,
	"text/json":             FormatJSON,
	"application/xml":       FormatXML,
	"text/xml":              FormatXML,
	"text/html":             FormatHTML,
	"application/xhtml+xml": FormatHTML,
	"application/x-yaml":    FormatYAML,
	"text/yaml":             FormatYAML,
	"text/markdown":         FormatMD,
	"text/x-markdown":       FormatMD,
}

var extToFormat = map[string]Format{
	".jpg":  FormatJPEG,
	".jpeg": FormatJPEG,
	".png":  FormatPNG,
	".gif":  FormatGIF,
	".webp": FormatWebP,
	".bmp":  FormatBMP,
	".svg":  FormatSVG,
	".tiff": FormatTIFF,
	".tif":  FormatTIFF,
	".ico":  FormatICO,
	".avif": FormatAVIF,
	".heic": FormatHEIC,
	".heif": FormatHEIC,
	".jp2":  FormatJPEG2000,
	".j2k":  FormatJPEG2000,
	".psd":  FormatPSD,

	".mp3":  FormatMP3,
	".wav":  FormatWAV,
	".ogg":  FormatOGG,
	".oga":  FormatOGG,
	".flac": FormatFLAC,
	".aac":  FormatAAC,
	".m4a":  FormatM4A,
	".wma":  FormatWMA,
	".amr":  FormatAMR,
	".mid":  FormatMIDI,
	".midi": FormatMIDI,

	".mp4":  FormatMP4,
	".webm": FormatWebM,
	".avi":  FormatAVI,
	".mkv":  FormatMKV,
	".mov":  FormatMOV,
	".wmv":  FormatWMV,
	".flv":  FormatFLV,
	".mpg":  FormatMPEG,
	".mpeg": FormatMPEG,
	".3gp":  Format3GP,

	".pdf":      FormatPDF,
	".doc":      FormatDOC,
	".docx":     FormatDOCX,
	".xls":      FormatXLS,
	".xlsx":     FormatXLSX,
	".ppt":      FormatPPT,
	".pptx":     FormatPPTX,
	".txt":      FormatTXT,
	".rtf":      FormatRTF,
	".csv":      FormatCSV,
	".zip":      FormatZIP,
	".epub":     FormatEPUB,
	".json":     FormatJSON,
	".xml":      FormatXML,
	".html":     FormatHTML,
	".htm":      FormatHTML,
	".yaml":     FormatYAML,
	".yml":      FormatYAML,
	".md":       FormatMD,
	".markdown": FormatMD,
}

type MediaType struct {
	Type     Type
	Format   Format
	MimeType string
}

type Detector struct {
	mimeOverrides map[string]Format
}

func NewDetector() *Detector {
	return &Detector{
		mimeOverrides: make(map[string]Format),
	}
}

func (d *Detector) RegisterMIMEOverride(mimeType string, format Format) {
	d.mimeOverrides[mimeType] = format
}

func (d *Detector) Detect(data []byte, filename string, mimeType string) MediaType {
	result := MediaType{
		Type:     TypeDoc,
		Format:   FormatUnknown,
		MimeType: mimeType,
	}

	if len(data) > 0 {
		result = d.detectFromBytes(data)
	}

	if mimeType != "" {
		mimeResult := d.detectFromMIME(mimeType)
		if mimeResult.Format != FormatUnknown {
			if result.Format == FormatUnknown || result.Format == FormatTXT {
				result = mimeResult
			}
		}
	}

	if result.Format == FormatUnknown && filename != "" {
		result = d.detectFromExtension(filename)
	}

	if result.Format != FormatUnknown && result.MimeType == "" {
		result.MimeType = formatToMIME(result.Format)
	}

	if result.Type == "" {
		result.Type = formatToType(result.Format)
	}

	return result
}

func (d *Detector) DetectFromBytes(data []byte) MediaType {
	if len(data) == 0 {
		return MediaType{Type: "", Format: FormatUnknown}
	}
	return d.detectFromBytes(data)
}

func (d *Detector) detectFromBytes(data []byte) MediaType {
	if len(data) < 2 {
		return MediaType{Type: "", Format: FormatUnknown}
	}

	if result := d.detectImage(data); result.Format != FormatUnknown {
		return result
	}
	if result := d.detectVideo(data); result.Format != FormatUnknown {
		return result
	}
	if result := d.detectAudio(data); result.Format != FormatUnknown {
		return result
	}
	if result := d.detectDocument(data); result.Format != FormatUnknown {
		return result
	}

	return MediaType{Type: "", Format: FormatUnknown}
}

func (d *Detector) detectImage(data []byte) MediaType {
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return MediaType{Type: TypeImage, Format: FormatJPEG, MimeType: "image/jpeg"}
	}

	if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A {
		return MediaType{Type: TypeImage, Format: FormatPNG, MimeType: "image/png"}
	}

	if len(data) >= 6 && bytes.Equal(data[:3], []byte("GIF")) &&
		data[3] == '8' && data[5] == 'a' && (data[4] == '7' || data[4] == '9') {
		return MediaType{Type: TypeImage, Format: FormatGIF, MimeType: "image/gif"}
	}

	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return MediaType{Type: TypeImage, Format: FormatWebP, MimeType: "image/webp"}
	}

	if len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D {
		return MediaType{Type: TypeImage, Format: FormatBMP, MimeType: "image/bmp"}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("MM\x00\x2A")) {
		return MediaType{Type: TypeImage, Format: FormatTIFF, MimeType: "image/tiff"}
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte("II\x2A\x00")) {
		return MediaType{Type: TypeImage, Format: FormatTIFF, MimeType: "image/tiff"}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("\x00\x00\x01\x00")) {
		return MediaType{Type: TypeImage, Format: FormatICO, MimeType: "image/x-icon"}
	}

	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
		ftypBrand := string(data[8:12])
		if ftypBrand == "avif" || ftypBrand == "avis" {
			return MediaType{Type: TypeImage, Format: FormatAVIF, MimeType: "image/avif"}
		}
		if ftypBrand == "heic" || ftypBrand == "heix" || ftypBrand == "mif1" || ftypBrand == "msf1" {
			return MediaType{Type: TypeImage, Format: FormatHEIC, MimeType: "image/heic"}
		}
		if ftypBrand == "jp2 " || ftypBrand == "jpx " || ftypBrand == "jpm " || ftypBrand == "mjp2" {
			return MediaType{Type: TypeImage, Format: FormatJPEG2000, MimeType: "image/jp2"}
		}
	}

	if len(data) > 5 {
		prefix := strings.TrimSpace(string(data[:min(len(data), 200)]))
		if strings.HasPrefix(prefix, "<svg") || strings.HasPrefix(prefix, "<?xml") && strings.Contains(prefix, "<svg") {
			return MediaType{Type: TypeImage, Format: FormatSVG, MimeType: "image/svg+xml"}
		}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("8BPS")) {
		return MediaType{Type: TypeImage, Format: FormatPSD, MimeType: "image/vnd.adobe.photoshop"}
	}

	return MediaType{Type: TypeImage, Format: FormatUnknown}
}

func (d *Detector) detectAudio(data []byte) MediaType {
	if len(data) >= 2 && bytes.Equal(data[:2], []byte("\xFF\xF1")) {
		return MediaType{Type: TypeAudio, Format: FormatAAC, MimeType: "audio/aac"}
	}

	if len(data) >= 3 && data[0] == 0xFF && (data[1]&0xF0) == 0xF0 {
		return MediaType{Type: TypeAudio, Format: FormatMP3, MimeType: "audio/mpeg"}
	}

	if len(data) >= 3 && bytes.Equal(data[:3], []byte("ID3")) {
		return MediaType{Type: TypeAudio, Format: FormatMP3, MimeType: "audio/mpeg"}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("RIFF")) && len(data) >= 12 &&
		bytes.Equal(data[8:12], []byte("WAVE")) {
		return MediaType{Type: TypeAudio, Format: FormatWAV, MimeType: "audio/wav"}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("OggS")) {
		if len(data) >= 36 && bytes.Equal(data[28:36], []byte("OpusHead")) {
			return MediaType{Type: TypeAudio, Format: FormatOGG, MimeType: "audio/ogg"}
		}
		if len(data) >= 36 && bytes.Equal(data[28:36], []byte("\x7fFLAC")) {
			return MediaType{Type: TypeAudio, Format: FormatFLAC, MimeType: "audio/flac"}
		}
		return MediaType{Type: TypeAudio, Format: FormatOGG, MimeType: "audio/ogg"}
	}

	if len(data) >= 5 && bytes.Equal(data[:5], []byte("\x7fFLAC")) {
		return MediaType{Type: TypeAudio, Format: FormatFLAC, MimeType: "audio/flac"}
	}

	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
		ftypBrand := string(data[8:12])
		if ftypBrand == "M4A " || ftypBrand == "M4B " || ftypBrand == "mp42" {
			return MediaType{Type: TypeAudio, Format: FormatM4A, MimeType: "audio/mp4"}
		}
	}

	if len(data) >= 2 && bytes.Equal(data[:2], []byte("#!")) {
		return MediaType{Type: TypeAudio, Format: FormatAMR, MimeType: "audio/amr"}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("MThd")) {
		return MediaType{Type: TypeAudio, Format: FormatMIDI, MimeType: "audio/midi"}
	}

	if len(data) >= 18 && bytes.Equal(data[:16], []byte{0x30, 0x26, 0xB2, 0x75, 0x8E, 0x66, 0xCF, 0x11, 0xA6, 0xD9, 0x00, 0xAA, 0x00, 0x62, 0xCE, 0x6C}) {
		objectType := data[17]
		if objectType == 0x02 {
			return MediaType{Type: TypeAudio, Format: FormatWMA, MimeType: "audio/x-ms-wma"}
		}
	}

	return MediaType{Type: TypeAudio, Format: FormatUnknown}
}

func (d *Detector) detectVideo(data []byte) MediaType {
	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
		ftypBrand := string(data[8:12])
		switch ftypBrand {
		case "mp41", "mp42", "isom", "iso2":
			return MediaType{Type: TypeVideo, Format: FormatMP4, MimeType: "video/mp4"}
		case "M4V ":
			return MediaType{Type: TypeVideo, Format: FormatMP4, MimeType: "video/mp4"}
		case "3gp4", "3gp5", "3gp6", "3g2a":
			return MediaType{Type: TypeVideo, Format: Format3GP, MimeType: "video/3gpp"}
		}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		if len(data) >= 8 && bytes.Contains(data[4:8], []byte("webm")) {
			return MediaType{Type: TypeVideo, Format: FormatWebM, MimeType: "video/webm"}
		}
		return MediaType{Type: TypeVideo, Format: FormatMKV, MimeType: "video/x-matroska"}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("RIFF")) && len(data) >= 12 &&
		bytes.Equal(data[8:12], []byte("AVI ")) {
		return MediaType{Type: TypeVideo, Format: FormatAVI, MimeType: "video/x-msvideo"}
	}

	if len(data) >= 8 && bytes.Equal(data[4:8], []byte("wide")) ||
		(len(data) >= 12 && bytes.Equal(data[4:8], []byte("moov"))) {
		return MediaType{Type: TypeVideo, Format: FormatMOV, MimeType: "video/quicktime"}
	}

	if len(data) >= 4 && bytes.Equal(data[:3], []byte{0x00, 0x00, 0x01}) &&
		(data[3] == 0xB0 || data[3] == 0xB3) {
		return MediaType{Type: TypeVideo, Format: FormatMPEG, MimeType: "video/mpeg"}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("FLV\x01")) {
		return MediaType{Type: TypeVideo, Format: FormatFLV, MimeType: "video/x-flv"}
	}

	if len(data) >= 18 && bytes.Equal(data[:16], []byte{0x30, 0x26, 0xB2, 0x75, 0x8E, 0x66, 0xCF, 0x11, 0xA6, 0xD9, 0x00, 0xAA, 0x00, 0x62, 0xCE, 0x6C}) {
		objectType := data[17]
		if objectType == 0x02 {
			return MediaType{Type: TypeAudio, Format: FormatWMA, MimeType: "audio/x-ms-wma"}
		}
		return MediaType{Type: TypeVideo, Format: FormatWMV, MimeType: "video/x-ms-wmv"}
	}

	return MediaType{Type: TypeVideo, Format: FormatUnknown}
}

func (d *Detector) detectDocument(data []byte) MediaType {
	if len(data) >= 5 && bytes.Equal(data[:5], []byte("%PDF-")) {
		return MediaType{Type: TypeDoc, Format: FormatPDF, MimeType: "application/pdf"}
	}

	if len(data) >= 2 && bytes.Equal(data[:2], []byte("PK")) {
		if len(data) >= 30 {
			content := string(data[2:min(len(data), 500)])
			if strings.Contains(content, "word/") {
				return MediaType{Type: TypeDoc, Format: FormatDOCX, MimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"}
			}
			if strings.Contains(content, "xl/") {
				return MediaType{Type: TypeDoc, Format: FormatXLSX, MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}
			}
			if strings.Contains(content, "ppt/") {
				return MediaType{Type: TypeDoc, Format: FormatPPTX, MimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation"}
			}
			if strings.Contains(content, "mimetypeapplication/epub+zip") {
				return MediaType{Type: TypeDoc, Format: FormatEPUB, MimeType: "application/epub+zip"}
			}
		}
		if len(data) >= 4 && data[2] == 0x03 && data[3] == 0x04 {
			return MediaType{Type: TypeDoc, Format: FormatZIP, MimeType: "application/zip"}
		}
	}

	if len(data) >= 8 && bytes.Equal(data[:8], []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1")) {
		return MediaType{Type: TypeDoc, Format: FormatDOC, MimeType: "application/msword"}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("{\\rt")) {
		return MediaType{Type: TypeDoc, Format: FormatRTF, MimeType: "text/rtf"}
	}

	if len(data) > 0 {
		text := string(data[:min(len(data), 500)])
		trimmed := strings.TrimSpace(text)

		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			if isJSON(trimmed) {
				return MediaType{Type: TypeDoc, Format: FormatJSON, MimeType: "application/json"}
			}
		}

		if strings.HasPrefix(trimmed, "<!DOCTYPE") || strings.HasPrefix(trimmed, "<html") || strings.HasPrefix(trimmed, "<HTML") {
			return MediaType{Type: TypeDoc, Format: FormatHTML, MimeType: "text/html"}
		}

		if strings.HasPrefix(trimmed, "<?xml") {
			return MediaType{Type: TypeDoc, Format: FormatXML, MimeType: "application/xml"}
		}

		if isMarkdown(trimmed) {
			return MediaType{Type: TypeDoc, Format: FormatMD, MimeType: "text/markdown"}
		}

		if isYAML(trimmed) {
			return MediaType{Type: TypeDoc, Format: FormatYAML, MimeType: "text/yaml"}
		}

		if isPlainText(text) {
			lines := strings.SplitN(text, "\n", 3)
			if len(lines) >= 2 {
				commasFirst := strings.Count(lines[0], ",")
				if commasFirst > 0 {
					isCSV := true
					for _, line := range lines[1:] {
						if strings.TrimSpace(line) != "" && strings.Count(line, ",") != commasFirst {
							isCSV = false
							break
						}
					}
					if isCSV {
						return MediaType{Type: TypeDoc, Format: FormatCSV, MimeType: "text/csv"}
					}
				}
			}
			return MediaType{Type: TypeDoc, Format: FormatTXT, MimeType: "text/plain"}
		}
	}

	return MediaType{Type: TypeDoc, Format: FormatUnknown}
}

func (d *Detector) detectFromMIME(mimeType string) MediaType {
	if override, ok := d.mimeOverrides[mimeType]; ok {
		return MediaType{
			Type:     formatToType(override),
			Format:   override,
			MimeType: mimeType,
		}
	}

	if format, ok := mimeToFormat[mimeType]; ok {
		return MediaType{
			Type:     formatToType(format),
			Format:   format,
			MimeType: mimeType,
		}
	}

	if strings.HasPrefix(mimeType, "image/") {
		return MediaType{Type: TypeImage, Format: FormatUnknown, MimeType: mimeType}
	}
	if strings.HasPrefix(mimeType, "audio/") {
		return MediaType{Type: TypeAudio, Format: FormatUnknown, MimeType: mimeType}
	}
	if strings.HasPrefix(mimeType, "video/") {
		return MediaType{Type: TypeVideo, Format: FormatUnknown, MimeType: mimeType}
	}
	if strings.HasPrefix(mimeType, "text/") {
		return MediaType{Type: TypeDoc, Format: FormatTXT, MimeType: mimeType}
	}

	return MediaType{Type: TypeDoc, Format: FormatUnknown, MimeType: mimeType}
}

func (d *Detector) detectFromExtension(filename string) MediaType {
	ext := strings.ToLower(filename)
	if idx := strings.LastIndex(ext, "."); idx >= 0 {
		ext = ext[idx:]
	} else {
		return MediaType{Type: TypeDoc, Format: FormatUnknown}
	}

	if format, ok := extToFormat[ext]; ok {
		return MediaType{
			Type:     formatToType(format),
			Format:   format,
			MimeType: formatToMIME(format),
		}
	}

	if mt := mime.TypeByExtension(ext); mt != "" {
		return d.detectFromMIME(mt)
	}

	return MediaType{Type: TypeDoc, Format: FormatUnknown}
}

func formatToType(format Format) Type {
	switch format {
	case FormatJPEG, FormatPNG, FormatGIF, FormatWebP, FormatBMP, FormatSVG, FormatTIFF, FormatICO, FormatAVIF, FormatHEIC, FormatJPEG2000, FormatPSD:
		return TypeImage
	case FormatMP3, FormatWAV, FormatOGG, FormatFLAC, FormatAAC, FormatM4A, FormatWMA, FormatAMR, FormatMIDI:
		return TypeAudio
	case FormatMP4, FormatWebM, FormatAVI, FormatMKV, FormatMOV, FormatWMV, FormatFLV, FormatMPEG, Format3GP:
		return TypeVideo
	default:
		return TypeDoc
	}
}

func formatToMIME(format Format) string {
	switch format {
	case FormatJPEG:
		return "image/jpeg"
	case FormatPNG:
		return "image/png"
	case FormatGIF:
		return "image/gif"
	case FormatWebP:
		return "image/webp"
	case FormatBMP:
		return "image/bmp"
	case FormatSVG:
		return "image/svg+xml"
	case FormatTIFF:
		return "image/tiff"
	case FormatICO:
		return "image/x-icon"
	case FormatAVIF:
		return "image/avif"
	case FormatHEIC:
		return "image/heic"
	case FormatMP3:
		return "audio/mpeg"
	case FormatWAV:
		return "audio/wav"
	case FormatOGG:
		return "audio/ogg"
	case FormatFLAC:
		return "audio/flac"
	case FormatAAC:
		return "audio/aac"
	case FormatM4A:
		return "audio/mp4"
	case FormatWMA:
		return "audio/x-ms-wma"
	case FormatAMR:
		return "audio/amr"
	case FormatMIDI:
		return "audio/midi"
	case FormatMP4:
		return "video/mp4"
	case FormatWebM:
		return "video/webm"
	case FormatAVI:
		return "video/x-msvideo"
	case FormatMKV:
		return "video/x-matroska"
	case FormatMOV:
		return "video/quicktime"
	case FormatWMV:
		return "video/x-ms-wmv"
	case FormatFLV:
		return "video/x-flv"
	case FormatMPEG:
		return "video/mpeg"
	case Format3GP:
		return "video/3gpp"
	case FormatPDF:
		return "application/pdf"
	case FormatDOC:
		return "application/msword"
	case FormatDOCX:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case FormatXLS:
		return "application/vnd.ms-excel"
	case FormatXLSX:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case FormatPPT:
		return "application/vnd.ms-powerpoint"
	case FormatPPTX:
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case FormatTXT:
		return "text/plain"
	case FormatRTF:
		return "text/rtf"
	case FormatCSV:
		return "text/csv"
	case FormatZIP:
		return "application/zip"
	case FormatEPUB:
		return "application/epub+zip"
	case FormatJSON:
		return "application/json"
	case FormatXML:
		return "application/xml"
	case FormatHTML:
		return "text/html"
	case FormatYAML:
		return "text/yaml"
	case FormatMD:
		return "text/markdown"
	case FormatJPEG2000:
		return "image/jp2"
	case FormatPSD:
		return "image/vnd.adobe.photoshop"
	default:
		return "application/octet-stream"
	}
}

func isPlainText(data string) bool {
	if len(data) == 0 {
		return false
	}

	nonPrintable := 0
	total := 0
	for _, r := range data {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 || (r >= 0x7F && r < 0xA0) {
			nonPrintable++
		}
		total++
	}

	if total == 0 {
		return true
	}

	return float64(nonPrintable)/float64(total) < 0.05
}

func isJSON(data string) bool {
	if len(data) < 2 {
		return false
	}
	trimmed := strings.TrimSpace(data)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return false
	}
	var js json.RawMessage
	return json.Unmarshal([]byte(trimmed), &js) == nil
}

func isYAML(data string) bool {
	lines := strings.Split(data, "\n")
	if len(lines) < 2 {
		return false
	}
	hasKeyValue := false
	for _, line := range lines[:min(len(lines), 10)] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			hasKeyValue = true
			continue
		}
		if strings.Contains(trimmed, ": ") || strings.HasSuffix(trimmed, ":") {
			hasKeyValue = true
		}
	}
	return hasKeyValue && isPlainText(data)
}

func isMarkdown(data string) bool {
	lines := strings.Split(data, "\n")
	checkLines := lines[:min(len(lines), 20)]
	for _, line := range checkLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			return true
		}
		if strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "===") {
			return true
		}
		if (strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "+ ")) && len(trimmed) > 2 {
			return true
		}
		if strings.HasPrefix(trimmed, "```") {
			return true
		}
		if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "](") && strings.HasSuffix(trimmed, ")") {
			return true
		}
	}
	return false
}

func DetectMediaType(data []byte, filename string, mimeType string) MediaType {
	d := NewDetector()
	return d.Detect(data, filename, mimeType)
}

func ExtractImageMetadata(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	meta := map[string]any{
		"width":  bounds.Dx(),
		"height": bounds.Dy(),
		"format": format,
	}

	return meta, nil
}

func ExtractAudioMetadata(data []byte) map[string]any {
	meta := map[string]any{}

	if len(data) >= 3 && bytes.Equal(data[:3], []byte("ID3")) {
		meta["has_id3"] = true
		if len(data) >= 10 {
			meta["id3_version"] = int(data[3])
		}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("RIFF")) && len(data) >= 12 &&
		bytes.Equal(data[8:12], []byte("WAVE")) {
		if len(data) >= 24 {
			channels := binary.LittleEndian.Uint16(data[22:24])
			meta["channels"] = channels
		}
		if len(data) >= 28 {
			sampleRate := binary.LittleEndian.Uint32(data[24:28])
			meta["sample_rate"] = sampleRate
		}
		if len(data) >= 34 {
			bitsPerSample := binary.LittleEndian.Uint16(data[32:34])
			meta["bits_per_sample"] = bitsPerSample
		}
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte("OggS")) {
		if len(data) >= 36 && bytes.Equal(data[28:36], []byte("OpusHead")) {
			meta["codec"] = "opus"
		} else if len(data) >= 36 && bytes.Equal(data[28:36], []byte("\x7fFLAC")) {
			meta["codec"] = "flac"
		} else {
			meta["codec"] = "vorbis"
		}
	}

	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
		meta["container"] = "mp4"
		meta["brand"] = string(data[8:12])
	}

	return meta
}

func ExtractVideoMetadata(data []byte) map[string]any {
	meta := map[string]any{}

	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
		meta["container"] = "mp4"
		meta["brand"] = string(data[8:12])
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		meta["container"] = "matroska"
		if len(data) >= 8 && bytes.Contains(data[4:8], []byte("webm")) {
			meta["container"] = "webm"
		}
	}

	if len(data) >= 4 && bytes.Equal(data[:3], []byte("FLV")) {
		meta["container"] = "flv"
		if len(data) >= 5 {
			flags := data[4]
			meta["has_audio"] = (flags&0x4 != 0)
			meta["has_video"] = (flags&0x1 != 0)
		}
	}

	if len(data) >= 8 && bytes.Equal(data[4:8], []byte("wide")) ||
		(len(data) >= 12 && bytes.Equal(data[4:8], []byte("moov"))) {
		meta["container"] = "quicktime"
	}

	return meta
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func IsUTF8Text(data []byte) bool {
	return isPlainText(string(data))
}

func ReadBoxSize(data []byte, offset int) (uint64, int) {
	if len(data) < offset+8 {
		return 0, 0
	}
	size := uint64(binary.BigEndian.Uint32(data[offset:]))
	headerSize := 8
	if size == 1 && len(data) >= offset+16 {
		size = binary.BigEndian.Uint64(data[offset+8:])
		headerSize = 16
	}
	return size, headerSize
}
