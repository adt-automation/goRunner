package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

func tsByteBuffer(timestamp int64) *bytes.Buffer {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, timestamp)
	return buf
}

func buildIv(reqTime time.Time) []byte {
	timestamp := reqTime.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
	buf := tsByteBuffer(timestamp)
	iv := make([]byte, 0, 16)
	iv = append(iv, buf.Bytes()[2:]...)
	iv = append(iv, buf.Bytes()[2:]...)
	iv = append(iv, buf.Bytes()[2:6]...)
	return iv
}

func buildKey(keyStr string) []byte {
	key := make([]byte, 0, 32)

	if strings.Count(keyStr, ",") != 31 {
		log.Fatal(fmt.Sprintf("32-byte key required, current key will be %d bytes", 1+strings.Count(keyStr, ",")))
	}

	for _, ds := range strings.Split(keyStr, ",") {
		ds = strings.TrimSpace(ds)
		di, err := strconv.Atoi(ds)
		if err != nil {
			log.Fatal(err.Error() + " during encryption key construction")
		} else {
			key = append(key, byte(di))
		}
	}
	return key
}

func encrypt(key, iv, text []byte) (ciphertextOut []byte, err error) {
	if len(text) < aes.BlockSize {
		err = errors.New(fmt.Sprintf("input text is %d bytes, too short to encrypt", len(text)))
		return
	}
	if len(text)%aes.BlockSize != 0 {
		err = errors.New(fmt.Sprintf("input text is %d bytes, must be a multiple of %d", len(text), aes.BlockSize))
		return
	}
	ciphertextOut = make([]byte, len(string(text)))
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	cfb := cipher.NewCBCEncrypter(block, iv)
	cfb.CryptBlocks(ciphertextOut, text)

	return
}

func decrypt(key, iv, ciphertext []byte) (plaintextOut []byte, err error) {
	plaintextOut = make([]byte, len(ciphertext))
	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	if len(ciphertext) < aes.BlockSize {
		err = errors.New("ciphertext too short")
		return
	}
	cfb := cipher.NewCBCDecrypter(block, iv)
	cfb.CryptBlocks(plaintextOut, ciphertext)
	return
}
