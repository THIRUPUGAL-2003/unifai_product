package handlers

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/storage/memory"
)

func repoHeadHash(st *memory.Storage) (plumbing.Hash, error) {
	ref, err := st.Reference(plumbing.HEAD)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if ref.Type() == plumbing.SymbolicReference {
		ref, err = st.Reference(ref.Target())
		if err != nil {
			return plumbing.ZeroHash, err
		}
	}
	if ref.Hash() == plumbing.ZeroHash {
		return plumbing.ZeroHash, fmt.Errorf("empty HEAD")
	}
	return ref.Hash(), nil
}

func advertiseRefsV1(head plumbing.Hash) []byte {
	caps := "ofs-delta symref=HEAD:refs/heads/main agent=gateway"
	first := fmt.Sprintf("%s HEAD\x00%s\n", head.String(), caps)
	second := fmt.Sprintf("%s refs/heads/main\n", head.String())
	var b bytes.Buffer
	b.Write(pktLine(first))
	b.Write(pktLine(second))
	b.Write(pktFlush())
	return b.Bytes()
}

func advertiseRefsV2() []byte {
	var b bytes.Buffer
	b.Write(pktLine("version 2\n"))
	b.Write(pktLine("ls-refs\n"))
	b.Write(pktLine("fetch=shallow\n"))
	b.Write(pktFlush())
	return b.Bytes()
}

func uploadPackResponse(st *memory.Storage, request []byte, protocolV2 bool) ([]byte, error) {
	head, err := repoHeadHash(st)
	if err != nil {
		return nil, err
	}
	lines := readPktLines(request)
	if protocolV2 {
		return uploadPackV2(st, head, lines)
	}
	return uploadPackV1(st, head, lines)
}

func uploadPackV1(st *memory.Storage, head plumbing.Hash, lines [][]byte) ([]byte, error) {
	if err := wantIsHead(lines, head); err != nil {
		return nil, err
	}
	pack, err := encodeRepoPack(st)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.Write(pktLine("NAK\n"))
	b.Write(pack)
	return b.Bytes(), nil
}

func uploadPackV2(st *memory.Storage, head plumbing.Hash, lines [][]byte) ([]byte, error) {
	command := ""
	var args [][]byte
	seenDelim := false
	for _, line := range lines {
		if line == nil {
			continue
		}
		if bytes.Equal(line, []byte{1}) {
			seenDelim = true
			continue
		}
		if !seenDelim && command == "" {
			command = strings.TrimSpace(string(line))
			continue
		}
		args = append(args, line)
	}

	switch {
	case strings.HasPrefix(command, "command=ls-refs"):
		return lsRefsV2(head, args), nil
	case strings.HasPrefix(command, "command=fetch"):
		if err := wantIsHead(args, head); err != nil {
			return nil, err
		}
		pack, err := encodeRepoPack(st)
		if err != nil {
			return nil, err
		}
		var b bytes.Buffer
		writeSidebandPack(&b, pack)
		return b.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported git command %q", command)
	}
}

func lsRefsV2(head plumbing.Hash, args [][]byte) []byte {
	prefixes := refPrefixes(args)
	var b bytes.Buffer
	if refAllowed(prefixes, "HEAD") {
		b.Write(pktLine(fmt.Sprintf("%s HEAD symref-target:refs/heads/main\n", head.String())))
	}
	if refAllowed(prefixes, "refs/heads/main") {
		b.Write(pktLine(fmt.Sprintf("%s refs/heads/main\n", head.String())))
	}
	b.Write(pktFlush())
	return b.Bytes()
}

func refPrefixes(args [][]byte) []string {
	var prefixes []string
	for _, arg := range args {
		text := strings.TrimSpace(string(arg))
		if strings.HasPrefix(text, "ref-prefix ") {
			prefixes = append(prefixes, strings.TrimPrefix(text, "ref-prefix "))
		}
	}
	return prefixes
}

func refAllowed(prefixes []string, name string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func wantIsHead(lines [][]byte, head plumbing.Hash) error {
	sawWant := false
	for _, line := range lines {
		if line == nil {
			continue
		}
		text := string(line)
		idx := strings.Index(text, "want ")
		if idx < 0 {
			continue
		}
		sawWant = true
		rest := strings.TrimSpace(text[idx+len("want "):])
		hashText := rest
		if sp := strings.IndexAny(rest, " \n"); sp >= 0 {
			hashText = rest[:sp]
		}
		hash := plumbing.NewHash(hashText)
		if hash != head {
			return fmt.Errorf("want %s is not HEAD %s", hashText, head.String())
		}
	}
	if !sawWant {
		return fmt.Errorf("git fetch missing want")
	}
	return nil
}

func encodeRepoPack(st *memory.Storage) ([]byte, error) {
	var hashes []plumbing.Hash
	iter, err := st.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return nil, err
	}
	err = iter.ForEach(func(obj plumbing.EncodedObject) error {
		hashes = append(hashes, obj.Hash())
		return nil
	})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := packfile.NewEncoder(&buf, st, false)
	if _, err := enc.Encode(hashes, 10); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeSidebandPack(dst *bytes.Buffer, pack []byte) {
	dst.Write(pktLine("packfile\n"))
	const chunkSize = 65515
	for len(pack) > 0 {
		n := len(pack)
		if n > chunkSize {
			n = chunkSize
		}
		chunk := pack[:n]
		pack = pack[n:]
		size := 4 + 1 + len(chunk)
		fmt.Fprintf(dst, "%04x", size)
		dst.WriteByte(1)
		dst.Write(chunk)
	}
	dst.Write(pktFlush())
}

// readPktLines splits a git pkt-line body. A nil entry is a flush packet.
// A one-byte {1} entry is a protocol-v2 delimiter.
func readPktLines(body []byte) [][]byte {
	r := bufio.NewReader(bytes.NewReader(body))
	var lines [][]byte
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return lines
		}
		if bytes.Equal(hdr[:], []byte("0000")) {
			lines = append(lines, nil)
			continue
		}
		n, err := strconv.ParseInt(string(hdr[:]), 16, 32)
		if err != nil || n < 4 {
			if n == 1 {
				lines = append(lines, []byte{1})
			}
			continue
		}
		payload := make([]byte, int(n)-4)
		if _, err := io.ReadFull(r, payload); err != nil {
			return lines
		}
		lines = append(lines, payload)
	}
}
