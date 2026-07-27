package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEstimateTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
}

func doEstimateRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/pricing/estimate", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	EstimateQuota(ctx)
	return recorder
}

func TestEstimateQuota_RatioMode(t *testing.T) {
	setupEstimateTest(t)

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-ratio-model": 5}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"test-ratio-model": 2}`))

	recorder := doEstimateRequest(t, `{
		"model": "test-ratio-model",
		"prompt_tokens": 1000,
		"completion_tokens": 500
	}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "ratio", data["billing_mode"])
	assert.Equal(t, float64(5), data["model_ratio"])
	assert.Equal(t, float64(2), data["completion_ratio"])
	// quota = (1000 + 500*2) * 5 = 2000 * 5 = 10000
	assert.Equal(t, float64(10000), data["quota_before_group"])
	assert.Equal(t, float64(10000), data["quota"].(float64))
}

func TestEstimateQuota_PriceMode(t *testing.T) {
	setupEstimateTest(t)

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"test-price-model": 0.04}`))

	recorder := doEstimateRequest(t, `{
		"model": "test-price-model",
		"request_body": "{\"n\": 3}"
	}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "ratio", data["billing_mode"])
	assert.Equal(t, float64(0.04), data["model_price"])
	// quota = 0.04 * 3 * 500000 = 60000
	assert.Equal(t, float64(60000), data["quota_before_group"])
	assert.Equal(t, float64(60000), data["quota"].(float64))
}

func TestEstimateQuota_PriceMode_DefaultN(t *testing.T) {
	setupEstimateTest(t)

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"test-price-model-n": 0.02}`))

	recorder := doEstimateRequest(t, `{
		"model": "test-price-model-n"
	}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))

	data := resp["data"].(map[string]interface{})
	// n defaults to 1: quota = 0.02 * 1 * 500000 = 10000
	assert.Equal(t, float64(10000), data["quota_before_group"])
}

func TestEstimateQuota_TieredExpr(t *testing.T) {
	setupEstimateTest(t)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"test-tiered-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"test-tiered-model":"tier(\"base\", p * 3 + c * 15)"}`,
	}))

	recorder := doEstimateRequest(t, `{
		"model": "test-tiered-model",
		"prompt_tokens": 1000,
		"completion_tokens": 500
	}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "tiered_expr", data["billing_mode"])
	assert.Equal(t, "base", data["matched_tier"])
	// rawCost = 1000*3 + 500*15 = 3000 + 7500 = 10500
	// quotaBeforeGroup = 10500 / 1_000_000 * 500_000 = 5250
	assert.Equal(t, float64(5250), data["quota_before_group"].(float64))
	assert.Equal(t, float64(5250), data["quota"].(float64))
}

func TestEstimateQuota_TieredExprWithRequestBody(t *testing.T) {
	setupEstimateTest(t)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"test-tiered-param":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"test-tiered-param":"tier(\"image\", param(\"n\") * 0.25 * 1000000)"}`,
	}))

	reqBody := `{"n": 4}`
	recorder := doEstimateRequest(t, `{
		"model": "test-tiered-param",
		"request_body": "{\"n\": 4}"
	}`)

	_ = reqBody
	assert.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "image", data["matched_tier"])
	// rawCost = 4 * 0.25 * 1000000 = 1000000
	// quotaBeforeGroup = 1000000 / 1_000_000 * 500_000 = 500000
	assert.Equal(t, float64(500000), data["quota_before_group"].(float64))
	assert.Equal(t, float64(500000), data["quota"].(float64))
}

func TestEstimateQuota_GroupRatio(t *testing.T) {
	setupEstimateTest(t)

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-group-model": 10}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"test-group-model": 1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip": 0.5}`))

	recorder := doEstimateRequest(t, `{
		"model": "test-group-model",
		"prompt_tokens": 1000,
		"completion_tokens": 1000,
		"group": "vip"
	}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "vip", data["group"])
	assert.Equal(t, float64(0.5), data["group_ratio"])
	// quotaBeforeGroup = (1000 + 1000*1) * 10 = 20000
	// quota = 20000 * 0.5 = 10000
	assert.Equal(t, float64(20000), data["quota_before_group"].(float64))
	assert.Equal(t, float64(10000), data["quota"].(float64))
}

