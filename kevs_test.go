package kevs

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func Test_ucs_to_utf8(t *testing.T) {
	tests := []struct {
		in  uint64
		n   uint8
		out []byte
	}{
		// valid

		// single byte
		{0x00000000, 1, []byte{0x00}}, // min
		{0x00000042, 1, []byte{0x42}},
		{0x0000007f, 1, []byte{0x7f}}, // max

		// two byte
		{0x00000080, 2, []byte{0xc2, 0x80}}, // min
		{0x000007ff, 2, []byte{0xdf, 0xbf}}, // max

		// three byte
		{0x00000800, 3, []byte{0xe0, 0xa0, 0x80}}, // min
		{0x0000d7ff, 3, []byte{0xed, 0x9f, 0xbf}}, // max < surogate
		{0x0000e000, 3, []byte{0xee, 0x80, 0x80}}, // min > surogate
		{0x0000ffff, 3, []byte{0xef, 0xbf, 0xbf}}, // max

		// four byte
		{0x00010000, 4, []byte{0xf0, 0x90, 0x80, 0x80}}, // min
		{0x0010ffff, 4, []byte{0xf4, 0x8f, 0xbf, 0xbf}}, // max

		// invalid
		{0x0000d800, 0, []byte{}}, // low surogate start
		{0x0000dbff, 0, []byte{}}, // low surogate end
		{0x0000dc00, 0, []byte{}}, // high surogate start
		{0x0000dfff, 0, []byte{}}, // high surogate end

		// edge cases
		{0x00110000, 0, []byte{}}, // after max
	}

	for _, test := range tests {
		out := ucs_to_utf8(test.in)
		if len(out) != len(test.out) {
			t.Errorf("len: want %d, have %d", len(out), len(test.out))
		}
		if !bytes.Equal(out, test.out) {
			t.Log("want:", hex.EncodeToString(out))
			t.Log("have:", hex.EncodeToString(test.out))
			t.Error("unexpected output")
		}
	}
}

func TestUnmarshal(t *testing.T) {
	type data struct {
		String  string   `kevs:"string"`
		Integer int      `kevs:"integer"`
		Boolean bool     `kevs:"boolean"`
		List    []string `kevs:"list"`
		Struct  struct {
			X int    `kevs:"x"`
			Y string `kevs:"y"`
		} `kevs:"struct"`
	}

	content := `
string = "42";
integer = 42;
boolean = true;
list = [ "aa"; "bb"; ];
struct = { x = 2; y = "3"; };
`

	root, err := Parse("none", content, Flags{})
	if err != nil {
		t.Fatal(err)
	}

	var d data
	if err := root.Unmarshal(&d); err != nil {
		t.Fatal(err)
	}

	t.Log(d)

	if d.String != "42" {
		t.Fatal("fail")
	}
	if d.Integer != 42 {
		t.Fatal("fail")
	}
	if d.Boolean != true {
		t.Fatal("fail")
	}
	if d.Struct.X != 2 {
		t.Fatal("fail")
	}
	if d.Struct.Y != "3" {
		t.Fatal("fail")
	}
	if d.List[0] != "aa" {
		t.Fatal("fail")
	}
	if d.List[1] != "bb" {
		t.Fatal("fail")
	}

}

// TODO: test list of structs
// TODO: test struct of lists
// TODO: test list of lists
// TODO: test list with different elem types
