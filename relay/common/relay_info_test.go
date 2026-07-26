package common

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestInitChannelMeta_IncludesChannelName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(string(constant.ContextKeyChannelId), 42)
	c.Set(string(constant.ContextKeyChannelName), "test-channel")
	c.Set(string(constant.ContextKeyChannelBaseUrl), "https://api.example.com")
	c.Set(string(constant.ContextKeyChannelType), 1)

	info := &RelayInfo{}
	info.InitChannelMeta(c)

	require.NotNil(t, info.ChannelMeta)
	require.Equal(t, "test-channel", info.ChannelMeta.ChannelName)
	require.Equal(t, 42, info.ChannelMeta.ChannelId)
	require.Equal(t, "https://api.example.com", info.ChannelMeta.ChannelBaseUrl)
}

func TestInitChannelMeta_EmptyChannelName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(string(constant.ContextKeyChannelId), 1)
	c.Set(string(constant.ContextKeyChannelType), 1)

	info := &RelayInfo{}
	info.InitChannelMeta(c)

	require.NotNil(t, info.ChannelMeta)
	require.Equal(t, "", info.ChannelMeta.ChannelName)
}

func TestRelayInfoToString_IncludesChannelName(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelType:    1,
			ChannelId:      10,
			ChannelName:    "my-channel",
			ChannelBaseUrl: "https://api.example.com",
		},
	}

	s := info.ToString()
	require.True(t, strings.Contains(s, `Name: "my-channel"`), "ToString should include channel name, got: %s", s)
}
