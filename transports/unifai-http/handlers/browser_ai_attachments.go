package handlers

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/unifai/unifai/framework/logstore"
)

const browserAIAttachmentMaxBytes = 20 << 20 // 20 MiB

// browserAIAttachmentTTL — temp View/Download window; log + filename stay forever.
const browserAIAttachmentTTL = 10 * time.Minute

var unsafeAttachmentNameChars = regexp.MustCompile(`[^a-zA-Z0-9._\- ]+`)

var (
	browserAIAttachCleanOnce sync.Once
	browserAIAttachCleanStop chan struct{}
)

// browserAIAttachmentDir stores intercepted uploads under APP_DIR/attachments (or ./data/attachments).
// Legacy APP_DIR/pdf is still resolved for older logs.
func browserAIAttachmentDir() string {
	base := strings.TrimSpace(os.Getenv("APP_DIR"))
	if base == "" {
		base = "data"
	}
	return filepath.Join(base, "attachments")
}

func browserAIPdfDir() string {
	base := strings.TrimSpace(os.Getenv("APP_DIR"))
	if base == "" {
		base = "data"
	}
	return filepath.Join(base, "pdf")
}

func ensureBrowserAIAttachmentDir() (string, error) {
	dir := browserAIAttachmentDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create attachments dir: %w", err)
	}
	return dir, nil
}

// ensureBrowserAIPdfDir kept for older call sites / entrypoints that mkdir pdf/.
func ensureBrowserAIPdfDir() (string, error) {
	dir := browserAIPdfDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create pdf dir: %w", err)
	}
	return dir, nil
}

func sanitizeAttachmentFileName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "..", "")
	name = unsafeAttachmentNameChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, " ._")
	if name == "" {
		name = "attachment"
	}
	runes := []rune(name)
	if len(runes) > 120 {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(string(runes), ext)
		if len([]rune(base)) > 100 {
			base = string([]rune(base)[:100])
		}
		name = base + ext
	}
	return name
}

func extractPDFBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	if len(data) >= 5 && string(data[:5]) == "%PDF-" {
		return trimPDFEOF(data)
	}
	idx := indexPDFMagic(data)
	if idx < 0 {
		return nil
	}
	return trimPDFEOF(data[idx:])
}

func trimPDFEOF(pdf []byte) []byte {
	if eof := lastIndexBytes(pdf, []byte("%%EOF")); eof >= 0 {
		end := eof + 5
		for end < len(pdf) && (pdf[end] == '\n' || pdf[end] == '\r') {
			end++
		}
		return pdf[:end]
	}
	return pdf
}

