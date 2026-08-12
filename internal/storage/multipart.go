package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMultipartNotFound    = errors.New("multipart upload not found")
	ErrMultipartPartMissing = errors.New("multipart upload part is missing")
	ErrMultipartChecksum    = errors.New("multipart upload checksum mismatch")
	ErrDirectUploadRequired = errors.New("multipart part must be uploaded directly to storage")
)

type MultipartCreateRequest struct {
	ObjectKey   string
	ContentType string
	SizeBytes   int64
	PartSize    int64
}

type MultipartUpload struct {
	UploadID    string
	ObjectKey   string
	ContentType string
	SizeBytes   int64
	PartSize    int64
	PartCount   int
	Driver      string
}

type CompletedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
	Checksum   string `json:"checksum,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
}

type MultipartPartTarget struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type MultipartStatus struct {
	Status         string          `json:"status"`
	CompletedParts []CompletedPart `json:"completed_parts"`
}

type CompletedMultipartObject struct {
	ObjectKey string
	SizeBytes int64
	SHA256    string
	ETag      string
}

// MultipartBackend is optional. Existing storage backends remain compatible
// when resumable uploads are not enabled for them.
type MultipartBackend interface {
	CreateMultipart(context.Context, MultipartCreateRequest) (MultipartUpload, error)
	SignMultipartPart(context.Context, MultipartUpload, int, string, time.Duration) (MultipartPartTarget, error)
	PutMultipartPart(context.Context, MultipartUpload, int, io.Reader, int64, string) (CompletedPart, error)
	MultipartStatus(context.Context, MultipartUpload) (MultipartStatus, error)
	CompleteMultipart(context.Context, MultipartUpload, []CompletedPart) (CompletedMultipartObject, error)
	AbortMultipart(context.Context, MultipartUpload) error
}

var (
	_ MultipartBackend = (*LocalBackend)(nil)
	_ MultipartBackend = (*S3Backend)(nil)
)

func validateMultipartCreate(req MultipartCreateRequest) (MultipartUpload, error) {
	if strings.TrimSpace(req.ObjectKey) == "" || req.SizeBytes <= 0 || req.PartSize <= 0 {
		return MultipartUpload{}, errors.New("multipart object key, size and part size are required")
	}
	partCount64 := (req.SizeBytes + req.PartSize - 1) / req.PartSize
	if partCount64 <= 0 || partCount64 > 10_000 {
		return MultipartUpload{}, errors.New("multipart part count must be between 1 and 10000")
	}
	return MultipartUpload{
		ObjectKey: strings.TrimSpace(req.ObjectKey), ContentType: strings.TrimSpace(req.ContentType),
		SizeBytes: req.SizeBytes, PartSize: req.PartSize, PartCount: int(partCount64),
	}, nil
}

func expectedMultipartPartSize(upload MultipartUpload, partNumber int) (int64, error) {
	if partNumber < 1 || partNumber > upload.PartCount || upload.PartSize <= 0 || upload.SizeBytes <= 0 {
		return 0, errors.New("invalid multipart part number")
	}
	if partNumber < upload.PartCount {
		return upload.PartSize, nil
	}
	return upload.SizeBytes - int64(upload.PartCount-1)*upload.PartSize, nil
}

func normalizeSHA256Checksum(value string) (hexValue, base64Value string, err error) {
	value = strings.TrimSpace(value)
	if decoded, decodeErr := hex.DecodeString(value); decodeErr == nil && len(decoded) == sha256.Size {
		return strings.ToLower(value), base64.StdEncoding.EncodeToString(decoded), nil
	}
	decoded, decodeErr := base64.StdEncoding.Strict().DecodeString(value)
	if decodeErr != nil || len(decoded) != sha256.Size {
		return "", "", errors.New("sha256 checksum must be 64 hex characters or base64 encoded 32 bytes")
	}
	return hex.EncodeToString(decoded), value, nil
}

func normalizeCompletedParts(parts []CompletedPart, expectedCount int) ([]CompletedPart, error) {
	if len(parts) != expectedCount {
		return nil, ErrMultipartPartMissing
	}
	result := append([]CompletedPart(nil), parts...)
	sort.Slice(result, func(i, j int) bool { return result[i].PartNumber < result[j].PartNumber })
	for index, part := range result {
		if part.PartNumber != index+1 || strings.TrimSpace(part.ETag) == "" {
			return nil, ErrMultipartPartMissing
		}
		result[index].ETag = strings.Trim(strings.TrimSpace(part.ETag), `"`)
	}
	return result, nil
}

