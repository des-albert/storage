package chunked

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"testing"
)

const jsonTOC = `
{
  "version": 1,
  "entries": [
    {
      "type": "symlink",
      "name": "bin",
      "linkName": "usr/bin",
      "mode": 511,
      "modtime": "1970-01-01T01:00:00+01:00",
      "accesstime": "0001-01-01T00:00:00Z",
      "changetime": "0001-01-01T00:00:00Z"
    },
    {
      "type": "dir",
      "name": "usr/bin",
      "mode": 511,
      "modtime": "2022-01-07T12:36:43+01:00",
      "accesstime": "0001-01-01T00:00:00Z",
      "changetime": "0001-01-01T00:00:00Z"
    },
    {
      "type": "reg",
      "name": "usr/bin/foo",
      "mode": 511,
      "size": 103867,
      "modtime": "1970-01-01T01:00:00+01:00",
      "accesstime": "0001-01-01T00:00:00Z",
      "changetime": "0001-01-01T00:00:00Z",
      "digest": "sha256:99fe908c699dc068438b23e28319cadff1f2153c3043bafb8e83a430bba0a2c6",
      "offset": 94149,
      "endOffset": 120135,
      "chunkSize": 17615,
      "chunkDigest": "sha256:2ce0d0f8eb2aa93d13007097763e4459c814c8d0e859e5a57465af924169b544"
    },
    {
      "type": "chunk",
      "name": "usr/bin/foo",
      "offset": 99939,
      "chunkSize": 86252,
      "chunkOffset": 17615,
      "chunkDigest": "sha256:2a9d3f1b6b37abc8bb35eb8fa98b893a2a2447bcb01184c3bafc8c6b40da099d"
    }
}
`

func TestPrepareMetadata(t *testing.T) {
	toc, err := prepareMetadata([]byte(jsonTOC))
	if err != nil {
		t.Errorf("got error from prepareMetadata: %w", err)
	}
	if len(toc) != 2 {
		t.Error("prepareMetadata returns the wrong length")
	}
}

type bigDataToBuffer struct {
	buf    *bytes.Buffer
	id     string
	key    string
	called bool
}

func (b *bigDataToBuffer) SetLayerBigData(id, key string, data io.Reader) error {
	b.id = id
	b.key = key
	if b.called {
		return fmt.Errorf("SetLayerBigData already called once")
	}
	b.called = true
	_, err := io.Copy(b.buf, data)
	return err
}

func TestWriteCache(t *testing.T) {
	toc, err := prepareMetadata([]byte(jsonTOC))
	if err != nil {
		t.Errorf("got error from prepareMetadata: %w", err)
	}

	dest := bigDataToBuffer{
		buf: bytes.NewBuffer(nil),
	}
	cache, err := writeCache([]byte(jsonTOC), "foobar", &dest)
	if err != nil {
		t.Errorf("got error from writeCache: %w", err)
	}
	if digest, _, _ := findTag("foobar", cache); digest != "" {
		t.Error("found invalid tag")
	}

	for _, r := range toc {
		if r.Digest != "" {
			// find the element in the cache by the digest checksum
			digest, off, len := findTag(r.Digest, cache)
			if digest == "" {
				t.Error("file tag not found")
			}
			if digest != r.Digest {
				t.Error("wrong file found")
			}
			expectedLocation := generateFileLocation(r.Name, 0)
			location := cache.vdata[off : off+len]
			if !bytes.Equal(location, expectedLocation) {
				t.Errorf("wrong file found %q instead of %q", location, expectedLocation)
			}

			fingerprint, err := calculateHardLinkFingerprint(r)
			if err != nil {
				t.Errorf("got error from writeCache: %w", err)
			}

			// find the element in the cache by the hardlink fingerprint
			digest, off, len = findTag(fingerprint, cache)
			if digest == "" {
				t.Error("file tag not found")
			}
			if digest != fingerprint {
				t.Error("wrong file found")
			}
			expectedLocation = generateFileLocation(r.Name, 0)
			location = cache.vdata[off : off+len]
			if !bytes.Equal(location, expectedLocation) {
				t.Errorf("wrong file found %q instead of %q", location, expectedLocation)
			}
		}
		if r.ChunkDigest != "" {
			// find the element in the cache by the chunk digest checksum
			digest, off, len := findTag(r.ChunkDigest, cache)
			if digest == "" {
				t.Error("chunk tag not found")
			}
			if digest != r.ChunkDigest {
				t.Error("wrong digest found")
			}
			expectedLocation := generateFileLocation(r.Name, uint64(r.ChunkOffset))
			location := cache.vdata[off : off+len]
			if !bytes.Equal(location, expectedLocation) {
				t.Errorf("wrong file found %q instead of %q", location, expectedLocation)
			}
		}
	}
}

func TestReadCache(t *testing.T) {
	dest := bigDataToBuffer{
		buf: bytes.NewBuffer(nil),
	}
	cache, err := writeCache([]byte(jsonTOC), "foobar", &dest)
	if err != nil {
		t.Errorf("got error from writeCache: %w", err)
	}

	cacheRead, err := readMetadataFromCache(dest.buf)
	if err != nil {
		t.Errorf("got error from readMetadataFromCache: %w", err)
	}
	if !reflect.DeepEqual(cache, cacheRead) {
		t.Errorf("read a different struct than what was written")
	}
}