func indexPDFMagic(data []byte) int {
	sig := []byte("%PDF-")
	limit := len(data) - len(sig) + 1
	if limit > 64*1024 {
		limit = 64 * 1024
	}
	for i := 0; i < limit; i++ {
		match := true
		for j := 0; j < len(sig); j++ {
			if data[i+j] != sig[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func lastIndexBytes(haystack, needle []byte) int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return -1
	}
	for i := len(haystack) - len(needle); i >= 0; i-- {
		ok := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func sniffAttachmentContentType(data []byte, name, hint string) string {
	if looksLikeJSONOnly(data) {
		return "application/json"
	}
	if len(data) >= 5 && string(data[:5]) == "%PDF-" {
		return "application/pdf"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg"
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	hint = strings.TrimSpace(strings.Split(hint, ";")[0])
	if hint != "" && !strings.EqualFold(hint, "application/octet-stream") && !strings.Contains(strings.ToLower(hint), "json") {
		return strings.ToLower(hint)
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".txt", ".log", ".md":
		return "text/plain"
	case ".csv":
		return "text/csv"
	case ".json":
		return "application/json"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".doc":
		return "application/msword"
	case ".zip":
		return "application/zip"
	}
	if utf8.Valid(data) && looksMostlyText(data) {
		return "text/plain"
	}
	return "application/octet-stream"
}

func looksMostlyText(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	limit := len(data)
	if limit > 4096 {
		limit = 4096
	}
	nonPrint := 0
	for i := 0; i < limit; i++ {
		b := data[i]
		if b == 0 {
			return false
		}
		if b < 9 || (b > 13 && b < 32) {
			nonPrint++
		}
	}
	return nonPrint*10 < limit
}

func ensureAttachmentExt(name, ctype string) string {
	name = sanitizeAttachmentFileName(name)
	if filepath.Ext(name) != "" {
		return name
	}
	ctype = strings.ToLower(ctype)
	switch {
	case strings.Contains(ctype, "pdf"):
		return name + ".pdf"
	case strings.Contains(ctype, "png"):
		return name + ".png"
	case strings.Contains(ctype, "jpeg"), strings.Contains(ctype, "jpg"):
		return name + ".jpg"
	case strings.Contains(ctype, "gif"):
		return name + ".gif"
	case strings.Contains(ctype, "webp"):
		return name + ".webp"
	case strings.Contains(ctype, "text/plain"):
		return name + ".txt"
	case strings.Contains(ctype, "csv"):
		return name + ".csv"
	default:
		return name
	}
}

// extractAttachmentPayload returns the best file bytes from a raw upload body (direct or multipart).
func extractAttachmentPayload(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	if pdf := extractPDFBytes(data); len(pdf) > 0 {
		return pdf
	}
	// Image / zip / office at start of body
	if looksLikeBinaryFile(data) {
		return data
	}
	// Multipart: prefer part with filename=
	if part := extractMultipartFilePart(data); len(part) > 0 {
		return part
	}
	// Large non-empty body (resumable upload chunk)
	if len(data) >= 64 && !looksLikeJSONOnly(data) {
		return data
	}
	return nil
}

func looksLikeBinaryFile(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	if len(data) >= 5 && string(data[:5]) == "%PDF-" {
		return true
	}
	if data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return true
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return true
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return true
	}
	if len(data) >= 4 && string(data[:2]) == "PK" { // zip/docx/xlsx
		return true
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return true
	}
	return false
}

func looksLikeJSONOnly(data []byte) bool {
	i := 0
	for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\r' || data[i] == '\n') {
		i++
	}
	if i >= len(data) {
		return false
	}
	return data[i] == '{' || data[i] == '['
}

func extractMultipartFilePart(data []byte) []byte {
	lower := strings.ToLower(string(data[:min(len(data), 64*1024)]))
	idx := strings.Index(lower, "filename=")
	if idx < 0 {
		idx = strings.Index(lower, "filename*=")
	}
	if idx < 0 {
		return nil
	}
	// Find header/body separator after this disposition
	rest := data[idx:]
	sep := []byte("\r\n\r\n")
	si := indexBytes(rest, sep)
	if si < 0 {
		sep = []byte("\n\n")
		si = indexBytes(rest, sep)
	}
	if si < 0 {
		return nil
	}
	body := rest[si+len(sep):]
	// End at next boundary line starting with --
	end := len(body)
	for i := 0; i+2 < len(body); i++ {
		if body[i] == '\n' && body[i+1] == '-' && body[i+2] == '-' {
			end = i
			if i > 0 && body[i-1] == '\r' {
				end = i - 1
			}
			break
		}
	}
	part := body[:end]
	if len(part) == 0 {
		return nil
	}
	if pdf := extractPDFBytes(part); len(pdf) > 0 {
		return pdf
	}
	return part
}

func indexBytes(haystack, needle []byte) int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return -1
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		ok := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// storeBrowserAIAttachment stores any intercepted upload (PDF, image, office, etc.).
func storeBrowserAIAttachment(logID, originalName string, data []byte, contentTypeHint string) (storedName string, contentType string, err error) {
	if len(data) == 0 {
		return "", "", fmt.Errorf("empty file")
	}
	if len(data) > browserAIAttachmentMaxBytes {
		return "", "", fmt.Errorf("file too large (max %d bytes)", browserAIAttachmentMaxBytes)
	}
	payload := extractAttachmentPayload(data)
	if len(payload) == 0 {
		if looksLikeJSONOnly(data) {
			return "", "", fmt.Errorf("json metadata is not a file")
		}
		payload = data
	} else if looksLikeJSONOnly(payload) {
		return "", "", fmt.Errorf("json metadata is not a file")
	}
	if len(payload) > browserAIAttachmentMaxBytes {
		return "", "", fmt.Errorf("file too large (max %d bytes)", browserAIAttachmentMaxBytes)
	}
	ctype := sniffAttachmentContentType(payload, originalName, contentTypeHint)
	safe := ensureAttachmentExt(originalName, ctype)
	dir, err := ensureBrowserAIAttachmentDir()
	if err != nil {
		return "", "", err
	}
	id := strings.TrimSpace(logID)
	if id == "" {
		id = uuid.New().String()
	}
	id = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			return r
		}
		return '-'
	}, id)
	storedName = id + "_" + safe
	path := filepath.Join(dir, storedName)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", "", err
	}
	return storedName, ctype, nil
}