func (b *S3Backend) CreateMultipart(ctx context.Context, req MultipartCreateRequest) (MultipartUpload, error) {
	upload, err := validateMultipartCreate(req)
	if err != nil {
		return MultipartUpload{}, err
	}
	request, err := b.newSignedMultipartRequest(ctx, http.MethodPost, upload.ObjectKey, url.Values{"uploads": {""}}, upload.ContentType, nil)
	if err != nil {
		return MultipartUpload{}, err
	}
	response, err := b.client.Do(request)
	if err != nil {
		return MultipartUpload{}, fmt.Errorf("create s3 multipart upload: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return MultipartUpload{}, fmt.Errorf("create s3 multipart upload: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload); err != nil {
		return MultipartUpload{}, fmt.Errorf("decode s3 multipart upload: %w", err)
	}
	if strings.TrimSpace(payload.UploadID) == "" {
		return MultipartUpload{}, errors.New("s3 multipart response did not include upload id")
	}
	upload.UploadID = strings.TrimSpace(payload.UploadID)
	upload.Driver = "s3"
	return upload, nil
}

func (b *S3Backend) SignMultipartPart(ctx context.Context, upload MultipartUpload, partNumber int, checksum string, expiry time.Duration) (MultipartPartTarget, error) {
	if err := contextError(ctx); err != nil {
		return MultipartPartTarget{}, err
	}
	if _, err := expectedMultipartPartSize(upload, partNumber); err != nil {
		return MultipartPartTarget{}, err
	}
	_, checksumBase64, err := normalizeSHA256Checksum(checksum)
	if err != nil {
		return MultipartPartTarget{}, err
	}
	key := b.normalizeKey(upload.ObjectKey)
	if key == "" || strings.TrimSpace(upload.UploadID) == "" {
		return MultipartPartTarget{}, errors.New("invalid s3 multipart upload")
	}
	requestURL, host, canonicalURI, err := b.requestTarget(key)
	if err != nil {
		return MultipartPartTarget{}, err
	}
	target, err := url.Parse(requestURL)
	if err != nil {
		return MultipartPartTarget{}, err
	}
	if expiry <= 0 {
		expiry = 5 * time.Minute
	}
	if expiry > 15*time.Minute {
		expiry = 15 * time.Minute
	}
	now := b.nowUTC()
	amzDate, dateStamp := now.Format("20060102T150405Z"), now.Format("20060102")
	credentialScope := dateStamp + "/" + b.region + "/s3/aws4_request"
	query := url.Values{
		"partNumber":          {strconv.Itoa(partNumber)},
		"uploadId":            {upload.UploadID},
		"X-Amz-Algorithm":     {"AWS4-HMAC-SHA256"},
		"X-Amz-Credential":    {b.accessKeyID + "/" + credentialScope},
		"X-Amz-Date":          {amzDate},
		"X-Amz-Expires":       {strconv.FormatInt(int64(expiry/time.Second), 10)},
		"X-Amz-SignedHeaders": {"host;x-amz-checksum-sha256"},
	}
	canonicalHeaders := "host:" + host + "\n" + "x-amz-checksum-sha256:" + checksumBase64 + "\n"
	canonicalRequest := strings.Join([]string{http.MethodPut, canonicalURI, awsCanonicalQuery(query), canonicalHeaders, "host;x-amz-checksum-sha256", "UNSIGNED-PAYLOAD"}, "\n")
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, credentialScope, sha256Hex([]byte(canonicalRequest))}, "\n")
	query.Set("X-Amz-Signature", hex.EncodeToString(hmacSHA256(b.signingKey(dateStamp), stringToSign)))
	target.RawQuery = awsCanonicalQuery(query)
	return MultipartPartTarget{
		URL: target.String(), Headers: map[string]string{"x-amz-checksum-sha256": checksumBase64}, ExpiresAt: now.Add(expiry),
	}, nil
}

func (*S3Backend) PutMultipartPart(context.Context, MultipartUpload, int, io.Reader, int64, string) (CompletedPart, error) {
	return CompletedPart{}, ErrDirectUploadRequired
}

