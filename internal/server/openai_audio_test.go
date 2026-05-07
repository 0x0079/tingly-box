package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// buildMultipartRequest builds a POST request body with a "file" part and the
// given form fields. fileBytes==nil means omit the file part entirely.
func buildMultipartRequest(t *testing.T, fileBytes []byte, filename string, formFields map[string]string, repeatedFields map[string][]string) *http.Request {
	t.Helper()

	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)

	if fileBytes != nil {
		fw, err := w.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write(fileBytes); err != nil {
			t.Fatalf("write file part: %v", err)
		}
	}

	for k, v := range formFields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("WriteField %s: %v", k, err)
		}
	}

	for k, vs := range repeatedFields {
		for _, v := range vs {
			if err := w.WriteField(k, v); err != nil {
				t.Fatalf("WriteField %s: %v", k, err)
			}
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/test", buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// newTestContext wraps an http.Request in a gin.Context for handler-helper tests.
func newTestContext(req *http.Request) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c
}

func TestParseAudioForm_RejectsMissingFile(t *testing.T) {
	req := buildMultipartRequest(t, nil, "", map[string]string{"model": "whisper-1"}, nil)
	c := newTestContext(req)

	if _, err := parseAudioForm(c); err == nil {
		t.Fatalf("expected error when file is missing, got nil")
	}
}

func TestParseAudioForm_RejectsMissingModel(t *testing.T) {
	req := buildMultipartRequest(t, []byte("fake audio"), "sample.m4a", nil, nil)
	c := newTestContext(req)

	_, err := parseAudioForm(c)
	if err == nil {
		t.Fatalf("expected error when model is missing, got nil")
	}
	if want := "model is required"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseAudioForm_RejectsEmptyFile(t *testing.T) {
	req := buildMultipartRequest(t, []byte{}, "empty.m4a", map[string]string{"model": "whisper-1"}, nil)
	c := newTestContext(req)

	if _, err := parseAudioForm(c); err == nil {
		t.Fatalf("expected error when file is empty, got nil")
	}
}

func TestParseAudioForm_RejectsInvalidTemperature(t *testing.T) {
	req := buildMultipartRequest(t, []byte("fake audio"), "sample.m4a", map[string]string{
		"model":       "whisper-1",
		"temperature": "not-a-number",
	}, nil)
	c := newTestContext(req)

	if _, err := parseAudioForm(c); err == nil {
		t.Fatalf("expected error for invalid temperature, got nil")
	}
}

func TestParseAudioForm_AcceptsAllFields(t *testing.T) {
	req := buildMultipartRequest(t, []byte("fake audio bytes"), "sample.m4a", map[string]string{
		"model":           "whisper-1",
		"language":        "en",
		"prompt":          "hello",
		"response_format": "verbose_json",
		"temperature":     "0.4",
	}, map[string][]string{
		"timestamp_granularities[]": {"word", "segment"},
	})
	c := newTestContext(req)

	fields, err := parseAudioForm(c)
	if err != nil {
		t.Fatalf("parseAudioForm: %v", err)
	}
	defer fields.file.Close()

	if fields.model != "whisper-1" {
		t.Errorf("model = %q, want whisper-1", fields.model)
	}
	if fields.language != "en" {
		t.Errorf("language = %q, want en", fields.language)
	}
	if fields.prompt != "hello" {
		t.Errorf("prompt = %q, want hello", fields.prompt)
	}
	if fields.responseFormat != "verbose_json" {
		t.Errorf("responseFormat = %q, want verbose_json", fields.responseFormat)
	}
	if fields.temperature == nil || *fields.temperature != 0.4 {
		t.Errorf("temperature = %v, want 0.4", fields.temperature)
	}
	if got := fields.timestampGranularities; len(got) != 2 || got[0] != "word" || got[1] != "segment" {
		t.Errorf("timestampGranularities = %v, want [word segment]", got)
	}
	if fields.header == nil || fields.header.Filename != "sample.m4a" {
		t.Errorf("header filename = %v, want sample.m4a", fields.header)
	}
}
