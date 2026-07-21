package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetResponseID_SetsContextOnFirstCall(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(common.RequestIdKey, "abc-123")

	id := GetResponseID(c)

	require.Equal(t, "chatcmpl-abc-123", id)
	assert.Equal(t, "chatcmpl-abc-123", c.GetString(common.ResponseIdKey))
}

func TestGetResponseID_DoesNotOverwriteExistingValue(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(common.RequestIdKey, "abc-123")
	c.Set(common.ResponseIdKey, "chatcmpl-real-upstream-id")

	id := GetResponseID(c)

	assert.Equal(t, "chatcmpl-abc-123", id)
	assert.Equal(t, "chatcmpl-real-upstream-id", c.GetString(common.ResponseIdKey),
		"should not overwrite pre-existing response ID from upstream")
}

func TestGetResponseID_EmptyRequestId(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	id := GetResponseID(c)

	assert.Equal(t, "chatcmpl-", id)
	assert.Equal(t, "chatcmpl-", c.GetString(common.ResponseIdKey))
}
