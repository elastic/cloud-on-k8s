// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"encoding/binary"
	"hash"
	"hash/crc32"
	"io"
)

// Lucene codec constants used by the Elasticsearch keystore format.
// See: https://github.com/apache/lucene/blob/main/lucene/core/src/java/org/apache/lucene/codecs/CodecUtil.java
const (
	// CodecMagic is the magic number at the start of a Lucene codec file
	CodecMagic uint32 = 0x3FD76C17
	// FooterMagic is the magic number at the start of the footer (~CodecMagic)
	FooterMagic uint32 = 0xC02893E8
	// CodecName is the codec identifier for Elasticsearch keystore files
	CodecName = "elasticsearch.keystore"
	// FooterLength is the size of the Lucene footer in bytes (magic + algorithmID + checksum)
	FooterLength = 16
)

// checksumWriter wraps an io.Writer and computes a CRC32 checksum of all written data.
type checksumWriter struct {
	w    io.Writer
	crc  hash.Hash32
	size int64
}

// newChecksumWriter creates a new checksumWriter that wraps the given writer.
// It uses CRC32 with the IEEE polynomial, which matches java.util.zip.CRC32
// used by Lucene's BufferedChecksumIndexInput.
func newChecksumWriter(w io.Writer) *checksumWriter {
	return &checksumWriter{
		w:   w,
		crc: crc32.NewIEEE(),
	}
}

// Write writes data to the underlying writer and updates the checksum.
func (c *checksumWriter) Write(p []byte) (n int, err error) {
	n, err = c.w.Write(p)
	if n > 0 {
		c.crc.Write(p[:n])
		c.size += int64(n)
	}
	return n, err
}

// WriteByte writes a single byte.
func (c *checksumWriter) WriteByte(b byte) error {
	_, err := c.Write([]byte{b})
	return err
}

// Checksum returns the current CRC32 checksum value.
func (c *checksumWriter) Checksum() uint32 {
	return c.crc.Sum32()
}

// Size returns the total number of bytes written.
func (c *checksumWriter) Size() int64 {
	return c.size
}

// WriteHeader writes the Lucene codec header to the writer.
// The header format is:
//   - Magic (4 bytes, big endian): 0x3FD76C17
//   - Codec name length (variable-length int, typically 1 byte for short names)
//   - Codec name (UTF-8 bytes)
//   - Version (4 bytes, big endian)
func WriteHeader(w *checksumWriter, version int32) error {
	// Write magic number (always big endian)
	if err := binary.Write(w, binary.BigEndian, CodecMagic); err != nil {
		return err
	}

	// Write codec name as a "String" in Lucene format
	// Lucene uses a variable-length encoding for string length
	codecBytes := []byte(CodecName)
	if err := writeVInt(w, int32(len(codecBytes))); err != nil {
		return err
	}
	if _, err := w.Write(codecBytes); err != nil {
		return err
	}

	// Write version (always big endian in header)
	if err := binary.Write(w, binary.BigEndian, version); err != nil {
		return err
	}

	return nil
}

// WriteFooter writes the Lucene codec footer to the writer.
// The footer format is:
//   - Footer magic (4 bytes, big endian): ~0x3FD76C17 = 0xC02893E8
//   - Algorithm ID (4 bytes, big endian): 0 (indicating CRC32)
//   - CRC32 checksum (8 bytes, big endian, as long)
//
// Note: The entire footer is always big-endian, matching Lucene's CodecUtil.writeFooter.
func WriteFooter(w *checksumWriter) error {
	// Footer magic (always big endian)
	if err := binary.Write(w, binary.BigEndian, FooterMagic); err != nil {
		return err
	}
	// Algorithm ID (always big endian, always 0)
	if err := binary.Write(w, binary.BigEndian, int32(0)); err != nil {
		return err
	}

	// Get checksum before writing it (includes everything written so far)
	checksum := w.Checksum()

	// Checksum is written as a long (8 bytes, always big endian)
	// Lucene always uses big-endian for the checksum in the footer
	if err := binary.Write(w, binary.BigEndian, int64(checksum)); err != nil {
		return err
	}

	return nil
}

// writeVInt writes a variable-length integer in Lucene's format.
// Lucene uses 7 bits per byte, with the high bit set if more bytes follow.
func writeVInt(w io.Writer, i int32) error {
	for i&^0x7F != 0 {
		if _, err := w.Write([]byte{byte((i & 0x7F) | 0x80)}); err != nil {
			return err
		}
		i >>= 7
	}
	_, err := w.Write([]byte{byte(i)})
	return err
}

// writeInt writes a 4-byte integer with the specified byte order.
func writeInt(w io.Writer, value int, useLittleEndian bool) error {
	var order binary.ByteOrder = binary.BigEndian
	if useLittleEndian {
		order = binary.LittleEndian
	}
	return binary.Write(w, order, int32(value))
}

// writeBytes writes a length-prefixed byte slice.
func writeBytes(w io.Writer, data []byte, useLittleEndian bool) error {
	if err := writeInt(w, len(data), useLittleEndian); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}
