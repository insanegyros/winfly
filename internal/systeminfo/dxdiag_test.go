package systeminfo

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestParseDirectXVersionXML(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?><DxDiag><SystemInformation><DirectXVersion>DirectX 12</DirectXVersion></SystemInformation></DxDiag>`)
	value, err := parseDirectXVersionXML(data)
	if err != nil {
		t.Fatal(err)
	}
	if value != "DirectX 12" {
		t.Fatalf("verze = %q, očekáváno %q", value, "DirectX 12")
	}
}

func TestNormalizeXMLTextAcceptsUTF16LEWithAndWithoutBOM(t *testing.T) {
	xmlText := `<?xml version="1.0" encoding="utf-16"?><DxDiag><DirectXVersion>DirectX 12</DirectXVersion></DxDiag>`
	encoded := encodeUTF16ForTest(xmlText, binary.LittleEndian)

	for _, data := range [][]byte{append([]byte{0xff, 0xfe}, encoded...), encoded} {
		normalized, err := normalizeXMLText(data)
		if err != nil {
			t.Fatal(err)
		}
		value, err := parseDirectXVersionXML(normalized)
		if err != nil {
			t.Fatal(err)
		}
		if value != "DirectX 12" {
			t.Fatalf("verze = %q, očekáváno %q", value, "DirectX 12")
		}
	}
}

func encodeUTF16ForTest(value string, order binary.ByteOrder) []byte {
	units := utf16.Encode([]rune(value))
	result := make([]byte, len(units)*2)
	for index, unit := range units {
		order.PutUint16(result[index*2:index*2+2], unit)
	}
	return result
}
