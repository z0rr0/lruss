// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

package trim

import "testing"

func TestEncode(t *testing.T) {
	suite := map[int64]string{
		-1:  "-1",
		-9:  "-9",
		-11: "-B",
		0:   "0",
		1:   "1",
		5:   "5",
		10:  "A",
		61:  "z",
		62:  "10",
		63:  "11",
		72:  "1A",
		124: "20",
		129: "25",
	}
	for k, v := range suite {
		if s := Encode(k); s != v {
			t.Errorf("incorrect values: %v != %v", s, v)
		}
		if num, err := Decode(v); (err != nil) || (num != k) {
			t.Errorf("incorrect values: %v, %v, %v", err, num, k)
		}
	}
	if _, err := Decode("34.56"); err == nil {
		t.Error("unexpected behavior")
	}
}

func BenchmarkEncode(b *testing.B) {
	// max 9223372036854775807 == AzL8n0Y58m7
	x := "AzL8n0Y58m7"
	for i := 0; i < b.N; i++ {
		num, err := Decode(x)
		if err != nil {
			b.Fatal(err)
		}
		if s := Encode(num); s != x {
			b.Fatalf("bad result: %v %v", s, x)
		}
	}
}