func TestEstimateQuota_InvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing model", func(t *testing.T) {
		recorder := doEstimateRequest(t, `{"prompt_tokens": 100}`)
		assert.Equal(t, http.StatusOK, recorder.Code)
		var resp map[string]interface{}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
		assert.False(t, resp["success"].(bool))
	})

	t.Run("negative tokens", func(t *testing.T) {
		recorder := doEstimateRequest(t, `{"model": "gpt-4", "prompt_tokens": -1}`)
		assert.Equal(t, http.StatusOK, recorder.Code)
		var resp map[string]interface{}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
		assert.False(t, resp["success"].(bool))
	})

	t.Run("invalid json", func(t *testing.T) {
		recorder := doEstimateRequest(t, `{invalid`)
		assert.Equal(t, http.StatusOK, recorder.Code)
		var resp map[string]interface{}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
		assert.False(t, resp["success"].(bool))
	})
}

func TestEstimateQuota_RealWorldExpressions(t *testing.T) {
	setupEstimateTest(t)

	billingModeJSON := `{
		"gpt-image-2":"tiered_expr",
		"gemini-3-pro-image-preview":"tiered_expr",
		"glm-4.6v":"tiered_expr",
		"gemini-3.1-pro-preview":"tiered_expr",
		"gemini-3.1-flash-image-preview":"tiered_expr",
		"doubao-seedance-2-0-260128":"tiered_expr",
		"doubao-seedance-2-0-fast-260128":"tiered_expr",
		"doubao-seedance-2-0-mini-260615":"tiered_expr",
		"doubao-seedream-4-5-251128":"tiered_expr",
		"doubao-seedream-5-0-260128":"tiered_expr",
		"doubao-seedream-5-0-pro-260628":"tiered_expr"
	}`

	billingExprJSON := `{
		"gpt-image-2":"channel(\"name\") == \"GPTProto\" ? tier(\"默认\", p * 53.76 + c * 201.6) : (channel(\"name\") == \"万界方舟\" ? tier(\"专线\", p * 53.76 + c * 256.68 + cr * 17.12) : (channel(\"name\") == \"UCloud\" ? tier(\"官转\", p * 42.72 + c * 85.44 + img * 68.35 + img_o * 256.32) :tier(\"默认\", p * 53.76 + c * 201.6)))",
		"gemini-3-pro-image-preview":"channel(\"name\") == \"GPTProto\" ? tier(\"默认\", 680000) : (channel(\"name\") == \"万界方舟\" ? tier(\"专线\", p * 14.55 + c * 872.71) : (channel(\"name\") == \"UCloud\" ? tier(\"官转\", p * 17.04 + c * 102.53 + ao * 102.53 + img_o * 1023.6) :tier(\"默认\", 680000)))",
		"glm-4.6v":"p <= 32000 ? tier(\"short_input\", p * 1 + c * 3 + cr * 0.2) : tier(\"long_input\", p * 2 + c * 6 + cr * 0.4)",
		"gemini-3.1-pro-preview":"p <= 200000 ? tier(\"short_context\", p * 14.24 + c * 85.44 + cr * 1.424) : tier(\"long_context\", p * 28.48 + c * 128.16 + cr * 2.848)",
		"gemini-3.1-flash-image-preview":"channel(\"name\") == \"wjfz\" ? tier(\"token\", p * 3.03 + c * 363.63) : (channel(\"name\") == \"ukd\" ? tier(\"combo\", p * 3.56 + c * 21.36 + ao * 21.36 + img_o * 427.2) :tier(\"token\", p * 3.03 + c * 363.63))",
		"doubao-seedance-2-0-260128":"param(\"metadata.content.#(type==\\\"video_url\\\")\") != nil ? \n(param(\"resolution\")=='480p'||param(\"resolution\")=='720p'? tier(\"有参考视频480P/720P\", c*28):(param(\"resolution\")=='1080p'?tier(\"有参考视频1080P\", c*31):tier(\"有参考视频4K\", c*16))):\n(param(\"resolution\")=='480p'||param(\"resolution\")=='720p'? tier(\"无参考视频480P/720P\", c*46):(param(\"resolution\")=='1080p'?tier(\"无参考视频1080P\", c*51):tier(\"无参考视频4K\", c*26)))",
		"doubao-seedance-2-0-fast-260128":"param(\"metadata.content.#(type==\\\"video_url\\\")\") != nil ? tier(\"有参考视频\", c*22):tier(\"无参考视频\", c*37)",
		"doubao-seedance-2-0-mini-260615":"param(\"metadata.content.#(type==\\\"video_url\\\")\") != nil ? tier(\"有参考视频\", c*14) : tier(\"无参考视频\", c*23)",
		"doubao-seedream-4-5-251128":"tier(\"￥0.25/张\", param(\"n\") * 0.25 * 1000000)",
		"doubao-seedream-5-0-260128":"tier(\"￥0.22/张\", param(\"n\") * 0.22 * 1000000)",
		"doubao-seedream-5-0-pro-260628":"tier(\"输入图0.02元/张（首张免费）,输出0.30-0.60元/张\", (max(param(\"n\") - 1, 0) * 0.02 + param(\"n\") * (param(\"size\") == \"1K\" ? 0.30 : 0.60)) * 1000000)"
	}`

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": billingModeJSON,
		"billing_setting.billing_expr": billingExprJSON,
	}))

	tests := []struct {
		name             string
		model            string
		promptTokens     int
		completionTokens int
		requestBody      string
		wantTier         string
		wantQuota        float64
	}{
		{
			name:             "gpt-image-2 默认(no channel)",
			model:            "gpt-image-2",
			promptTokens:     1000,
			completionTokens: 500,
			wantTier:         "默认",
			// rawCost = 1000*53.76 + 500*201.6 = 53760 + 100800 = 154560
			// quota = 154560 / 1_000_000 * 500_000 = 77280
			wantQuota: 77280,
		},
		{
			name:             "glm-4.6v short_input (p<=32000)",
			model:            "glm-4.6v",
			promptTokens:     10000,
			completionTokens: 2000,
			wantTier:         "short_input",
			// rawCost = 10000*1 + 2000*3 = 10000 + 6000 = 16000
			// quota = 16000 / 1_000_000 * 500_000 = 8000
			wantQuota: 8000,
		},
		{
			name:             "glm-4.6v long_input (p>32000)",
			model:            "glm-4.6v",
			promptTokens:     50000,
			completionTokens: 2000,
			wantTier:         "long_input",
			// rawCost = 50000*2 + 2000*6 = 100000 + 12000 = 112000
			// quota = 112000 / 1_000_000 * 500_000 = 56000
			wantQuota: 56000,
		},
		{
			name:             "gemini-3.1-pro-preview short_context (p<=200000)",
			model:            "gemini-3.1-pro-preview",
			promptTokens:     100000,
			completionTokens: 5000,
			wantTier:         "short_context",
			// rawCost = 100000*14.24 + 5000*85.44 = 1424000 + 427200 = 1851200
			// quota = 1851200 / 1_000_000 * 500_000 = 925600
			wantQuota: 925600,
		},
		{
			name:             "gemini-3.1-pro-preview long_context (p>200000)",
			model:            "gemini-3.1-pro-preview",
			promptTokens:     300000,
			completionTokens: 5000,
			wantTier:         "long_context",
			// rawCost = 300000*28.48 + 5000*128.16 = 8544000 + 640800 = 9184800
			// quota = 9184800 / 1_000_000 * 500_000 = 4592400
			wantQuota: 4592400,
		},
		{
			name:             "gemini-3-pro-image-preview 默认(no channel, fixed cost)",
			model:            "gemini-3-pro-image-preview",
			promptTokens:     1000,
			completionTokens: 500,
			wantTier:         "默认",
			// rawCost = 680000 (fixed)
			// quota = 680000 / 1_000_000 * 500_000 = 340000
			wantQuota: 340000,
		},
		{
			name:             "gemini-3.1-flash-image-preview 默认(no channel)",
			model:            "gemini-3.1-flash-image-preview",
			promptTokens:     2000,
			completionTokens: 1000,
			wantTier:         "token",
			// rawCost = 2000*3.03 + 1000*363.63 = 6060 + 363630 = 369690
			// quota = 369690 / 1_000_000 * 500_000 = 184845
			wantQuota: 184845,
		},
		{
			name:             "doubao-seedance-2-0-260128 无参考视频 480p",
			model:            "doubao-seedance-2-0-260128",
			completionTokens: 10000,
			requestBody:      `{"resolution":"480p"}`,
			wantTier:         "无参考视频480P/720P",
			// rawCost = 10000*46 = 460000
			// quota = 460000 / 1_000_000 * 500_000 = 230000
			wantQuota: 230000,
		},
		{
			name:             "doubao-seedance-2-0-260128 无参考视频 1080p",
			model:            "doubao-seedance-2-0-260128",
			completionTokens: 10000,
			requestBody:      `{"resolution":"1080p"}`,
			wantTier:         "无参考视频1080P",
			// rawCost = 10000*51 = 510000
			// quota = 510000 / 1_000_000 * 500_000 = 255000
			wantQuota: 255000,
		},
		{
			name:             "doubao-seedance-2-0-260128 无参考视频 4k",
			model:            "doubao-seedance-2-0-260128",
			completionTokens: 10000,
			requestBody:      `{"resolution":"4k"}`,
			wantTier:         "无参考视频4K",
			// rawCost = 10000*26 = 260000
			// quota = 260000 / 1_000_000 * 500_000 = 130000
			wantQuota: 130000,
		},
		{
			name:             "doubao-seedance-2-0-260128 有参考视频 720p",
			model:            "doubao-seedance-2-0-260128",
			completionTokens: 10000,
			requestBody:      `{"resolution":"720p","metadata":{"content":[{"type":"video_url","video_url":"https://example.com/v.mp4"}]}}`,
			wantTier:         "有参考视频480P/720P",
			// rawCost = 10000*28 = 280000
			// quota = 280000 / 1_000_000 * 500_000 = 140000
			wantQuota: 140000,
		},
		{
			name:             "doubao-seedance-2-0-260128 有参考视频 1080p",
			model:            "doubao-seedance-2-0-260128",
			completionTokens: 10000,
			requestBody:      `{"resolution":"1080p","metadata":{"content":[{"type":"video_url","video_url":"https://example.com/v.mp4"}]}}`,
			wantTier:         "有参考视频1080P",
			// rawCost = 10000*31 = 310000
			// quota = 310000 / 1_000_000 * 500_000 = 155000
			wantQuota: 155000,
		},
		{
			name:             "doubao-seedance-2-0-260128 有参考视频 4k",
			model:            "doubao-seedance-2-0-260128",
			completionTokens: 10000,
			requestBody:      `{"resolution":"4k","metadata":{"content":[{"type":"video_url","video_url":"https://example.com/v.mp4"}]}}`,
			wantTier:         "有参考视频4K",
			// rawCost = 10000*16 = 160000
			// quota = 160000 / 1_000_000 * 500_000 = 80000
			wantQuota: 80000,
		},
		{
			name:             "doubao-seedance-2-0-fast 无参考视频",
			model:            "doubao-seedance-2-0-fast-260128",
			completionTokens: 10000,
			requestBody:      `{}`,
			wantTier:         "无参考视频",
			// rawCost = 10000*37 = 370000
			// quota = 370000 / 1_000_000 * 500_000 = 185000
			wantQuota: 185000,
		},
		{
			name:             "doubao-seedance-2-0-fast 有参考视频",
			model:            "doubao-seedance-2-0-fast-260128",
			completionTokens: 10000,
			requestBody:      `{"metadata":{"content":[{"type":"video_url","video_url":"https://example.com/v.mp4"}]}}`,
			wantTier:         "有参考视频",
			// rawCost = 10000*22 = 220000
			// quota = 220000 / 1_000_000 * 500_000 = 110000
			wantQuota: 110000,
		},
		{
			name:             "doubao-seedance-2-0-mini 无参考视频",
			model:            "doubao-seedance-2-0-mini-260615",
			completionTokens: 10000,
			requestBody:      `{}`,
			wantTier:         "无参考视频",
			// rawCost = 10000*23 = 230000
			// quota = 230000 / 1_000_000 * 500_000 = 115000
			wantQuota: 115000,
		},
		{
			name:             "doubao-seedance-2-0-mini 有参考视频",
			model:            "doubao-seedance-2-0-mini-260615",
			completionTokens: 10000,
			requestBody:      `{"metadata":{"content":[{"type":"video_url","video_url":"https://example.com/v.mp4"}]}}`,
			wantTier:         "有参考视频",
			// rawCost = 10000*14 = 140000
			// quota = 140000 / 1_000_000 * 500_000 = 70000
			wantQuota: 70000,
		},
		{
			name:        "doubao-seedream-4-5 3张",
			model:       "doubao-seedream-4-5-251128",
			requestBody: `{"n":3}`,
			wantTier:    "￥0.25/张",
			// rawCost = 3 * 0.25 * 1000000 = 750000
			// quota = 750000 / 1_000_000 * 500_000 = 375000
			wantQuota: 375000,
		},
		{
			name:        "doubao-seedream-5-0 2张",
			model:       "doubao-seedream-5-0-260128",
			requestBody: `{"n":2}`,
			wantTier:    "￥0.22/张",
			// rawCost = 2 * 0.22 * 1000000 = 440000
			// quota = 440000 / 1_000_000 * 500_000 = 220000
			wantQuota: 220000,
		},
		{
			name:        "doubao-seedream-5-0-pro 输出1K 3张",
			model:       "doubao-seedream-5-0-pro-260628",
			requestBody: `{"n":3,"size":"1K"}`,
			wantTier:    "输入图0.02元/张（首张免费）,输出0.30-0.60元/张",
			// rawCost = (max(3-1,0)*0.02 + 3*0.30) * 1000000 = (0.04 + 0.90) * 1000000 = 940000
			// quota = 940000 / 1_000_000 * 500_000 = 470000
			wantQuota: 470000,
		},
		{
			name:        "doubao-seedream-5-0-pro 输出2K 3张",
			model:       "doubao-seedream-5-0-pro-260628",
			requestBody: `{"n":3,"size":"2K"}`,
			wantTier:    "输入图0.02元/张（首张免费）,输出0.30-0.60元/张",
			// rawCost = (max(3-1,0)*0.02 + 3*0.60) * 1000000 = (0.04 + 1.80) * 1000000 = 1840000
			// quota = 1840000 / 1_000_000 * 500_000 = 920000
			wantQuota: 920000,
		},
		{
			name:        "doubao-seedream-5-0-pro 1张 1K (首张免费无输入图费用)",
			model:       "doubao-seedream-5-0-pro-260628",
			requestBody: `{"n":1,"size":"1K"}`,
			wantTier:    "输入图0.02元/张（首张免费）,输出0.30-0.60元/张",
			// rawCost = (max(1-1,0)*0.02 + 1*0.30) * 1000000 = (0 + 0.30) * 1000000 = 300000
			// quota = 300000 / 1_000_000 * 500_000 = 150000
			wantQuota: 150000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"model":"` + tt.model + `"`
			if tt.promptTokens > 0 {
				body += `,"prompt_tokens":` + intToStr(tt.promptTokens)
			}
			if tt.completionTokens > 0 {
				body += `,"completion_tokens":` + intToStr(tt.completionTokens)
			}
			if tt.requestBody != "" {
				escaped, _ := common.Marshal(tt.requestBody)
				body += `,"request_body":` + string(escaped)
			}
			body += `}`

			recorder := doEstimateRequest(t, body)
			require.Equal(t, http.StatusOK, recorder.Code)

			var resp map[string]interface{}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
			require.True(t, resp["success"].(bool), "request failed: %s", resp["message"])

			data := resp["data"].(map[string]interface{})
			assert.Equal(t, "tiered_expr", data["billing_mode"])
			assert.Equal(t, tt.wantTier, data["matched_tier"])
			assert.Equal(t, tt.wantQuota, data["quota"].(float64), "quota mismatch for %s", tt.name)
		})
	}
}

func intToStr(n int) string {
	return fmt.Sprintf("%d", n)
}

func TestEstimateQuota_TieredExprMissingExpr(t *testing.T) {
	setupEstimateTest(t)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"test-missing-expr":"tiered_expr"}`,
		"billing_setting.billing_expr": `{}`,
	}))

	recorder := doEstimateRequest(t, `{
		"model": "test-missing-expr",
		"prompt_tokens": 100
	}`)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.False(t, resp["success"].(bool))
	assert.Contains(t, resp["message"].(string), "no billing expression")
}