func (b *S3Backend) MultipartStatus(ctx context.Context, upload MultipartUpload) (MultipartStatus, error) {
	request, err := b.newSignedMultipartRequest(ctx, http.MethodGet, upload.ObjectKey, url.Values{"uploadId": {upload.UploadID}}, "", nil)
	if err != nil {
		return MultipartStatus{}, err
	}
	response, err := b.client.Do(request)
	if err != nil {
		return MultipartStatus{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return MultipartStatus{}, ErrMultipartNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return MultipartStatus{}, fmt.Errorf("list s3 multipart parts: status=%d", response.StatusCode)
	}
	var payload struct {
		Parts []struct {
			PartNumber int    `xml:"PartNumber"`
			ETag       string `xml:"ETag"`
			Size       int64  `xml:"Size"`
			Checksum   string `xml:"ChecksumSHA256"`
		} `xml:"Part"`
	}
	if err := xml.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return MultipartStatus{}, err
	}
	status := MultipartStatus{Status: "uploading", CompletedParts: make([]CompletedPart, 0, len(payload.Parts))}
	for _, part := range payload.Parts {
		status.CompletedParts = append(status.CompletedParts, CompletedPart{PartNumber: part.PartNumber, ETag: strings.Trim(part.ETag, `"`), SizeBytes: part.Size, Checksum: part.Checksum})
	}
	return status, nil
}

func (b *S3Backend) CompleteMultipart(ctx context.Context, upload MultipartUpload, parts []CompletedPart) (CompletedMultipartObject, error) {
	parts, err := normalizeCompletedParts(parts, upload.PartCount)
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	type completePart struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
	}
	payload := struct {
		XMLName xml.Name       `xml:"CompleteMultipartUpload"`
		Parts   []completePart `xml:"Part"`
	}{Parts: make([]completePart, 0, len(parts))}
	for _, part := range parts {
		payload.Parts = append(payload.Parts, completePart{PartNumber: part.PartNumber, ETag: `"` + part.ETag + `"`})
	}
	body, err := xml.Marshal(payload)
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	request, err := b.newSignedMultipartRequest(ctx, http.MethodPost, upload.ObjectKey, url.Values{"uploadId": {upload.UploadID}}, "application/xml", body)
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	response, err := b.client.Do(request)
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 256<<10))
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return CompletedMultipartObject{}, fmt.Errorf("complete s3 multipart upload: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var completed struct {
		ETag string `xml:"ETag"`
	}
	if err := xml.Unmarshal(responseBody, &completed); err != nil {
		return CompletedMultipartObject{}, err
	}
	return CompletedMultipartObject{ObjectKey: upload.ObjectKey, SizeBytes: upload.SizeBytes, ETag: strings.Trim(strings.TrimSpace(completed.ETag), `"`)}, nil
}

func (b *S3Backend) AbortMultipart(ctx context.Context, upload MultipartUpload) error {
	request, err := b.newSignedMultipartRequest(ctx, http.MethodDelete, upload.ObjectKey, url.Values{"uploadId": {upload.UploadID}}, "", nil)
	if err != nil {
		return err
	}
	response, err := b.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusNoContent || (response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices) {
		return nil
	}
	return fmt.Errorf("abort s3 multipart upload: status=%d", response.StatusCode)
}

func (b *S3Backend) newSignedMultipartRequest(ctx context.Context, method, objectKey string, query url.Values, contentType string, body []byte) (*http.Request, error) {
	key := b.normalizeKey(objectKey)
	if key == "" {
		return nil, fmt.Errorf("invalid s3 object key %q", objectKey)
	}
	requestURL, host, canonicalURI, err := b.requestTarget(key)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(requestURL)
	if err != nil {
		return nil, err
	}
	target.RawQuery = awsCanonicalQuery(query)
	payloadHash := sha256Hex(body)
	now := b.nowUTC()
	amzDate, dateStamp := now.Format("20060102T150405Z"), now.Format("20060102")
	request, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Host = host
	request.ContentLength = int64(len(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	request.Header.Set("Host", host)
	request.Header.Set("X-Amz-Content-Sha256", payloadHash)
	request.Header.Set("X-Amz-Date", amzDate)
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{method, canonicalURI, awsCanonicalQuery(query), canonicalHeaders, signedHeaders, payloadHash}, "\n")
	credentialScope := dateStamp + "/" + b.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, credentialScope, sha256Hex([]byte(canonicalRequest))}, "\n")
	signature := hex.EncodeToString(hmacSHA256(b.signingKey(dateStamp), stringToSign))
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+b.accessKeyID+"/"+credentialScope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return request, nil
}
