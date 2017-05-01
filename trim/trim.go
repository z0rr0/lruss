// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

//Package trim implements methods and structures to convert users' URLs.
package trim

import (
	"fmt"
	"math"
	"regexp"
	"sort"
)

const (
	// Alphabet is a sorted set of basis numeral system chars.
	Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// maxLen is maximum available short link length
	maxLen = 11
)

var (
	// isShortURL is regexp pattern to check short URL,
	// max int64 9223372036854775807 => AzL8n0Y58m7
	// real, max decode/encode 839299365868340223 <=> zzzzzzzzzz
	isShortURL = regexp.MustCompile(fmt.Sprintf("^[%s]{1,10}$", Alphabet))

	// basis is a numeral system basis
	basis = int64(len(Alphabet))
)

// pow returns x**y, only uses int64 types instead float64.
func pow(x, y int64) int64 {
	return int64(math.Pow(float64(x), float64(y)))
}

// Encode converts a decimal number to Alphabet-base numeral system.
func Encode(x int64) string {
	var result, sign string
	if x == 0 {
		return "0"
	}
	if x < 0 {
		sign, x = "-", -x
	}
	for x > 0 {
		i := int(x % basis)
		result = string(Alphabet[i]) + result
		x = x / basis
	}
	return sign + result
}

// Decode converts a Alphabet-base number to decimal one.
func Decode(x string) (int64, error) {
	var (
		result, j int64
		sign      bool
	)
	l := len(x)
	if l == 0 {
		return 0, nil
	}
	if x[0] == '-' {
		sign, x = true, x[1:l]
		l--
	}
	for i := l - 1; i >= 0; i-- {
		c := x[i]
		k := sort.Search(int(basis), func(t int) bool { return Alphabet[t] >= c })
		p := int64(k)
		if !((p < basis) && (Alphabet[k] == c)) {
			return 0, fmt.Errorf("can't convert %q", c)
		}
		result = result + p*pow(basis, j)
		j++
	}
	if sign {
		result = -result
	}
	return result, nil
}

// IsShort checks a link can be short URL.
func IsShort(pattern string) bool {
	if len(pattern) > maxLen {
		return false
	}
	return isShortURL.MatchString(pattern)
}
