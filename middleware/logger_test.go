package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetUpLogger_WithUpstreamRequestId(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logOutput string
	router := gin.New()
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		var upstreamRequestID string
		if param.Keys != nil {
			upstreamRequestID, _ = param.Keys[common.UpstreamRequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		if upstreamRequestID != "" {
			logOutput = "[GIN] | " + tag + " | " + requestID + " | " + upstreamRequestID + " |"
		} else {
			logOutput = "[GIN] | " + tag + " | " + requestID + " |"
		}
		return logOutput
	}))
	router.GET("/test", func(c *gin.Context) {
		c.Set(common.RequestIdKey, "req-local-001")
		c.Set(common.UpstreamRequestIdKey, "upstream-openai-xyz")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Contains(t, logOutput, "upstream-openai-xyz")
	require.Contains(t, logOutput, "req-local-001")
	assert.Equal(t, 4, strings.Count(logOutput, "|"))
}

func TestSetUpLogger_WithoutUpstreamRequestId(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logOutput string
	router := gin.New()
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		var upstreamRequestID string
		if param.Keys != nil {
			upstreamRequestID, _ = param.Keys[common.UpstreamRequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		if upstreamRequestID != "" {
			logOutput = "[GIN] | " + tag + " | " + requestID + " | " + upstreamRequestID + " |"
		} else {
			logOutput = "[GIN] | " + tag + " | " + requestID + " |"
		}
		return logOutput
	}))
	router.GET("/test", func(c *gin.Context) {
		c.Set(common.RequestIdKey, "req-local-002")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Contains(t, logOutput, "req-local-002")
	assert.NotContains(t, logOutput, "upstream")
	assert.Equal(t, 3, strings.Count(logOutput, "|"))
}

func TestSetUpLogger_RouteTagApplied(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logOutput string
	router := gin.New()
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var upstreamRequestID string
		if param.Keys != nil {
			upstreamRequestID, _ = param.Keys[common.UpstreamRequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		if upstreamRequestID != "" {
			logOutput = tag + " | " + upstreamRequestID
		} else {
			logOutput = tag
		}
		return logOutput
	}))
	router.GET("/api/test", RouteTag("api"), func(c *gin.Context) {
		c.Set(common.UpstreamRequestIdKey, "aws-req-123")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Contains(t, logOutput, "api")
	require.Contains(t, logOutput, "aws-req-123")
}