// storeBrowserAIPDF keeps older call sites working — stores via the generic attachment path.
func storeBrowserAIPDF(logID, originalName string, data []byte) (storedName string, contentType string, err error) {
	return storeBrowserAIAttachment(logID, originalName, data, "application/pdf")
}

func resolveBrowserAIAttachmentPath(storedName string) (string, error) {
	storedName = filepath.Base(strings.TrimSpace(storedName))
	if storedName == "" || storedName == "." || storedName == ".." {
		return "", fmt.Errorf("invalid attachment")
	}
	for _, dirFn := range []func() string{browserAIAttachmentDir, browserAIPdfDir} {
		dir := dirFn()
		full := filepath.Join(dir, storedName)
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		absFile, err := filepath.Abs(full)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absDir, absFile)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if _, err := os.Stat(absFile); err == nil {
			return absFile, nil
		}
	}
	return "", fmt.Errorf("attachment not found")
}

func browserAIAttachmentExpired(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > browserAIAttachmentTTL
}

// startBrowserAIAttachmentCleanup deletes temp upload files older than TTL.
// Prompt log rows and attachment_name stay permanent; only disk bytes are removed.
func startBrowserAIAttachmentCleanup(manager *logstore.BrowserAIManager) {
	browserAIAttachCleanOnce.Do(func() {
		browserAIAttachCleanStop = make(chan struct{})
		go func() {
			// Initial sweep shortly after boot
			purgeExpiredBrowserAIAttachments(manager)
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					purgeExpiredBrowserAIAttachments(manager)
				case <-browserAIAttachCleanStop:
					return
				}
			}
		}()
	})
}

func purgeExpiredBrowserAIAttachments(manager *logstore.BrowserAIManager) {
	cutoff := time.Now().Add(-browserAIAttachmentTTL)
	for _, dirFn := range []func() string{browserAIAttachmentDir, browserAIPdfDir} {
		dir := dirFn()
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			name := ent.Name()
			info, err := ent.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(cutoff) {
				continue
			}
			path := filepath.Join(dir, name)
			_ = os.Remove(path)
			if manager != nil {
				_ = manager.ClearLogAttachmentFileByStoredName(context.Background(), name)
			}
		}
	}
}

// readMultipartUploadFile reads the optional "file" form part (temp View storage).
func readMultipartUploadFile(formFiles map[string][]*multipart.FileHeader) (filename string, data []byte, err error) {
	if formFiles == nil {
		return "", nil, nil
	}
	headers := formFiles["file"]
	if len(headers) == 0 {
		// Some clients may use "upload" / "attachment"
		for _, key := range []string{"upload", "attachment", "file_data"} {
			if len(formFiles[key]) > 0 {
				headers = formFiles[key]
				break
			}
		}
	}
	if len(headers) == 0 {
		return "", nil, nil
	}
	fh := headers[0]
	if fh == nil {
		return "", nil, nil
	}
	if fh.Size > browserAIAttachmentMaxBytes {
		return "", nil, fmt.Errorf("file too large")
	}
	f, err := fh.Open()
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	limited := io.LimitReader(f, browserAIAttachmentMaxBytes+1)
	data, err = io.ReadAll(limited)
	if err != nil {
		return "", nil, err
	}
	if len(data) > browserAIAttachmentMaxBytes {
		return "", nil, fmt.Errorf("file too large")
	}
	name := strings.TrimSpace(fh.Filename)
	return name, data, nil
}
