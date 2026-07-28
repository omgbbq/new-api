package channel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestCaptureUpstreamRequestId_Priority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "cascade X-Oneapi-Request-Id takes highest priority",
			headers:  map[string]string{common.RequestIdKey: "oneapi-123", "x-request-id": "openai-456"},
			expected: "oneapi-123",
		},
		{
			name:     "OpenAI x-request-id",
			headers:  map[string]string{"x-request-id": "openai-456"},
			expected: "openai-456",
		},
		{
			name:     "Claude request-id",
			headers:  map[string]string{"request-id": "claude-789"},
			expected: "claude-789",
		},
		{
			name:     "Azure apim-request-id",
			headers:  map[string]string{"apim-request-id": "azure-abc"},
			expected: "azure-abc",
		},
		{
			name:     "Gemini x-goog-request-id",
			headers:  map[string]string{"x-goog-request-id": "gemini-def"},
			expected: "gemini-def",
		},
		{
			name:     "AWS x-amzn-requestid",
			headers:  map[string]string{"x-amzn-requestid": "aws-ghi"},
			expected: "aws-ghi",
		},
		{
			name:     "ByteDance x-tt-logid",
			headers:  map[string]string{"x-tt-logid": "doubao-jkl"},
			expected: "doubao-jkl",
		},
		{
			name:     "Cloudflare cf-ray",
			headers:  map[string]string{"cf-ray": "cf-mno"},
			expected: "cf-mno",
		},
		{
			name:     "no matching header returns empty",
			headers:  map[string]string{"x-custom-id": "custom-123"},
			expected: "",
		},
		{
			name:     "x-request-id wins over lower-priority headers",
			headers:  map[string]string{"x-request-id": "openai-456", "x-amzn-requestid": "aws-ghi", "cf-ray": "cf-mno"},
			expected: "openai-456",
		},
		{
			name:     "empty header value is skipped",
			headers:  map[string]string{"x-request-id": "", "request-id": "claude-789"},
			expected: "claude-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

			respHeader := http.Header{}
			for k, v := range tt.headers {
				respHeader.Set(k, v)
			}

			captureUpstreamRequestId(ctx, respHeader)

			got, _ := ctx.Get(common.UpstreamRequestIdKey)
			if tt.expected == "" {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestCaptureUpstreamRequestId_AllRelayPaths(t *testing.T) {
	t.Parallel()

	paths := []struct {
		name   string
		path   string
		header string
		value  string
	}{
		{
			name:   "chat completions - OpenAI",
			path:   "/v1/chat/completions",
			header: "x-request-id",
			value:  "chatcmpl-req-001",
		},
		{
			name:   "completions - OpenAI",
			path:   "/v1/completions",
			header: "x-request-id",
			value:  "cmpl-req-002",
		},
		{
			name:   "images generations - OpenAI",
			path:   "/v1/images/generations",
			header: "x-request-id",
			value:  "img-req-003",
		},
		{
			name:   "video generations - Doubao/Volcengine",
			path:   "/v1/video/generations",
			header: "x-tt-logid",
			value:  "video-logid-004",
		},
		{
			name:   "chat completions - Claude via AWS Bedrock HTTP",
			path:   "/v1/chat/completions",
			header: "x-amzn-requestid",
			value:  "aws-bedrock-005",
		},
		{
			name:   "chat completions - Azure",
			path:   "/v1/chat/completions",
			header: "apim-request-id",
			value:  "azure-006",
		},
		{
			name:   "chat completions - Gemini",
			path:   "/v1/chat/completions",
			header: "x-goog-request-id",
			value:  "gemini-007",
		},
		{
			name:   "chat completions - Claude/Anthropic",
			path:   "/v1/chat/completions",
			header: "request-id",
			value:  "claude-008",
		},
		{
			name:   "images generations - Cloudflare",
			path:   "/v1/images/generations",
			header: "cf-ray",
			value:  "cf-ray-009",
		},
		{
			name:   "chat completions - cascade new-api",
			path:   "/v1/chat/completions",
			header: common.RequestIdKey,
			value:  "cascade-010",
		},
	}

	for _, tt := range paths {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)

			respHeader := http.Header{}
			respHeader.Set(tt.header, tt.value)

			captureUpstreamRequestId(ctx, respHeader)

			got, exists := ctx.Get(common.UpstreamRequestIdKey)
			assert.True(t, exists, "upstream request id should be captured for %s", tt.name)
			assert.Equal(t, tt.value, got)
		})
	}
}
