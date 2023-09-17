package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/text/transform"
)

type UaoDecoder struct {
	transform.NopResetter
}

func (c *UaoDecoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	size := 0
	for ; nSrc < len(src); nSrc += size {
		byteW := src[nSrc]
		if byteW > 0x80 {
			k := binary.BigEndian.Uint16(src[nSrc : nSrc+2])
			r, ok := B2U[int(k)]
			if !ok {
				fmt.Printf("decode fail: %d %c %s\n", k, byteW, src[nSrc:nSrc+2])
				dst[nDst] = src[nSrc]
				size = 1
				nDst = 1
				if nSrc+1 < len(src) {
					dst[nDst+1] = src[nSrc+1]
					size += 1
					nDst += 1
				}
				continue
			}
			elems := []byte(string(r))
			for i := 0; i < len(elems); i++ {
				dst[nDst+i] = elems[i]
			}
			size = 2
			nDst += len(elems)
		} else {
			dst[nDst] = byteW
			size = 1
			nDst += 1
		}
	}
	return nDst, nSrc, nil
}

func NewUaoDecoder() *UaoDecoder {
	return &UaoDecoder{}
}

func Utf8ToUaoBig5(src string) (dst string, err error) {
	for _, s := range src {
		if s <= 0x80 {
			dst += string(s)
			continue
		}
		t, ok := U2B[string(s)]
		if !ok {
			fmt.Printf("%c\n", s)
			return "", errors.New("encode error")
		}
		dst += t
	}
	return dst, nil
}
